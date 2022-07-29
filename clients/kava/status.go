package kava

import "time"

const (
	StatusEndpointPath = "/status"
)

// NodeState wraps values for the current
// sync state of a single kava node
type NodeState struct {
	NodeInfo NodeInfo `json:"node_info"`
	SyncInfo SyncInfo `json:"sync_info"`
}

// NodeInfo wraps values for the network
// identifiers for a kava node
type NodeInfo struct {
	Id      string `json:"id"`
	Moniker string `json:"moniker"`
}

// SyncInfo wraps values for a kava node's
// sync status that are useful for performance
// benchmarking and health monitoring
type SyncInfo struct {
	LatestBlockHeight int64     `json:"latest_block_height,string"`
	LatestBlockTime   time.Time `json:"latest_block_time,string"`
	CatchingUp        bool      `json:"catching_up"`
}

// JSON-RPC generic response wrapper
type nodeStateResponse struct {
	Result NodeState `json:"result"`
}

// GetNodeState gets the current status
// of the kava node, returning the state
// and error (if any)
func (c *Client) GetNodeState() (NodeState, error) {
	var nodeState nodeStateResponse

	path := c.config.JSONRPCURL + StatusEndpointPath

	request, err := PrepareJSONRequest("GET", path, nil)

	if err != nil {
		return NodeState{}, err
	}

	_, err = MakeJSONRequest(c.Client, request, &nodeState)

	if err != nil {
		return NodeState{}, err
	}

	return nodeState.Result, nil
}
