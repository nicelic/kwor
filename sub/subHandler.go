package sub

import (
	"errors"
	"fmt"
	"strings"

	"github.com/alireza0/s-ui/database/model"
	"github.com/alireza0/s-ui/logger"
	"github.com/alireza0/s-ui/service"

	"github.com/gin-gonic/gin"
	"golang.org/x/sync/singleflight"
)

type SubHandler struct {
	service.SettingService
	SubService
	JsonService
	ClashService
	SubManagerSubService
}

const (
	// Rendering parses and merges a full extension and marshals YAML/JSON. Keep
	// the render pool below the request pool so bursts of distinct cache keys
	// cannot create a large simultaneous heap peak.
	subscriptionRenderMaxConcurrent  = 8
	subscriptionRequestMaxConcurrent = 64
)

var (
	subscriptionRequestSlots  = make(chan struct{}, subscriptionRequestMaxConcurrent)
	subscriptionRenderSlots   = make(chan struct{}, subscriptionRenderMaxConcurrent)
	subscriptionRenderFlight  singleflight.Group
	errSubscriptionRenderBusy = errors.New("subscription renderer is busy")
)

type clientSubscriptionRenderResult struct {
	content      string
	headers      []string
	showUserInfo bool
}

func tryAcquireSubscriptionRenderSlot() bool {
	select {
	case subscriptionRenderSlots <- struct{}{}:
		return true
	default:
		return false
	}
}

func releaseSubscriptionRenderSlot() {
	<-subscriptionRenderSlots
}

func tryAcquireSubscriptionRequestSlot() bool {
	select {
	case subscriptionRequestSlots <- struct{}{}:
		return true
	default:
		return false
	}
}

func releaseSubscriptionRequestSlot() {
	<-subscriptionRequestSlots
}

func writeSubscriptionRenderError(c *gin.Context, err error) {
	if errors.Is(err, errSubscriptionRenderBusy) {
		c.Header("Retry-After", "1")
		c.String(429, "Too Many Requests")
		return
	}
	logger.Error(err)
	c.String(400, "Error!")
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
	subId := resolveClientSubscriptionID(c)
	format, isFormat := c.GetQuery("format")
	if isFormat && format != "json" && format != "clash" {
		c.String(400, "Error!")
		return
	}
	if !tryAcquireSubscriptionRequestSlot() {
		writeSubscriptionRenderError(c, errSubscriptionRenderBusy)
		return
	}
	defer releaseSubscriptionRequestSlot()

	settingsGeneration := service.SubscriptionRuntimeSettingsGeneration()
	key := fmt.Sprintf("client|mihomo=%t|id=%s|format=%s|settings=%d", mihomo, subId, format, settingsGeneration)
	raw, err, _ := subscriptionRenderFlight.Do(key, func() (interface{}, error) {
		if !tryAcquireSubscriptionRenderSlot() {
			return nil, errSubscriptionRenderBusy
		}
		defer releaseSubscriptionRenderSlot()

		showUserInfo, showInfoErr := s.SettingService.GetSubShowInfo()
		if showInfoErr != nil {
			return nil, showInfoErr
		}
		var result *string
		var headers []string
		var renderErr error
		switch format {
		case "json":
			if mihomo {
				result, headers, renderErr = s.JsonService.GetMihomoJson(subId, format)
			} else {
				result, headers, renderErr = s.JsonService.GetJson(subId, format)
			}
		case "clash":
			if mihomo {
				result, headers, renderErr = s.ClashService.GetMihomoClash(subId)
			} else {
				result, headers, renderErr = s.ClashService.GetClash(subId)
			}
		default:
			if mihomo {
				result, headers, renderErr = s.SubService.GetMihomoSubs(subId)
			} else {
				result, headers, renderErr = s.SubService.GetSubs(subId)
			}
		}
		if renderErr != nil {
			return nil, renderErr
		}
		if result == nil {
			return nil, fmt.Errorf("subscription renderer returned no content")
		}
		return clientSubscriptionRenderResult{content: *result, headers: append([]string(nil), headers...), showUserInfo: showUserInfo}, nil
	})
	if err != nil {
		writeSubscriptionRenderError(c, err)
		return
	}
	response, ok := raw.(clientSubscriptionRenderResult)
	if !ok {
		logger.Error("invalid subscription render response")
		c.String(500, "Error!")
		return
	}
	if response.showUserInfo {
		s.addHeaders(c, response.headers)
	}

	c.String(200, response.content)
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

	if showUserInfo, showInfoErr := s.SettingService.GetSubShowInfo(); showInfoErr == nil && showUserInfo {
		headers := s.SubService.getClientHeaders(client)
		s.addHeaders(c, headers)
	}

	c.Status(200)
}

