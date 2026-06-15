package model

import (
	"encoding/json"
	"time"
)

func copyRawMessage(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return nil
	}
	cloned := make([]byte, len(raw))
	copy(cloned, raw)
	return json.RawMessage(cloned)
}

var mihomoServerTLSKeysToStrip = []string{
	"min_version",
	"max_version",
	"cipher_suites",
	"client_authentication",
	"client_certificate",
	"client_certificate_path",
	"client_certificate_public_key_sha256",
}

var mihomoClientTLSKeysToStrip = []string{
	"store",
	"tls_store",
	"mihomo_use_fingerprint",
	"certificate",
	"certificate_path",
	"client_certificate",
	"client_certificate_path",
	"client_key",
	"client_key_path",
}

var mihomoOutboundKeysToStrip = []string{
	"inet4_bind_address",
	"inet6_bind_address",
	"reuse_addr",
	"udp_fragment",
	"connect_timeout",
	"domain_resolver",
}

var mihomoOutboundUTLSSupportedTypes = map[string]struct{}{
	"vmess":       {},
	"vless":       {},
	"trojan":      {},
	"anytls":      {},
	"shadowtls":   {},
	"trusttunnel": {},
}

func sanitizeMihomoTLSRaw(raw json.RawMessage, keys ...string) json.RawMessage {
	if len(raw) == 0 {
		return nil
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return copyRawMessage(raw)
	}

	for _, key := range keys {
		delete(payload, key)
	}

	if len(payload) == 0 {
		return json.RawMessage([]byte("{}"))
	}

	sanitized, err := json.Marshal(payload)
	if err != nil {
		return copyRawMessage(raw)
	}

	return json.RawMessage(sanitized)
}

func sanitizeMihomoOutboundRaw(raw json.RawMessage, outType string) json.RawMessage {
	if len(raw) == 0 {
		return nil
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return copyRawMessage(raw)
	}

	for _, key := range mihomoOutboundKeysToStrip {
		delete(payload, key)
	}

	switch outType {
	case "selector":
		delete(payload, "default")
		delete(payload, "url")
		delete(payload, "interval")
		delete(payload, "tolerance")
		delete(payload, "idle_timeout")
		delete(payload, "interrupt_exist_connections")
	case "urltest":
		delete(payload, "default")
		delete(payload, "idle_timeout")
		delete(payload, "interrupt_exist_connections")
	}

	if tlsMap, ok := payload["tls"].(map[string]interface{}); ok && tlsMap != nil {
		sanitizeMihomoOutboundTLSMap(tlsMap, outType)
	}

	if len(payload) == 0 {
		return json.RawMessage([]byte("{}"))
	}

	sanitized, err := json.Marshal(payload)
	if err != nil {
		return copyRawMessage(raw)
	}

	return json.RawMessage(sanitized)
}

func sanitizeMihomoOutboundTLSMap(tlsMap map[string]interface{}, outType string) {
	if tlsMap == nil {
		return
	}

	if _, ok := mihomoOutboundUTLSSupportedTypes[outType]; !ok {
		delete(tlsMap, "utls")
	}
	if outType == "anytls" {
		delete(tlsMap, "reality")
	}
}

type MihomoTls struct {
	Id                  uint            `json:"id" form:"id" gorm:"primaryKey;autoIncrement"`
	Name                string          `json:"name" form:"name"`
	CertificateRecordID uint            `json:"certificateRecordId" form:"certificateRecordId" gorm:"column:certificate_record_id;not null;default:0;index"`
	Server              json.RawMessage `json:"server" form:"server"`
	Client              json.RawMessage `json:"client" form:"client"`
}

func (MihomoTls) TableName() string {
	return "mihomo_tls"
}

func (t *MihomoTls) Sanitize() {
	if t == nil {
		return
	}
	t.Server = sanitizeMihomoTLSRaw(t.Server, mihomoServerTLSKeysToStrip...)
	t.Client = sanitizeMihomoTLSRaw(t.Client, mihomoClientTLSKeysToStrip...)
}

func (t *MihomoTls) ToBase() *Tls {
	if t == nil {
		return nil
	}
	cloned := &MihomoTls{
		Id:                  t.Id,
		Name:                t.Name,
		CertificateRecordID: t.CertificateRecordID,
		Server:              copyRawMessage(t.Server),
		Client:              copyRawMessage(t.Client),
	}
	cloned.Sanitize()
	return &Tls{
		Id:                  cloned.Id,
		Name:                cloned.Name,
		CertificateRecordID: cloned.CertificateRecordID,
		Server:              copyRawMessage(cloned.Server),
		Client:              copyRawMessage(cloned.Client),
	}
}

func mihomoTlsFromBase(base *Tls) *MihomoTls {
	if base == nil {
		return nil
	}
	tls := &MihomoTls{
		Id:                  base.Id,
		Name:                base.Name,
		CertificateRecordID: base.CertificateRecordID,
		Server:              copyRawMessage(base.Server),
		Client:              copyRawMessage(base.Client),
	}
	tls.Sanitize()
	return tls
}

