package sub

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
	"github.com/alireza0/s-ui/service"
	"github.com/alireza0/s-ui/util"
)

type SubService struct {
	service.SettingService
	LinkService
}

func (s *SubService) GetSubs(subId string) (*string, []string, error) {
	client, err := s.getClientBySubId(subId)
	if err != nil {
		return nil, nil, err
	}

	return s.buildSubsForClient(client, false)
}

func (s *SubService) GetMihomoSubs(subId string) (*string, []string, error) {
	client, err := s.getMihomoClientBySubId(subId)
	if err != nil {
		return nil, nil, err
	}
	return s.buildSubsForClient(client, true)
}

func (s *SubService) buildSubsForClient(client *model.Client, mihomo bool) (*string, []string, error) {
	clientInfo := ""
	subShowInfo, _ := s.SettingService.GetSubShowInfo()
	if subShowInfo {
		clientInfo = s.getClientInfo(client)
	}

	linksArray := s.LinkService.GetLinks(&client.Links, "all", clientInfo)
	if util.NormalizeSubscriptionServerHost(client.ServerIp) != "" {
		if localLinks, err := s.buildCurrentLocalLinks(client, mihomo, clientInfo); err == nil {
			linksArray = append(localLinks, s.LinkService.GetLinks(&client.Links, "external", "")...)
		}
	}
	result := strings.Join(linksArray, "\n")

	headers := s.getClientHeaders(client)

	subEncode, _ := s.SettingService.GetSubEncode()
	if subEncode {
		result = base64.StdEncoding.EncodeToString([]byte(result))
	}

	return &result, headers, nil
}

func (s *SubService) buildCurrentLocalLinks(client *model.Client, mihomo bool, clientInfo string) ([]string, error) {
	if client == nil {
		return nil, nil
	}

	var inboundIDs []uint
	if err := json.Unmarshal(client.Inbounds, &inboundIDs); err != nil {
		return nil, err
	}
	if len(inboundIDs) == 0 {
		return nil, nil
	}

	localLinks := make([]Link, 0)
	if mihomo {
		var inbounds []model.MihomoInbound
		if err := database.GetDB().Model(model.MihomoInbound{}).Preload("Tls").Where("id in ?", inboundIDs).Find(&inbounds).Error; err != nil {
			return nil, err
		}
		inbounds = util.OrderMihomoInboundValuesByIDs(inboundIDs, inbounds)
		for _, inbound := range inbounds {
			base := inbound.ToBase()
			serverHost := util.ResolveSubscriptionServerHost(client.ServerIp, &base, "")
			for _, uri := range util.LinkGenerator(client.Config, &base, serverHost) {
				localLinks = append(localLinks, Link{
					Type:   "local",
					Remark: inbound.Tag,
					Uri:    uri,
				})
			}
		}
	} else {
		var inbounds []model.Inbound
		if err := database.GetDB().Model(model.Inbound{}).Preload("Tls").Where("id in ?", inboundIDs).Find(&inbounds).Error; err != nil {
			return nil, err
		}
		inbounds = util.OrderBaseInboundValuesByIDs(inboundIDs, inbounds)
		for _, inbound := range inbounds {
			serverHost := util.ResolveSubscriptionServerHost(client.ServerIp, &inbound, "")
			for _, uri := range util.LinkGenerator(client.Config, &inbound, serverHost) {
				localLinks = append(localLinks, Link{
					Type:   "local",
					Remark: inbound.Tag,
					Uri:    uri,
				})
			}
		}
	}

	if len(localLinks) == 0 {
		return nil, nil
	}

	raw, err := json.Marshal(localLinks)
	if err != nil {
		return nil, err
	}
	payload := json.RawMessage(raw)
	return s.LinkService.GetLinks(&payload, "all", clientInfo), nil
}

func (j *SubService) getClientBySubId(subId string) (*model.Client, error) {
	db := database.GetDB()
	client := &model.Client{}
	err := db.Model(model.Client{}).Where("enable = true and name = ?", subId).First(client).Error
	if err != nil {
		return nil, err
	}
	return client, nil
}

func (j *SubService) getMihomoClientBySubId(subId string) (*model.Client, error) {
	return loadMihomoClientBySubID(subId)
}

func (s *SubService) getClientHeaders(client *model.Client) []string {
	updateInterval, _ := s.SettingService.GetSubUpdates()
	return util.GetHeaders(client, updateInterval)
}

func (s *SubService) getClientInfo(c *model.Client) string {
	now := time.Now().Unix()

	var result []string
	if vol := c.Volume - (c.Up + c.Down); vol > 0 {
		result = append(result, fmt.Sprintf("%s%s", s.formatTraffic(vol), "📊"))
	}
	if c.Expiry > 0 {
		result = append(result, fmt.Sprintf("%d%s⏳", (c.Expiry-now)/86400, "Days"))
	}
	if len(result) > 0 {
		return " " + strings.Join(result, " ")
	} else {
		return " ♾"
	}
}

func (s *SubService) formatTraffic(trafficBytes int64) string {
	if trafficBytes < 1024 {
		return fmt.Sprintf("%.2fB", float64(trafficBytes)/float64(1))
	} else if trafficBytes < (1024 * 1024) {
		return fmt.Sprintf("%.2fKB", float64(trafficBytes)/float64(1024))
	} else if trafficBytes < (1024 * 1024 * 1024) {
		return fmt.Sprintf("%.2fMB", float64(trafficBytes)/float64(1024*1024))
	} else if trafficBytes < (1024 * 1024 * 1024 * 1024) {
		return fmt.Sprintf("%.2fGB", float64(trafficBytes)/float64(1024*1024*1024))
	} else if trafficBytes < (1024 * 1024 * 1024 * 1024 * 1024) {
		return fmt.Sprintf("%.2fTB", float64(trafficBytes)/float64(1024*1024*1024*1024))
	} else {
		return fmt.Sprintf("%.2fEB", float64(trafficBytes)/float64(1024*1024*1024*1024*1024))
	}
}
