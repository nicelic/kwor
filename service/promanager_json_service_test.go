package service

import (
	"encoding/json"
	"testing"

	"github.com/alireza0/s-ui/database/model"
)

type stubClientJSONService struct {
	called bool
	subID  string
	format string
	result *string
	err    error
}

func (s *stubClientJSONService) GetJson(subID string, format string) (*string, []string, error) {
	s.called = true
	s.subID = subID
	s.format = format
	return s.result, nil, s.err
}

func TestGetClientJsonSubscription_UsesConfiguredJsonService(t *testing.T) {
	payload := "{\"outbounds\":[{\"tag\":\"naive-a\",\"type\":\"naive\",\"username\":\"alice\"}]}"
	jsonService := &stubClientJSONService{result: &payload}

	proManager := &ProManagerService{}
	proManager.SetJsonService(jsonService)

	result, err := proManager.getClientJsonSubscription("client-a")
	if err != nil {
		t.Fatalf("getClientJsonSubscription returned error: %v", err)
	}
	if !jsonService.called {
		t.Fatalf("expected configured json service to be called")
	}
	if jsonService.subID != "client-a" {
		t.Fatalf("expected subID client-a, got %q", jsonService.subID)
	}
	if jsonService.format != "json" {
		t.Fatalf("expected format json, got %q", jsonService.format)
	}
	if result == nil || *result != payload {
		t.Fatalf("unexpected subscription payload: %#v", result)
	}
}

func TestBuildClientOutboundsFallbackExcludesMixedAndMieru(t *testing.T) {
	client := &model.Client{
		Config: json.RawMessage(`{
			"mixed": {"username": "mixed-user", "password": "mixed-secret"},
			"mieru": {"username": "mieru-user", "password": "mieru-secret"}
		}`),
	}
	inbounds := []*model.Inbound{
		{
			Type: "mixed",
			OutJson: json.RawMessage(`{
				"type": "mixed",
				"tag": "mixed-node",
				"server": "panel.example.com",
				"server_port": 1080
			}`),
		},
		{
			Type: "mieru",
			OutJson: json.RawMessage(`{
				"type": "mieru",
				"tag": "mieru-node",
				"server": "panel.example.com",
				"server_port": 16939
			}`),
		},
	}

	outbounds, tags, err := (&ProManagerService{}).buildClientOutbounds(client, inbounds)
	if err != nil {
		t.Fatalf("buildClientOutbounds returned error: %v", err)
	}
	if len(*outbounds) != 0 || len(*tags) != 0 {
		t.Fatalf("mixed and Mieru must be absent from sing-box fallback output, got outbounds=%#v tags=%#v", outbounds, tags)
	}
}
