package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestDelayedFake504RejectsSaturatedDelaySlotsImmediately(t *testing.T) {
	for index := 0; index < cap(fakeProbeSlots); index++ {
		fakeProbeSlots <- struct{}{}
	}
	t.Cleanup(func() {
		for index := 0; index < cap(fakeProbeSlots); index++ {
			<-fakeProbeSlots
		}
	})

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodGet, "/not-found", nil)
	started := time.Now()
	DelayedFake504(context)
	if elapsed := time.Since(started); elapsed > 100*time.Millisecond {
		t.Fatalf("saturated anti-probe response waited %s", elapsed)
	}
	if recorder.Code != http.StatusGatewayTimeout {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusGatewayTimeout)
	}
}

func TestDelayedFake504StopsWhenClientContextIsCanceled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ginContext, _ := gin.CreateTestContext(recorder)
	requestContext, cancel := context.WithCancel(context.Background())
	cancel()
	ginContext.Request = httptest.NewRequest(http.MethodGet, "/not-found", nil).WithContext(requestContext)
	started := time.Now()
	DelayedFake504(ginContext)
	if elapsed := time.Since(started); elapsed > 100*time.Millisecond {
		t.Fatalf("canceled anti-probe request waited %s", elapsed)
	}
}
