package sub

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/alireza0/s-ui/database/model"
	"github.com/gin-gonic/gin"
)

func TestClientSubscriptionQueryRouteAcceptsSpecialCharacters(t *testing.T) {
	db := initSubGroupSubscriptionTestDB(t, "client-query-route.db")

	client := &model.Client{
		Enable:   true,
		Name:     "alice/default #1",
		Config:   []byte(`{}`),
		Inbounds: []byte(`[]`),
	}
	if err := db.Create(client).Error; err != nil {
		t.Fatalf("create client failed: %v", err)
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	(&SubHandler{}).initRouter(router.Group("/"))

	req := httptest.NewRequest(http.MethodGet, "/q/client?name="+url.QueryEscape(client.Name), nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d with body %q", recorder.Code, recorder.Body.String())
	}
}

func TestMihomoSubscriptionQueryRouteAcceptsSpecialCharacters(t *testing.T) {
	db := initSubGroupSubscriptionTestDB(t, "mihomo-query-route.db")

	client := &model.MihomoClient{
		Enable:   true,
		Name:     "mihomo/default #1",
		Config:   []byte(`{}`),
		Inbounds: []byte(`[]`),
	}
	if err := db.Create(client).Error; err != nil {
		t.Fatalf("create mihomo client failed: %v", err)
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	(&SubHandler{}).initRouter(router.Group("/"))

	req := httptest.NewRequest(http.MethodGet, "/q/mihomo?name="+url.QueryEscape(client.Name), nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d with body %q", recorder.Code, recorder.Body.String())
	}
}

func TestSubscriptionRenderReturnsRetryAfterWhenRendererIsBusy(t *testing.T) {
	for index := 0; index < subscriptionRenderMaxConcurrent; index++ {
		subscriptionRenderSlots <- struct{}{}
	}
	t.Cleanup(func() {
		for index := 0; index < subscriptionRenderMaxConcurrent; index++ {
			<-subscriptionRenderSlots
		}
	})

	gin.SetMode(gin.TestMode)
	router := gin.New()
	(&SubHandler{}).initRouter(router.Group("/"))

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/q/client?name=busy", nil))

	if recorder.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusTooManyRequests)
	}
	if got := recorder.Header().Get("Retry-After"); got != "1" {
		t.Fatalf("Retry-After = %q, want %q", got, "1")
	}
}