type MihomoClient struct {
	Id                    uint            `json:"id" form:"id" gorm:"primaryKey;autoIncrement"`
	Enable                bool            `json:"enable" form:"enable"`
	Name                  string          `json:"name" form:"name"`
	Config                json.RawMessage `json:"config,omitempty" form:"config"`
	Inbounds              json.RawMessage `json:"inbounds" form:"inbounds"`
	Links                 json.RawMessage `json:"links,omitempty" form:"links"`
	Volume                int64           `json:"volume" form:"volume"`
	Expiry                int64           `json:"expiry" form:"expiry"`
	Down                  int64           `json:"down" form:"down"`
	Up                    int64           `json:"up" form:"up"`
	Desc                  string          `json:"desc" form:"desc"`
	Group                 string          `json:"group" form:"group"`
	ServerIp              string          `json:"serverIp" form:"serverIp"`
	SpeedLimitMbps        int             `json:"speedLimitMbps" form:"speedLimitMbps"`
	Extra                 int             `json:"extra" form:"extra"`
	LastReset             int64           `json:"lastReset" form:"lastReset"`
	TrafficResetRequested bool            `json:"trafficResetRequested" form:"trafficResetRequested" gorm:"-"`
}

func (MihomoClient) TableName() string {
	return "mihomo_clients"
}

type MihomoInbound struct {
	Id   uint   `json:"id" form:"id" gorm:"primaryKey;autoIncrement"`
	Type string `json:"type" form:"type"`
	Tag  string `json:"tag" form:"tag" gorm:"unique"`

	TlsId uint       `json:"tls_id" form:"tls_id"`
	Tls   *MihomoTls `json:"tls" form:"tls" gorm:"foreignKey:TlsId;references:Id"`

	Addrs   json.RawMessage `json:"addrs" form:"addrs"`
	OutJson json.RawMessage `json:"out_json" form:"out_json"`
	Options json.RawMessage `json:"-" form:"-"`
}

func (MihomoInbound) TableName() string {
	return "mihomo_inbounds"
}

func (i *MihomoInbound) UnmarshalJSON(data []byte) error {
	var base Inbound
	if err := base.UnmarshalJSON(data); err != nil {
		return err
	}
	i.FromBase(base)
	return nil
}

func (i MihomoInbound) MarshalJSON() ([]byte, error) {
	base := i.ToBase()
	return base.MarshalJSON()
}

func (i MihomoInbound) MarshalFull() (*map[string]interface{}, error) {
	base := i.ToBase()
	return base.MarshalFull()
}

func (i MihomoInbound) ToBase() Inbound {
	base := Inbound{
		Id:      i.Id,
		Type:    i.Type,
		Tag:     i.Tag,
		TlsId:   i.TlsId,
		Addrs:   copyRawMessage(i.Addrs),
		OutJson: copyRawMessage(i.OutJson),
		Options: copyRawMessage(i.Options),
	}
	base.Tls = i.Tls.ToBase()
	return base
}

func (i *MihomoInbound) FromBase(base Inbound) {
	i.Id = base.Id
	i.Type = base.Type
	i.Tag = base.Tag
	i.TlsId = base.TlsId
	i.Tls = mihomoTlsFromBase(base.Tls)
	i.Addrs = copyRawMessage(base.Addrs)
	i.OutJson = copyRawMessage(base.OutJson)
	i.Options = copyRawMessage(base.Options)
}

type MihomoOutbound struct {
	Id           uint            `json:"id" form:"id" gorm:"primaryKey;autoIncrement"`
	Type         string          `json:"type" form:"type"`
	Tag          string          `json:"tag" form:"tag" gorm:"unique"`
	Options      json.RawMessage `json:"-" form:"-"`
	RawOutbound  json.RawMessage `json:"-" form:"-" gorm:"type:text"`
	RawClashYAML []byte          `json:"-" form:"-" gorm:"type:blob"`
}

func (MihomoOutbound) TableName() string {
	return "mihomo_outbounds"
}

func (o *MihomoOutbound) UnmarshalJSON(data []byte) error {
	var base Outbound
	if err := base.UnmarshalJSON(data); err != nil {
		return err
	}
	o.FromBase(base)
	return nil
}

func (o MihomoOutbound) MarshalJSON() ([]byte, error) {
	base := o.ToBase()
	return base.MarshalJSON()
}

func (o MihomoOutbound) ToBase() Outbound {
	return Outbound{
		Id:      o.Id,
		Type:    o.Type,
		Tag:     o.Tag,
		Options: sanitizeMihomoOutboundRaw(copyRawMessage(o.Options), o.Type),
	}
}

func (o *MihomoOutbound) FromBase(base Outbound) {
	o.Id = base.Id
	o.Type = base.Type
	o.Tag = base.Tag
	o.Options = sanitizeMihomoOutboundRaw(copyRawMessage(base.Options), base.Type)
}

type MihomoOutboundGroup struct {
	Id              uint      `json:"id" gorm:"primaryKey;autoIncrement"`
	Name            string    `json:"name" gorm:"unique;not null"`
	SortOrder       int       `json:"sort_order" gorm:"default:0;index"`
	Outbounds       string    `json:"outbounds" gorm:"type:text"`
	SubscriptionUrl string    `json:"subscription_url" gorm:"type:text"`
	AllowInsecure   bool      `json:"allow_insecure" gorm:"default:false"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

func (MihomoOutboundGroup) TableName() string {
	return "mihomo_outbound_groups"
}
