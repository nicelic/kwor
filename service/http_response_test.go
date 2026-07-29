package service

import (
	"bytes"
	"testing"
)

func TestReadBoundedHTTPResponseBody(t *testing.T) {
	body, err := readBoundedHTTPResponseBody(bytes.NewBufferString("1234"), 4)
	if err != nil {
		t.Fatalf("read bounded response failed: %v", err)
	}
	if string(body) != "1234" {
		t.Fatalf("unexpected response body: %q", string(body))
	}

	if _, err := readBoundedHTTPResponseBody(bytes.NewBufferString("12345"), 4); err == nil {
		t.Fatal("expected oversized response to be rejected")
	}
}

func TestUnmarshalBoundedHTTPResponseJSON(t *testing.T) {
	type payload struct {
		Name string `json:"name"`
	}

	value := payload{}
	if err := unmarshalBoundedHTTPResponseJSON(bytes.NewBufferString(`{"name":"kwor"}`), 32, &value); err != nil {
		t.Fatalf("decode bounded JSON failed: %v", err)
	}
	if value.Name != "kwor" {
		t.Fatalf("decoded name = %q, want kwor", value.Name)
	}

	if err := unmarshalBoundedHTTPResponseJSON(bytes.NewBufferString(`{"name":"too-long"}`), 8, &value); err == nil {
		t.Fatal("expected oversized JSON response to be rejected")
	}
	if err := unmarshalBoundedHTTPResponseJSON(bytes.NewBufferString(`{`), 32, &value); err == nil {
		t.Fatal("expected invalid JSON response to be rejected")
	}
}
