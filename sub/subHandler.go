package sub

import (
	"strings"

	"github.com/alireza0/s-ui/database/model"
	"github.com/alireza0/s-ui/logger"
	"github.com/alireza0/s-ui/service"

	"github.com/gin-gonic/gin"
)

type SubHandler struct {
	service.SettingService
	SubService
	JsonService
	ClashService
	SubManagerSubService
}

func NewSubHandler(g *gin.RouterGroup) {
	a := &SubHandler{}
	a.initRouter(g)
}

func (s *SubHandler) initRouter(g *gin.RouterGroup) {
	g.GET("/q/client", s.subs)
	g.HEAD("/q/client", s.subHeaders)
	g.GET("/q/sm", s.subManagerSubs)
	g.GET("/sm/:tag", s.subManagerSubs)
	g.GET("/q/group", s.subGroupSubs)
	g.GET("/group/:groupName", s.subGroupSubs)
	g.GET("/q/mihomo", s.mihomoSubs)
	g.GET("/mihomo/:subid", s.mihomoSubs)
	g.HEAD("/q/mihomo", s.mihomoSubHeaders)
	g.HEAD("/mihomo/:subid", s.mihomoSubHeaders)
	g.GET("/:subid", s.subs)
	g.HEAD("/:subid", s.subHeaders)
}

func (s *SubHandler) subs(c *gin.Context) {
	s.renderClientSub(c, false)
}

func (s *SubHandler) mihomoSubs(c *gin.Context) {
	s.renderClientSub(c, true)
}

func (s *SubHandler) renderClientSub(c *gin.Context, mihomo bool) {
	var headers []string
	var result *string
	var err error
	subId := resolveClientSubscriptionID(c)
	format, isFormat := c.GetQuery("format")
	if isFormat {
		switch format {
		case "json":
			if mihomo {
				result, headers, err = s.JsonService.GetMihomoJson(subId, format)
			} else {
				result, headers, err = s.JsonService.GetJson(subId, format)
			}
		case "clash":
			if mihomo {
				result, headers, err = s.ClashService.GetMihomoClash(subId)
			} else {
				result, headers, err = s.ClashService.GetClash(subId)
			}
		}
		if err != nil || result == nil {
			logger.Error(err)
			c.String(400, "Error!")
			return
		}
	} else {
		if mihomo {
			result, headers, err = s.SubService.GetMihomoSubs(subId)
		} else {
			result, headers, err = s.SubService.GetSubs(subId)
		}
		if err != nil || result == nil {
			logger.Error(err)
			c.String(400, "Error!")
			return
		}
	}

	s.addHeaders(c, headers)

	c.String(200, *result)
}

func (s *SubHandler) subHeaders(c *gin.Context) {
	s.renderClientSubHeaders(c, false)
}

func (s *SubHandler) mihomoSubHeaders(c *gin.Context) {
	s.renderClientSubHeaders(c, true)
}

func (s *SubHandler) renderClientSubHeaders(c *gin.Context, mihomo bool) {
	subId := resolveClientSubscriptionID(c)
	var (
		client *model.Client
		err    error
	)
	if mihomo {
		client, err = s.SubService.getMihomoClientBySubId(subId)
	} else {
		client, err = s.SubService.getClientBySubId(subId)
	}
	if err != nil {
		logger.Error(err)
		c.String(400, "Error!")
		return
	}

	headers := s.SubService.getClientHeaders(client)
	s.addHeaders(c, headers)

	c.Status(200)
}

func (s *SubHandler) subManagerSubs(c *gin.Context) {
	tag := strings.TrimSpace(c.Param("tag"))
	if tag == "" {
		tag = strings.TrimSpace(c.Query("tag"))
	}
	format, _ := c.GetQuery("format")

	var result *string
	var err error

	switch format {
	case "json":
		result, err = s.SubManagerSubService.GetSubManagerJson(tag)
	case "clash":
		result, err = s.SubManagerSubService.GetSubManagerClash(tag)
	default:
		// 默认返回 JSON 格式
		result, err = s.SubManagerSubService.GetSubManagerJson(tag)
	}

	if err != nil || result == nil {
		logger.Error(err)
		c.String(400, "Error!")
		return
	}

	c.String(200, *result)
}

func (s *SubHandler) subGroupSubs(c *gin.Context) {
	groupName := strings.TrimSpace(c.Param("groupName"))
	if groupName == "" {
		groupName = strings.TrimSpace(c.Query("name"))
	}
	format, _ := c.GetQuery("format")

	var result *string
	var err error

	switch format {
	case "json":
		result, err = s.SubManagerSubService.GetSubGroupJson(groupName)
	case "clash":
		result, err = s.SubManagerSubService.GetSubGroupClash(groupName)
	default:
		// 默认返回 JSON 格式
		result, err = s.SubManagerSubService.GetSubGroupJson(groupName)
	}

	if err != nil || result == nil {
		logger.Error(err)
		c.String(400, "Error!")
		return
	}

	c.String(200, *result)
}

func (s *SubHandler) addHeaders(c *gin.Context, headers []string) {
	c.Writer.Header().Set("Subscription-Userinfo", headers[0])
	c.Writer.Header().Set("Profile-Update-Interval", headers[1])
	c.Writer.Header().Set("Profile-Title", headers[2])
}

func resolveClientSubscriptionID(c *gin.Context) string {
	subID := strings.TrimSpace(c.Param("subid"))
	if subID != "" {
		return subID
	}

	subID = strings.TrimSpace(c.Query("name"))
	if subID != "" {
		return subID
	}

	return strings.TrimSpace(c.Query("subid"))
}
