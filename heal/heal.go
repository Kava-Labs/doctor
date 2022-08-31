package heal

import (
	"fmt"
	"os/exec"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/ec2metadata"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/autoscaling"
	"github.com/kava-labs/doctor/clients/kava"
)

// AwsDoctor is a doctor that is capable
// of healing a kava node running in AWS
type AwsDoctor struct {
	autoscalingClient *autoscaling.AutoScaling
	instanceId        string
	kava              *kava.Client
}

var (
	awsDoctor        *AwsDoctor
	initErrorMessage *string
)

// ActiveHealerCounter wraps values
// for safe and accurate concurrent
// book keeping of the number of active
// healer routines in flight for a given host
type ActiveHealerCounter struct {
	sync.Mutex
	Count int
}

// StandbyNodeUntilCaughtUp will keep the ec2 instance the kava node is
// on in standby (to shift resources that would be consumed by an api node
// serving production client requests towards synching up to live faster)
// until it catches back up.
func StandbyNodeUntilCaughtUp(logMessages chan<- string, kavaClient *kava.Client) {
	if initErrorMessage != nil {
		logMessages <- fmt.Sprintf("healer init failed with error %s, skipping attempt to heal via StandbyNodeUntilCaughtUp", *initErrorMessage)
		return
	}

	awsDoctor.kava = kavaClient

	awsInstanceId := awsDoctor.instanceId

	// check to see if the host is in service
	autoscalingInstances, err := awsDoctor.autoscalingClient.DescribeAutoScalingInstances(&autoscaling.DescribeAutoScalingInstancesInput{
		InstanceIds: []*string{
			aws.String(awsInstanceId),
		},
	})

	if err != nil {
		logMessages <- fmt.Sprintf("error %s checking autoscaling state for instance %s", err, awsInstanceId)
		return
	}

	if len(autoscalingInstances.AutoScalingInstances) != 1 {
		logMessages <- fmt.Sprintf("expected exactly one instance with id %s, got %+v", awsInstanceId, autoscalingInstances.AutoScalingInstances)
		return
	}

	autoscalingGroupName := autoscalingInstances.AutoScalingInstances[0].AutoScalingGroupName

	// place the host in standby with the autoscaling group
	// if it's not already in standby
	if *autoscalingInstances.AutoScalingInstances[0].LifecycleState != autoscaling.LifecycleStateStandby {
		state, err := awsDoctor.autoscalingClient.EnterStandby(&autoscaling.EnterStandbyInput{
			AutoScalingGroupName: autoscalingGroupName,
			InstanceIds: []*string{
				aws.String(awsInstanceId),
			},
			ShouldDecrementDesiredCapacity: aws.Bool(true),
		})

		if err != nil {
			logMessages <- fmt.Sprintf("error %s placing host on standby", err)

			return
		}

		logMessages <- fmt.Sprintf("host entered standby state with autoscaling group %+v", state.Activities)
	} else {
		logMessages <- "host is already in standby state with the autoscaling group"
	}

	// restart the kava process if it thinks it is not catching up
	kavaStatus, err := awsDoctor.kava.GetNodeState()

	if err != nil {
		logMessages <- fmt.Sprintf("error %s attempting to get kava status", err)
		return
	}

	if !kavaStatus.SyncInfo.CatchingUp {
		logMessages <- "StandbyNodeUntilCaughtUp: node is out of sync and doesn't know it, restarting kava"

		err = RestartKavaService()

		if err != nil {
			logMessages <- fmt.Sprintf("error %s restarting kava service attempting to heal via StandbyNodeUntilCaughtUp", err)
		}
	} else {
		logMessages <- "StandbyNodeUntilCaughtUp: node is out of sync and knows it, not restarting kava"

	}

	// wait until the kava process catches back up to live
	for {
		kavaStatus, err := awsDoctor.kava.GetNodeState()

		if err != nil {
			logMessages <- fmt.Sprintf("error %s attempting to get kava status", err)
			continue
		}

		if !kavaStatus.SyncInfo.CatchingUp {
			break
		}

		logMessages <- "StandbyNodeUntilCaughtUp: node is still catching up"

		time.Sleep(1 * time.Minute)
	}

	// put the node back in service
	var exitedStandby bool
	for !exitedStandby {
		state, err := awsDoctor.autoscalingClient.ExitStandby(&autoscaling.ExitStandbyInput{
			AutoScalingGroupName: autoscalingGroupName,
			InstanceIds: []*string{
				aws.String(awsInstanceId),
			},
		})

		// keep trying if we encountered an error
		if err != nil {
			logMessages <- fmt.Sprintf("error %s attempting to exit standby", err)

			continue
		}

		logMessages <- fmt.Sprintf("StandbyNodeUntilCaughtUp: host exited standby %+v", state.Activities)

		exitedStandby = true
	}
}

// RestartKavaService restarts the kava service
// returning error (if any)
func RestartKavaService() error {
	cmd := exec.Command("bash", "-c", "sudo systemctl restart kava")
	output, err := cmd.CombinedOutput()

	if err != nil {
		return fmt.Errorf("error %s starting kava service output %s", err, string(output))
	}

	return nil
}

// NewAwsDoctor returns a new AwsDoctor for healing a kava node
// that is running in aws, and error (if any)
func NewAwsDoctor() (*AwsDoctor, error) {
	// gather information about the host needed by the healer

	// create a new client using the default credential chain provider
	awsSession, err := session.NewSession()

	if err != nil {
		return nil, fmt.Errorf("error %s creating valid aws session", err)
	}

	ec2MetadataClient := ec2metadata.New(awsSession)

	// get the ec2 instance id and region of the host
	eC2IdentityDocument, err := ec2MetadataClient.GetInstanceIdentityDocument()

	if err != nil {
		return nil, fmt.Errorf("error %s getting ec2 identity document for host to sync from", err)
	}

	// re-initialize aws session using the region of the instance
	// to allow for calling external (to the instance) AWS services
	instanceAWSRegion := eC2IdentityDocument.Region
	awsSession, err = session.NewSession(
		&aws.Config{
			Region: &instanceAWSRegion,
		},
	)

	if err != nil {
		return nil, fmt.Errorf("error %s creating valid aws session using region %s", err, instanceAWSRegion)
	}

	autoscalingClient := autoscaling.New(awsSession)

	return &AwsDoctor{
		instanceId:        eC2IdentityDocument.InstanceID,
		autoscalingClient: autoscalingClient,
	}, nil
}

// init creates a singleton instance of the healer that is
// safe for concurrent usage across multiple go-routines
func init() {
	healer, err := NewAwsDoctor()

	if err != nil {
		initErrorMessage = aws.String(err.Error())

		return
	}

	awsDoctor = healer
}
