package model

import "encoding/json"

// DnsServer stores one reusable sing-box DNS server card. The core-facing JSON
// is kept in Options so only the DNS server selected by dns.final needs to be
// rendered into the final sing-box configuration.
type DnsServer struct {
	Id      uint            `json:"id" form:"id" gorm:"primaryKey;autoIncrement"`
	Type    string          `json:"type" form:"type"`
	Tag     string          `json:"tag" form:"tag" gorm:"unique"`
	Options json.RawMessage `json:"-" form:"-"`
}

func (s *DnsServer) UnmarshalJSON(data []byte) error {
	raw := map[string]json.RawMessage{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	if value, ok := raw["id"]; ok {
		if err := json.Unmarshal(value, &s.Id); err != nil {
			return err
		}
	}
	delete(raw, "id")

	if value, ok := raw["type"]; ok {
		if err := json.Unmarshal(value, &s.Type); err != nil {
			return err
		}
	}
	delete(raw, "type")

	if value, ok := raw["tag"]; ok {
		if err := json.Unmarshal(value, &s.Tag); err != nil {
			return err
		}
	}
	delete(raw, "tag")

	options, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return err
	}
	s.Options = options
	return nil
}

// MarshalJSON returns the sing-box DNS server shape. Database-only fields such
// as id are deliberately excluded from the generated core configuration.
func (s DnsServer) MarshalJSON() ([]byte, error) {
	combined := map[string]interface{}{
		"type": s.Type,
		"tag":  s.Tag,
	}
	if len(s.Options) > 0 {
		var options map[string]json.RawMessage
		if err := json.Unmarshal(s.Options, &options); err != nil {
			return nil, err
		}
		for key, value := range options {
			combined[key] = value
		}
	}
	return json.Marshal(combined)
}

func (s DnsServer) MarshalFull() (map[string]interface{}, error) {
	combined := map[string]interface{}{
		"id":   s.Id,
		"type": s.Type,
		"tag":  s.Tag,
	}
	if len(s.Options) > 0 {
		var options map[string]interface{}
		if err := json.Unmarshal(s.Options, &options); err != nil {
			return nil, err
		}
		for key, value := range options {
			combined[key] = value
		}
	}
	return combined, nil
}
