package api

import (
	"errors"

	"github.com/alireza0/s-ui/database/model"
	"github.com/alireza0/s-ui/service/ddns"
	"github.com/gin-gonic/gin"
)

func (a *ApiService) GetDDNSOverview(c *gin.Context) {
	overview, err := a.DDNSService.GetOverview()
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, overview, nil)
}

func (a *ApiService) SaveDDNSRule(c *gin.Context) {
	var rule model.DDNSRule
	if err := c.ShouldBindJSON(&rule); err != nil {
		jsonMsg(c, "Invalid request payload", err)
		return
	}

	if err := a.DDNSService.SaveRule(&rule); err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonMsg(c, "DDNS rule saved successfully", nil)
}

type toggleDDNSRuleForm struct {
	ID      uint `json:"id" binding:"required"`
	Enabled bool `json:"enabled"`
}

func (a *ApiService) ToggleDDNSRule(c *gin.Context) {
	var form toggleDDNSRuleForm
	if err := c.ShouldBindJSON(&form); err != nil {
		jsonMsg(c, "Invalid request payload", err)
		return
	}

	if err := a.DDNSService.ToggleRule(form.ID, form.Enabled); err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonMsg(c, "DDNS rule status updated", nil)
}

type idForm struct {
	ID uint `json:"id" binding:"required"`
}

func (a *ApiService) DeleteDDNSRule(c *gin.Context) {
	var form idForm
	if err := c.ShouldBindJSON(&form); err != nil {
		jsonMsg(c, "Invalid request payload", err)
		return
	}

	if err := a.DDNSService.DeleteRule(form.ID); err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonMsg(c, "DDNS rule deleted", nil)
}

func (a *ApiService) SyncDDNSRule(c *gin.Context) {
	var form idForm
	if err := c.ShouldBindJSON(&form); err != nil {
		jsonMsg(c, "Invalid request payload", err)
		return
	}

	if err := a.DDNSService.SyncRule(form.ID); err != nil {
		jsonMsg(c, "Sync failed", err)
		return
	}
	jsonMsg(c, "Sync completed successfully", nil)
}

func (a *ApiService) SyncAllDDNSRules(c *gin.Context) {
	if err := a.DDNSService.SyncAllRules(); err != nil {
		jsonMsg(c, "Some rules failed to sync", err)
		return
	}
	jsonMsg(c, "All active rules synced", nil)
}

func (a *ApiService) SaveDDNSAccount(c *gin.Context) {
	var account model.DDNSAccount
	if err := c.ShouldBindJSON(&account); err != nil {
		jsonMsg(c, "Invalid request payload", err)
		return
	}

	if err := a.DDNSService.SaveAccount(&account); err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonMsg(c, "DNS account saved successfully", nil)
}

func (a *ApiService) DeleteDDNSAccount(c *gin.Context) {
	var form idForm
	if err := c.ShouldBindJSON(&form); err != nil {
		jsonMsg(c, "Invalid request payload", err)
		return
	}

	if err := a.DDNSService.DeleteAccount(form.ID); err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonMsg(c, "DNS account deleted", nil)
}

func (a *ApiService) TestDDNSAccountAuth(c *gin.Context) {
	var account model.DDNSAccount
	if err := c.ShouldBindJSON(&account); err != nil {
		jsonMsg(c, "Invalid request payload", err)
		return
	}

	if err := a.DDNSService.TestAccountAuth(&account); err != nil {
		jsonMsg(c, "Authentication failed: "+err.Error(), errors.New("test failed"))
		return
	}
	jsonMsg(c, "Credentials verified successfully", nil)
}

func (a *ApiService) GetDDNSInterfaces(c *gin.Context) {
	ifaces, err := ddns.GetSystemInterfaces()
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, ifaces, nil)
}
