package service

import (
	"encoding/json"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
	"github.com/alireza0/s-ui/util/common"

	"gorm.io/gorm"
)

type EndpointService struct {
	WarpService
}

// marshalEndpointSavePayload keeps the database-facing endpoint shape intact.
// Endpoint.MarshalJSON intentionally emits the Core-facing configuration shape,
// which turns WARP into wireguard and omits private Ext credentials.
func marshalEndpointSavePayload(endpoint *model.Endpoint) (json.RawMessage, error) {
	if endpoint == nil {
		return nil, common.NewError("endpoint is nil")
	}

	payload := make(map[string]json.RawMessage)
	if len(endpoint.Options) > 0 {
		if err := json.Unmarshal(endpoint.Options, &payload); err != nil {
			return nil, err
		}
	}

	id, err := json.Marshal(endpoint.Id)
	if err != nil {
		return nil, err
	}
	typeValue, err := json.Marshal(endpoint.Type)
	if err != nil {
		return nil, err
	}
	tag, err := json.Marshal(endpoint.Tag)
	if err != nil {
		return nil, err
	}
	ext := endpoint.Ext
	if len(ext) == 0 {
		ext = json.RawMessage("null")
	}

	payload["id"] = id
	payload["type"] = typeValue
	payload["tag"] = tag
	payload["ext"] = ext
	return json.Marshal(payload)
}

func (o *EndpointService) GetAll() (*[]map[string]interface{}, error) {
	db := database.GetDB()
	endpoints := []*model.Endpoint{}
	err := db.Model(model.Endpoint{}).Scan(&endpoints).Error
	if err != nil {
		return nil, err
	}
	var data []map[string]interface{}
	for _, endpoint := range endpoints {
		routeTag := deriveEffectiveEndpointRouteTagFromRaw(endpoint.Tag, endpoint.Options)
		epData := map[string]interface{}{
			"id":        endpoint.Id,
			"type":      endpoint.Type,
			"tag":       endpoint.Tag,
			"route_tag": routeTag,
			"ext":       endpoint.Ext,
		}
		if endpoint.Options != nil {
			var restFields map[string]json.RawMessage
			if err := json.Unmarshal(endpoint.Options, &restFields); err != nil {
				return nil, err
			}
			for k, v := range restFields {
				epData[k] = v
			}
		}
		data = append(data, epData)
	}
	return &data, nil
}

func (o *EndpointService) GetAllConfig(db *gorm.DB) ([]json.RawMessage, error) {
	var endpointsJson []json.RawMessage
	var endpoints []*model.Endpoint
	err := db.Model(model.Endpoint{}).Scan(&endpoints).Error
	if err != nil {
		return nil, err
	}
	for _, endpoint := range endpoints {
		endpointJson, err := endpoint.MarshalJSON()
		if err != nil {
			return nil, err
		}
		endpointsJson = append(endpointsJson, endpointJson)
	}
	return endpointsJson, nil
}

// PrepareSave performs WARP's remote API work before ConfigService opens its
// SQLite transaction. The endpoint page is hidden, but its compatibility API
// remains reachable and must not hold the single connection on network I/O.
func (s *EndpointService) PrepareSave(act string, data json.RawMessage) (json.RawMessage, error) {
	if act != "new" && act != "edit" {
		return data, nil
	}

	var endpoint model.Endpoint
	if err := endpoint.UnmarshalJSON(data); err != nil {
		return nil, err
	}
	if endpoint.Type != "warp" {
		return data, nil
	}

	if act == "new" {
		if err := s.WarpService.RegisterWarp(&endpoint); err != nil {
			return nil, err
		}
	} else {
		var oldLicense string
		if err := database.GetDB().Model(model.Endpoint{}).
			Select("json_extract(ext, '$.license_key')").
			Where("id = ?", endpoint.Id).
			Find(&oldLicense).Error; err != nil {
			return nil, err
		}
		if err := s.WarpService.SetWarpLicense(oldLicense, &endpoint); err != nil {
			return nil, err
		}
	}

	return marshalEndpointSavePayload(&endpoint)
}

func (s *EndpointService) Save(tx *gorm.DB, act string, data json.RawMessage) error {
	var err error

	switch act {
	case "new", "edit":
		var endpoint model.Endpoint
		err = endpoint.UnmarshalJSON(data)
		if err != nil {
			return err
		}

		err = tx.Save(&endpoint).Error
		if err != nil {
			return err
		}
	case "del":
		var tag string
		err = json.Unmarshal(data, &tag)
		if err != nil {
			return err
		}
		err = tx.Where("tag = ?", tag).Delete(model.Endpoint{}).Error
		if err != nil {
			return err
		}
	default:
		return common.NewErrorf("unknown action: %s", act)
	}
	return nil
}
