package api

import (
	"errors"
	"fmt"

	"github.com/alireza0/s-ui/service"
	"github.com/gin-gonic/gin"
)

func (a *ApiService) SaveMihomoRoutePatch(c *gin.Context, actor string) {
	request := service.MihomoRoutePatchRequest{}
	if err := c.ShouldBindJSON(&request); err != nil {
		jsonMsg(c, "", err)
		return
	}
	result, err := a.ConfigService.SaveMihomoRoutePatch(request, actor, getHostname(c))
	if err != nil {
		var conflict *service.MihomoConfigRevisionConflictError
		if errors.As(err, &conflict) {
			jsonMsgObj(c, "save", gin.H{
				"code":            "revision_conflict",
				"currentRevision": conflict.CurrentRevision,
			}, err)
			return
		}
		var committedErr *service.CommittedSaveError
		if errors.As(err, &committedErr) && result != nil {
			jsonMsgObj(c, "save", gin.H{
				"config":       result.Config,
				"changed":      result.Changed,
				"revision":     result.Revision,
				"committed":    true,
				"retryRuntime": true,
			}, fmt.Errorf("route data was saved, but Mihomo runtime configuration refresh failed: %w", committedErr))
			return
		}
	}
	jsonMsgObj(c, "save", result, err)
}
