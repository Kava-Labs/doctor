package kava

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
)

type ClientConfig struct {
	JSONRPCURL string
}

type Client struct {
	config ClientConfig
	*log.Logger
}

func New(config ClientConfig) (*Client, error) {
	return &Client{
		config: config,
	}, nil
}

// RequestError provides additional details about the failed request.
type RequestError struct {
	message    string
	URL        string
	StatusCode int
}

// Error implements the error interface for RequestError.
func (err *RequestError) Error() string {
	return err.message
}

// NewError creates a new RequestError
func NewError(message, url string, statusCode int) error {
	return &RequestError{message, url, statusCode}
}

// MakeRawServiceCall sends a request, auto decoding the response to the result interface if sent.
func MakeRawServiceCall(client request.Requester, req *http.Request, result interface{}) error {
	response, err := client.Do(req)
	if err != nil {
		return &RequestError{
			URL:     req.URL.String(),
			message: err.Error(),
		}
	}
	defer response.Body.Close()
	if !(response.StatusCode >= 200 && response.StatusCode <= 299) {
		requestURL := req.URL.String()
		return &RequestError{
			StatusCode: response.StatusCode,
			URL:        requestURL,
			message:    fmt.Sprintf("e3db: %s: server http error %d", requestURL, response.StatusCode),
		}
	}
	// If no result is expected, don't attempt to decode a potentially
	// empty response stream and avoid incurring EOF errors
	if result == nil {
		return nil
	}

	err = json.NewDecoder(response.Body).Decode(&result)
	if err != nil {
		return &RequestError{
			URL:     req.URL.String(),
			message: err.Error(),
		}
	}
	return nil
}

// ReturnRawServiceCall sends a req, auto decoding the response to the result interface and returning Response.
func ReturnRawServiceCall(client request.Requester, req *http.Request, result interface{}) (*http.Response, error) {
	response, err := client.Do(req)
	if err != nil {
		return response, &RequestError{
			URL:     req.URL.String(),
			message: err.Error(),
		}
	}
	defer response.Body.Close()
	if !(response.StatusCode >= 200 && response.StatusCode <= 299) {
		requestURL := req.URL.String()
		return response, &RequestError{
			StatusCode: response.StatusCode,
			URL:        requestURL,
			message:    fmt.Sprintf("e3db: %s: server http error %d", requestURL, response.StatusCode),
		}
	}
	// If no result is expected, don't attempt to decode a potentially
	// empty response stream and avoid incurring EOF errors
	if result == nil {
		return response, nil
	}
	err = json.NewDecoder(response.Body).Decode(&result)
	if err != nil {
		return response, &RequestError{
			URL:     req.URL.String(),
			message: err.Error(),
		}
	}
	return response, nil
}

// CreateRequest isolates duplicate code in creating http search request.
func CreateRequest(method string, path string, params interface{}) (*http.Request, error) {
	var buf bytes.Buffer
	var req *http.Request
	err := json.NewEncoder(&buf).Encode(&params)
	if err != nil {
		return req, err
	}
	req, err = http.NewRequest(method, path, &buf)
	if err != nil {
		return req, &RequestError{
			URL:     path,
			message: err.Error(),
		}
	}
	return req, nil
}
