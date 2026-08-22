package api

import (
	"errors"
	"fmt"

	"github.com/alireza0/s-ui/service"
	"github.com/gin-gonic/gin"
)

func (a *ApiService) GetSingboxBasicsEditorContext(c *gin.Context) {
	context, err := a.ConfigService.GetSingboxBasicsEditorContext()
	jsonObj(c, context, err)
}

func (a *ApiService) SaveSingboxBasics(c *gin.Context, actor string) {
	request := service.SingboxBasicsSaveRequest{}
	if err := c.ShouldBindJSON(&request); err != nil {
		jsonMsg(c, "", err)
		return
	}
	result, err := a.ConfigService.SaveSingboxBasics(request, actor)
	if err != nil {
		var committedErr *service.CommittedSaveError
		if errors.As(err, &committedErr) && result != nil {
			jsonMsgObj(c, "save", gin.H{
				"committed":    true,
				"retryRuntime": true,
				"result":       result,
			}, fmt.Errorf("basics data was saved, but sing-box runtime configuration refresh failed: %w", committedErr))
			return
		}
		var conflict *service.SingboxBasicsRevisionConflictError
		if errors.As(err, &conflict) && conflict != nil {
			jsonMsgObj(c, "", gin.H{
				"code":            "revision_conflict",
				"currentRevision": conflict.CurrentRevision,
			}, fmt.Errorf("sing-box basics configuration has changed in another page or window; reload before saving"))
			return
		}
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, result, nil)
}