func (s *SubHandler) subManagerSubs(c *gin.Context) {
	tag := strings.TrimSpace(c.Param("tag"))
	if tag == "" {
		tag = strings.TrimSpace(c.Query("tag"))
	}
	format, _ := c.GetQuery("format")

	if format != "" && format != "json" && format != "clash" {
		c.String(400, "Error!")
		return
	}
	if !tryAcquireSubscriptionRequestSlot() {
		writeSubscriptionRenderError(c, errSubscriptionRenderBusy)
		return
	}
	defer releaseSubscriptionRequestSlot()

	key := fmt.Sprintf("sub-manager|tag=%s|format=%s|settings=%d", tag, format, service.SubscriptionRuntimeSettingsGeneration())
	raw, err, _ := subscriptionRenderFlight.Do(key, func() (interface{}, error) {
		if !tryAcquireSubscriptionRenderSlot() {
			return nil, errSubscriptionRenderBusy
		}
		defer releaseSubscriptionRenderSlot()

		var result *string
		var renderErr error
		if format == "clash" {
			result, renderErr = s.SubManagerSubService.GetSubManagerClash(tag)
		} else {
			result, renderErr = s.SubManagerSubService.GetSubManagerJson(tag)
		}
		if renderErr != nil {
			return nil, renderErr
		}
		if result == nil {
			return nil, fmt.Errorf("subscription renderer returned no content")
		}
		return clientSubscriptionRenderResult{content: *result}, nil
	})
	if err != nil {
		writeSubscriptionRenderError(c, err)
		return
	}
	result, ok := raw.(clientSubscriptionRenderResult)
	if !ok {
		logger.Error("invalid subscription render response")
		c.String(500, "Error!")
		return
	}
	c.String(200, result.content)
}

func (s *SubHandler) subGroupSubs(c *gin.Context) {
	groupName := strings.TrimSpace(c.Param("groupName"))
	if groupName == "" {
		groupName = strings.TrimSpace(c.Query("name"))
	}
	format, _ := c.GetQuery("format")

	if format != "" && format != "json" && format != "clash" {
		c.String(400, "Error!")
		return
	}
	if !tryAcquireSubscriptionRequestSlot() {
		writeSubscriptionRenderError(c, errSubscriptionRenderBusy)
		return
	}
	defer releaseSubscriptionRequestSlot()

	key := fmt.Sprintf("sub-group|name=%s|format=%s|settings=%d", groupName, format, service.SubscriptionRuntimeSettingsGeneration())
	raw, err, _ := subscriptionRenderFlight.Do(key, func() (interface{}, error) {
		if !tryAcquireSubscriptionRenderSlot() {
			return nil, errSubscriptionRenderBusy
		}
		defer releaseSubscriptionRenderSlot()

		var result *string
		var renderErr error
		if format == "clash" {
			result, renderErr = s.SubManagerSubService.GetSubGroupClash(groupName)
		} else {
			result, renderErr = s.SubManagerSubService.GetSubGroupJson(groupName)
		}
		if renderErr != nil {
			return nil, renderErr
		}
		if result == nil {
			return nil, fmt.Errorf("subscription renderer returned no content")
		}
		return clientSubscriptionRenderResult{content: *result}, nil
	})
	if err != nil {
		writeSubscriptionRenderError(c, err)
		return
	}
	result, ok := raw.(clientSubscriptionRenderResult)
	if !ok {
		logger.Error("invalid subscription render response")
		c.String(500, "Error!")
		return
	}
	c.String(200, result.content)
}

func (s *SubHandler) addHeaders(c *gin.Context, headers []string) {
	if len(headers) < 3 {
		logger.Error("subscription renderer returned incomplete client headers")
		return
	}
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
