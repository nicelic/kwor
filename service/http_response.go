package service

import (
	"encoding/json"
	"fmt"
	"io"
)

// readBoundedHTTPResponseBody keeps metadata endpoints from allocating without a limit.
func readBoundedHTTPResponseBody(body io.Reader, maxBytes int64) ([]byte, error) {
	if body == nil {
		return nil, fmt.Errorf("response body is nil")
	}
	if maxBytes <= 0 {
		return nil, fmt.Errorf("response body size limit must be positive")
	}

	data, err := io.ReadAll(io.LimitReader(body, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxBytes {
		return nil, fmt.Errorf("response body exceeds %d bytes", maxBytes)
	}
	return data, nil
}

// unmarshalBoundedHTTPResponseJSON decodes small metadata responses without
// letting an upstream server control this process's memory use.
func unmarshalBoundedHTTPResponseJSON(body io.Reader, maxBytes int64, target any) error {
	data, err := readBoundedHTTPResponseBody(body, maxBytes)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, target); err != nil {
		return err
	}
	return nil
}
