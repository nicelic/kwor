package model

import "encoding/json"

type Setting struct {
	Id    uint   `json:"id" form:"id" gorm:"primaryKey;autoIncrement"`
	Key   string `json:"key" form:"key"`
	Value string `json:"value" form:"value"`
}

type Tls struct {
	Id                  uint            `json:"id" form:"id" gorm:"primaryKey;autoIncrement"`
	Name                string          `json:"name" form:"name"`
	CertificateRecordID uint            `json:"certificateRecordId" form:"certificateRecordId" gorm:"column:certificate_record_id;not null;default:0;index"`
	Server              json.RawMessage `json:"server" form:"server"`
	Client              json.RawMessage `json:"client" form:"client"`
}

type User struct {
	Id         uint   `json:"id" form:"id" gorm:"primaryKey;autoIncrement"`
	Username   string `json:"username" form:"username"`
	Password   string `json:"password" form:"password"`
	LastLogins string `json:"lastLogin"`
}

type Client struct {
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

type Stats struct {
	Id        uint64 `json:"id" gorm:"primaryKey;autoIncrement"`
	DateTime  int64  `json:"dateTime"`
	Resource  string `json:"resource"`
	Tag       string `json:"tag"`
	Direction bool   `json:"direction"`
	Traffic   int64  `json:"traffic"`
}

type Changes struct {
	Id       uint64          `json:"id" gorm:"primaryKey;autoIncrement"`
	DateTime int64           `json:"dateTime"`
	Actor    string          `json:"actor"`
	Key      string          `json:"key"`
	Action   string          `json:"action"`
	Obj      json.RawMessage `json:"obj"`
}

type Tokens struct {
	Id     uint   `json:"id" form:"id" gorm:"primaryKey;autoIncrement"`
	Desc   string `json:"desc" form:"desc"`
	Token  string `json:"token" form:"token"`
	Expiry int64  `json:"expiry" form:"expiry"`
	UserId uint   `json:"userId" form:"userId"`
	User   *User  `json:"user" gorm:"foreignKey:UserId;references:Id"`
}
