package api

import (
	"errors"
	"fmt"

	"github.com/alireza0/s-ui/service"
	"github.com/gin-gonic/gin"
)

func (a *ApiService) GetSingboxDNSEditorContext(c *gin.Context) {
	context, err := a.ConfigService.GetSingboxDNSSnapshot()
	jsonObj(c, context, err)
}

func (a *ApiService) SaveSingboxDNS(c *gin.Context, actor string) {
	request := service.SingboxDNSMutationRequest{}
	if err := c.ShouldBindJSON(&request); err != nil {
		jsonMsg(c, "", err)
		return
	}
	result, err := a.ConfigService.SaveSingboxDNS(request, actor)
	if err != nil {
		var committedErr *service.CommittedSaveError
		if errors.As(err, &committedErr) && result != nil {
			jsonMsgObj(c, "save", gin.H{
				"committed":    true,
				"retryRuntime": true,
				"result":       result,
			}, fmt.Errorf("DNS data was saved, but sing-box runtime configuration refresh failed: %w", committedErr))
			return
		}
		var conflict *service.SingboxDNSRevisionConflictError
		if errors.As(err, &conflict) && conflict != nil {
			jsonMsgObj(c, "", gin.H{
				"code":            "revision_conflict",
				"currentRevision": conflict.CurrentRevision,
			}, fmt.Errorf("sing-box DNS configuration has changed in another page or window; reload before saving"))
			return
		}
	}
	jsonObj(c, result, err)
}
