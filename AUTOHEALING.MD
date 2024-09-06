# Autohealing routines

Doctor is capable of automatically detecting and trying to remediate performance or availability issues with the endpoint it is monitoring.

## Configuration

excerpt of config values specific to autohealing process below:

```bash
$ doctor --help
Usage of doctor:
      --autoheal                                          whether doctor should take active measures to attempt to heal the kava process (e.g. place on standby if it falls significantly behind live)
      --autoheal_blockchain_service_name string           the name of the systemd service running the blockchain. this is the service that gets restarted in the autoheal process (default "kava")
      --autoheal_initial_delay_seconds int                initial delay before autoheal attempts a restart. useful for allowing longer startup time for the chain, like during statesync initialization
      --autoheal_restart_delay_seconds int                number of seconds autohealing routines will wait to restart the endpoint, effective from the last time it was restarted and over riding the values downtime_restart_threshold_seconds no_new_blocks_restart_threshold_seconds (default 2700)
      --autoheal_sync_latency_tolerance_seconds int       how far behind live the node is allowed to fall before autohealing actions are attempted (default 120)
      --autoheal_sync_to_live_tolerance_seconds int       how close to the current time the node must resync to before being considered in sync again (default 12)
      --default_monitoring_interval_seconds int           default interval doctor will use for the various monitoring routines (default 5)
      --downtime_restart_threshold_seconds int            how many continuous seconds the endpoint being monitored has to be offline or unresponsive before autohealing will be attempted (default 300)
      --health_check_timeout_seconds int                  max number of seconds doctor will wait for a health check response from the endpoint (default 10)
      --no_new_blocks_restart_threshold_seconds int       how many continuous seconds the endpoint being monitored has not produce a new bloc before autohealing will be attempted (default 300)
```

[Default Kava Mainnet Doctor Config](./https://github.com/Kava-Labs/infrastructure/blob/master/ansible/roles/ops/templates/doctor-config.json)

## Auto Healing Routines

Only if `autoheal` is set to true will autohealing routines be triggered.

Routines can run concurrently, e.g. a node may fall behind live and put on standby by one routine until it catches up, and if the node goes offline or stops making new blocks during the time it is on standby another routine will restart the kava process, and if the node syncs back to live the first auto healing process will put the node back in service with the autoscaling group.

Upon initial start of the service, `autoheal` will wait `autoheal_initial_delay_seconds` before performing the first restart of the chain process.

### Node API Offline

If doctor detects that the node is offline for more than `downtime_restart_threshold_seconds`, it will attempt to restart the kava process on the node.

### Node API Out of Sync

[Out of Sync Heuristic and Auto Healing Workflow](./docs/imgs/doctor-out-of-sync-heuristic-auto-healing-workflow.jpg)

If doctor detects that the node has fallen more than `autoheal_sync_latency_tolerance_seconds` behind the current time (comparing the latest block time for the node and the current time), it will attempt to place the node in standby with the autoscaling group so it won't have to serve requests and can sync faster, and if the node returns to within `autoheal_sync_to_live_tolerance_seconds` of the current time it will be placed back in service.

### Node API Frozen

If doctor detects that the node has not synched a new block in more than `no_new_blocks_restart_threshold_seconds`, it will attempt to restart the kava process on the node.

## Configurable service name

The autohealing process assumes the chain is running via a systemd service. It uses a systemd restart to restart the chain. The name of this service is configurable via the configuration option `autoheal_blockchain_service_name`. By default, doctor uses the service name `kava`.
