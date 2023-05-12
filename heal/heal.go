package heal

import (
	"fmt"
	"os/exec"
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

// HealerConfig wraps values for use by one or more
// runs of one or more healer routines
type HealerConfig struct {
	AutohealSyncToLiveToleranceSeconds int
}

// GetNodeAutoscalingState gets the autoscaling state of the node based off it's instance id
// returning the state and error (if any).
func GetNodeAutoscalingState(instanceId string, client *autoscaling.AutoScaling) (string, error) {
	autoscalingInstances, err := awsDoctor.autoscalingClient.DescribeAutoScalingInstances(&autoscaling.DescribeAutoScalingInstancesInput{
		InstanceIds: []*string{
			aws.String(instanceId),
		},
	})

	if err != nil {
		return "", fmt.Errorf("StandbyNodeUntilCaughtUp: error %s checking autoscaling state for instance %s", err, instanceId)
	}

	if len(autoscalingInstances.AutoScalingInstances) != 1 {
		return "", fmt.Errorf("StandbyNodeUntilCaughtUp: expected exactly one instance with id %s, got %+v", instanceId, autoscalingInstances.AutoScalingInstances)
	}

	return *autoscalingInstances.AutoScalingInstances[0].LifecycleState, nil
}

// StandbyNodeUntilCaughtUp will keep the ec2 instance the kava node is
// on in standby (to shift resources that would be consumed by an api node
// serving production client requests towards synching up to live faster)
// until it catches back up.
func StandbyNodeUntilCaughtUp(logMessages chan<- string, kavaClient *kava.Client, healerConfig HealerConfig) {
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
		logMessages <- fmt.Sprintf("StandbyNodeUntilCaughtUp: error %s checking autoscaling state for instance %s", err, awsInstanceId)
		return
	}

	if len(autoscalingInstances.AutoScalingInstances) != 1 {
		logMessages <- fmt.Sprintf("StandbyNodeUntilCaughtUp: expected exactly one instance with id %s, got %+v", awsInstanceId, autoscalingInstances.AutoScalingInstances)
		return
	}

	autoscalingGroupName := autoscalingInstances.AutoScalingInstances[0].AutoScalingGroupName

	var placedOnStandby bool
	// place the host in standby with the autoscaling group
	// if it's in service so it won't get any more requests
	// until it syncs back to live
	if *autoscalingInstances.AutoScalingInstances[0].LifecycleState == autoscaling.LifecycleStateInService {
		state, err := awsDoctor.autoscalingClient.EnterStandby(&autoscaling.EnterStandbyInput{
			AutoScalingGroupName: autoscalingGroupName,
			InstanceIds: []*string{
				aws.String(awsInstanceId),
			},
			ShouldDecrementDesiredCapacity: aws.Bool(true),
		})

		if err != nil {
			logMessages <- fmt.Sprintf("StandbyNodeUntilCaughtUp: error %s placing host on standby", err)

			return
		}

		placedOnStandby = true

		logMessages <- fmt.Sprintf("StandbyNodeUntilCaughtUp: host entered standby state with autoscaling group %+v", state.Activities)
	} else {
		logMessages <- "StandbyNodeUntilCaughtUp: host is not currently in service, not moving to standby"
	}

	if *autoscalingInstances.AutoScalingInstances[0].LifecycleState == autoscaling.LifecycleStateStandby {
		logMessages <- "StandbyNodeUntilCaughtUp: host was already on standby, will place in service once caught up"
		placedOnStandby = true
	}

	// wait until the kava process catches back up to live
	for {
		kavaStatus, err := awsDoctor.kava.GetNodeState()

		if err != nil {
			logMessages <- fmt.Sprintf("StandbyNodeUntilCaughtUp: error %s attempting to get kava status, sleeping for a minute before retrying", err)

			time.Sleep(1 * time.Minute)

			continue
		}

		var secondsBehindLive int64
		currentSyncTime := kavaStatus.SyncInfo.LatestBlockTime
		secondsBehindLive = int64(time.Since(currentSyncTime).Seconds())

		if secondsBehindLive <= int64(healerConfig.AutohealSyncToLiveToleranceSeconds) {
			logMessages <- (fmt.Sprintf("StandbyNodeUntilCaughtUp: node caught back up to %d seconds behind current time", healerConfig.AutohealSyncToLiveToleranceSeconds))
			break
		}

		logMessages <- "StandbyNodeUntilCaughtUp: node is still catching up"

		time.Sleep(1 * time.Minute)
	}

	// put the node back in service
	if placedOnStandby {
		var exitedStandby bool
		for !exitedStandby {
			currentState, err := GetNodeAutoscalingState(awsInstanceId, awsDoctor.autoscalingClient)

			if err != nil {
				logMessages <- err.Error()

				continue
			}

			if currentState == autoscaling.LifecycleStateInService {
				logMessages <- "StandbyNodeUntilCaughtUp: host is no longer on standby"

				exitedStandby = true

				break
			}

			state, err := awsDoctor.autoscalingClient.ExitStandby(&autoscaling.ExitStandbyInput{
				AutoScalingGroupName: autoscalingGroupName,
				InstanceIds: []*string{
					aws.String(awsInstanceId),
				},
			})

			// keep trying if we encountered an error
			if err != nil {
				logMessages <- fmt.Sprintf("StandbyNodeUntilCaughtUp: error %s attempting to exit standby", err)

				continue
			}

			logMessages <- fmt.Sprintf("StandbyNodeUntilCaughtUp: host exited standby %+v", state.Activities)

			exitedStandby = true
		}
	}

	logMessages <- "StandbyNodeUntilCaughtUp: node healed successfully by doctor"
}

// RestartBlockchainService restarts the blockchain service
// returning error (if any)
func RestartBlockchainService() error {
	cmd := exec.Command("bash", "-c", "sudo systemctl restart kava")
	output, err := cmd.CombinedOutput()

	if err != nil {
		return fmt.Errorf("error %s starting blockchain service output %s", err, string(output))
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
