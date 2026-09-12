package api

import (
	"errors"
	"strconv"

	"github.com/alireza0/s-ui/database/model"
	"github.com/alireza0/s-ui/service/ddns"
	"github.com/gin-gonic/gin"
)

func (a *ApiService) GetDNSOverview(c *gin.Context) {
	overview, err := a.DNSService.GetOverview()
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, overview, nil)
}

func (a *ApiService) ListDNSRecords(c *gin.Context) {
	accountIDStr := c.Query("accountId")
	domain := c.Query("domain")

	if accountIDStr == "" || domain == "" {
		jsonMsg(c, "Missing accountId or domain parameter", errors.New("missing parameters"))
		return
	}

	accountID, err := strconv.ParseUint(accountIDStr, 10, 64)
	if err != nil || accountID == 0 {
		jsonMsg(c, "Invalid account ID", errors.New("invalid accountId"))
		return
	}

	records, err := a.DNSService.ListRecords(uint(accountID), domain)
	if err != nil {
		jsonMsg(c, "", err)
		return
	}

	jsonObj(c, records, nil)
}

func (a *ApiService) ListDNSDomains(c *gin.Context) {
	accountIDStr := c.Query("accountId")
	if accountIDStr == "" {
		jsonMsg(c, "Missing accountId parameter", errors.New("missing parameters"))
		return
	}

	accountID, err := strconv.ParseUint(accountIDStr, 10, 64)
	if err != nil || accountID == 0 {
		jsonMsg(c, "Invalid account ID", errors.New("invalid accountId"))
		return
	}

	domains, err := a.DNSService.ListDomains(uint(accountID))
	if err != nil {
		jsonMsg(c, "", err)
		return
	}

	jsonObj(c, domains, nil)
}

type dnsRecordOpForm struct {
	AccountID uint           `json:"accountId" binding:"required"`
	Domain    string         `json:"domain" binding:"required"`
	Record    ddns.DNSRecord `json:"record" binding:"required"`
}

func (a *ApiService) CreateDNSRecord(c *gin.Context) {
	var form dnsRecordOpForm
	if err := c.ShouldBindJSON(&form); err != nil {
		jsonMsg(c, "Invalid request payload", err)
		return
	}

	createdID, err := a.DNSService.CreateRecord(form.AccountID, form.Domain, form.Record)
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, gin.H{"id": createdID}, nil)
}

func (a *ApiService) UpdateDNSRecord(c *gin.Context) {
	var form dnsRecordOpForm
	if err := c.ShouldBindJSON(&form); err != nil {
		jsonMsg(c, "Invalid request payload", err)
		return
	}

	if err := a.DNSService.UpdateRecord(form.AccountID, form.Domain, form.Record); err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonMsg(c, "DNS record updated successfully", nil)
}

type dnsRecordDeleteForm struct {
	AccountID uint   `json:"accountId" binding:"required"`
	Domain    string `json:"domain" binding:"required"`
	RecordID  string `json:"recordId" binding:"required"`
}

func (a *ApiService) DeleteDNSRecord(c *gin.Context) {
	var form dnsRecordDeleteForm
	if err := c.ShouldBindJSON(&form); err != nil {
		jsonMsg(c, "Invalid request payload", err)
		return
	}

	if err := a.DNSService.DeleteRecord(form.AccountID, form.Domain, form.RecordID); err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonMsg(c, "DNS record deleted successfully", nil)
}

func (a *ApiService) SaveDNSFavoriteDomain(c *gin.Context) {
	var fav model.DNSDomainFavorite
	if err := c.ShouldBindJSON(&fav); err != nil {
		jsonMsg(c, "Invalid request payload", err)
		return
	}

	if err := a.DNSService.SaveFavoriteDomain(&fav); err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonMsg(c, "Domain saved successfully", nil)
}

func (a *ApiService) DeleteDNSFavoriteDomain(c *gin.Context) {
	var form idForm
	if err := c.ShouldBindJSON(&form); err != nil {
		jsonMsg(c, "Invalid request payload", err)
		return
	}

	if err := a.DNSService.DeleteFavoriteDomain(form.ID); err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonMsg(c, "Domain deleted successfully", nil)
}

func (a *ApiService) SaveDNSAccount(c *gin.Context) {
	var account model.DNSAccount
	if err := c.ShouldBindJSON(&account); err != nil {
		jsonMsg(c, "Invalid request payload", err)
		return
	}

	if err := a.DNSService.SaveAccount(&account); err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonMsg(c, "DNS account saved successfully", nil)
}

func (a *ApiService) DeleteDNSAccount(c *gin.Context) {
	var form idForm
	if err := c.ShouldBindJSON(&form); err != nil {
		jsonMsg(c, "Invalid request payload", err)
		return
	}

	if err := a.DNSService.DeleteAccount(form.ID); err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonMsg(c, "DNS account deleted successfully", nil)
}

func (a *ApiService) TestDNSAccountAuth(c *gin.Context) {
	var account model.DNSAccount
	if err := c.ShouldBindJSON(&account); err != nil {
		jsonMsg(c, "Invalid request payload", err)
		return
	}

	if err := a.DNSService.TestAccountAuth(&account); err != nil {
		jsonMsg(c, "Authentication failed", err)
		return
	}
	jsonMsg(c, "Credentials verified successfully", nil)
}
