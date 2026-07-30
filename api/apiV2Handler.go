package api

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/alireza0/s-ui/logger"
	"github.com/alireza0/s-ui/service"
	"github.com/alireza0/s-ui/util/common"

	"github.com/gin-gonic/gin"
)

type TokenInMemory struct {
	Token    string
	Expiry   int64
	Username string
}

type APIv2Handler struct {
	ApiService
	tokensMu sync.RWMutex
	tokens   []TokenInMemory
}

func NewAPIv2Handler(g *gin.RouterGroup) *APIv2Handler {
	return NewAPIv2HandlerWithCoreManagers(g, nil, nil)
}

func NewAPIv2HandlerWithCoreManagers(g *gin.RouterGroup, coreManager *service.CoreManagerService, mihomoCoreManager *service.MihomoCoreManagerService) *APIv2Handler {
	a := &APIv2Handler{}
	a.ApiService.SetCoreManagers(coreManager, mihomoCoreManager)
	a.ReloadTokens()
	a.initRouter(g)
	return a
}

func (a *APIv2Handler) initRouter(g *gin.RouterGroup) {
	g.Use(noStoreDynamicAPIResponse)
	g.Use(func(c *gin.Context) {
		a.checkToken(c)
	})
	g.POST("/:postAction", a.postHandler)
	g.GET("/:getAction", a.getHandler)
}

func (a *APIv2Handler) postHandler(c *gin.Context) {
	username := a.findUsername(c)
	action := c.Param("postAction")
	applyAPIRequestBodyLimit(c, action)
	operation, err := service.BeginKworInProcessOperation("apiv2-post-" + action)
	if err != nil {
		jsonMsg(c, "failed", common.NewError("卸载停工状态拒绝当前写请求: ", err))
		return
	}
	defer operation.Done()

	switch action {
	case "save":
		a.ApiService.Save(c, username)
	case "settings-patch":
		a.ApiService.SaveSettingsPatch(c, username)
	case "subscription-initial-reset":
		a.ApiService.ResetSubscriptionToInitialState(c, username)
	case "subscription-ruleset-probe":
		a.ApiService.ProbeSubscriptionRuleSets(c)
	case "restartApp":
		a.ApiService.RestartApp(c)
	case "linkConvert":
		a.ApiService.LinkConvert(c)
	case "importdb":
		a.ApiService.ImportDb(c)
	case "restore-db-backup":
		a.ApiService.RestoreDBBackup(c)
	case "portOccupancy":
		a.ApiService.CheckPortOccupancy(c)
	default:
		jsonMsg(c, "failed", common.NewError("unknown action: ", action))
	}
}

func (a *APIv2Handler) getHandler(c *gin.Context) {
	action := c.Param("getAction")

	switch action {
	case "load":
		a.ApiService.LoadData(c)
	case "inbounds", "outbounds", "endpoints", "services", "tls", "clients", "config":
		err := a.ApiService.LoadPartialData(c, []string{action})
		if err != nil {
			jsonMsg(c, action, err)
		}
		return
	case "users":
		a.ApiService.GetUsers(c)
	case "settings":
		a.ApiService.GetSettings(c)
	case "settings-snapshot":
		a.ApiService.GetSettingsSnapshot(c)
	case "subscription-settings-snapshot":
		a.ApiService.GetSubscriptionSettingsSnapshot(c)
	case "subscription-uri":
		a.ApiService.GetSubscriptionURI(c)
	case "panel-time-context":
		a.ApiService.GetPanelTimeContext(c)
	case "system-timezone":
		a.ApiService.GetSystemTimeZone(c)
	case "stats":
		a.ApiService.GetStats(c)
	case "status":
		a.ApiService.GetStatus(c)
	case "onlines":
		a.ApiService.GetOnlines(c)
	case "logs":
		a.ApiService.GetLogs(c)
	case "changes":
		a.ApiService.CheckChanges(c)
	case "keypairs":
		a.ApiService.GetKeypairs(c)
	case "tlsSelfSignedTemplates":
		a.ApiService.GetTLSSelfSignedTemplates(c)
	case "getdb":
		a.ApiService.GetDb(c)
	case "download-db-backup":
		a.ApiService.DownloadDBBackup(c)
	default:
		jsonMsg(c, "failed", common.NewError("unknown action: ", action))
	}
}

func (a *APIv2Handler) findUsername(c *gin.Context) string {
	if c == nil || c.Request == nil {
		return ""
	}
	token := c.Request.Header.Get("Token")
	if token == "" {
		return ""
	}

	now := time.Now().Unix()
	a.tokensMu.RLock()
	defer a.tokensMu.RUnlock()
	for _, t := range a.tokens {
		if t.Expiry > 0 && t.Expiry < now {
			continue
		}
		if t.Token == token {
			return t.Username
		}
	}
	return ""
}

func (a *APIv2Handler) setTokens(tokens []TokenInMemory) {
	newTokens := append([]TokenInMemory(nil), tokens...)
	a.tokensMu.Lock()
	a.tokens = newTokens
	a.tokensMu.Unlock()
}

func (a *APIv2Handler) checkToken(c *gin.Context) {
	username := a.findUsername(c)
	if username != "" {
		c.Next()
		return
	}
	jsonMsg(c, "", common.NewError("invalid token"))
	c.Abort()
}

func (a *APIv2Handler) ReloadTokens() {
	tokens, err := a.ApiService.LoadTokens()
	if err == nil {
		var newTokens []TokenInMemory
		err = json.Unmarshal(tokens, &newTokens)
		if err != nil {
			logger.Error("unable to load tokens: ", err)
			return
		}
		a.setTokens(newTokens)
	} else {
		logger.Error("unable to load tokens: ", err)
	}
}
