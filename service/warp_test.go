package service

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alireza0/s-ui/database/model"
)

func TestRegisterWarpBuildsPersistableEndpointData(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/reg":
			if request.Method != http.MethodPost {
				t.Errorf("unexpected registration method: %s", request.Method)
				http.Error(writer, "unexpected method", http.StatusMethodNotAllowed)
				return
			}
			if request.Header.Get("Content-Type") != "application/json" {
				t.Errorf("unexpected registration content type: %s", request.Header.Get("Content-Type"))
				http.Error(writer, "unexpected content type", http.StatusBadRequest)
				return
			}
			_, _ = io.Copy(io.Discard, request.Body)
			_, _ = writer.Write([]byte(`{"id":"device-id","token":"access-token","account":{"license":"initial-license"}}`))
		case "/reg/device-id":
			if request.Method != http.MethodGet {
				t.Errorf("unexpected info method: %s", request.Method)
				http.Error(writer, "unexpected method", http.StatusMethodNotAllowed)
				return
			}
			if request.Header.Get("Authorization") != "Bearer access-token" {
				t.Errorf("unexpected authorization header: %s", request.Header.Get("Authorization"))
				http.Error(writer, "unexpected authorization", http.StatusUnauthorized)
				return
			}
			_, _ = writer.Write([]byte(`{"config":{"client_id":"AQID","interface":{"addresses":{"v4":"172.16.0.2","v6":"2606:4700:110:8f5d:1234:5678:9abc:def0"}},"peers":[{"endpoint":{"host":"162.159.192.1:2408"},"public_key":"peer-public-key"}]}}`))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	previousBaseURL := warpAPIBaseURL
	warpAPIBaseURL = server.URL
	t.Cleanup(func() {
		warpAPIBaseURL = previousBaseURL
	})

	endpoint := &model.Endpoint{
		Type:    "warp",
		Tag:     "warp-primary",
		Options: json.RawMessage(`{"mtu":1280}`),
	}
	if err := (&WarpService{}).RegisterWarp(endpoint); err != nil {
		t.Fatalf("register warp: %v", err)
	}

	var credentials warpCredentials
	if err := json.Unmarshal(endpoint.Ext, &credentials); err != nil {
		t.Fatalf("unmarshal credentials: %v", err)
	}
	if credentials.DeviceID != "device-id" || credentials.AccessToken != "access-token" || credentials.LicenseKey != "initial-license" {
		t.Fatalf("unexpected WARP credentials: %#v", credentials)
	}

	var options map[string]json.RawMessage
	if err := json.Unmarshal(endpoint.Options, &options); err != nil {
		t.Fatalf("unmarshal endpoint options: %v", err)
	}
	if _, ok := options["private_key"]; !ok {
		t.Fatalf("generated private key missing from options: %s", endpoint.Options)
	}
	if string(options["mtu"]) != "1280" {
		t.Fatalf("existing endpoint option was lost: %s", endpoint.Options)
	}
	var addresses []string
	if err := json.Unmarshal(options["address"], &addresses); err != nil {
		t.Fatalf("unmarshal WARP addresses: %v", err)
	}
	if len(addresses) != 2 || addresses[0] != "172.16.0.2/32" || addresses[1] != "2606:4700:110:8f5d:1234:5678:9abc:def0/128" {
		t.Fatalf("unexpected WARP addresses: %#v", addresses)
	}
}
