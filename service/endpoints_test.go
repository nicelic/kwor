package service

import (
	"encoding/json"
	"testing"

	"github.com/alireza0/s-ui/database/model"
)

func TestMarshalEndpointSavePayloadPreservesWarpPersistenceFields(t *testing.T) {
	original := &model.Endpoint{
		Id:      42,
		Type:    "warp",
		Tag:     "warp-primary",
		Options: json.RawMessage(`{"private_key":"private","address":["10.0.0.2/32"],"peers":[{"port":2408}]}`),
		Ext:     json.RawMessage(`{"access_token":"token","device_id":"device","license_key":"license"}`),
	}

	payload, err := marshalEndpointSavePayload(original)
	if err != nil {
		t.Fatalf("marshal endpoint save payload: %v", err)
	}

	var restored model.Endpoint
	if err := restored.UnmarshalJSON(payload); err != nil {
		t.Fatalf("unmarshal endpoint save payload: %v", err)
	}
	if restored.Id != original.Id || restored.Type != "warp" || restored.Tag != original.Tag {
		t.Fatalf("persistence identity changed: %#v", restored)
	}
	var credentials map[string]string
	if err := json.Unmarshal(restored.Ext, &credentials); err != nil {
		t.Fatalf("unmarshal restored WARP credentials: %v", err)
	}
	if credentials["access_token"] != "token" || credentials["device_id"] != "device" || credentials["license_key"] != "license" {
		t.Fatalf("WARP credentials were not preserved: %s", restored.Ext)
	}

	var options map[string]json.RawMessage
	if err := json.Unmarshal(restored.Options, &options); err != nil {
		t.Fatalf("unmarshal restored options: %v", err)
	}
	if _, ok := options["private_key"]; !ok {
		t.Fatalf("WARP options were not preserved: %s", restored.Options)
	}
}
