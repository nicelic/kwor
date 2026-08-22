package service

import (
	"encoding/json"
	"testing"

	"github.com/alireza0/s-ui/database/model"
)

func TestNormalizeClientArrayFieldsConvertsNullArrays(t *testing.T) {
	client := &model.Client{
		Inbounds: json.RawMessage(" null "),
		Links:    json.RawMessage("null"),
	}

	normalizeClientArrayFields(client)

	if string(client.Inbounds) != "[]" {
		t.Fatalf("expected inbounds to be [], got %s", client.Inbounds)
	}
	if string(client.Links) != "[]" {
		t.Fatalf("expected links to be [], got %s", client.Links)
	}
}
