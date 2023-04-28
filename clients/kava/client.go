// package kava provides definitions and implementations
// for making requests to the JSON RPC API for a kava node
package kava

import (
	"log"
	"net/http"
	"time"
)

// ClientConfig wraps parameters
// for configuring a kava node client
type ClientConfig struct {
	JSONRPCURL             string
	HTTPReadTimeoutSeconds int
}

// Client is used for communicating with
// the api for a kava node
type Client struct {
	config ClientConfig
	*http.Client
	*log.Logger
}

// New returns a new client configured with
// the provided config, and error (if any)
func New(config ClientConfig) (*Client, error) {
	return &Client{
		Client: &http.Client{
			Timeout: time.Duration(time.Duration(config.HTTPReadTimeoutSeconds) * time.Second),
		},
		config: config,
	}, nil
}
