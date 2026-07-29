package service

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

type githubReleasePageRoundTripper func(*http.Request) (*http.Response, error)

func (fn githubReleasePageRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func newGitHubReleasePageTestClient(t *testing.T, payload []byte) *http.Client {
	t.Helper()
	return &http.Client{Transport: githubReleasePageRoundTripper(func(request *http.Request) (*http.Response, error) {
		if got := request.URL.Query().Get("per_page"); got != "20" {
			t.Errorf("per_page = %q, want 20", got)
		}
		if got := request.URL.Query().Get("page"); got != "1" {
			t.Errorf("page = %q, want 1", got)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(bytes.NewReader(payload)),
			Request:    request,
		}, nil
	})}
}

func TestFetchGitHubReleasePageForRepoResponseLimit(t *testing.T) {
	payload, err := json.Marshal([]map[string]string{{
		"tag_name": "v1.0.0",
		"body":     strings.Repeat("x", int(coreGitHubResponseMaxBytes)+1),
	}})
	if err != nil {
		t.Fatalf("marshal release payload: %v", err)
	}
	if int64(len(payload)) <= coreGitHubResponseMaxBytes {
		t.Fatalf("test payload is not larger than the standard limit: %d", len(payload))
	}
	if int64(len(payload)) >= coreGitHubReleaseListMaxBytes {
		t.Fatalf("test payload exceeds the Core release-list limit: %d", len(payload))
	}

	t.Run("Core list accepts response above standard limit", func(t *testing.T) {
		releases, err := fetchGitHubReleasePageForRepo(
			"SagerNet/sing-box",
			newGitHubReleasePageTestClient(t, payload),
			1,
			coreReleaseGitHubPerPage,
			coreGitHubReleaseListMaxBytes,
		)
		if err != nil {
			t.Fatalf("fetch Core release page: %v", err)
		}
		if len(releases) != 1 || releases[0].TagName != "v1.0.0" {
			t.Fatalf("unexpected releases: %+v", releases)
		}
	})

	t.Run("standard list keeps four MiB limit", func(t *testing.T) {
		_, err := fetchGitHubReleasePageForRepo(
			"nicelic/kwor",
			newGitHubReleasePageTestClient(t, payload),
			1,
			coreReleaseGitHubPerPage,
			coreGitHubResponseMaxBytes,
		)
		if err == nil {
			t.Fatal("expected standard release list to reject the oversized response")
		}
	})
}
