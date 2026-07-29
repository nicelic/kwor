package api

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func apiV2TokenContext(token string) *gin.Context {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	ctx.Request.Header.Set("Token", token)
	return ctx
}

func TestAPIv2HandlerFindUsernameIgnoresExpiredTokens(t *testing.T) {
	handler := &APIv2Handler{}
	now := time.Now().Unix()
	handler.setTokens([]TokenInMemory{
		{Token: "active-token", Username: "alice", Expiry: now + 60},
		{Token: "expired-token", Username: "bob", Expiry: now - 1},
	})

	if got := handler.findUsername(apiV2TokenContext("active-token")); got != "alice" {
		t.Fatalf("active token username = %q, want %q", got, "alice")
	}
	if got := handler.findUsername(apiV2TokenContext("expired-token")); got != "" {
		t.Fatalf("expired token username = %q, want empty", got)
	}
	if got := handler.findUsername(apiV2TokenContext("")); got != "" {
		t.Fatalf("empty token username = %q, want empty", got)
	}
}

func TestAPIv2HandlerTokenCacheConcurrentAccess(t *testing.T) {
	handler := &APIv2Handler{}
	context := apiV2TokenContext("active-token")

	var waitGroup sync.WaitGroup
	for i := 0; i < 16; i++ {
		waitGroup.Add(2)
		go func() {
			defer waitGroup.Done()
			for j := 0; j < 100; j++ {
				handler.setTokens([]TokenInMemory{{Token: "active-token", Username: "alice"}})
			}
		}()
		go func() {
			defer waitGroup.Done()
			for j := 0; j < 100; j++ {
				_ = handler.findUsername(context)
			}
		}()
	}
	waitGroup.Wait()
}
