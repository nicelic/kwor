package api

import (
	"github.com/alireza0/s-ui/service"
	"github.com/gin-gonic/gin"
)

func (a *ApiService) ProbeSubscriptionRuleSets(c *gin.Context) {
	request := service.SubscriptionRuleSetProbeRequest{}
	if err := c.ShouldBindJSON(&request); err != nil {
		jsonMsg(c, "", err)
		return
	}
	results, err := service.ProbeSubscriptionRuleSets(c.Request.Context(), request)
	jsonObj(c, results, err)
}
