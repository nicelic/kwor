package model

import "encoding/json"

// SubOutbound 订阅出站配置（与 Outbound 结构完全一致，但存储在独立的表中）
// 用于避免与出站管理的数据冲突
type SubOutbound struct {
	Id              uint            `json:"id" form:"id" gorm:"primaryKey;autoIncrement"`
	Type            string          `json:"type" form:"type"`
	Tag             string          `json:"tag" form:"tag" gorm:"unique"`
	Options         json.RawMessage `json:"-" form:"-"`
	RawOutbound     json.RawMessage `json:"-" form:"-" gorm:"type:text"`
	ClashOptions    json.RawMessage `json:"-" form:"-" gorm:"type:text"`
	RawClashYAML    []byte          `json:"-" form:"-" gorm:"type:blob"`
	SourceType      string          `json:"-" form:"-" gorm:"index:idx_sub_outbound_source,priority:1"`
	SourceClientId  uint            `json:"-" form:"-" gorm:"index:idx_sub_outbound_source,priority:2"`
	SourceInboundId uint            `json:"-" form:"-" gorm:"index"`
}

func (o *SubOutbound) UnmarshalJSON(data []byte) error {
	var err error
	var raw map[string]interface{}
	if err = json.Unmarshal(data, &raw); err != nil {
		return err
	}

	// Extract fixed fields and store the rest in Options
	if val, exists := raw["id"].(float64); exists {
		o.Id = uint(val)
	}
	delete(raw, "id")
	o.Type, _ = raw["type"].(string)
	delete(raw, "type")
	o.Tag = raw["tag"].(string)
	delete(raw, "tag")

	// Remaining fields
	o.Options, err = json.MarshalIndent(raw, "", "  ")
	return err
}

// MarshalJSON customizes marshalling
func (o SubOutbound) MarshalJSON() ([]byte, error) {
	// Combine fixed fields and dynamic fields into one map
	combined := make(map[string]interface{})
	combined["type"] = o.Type
	combined["tag"] = o.Tag

	if o.Options != nil {
		var restFields map[string]json.RawMessage
		if err := json.Unmarshal(o.Options, &restFields); err != nil {
			return nil, err
		}

		for k, v := range restFields {
			combined[k] = v
		}
	}

	return json.Marshal(combined)
}
