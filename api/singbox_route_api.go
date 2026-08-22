package api

import (
	"errors"
	"fmt"

	"github.com/alireza0/s-ui/service"
	"github.com/gin-gonic/gin"
)

func (a *ApiService) GetSingboxRouteEditorContext(c *gin.Context) {
	context, err := a.ConfigService.GetSingboxRouteEditorContext()
	jsonObj(c, context, err)
}

func (a *ApiService) SaveSingboxRoute(c *gin.Context, actor string) {
	request := service.SingboxRouteSaveRequest{}
	if err := c.ShouldBindJSON(&request); err != nil {
		jsonMsg(c, "", err)
		return
	}
	result, err := a.ConfigService.SaveSingboxRoute(request, actor)
	if err != nil {
		var committedErr *service.CommittedSaveError
		if errors.As(err, &committedErr) {
			jsonMsgObj(c, "", gin.H{
				"committed":    true,
				"retryRuntime": true,
				"result":       result,
			}, fmt.Errorf("route data was saved, but sing-box runtime configuration refresh failed: %w", committedErr))
			return
		}
		var conflict *service.SingboxRouteRevisionConflictError
		if errors.As(err, &conflict) && conflict != nil {
			jsonMsgObj(c, "", gin.H{
				"code":            "revision_conflict",
				"currentRevision": conflict.CurrentRevision,
			}, fmt.Errorf("route configuration has changed in another page or window; reload before saving"))
			return
		}
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, result, nil)
}
