package api

import (
	"encoding/json"
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/alireza0/s-ui/service"
	"github.com/gin-gonic/gin"
)

func TestCommittedSaveFailureResponsePreservesRetryRuntimeContract(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, test := range []struct {
		name  string
		retry bool
	}{
		{name: "sing-box runtime retry required", retry: true},
		{name: "no sing-box runtime retry", retry: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			context, _ := gin.CreateTestContext(recorder)
			writeCommittedSaveFailure(context, &service.CommittedSaveError{
				Err:                 errors.New("post-commit failure"),
				RetrySingboxRuntime: test.retry,
			})

			response := Msg{}
			if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
				t.Fatalf("decode response %q: %v", recorder.Body.String(), err)
			}
			if response.Success {
				t.Fatalf("committed save response unexpectedly succeeded: %#v", response)
			}
			payload, ok := response.Obj.(map[string]any)
			if !ok {
				t.Fatalf("committed save response payload = %#v", response.Obj)
			}
			if payload["committed"] != true {
				t.Fatalf("committed flag = %#v, want true", payload["committed"])
			}
			if payload["retryRuntime"] != test.retry {
				t.Fatalf("retryRuntime = %#v, want %v", payload["retryRuntime"], test.retry)
			}
		})
	}
}

func TestCommittedPartialLoadFailureResponsePreservesCommittedContract(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	writeCommittedPartialLoadFailure(context, "inbounds", errors.New("partial data unavailable"))

	response := Msg{}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response %q: %v", recorder.Body.String(), err)
	}
	if response.Success {
		t.Fatalf("partial-load failure response unexpectedly succeeded: %#v", response)
	}
	payload, ok := response.Obj.(map[string]any)
	if !ok {
		t.Fatalf("partial-load failure payload = %#v", response.Obj)
	}
	if payload["committed"] != true || payload["refreshFailed"] != true {
		t.Fatalf("committed refresh-failure payload = %#v", payload)
	}
	if payload["retryRuntime"] != false {
		t.Fatalf("retryRuntime = %#v, want false", payload["retryRuntime"])
	}
}
