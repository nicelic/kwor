package sub

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestLinkServiceFetchesBoundedExternalSubscription(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, _ = writer.Write([]byte("vmess://first\ntrojan://second"))
	}))
	defer server.Close()

	got := (&LinkService{}).getExternalSub(server.URL)
	want := []string{"vmess://first", "trojan://second"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("external subscription links = %#v, want %#v", got, want)
	}
}

func TestLinkServiceRejectsFailedExternalSubscriptionResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusBadGateway)
	}))
	defer server.Close()

	if got := (&LinkService{}).getExternalSub(server.URL); got != nil {
		t.Fatalf("failed external subscription links = %#v, want nil", got)
	}
}

func TestLinkServiceKeepsMalformedVMessLinkWithoutPanicking(t *testing.T) {
	raw := base64.StdEncoding.EncodeToString([]byte(`{"v":"2"}`))
	uri := "vmess://" + raw
	if got := (&LinkService{}).addClientInfo(uri, "-client"); got != uri {
		t.Fatalf("malformed vmess link = %q, want original %q", got, uri)
	}
}
