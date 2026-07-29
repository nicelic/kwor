package api

import (
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alireza0/s-ui/database"
	"github.com/gin-gonic/gin"
)

func TestReverseProxyRevisionConflictUsesStructuredObject(t *testing.T) {
	if err := database.InitDB(filepath.Join(t.TempDir(), "reverse-proxy-api.db")); err != nil {
		t.Fatalf("initialize test database failed: %v", err)
	}
	if sqlDB, err := database.GetDB().DB(); err == nil && sqlDB != nil {
		t.Cleanup(func() { _ = sqlDB.Close() })
	}
	svc := &ApiService{}
	revision, err := svc.ReverseProxyService.CurrentRevision()
	if err != nil {
		t.Fatalf("load revision failed: %v", err)
	}
	_, created := performReverseProxyJSONPost(t, svc.SaveReverseProxyRule, `{
		"expectedRevision":`+uintJSON(uint(revision))+`,
		"name":"api-revision",
		"enabled":false,
		"listenProtocol":"http",
		"listenIPs":"127.0.0.1",
		"listenPort":18080,
		"targetProtocol":"http",
		"targetAddresses":"127.0.0.1",
		"targetPort":18081,
		"ipStrategy":"prefer_ipv4"
	}`)
	if !created.Success {
		t.Fatalf("create reverse proxy rule failed: %s", created.Msg)
	}

	var ruleID uint
	if err := database.GetDB().Table("reverse_proxy_rules").Where("name = ?", "api-revision").Pluck("id", &ruleID).Error; err != nil || ruleID == 0 {
		t.Fatalf("load created rule failed: id=%d err=%v", ruleID, err)
	}
	_, conflict := performReverseProxyJSONPost(t, svc.SetReverseProxyRuleStatus, `{
		"id":`+uintJSON(ruleID)+`,
		"enabled":true,
		"expectedRevision":`+uintJSON(uint(revision))+`
	}`)
	if conflict.Success {
		t.Fatal("stale status update unexpectedly succeeded")
	}
	obj, ok := conflict.Obj.(map[string]interface{})
	if !ok || obj["code"] != "revision_conflict" {
		t.Fatalf("missing structured revision conflict object: %#v", conflict.Obj)
	}
	if _, ok := obj["currentRevision"].(float64); !ok {
		t.Fatalf("missing current revision: %#v", conflict.Obj)
	}
}

func performReverseProxyJSONPost(t *testing.T, handler func(*gin.Context), body string) (*httptest.ResponseRecorder, Msg) {
	t.Helper()
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest("POST", "/api/reverse-proxy", strings.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	handler(ctx)
	return recorder, decodeAPIMessage(t, recorder.Body.String())
}
