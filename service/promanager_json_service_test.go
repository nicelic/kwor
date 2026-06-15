package service

import "testing"

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
