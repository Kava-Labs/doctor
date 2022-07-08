package kava

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
)

// MakeJSONRequest makes an http request, decoding
// the response to the provided result interface
// (if not nil) and returning the raw response and error (if any).
func MakeJSONRequest(client *http.Client, request *http.Request, result interface{}) (*http.Response, error) {
	// make the request
	response, err := client.Do(request)

	if err != nil {
		return response, err
	}

	// parse response body
	defer response.Body.Close()

	// only if a 200 level response code
	if !(response.StatusCode >= 200 && response.StatusCode <= 299) {
		return response, fmt.Errorf("non 200 response %+v for request %+v", response, request)

	}

	// if no result is expected, don't attempt to decode a potentially
	// empty response stream and avoid incurring EOF errors
	if result == nil {
		return response, nil
	}

	// parse response to json
	err = json.NewDecoder(response.Body).Decode(&result)

	if err != nil {
		return response, err
	}

	return response, nil
}

// PrepareJSONRequest creates an http request to the specified endpoint,
// encoding the request body (if any) to json, returning the prepared
// request and error (if any).
func PrepareJSONRequest(method string, path string, params interface{}) (*http.Request, error) {
	var body bytes.Buffer
	var request *http.Request

	err := json.NewEncoder(&body).Encode(&params)

	if err != nil {
		return request, err
	}

	request, err = http.NewRequest(method, path, &body)

	if err != nil {
		return request, err
	}

	return request, nil
}
