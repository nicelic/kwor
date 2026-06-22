package model

import "time"

// ReverseProxyRule stores panel-managed reverse proxy rules.
// Runtime matching order follows ListOrder first, then database ID.
type ReverseProxyRule struct {
	Id uint `json:"id" gorm:"primaryKey;autoIncrement"`

	DisplayID uint64 `json:"displayId" gorm:"column:display_id;not null;default:0;index"`
	ListOrder int64  `json:"listOrder" gorm:"column:list_order;not null;default:0;index"`

	Name    string `json:"name" gorm:"size:255;not null;default:''"`
	Enabled bool   `json:"enabled" gorm:"not null;default:true"`

	ListenProtocol      string `json:"listenProtocol" gorm:"size:16;not null;default:'http';index"`
	ListenProtocolAlias string `json:"listenProtocolAlias" gorm:"column:listen_protocol_alias;size:16;not null;default:''"`
	ListenIP            string `json:"listenIP" gorm:"size:255;not null;default:'';index"`
	ListenIPList        string `json:"listenIPs" gorm:"column:listen_ip_list;type:text;not null;default:''"`
	ListenPort          int    `json:"listenPort" gorm:"not null;default:0;index"`

	HostList      string `json:"hostList" gorm:"type:text;not null;default:''"`
	PathPrefix    string `json:"pathPrefix" gorm:"size:1024;not null;default:'/'"`
	ListenDNSPath string `json:"listenDnsPath" gorm:"column:listen_dns_path;size:1024;not null;default:''"`

	TargetProtocol      string `json:"targetProtocol" gorm:"size:16;not null;default:'http'"`
	TargetProtocolAlias string `json:"targetProtocolAlias" gorm:"column:target_protocol_alias;size:16;not null;default:''"`
	TargetAddresses     string `json:"targetAddresses" gorm:"type:text;not null;default:''"`
	TargetPort          int    `json:"targetPort" gorm:"not null;default:0"`
	TargetPath          string `json:"targetPath" gorm:"size:1024;not null;default:''"`
	TargetDNSPath       string `json:"targetDnsPath" gorm:"column:target_dns_path;size:1024;not null;default:''"`

	EDNSEnabled            bool   `json:"ednsEnabled" gorm:"column:edns_enabled;not null;default:false"`
	EDNSMode               string `json:"ednsMode" gorm:"column:edns_mode;size:32;not null;default:'auto'"`
	EDNSCustomIP           string `json:"ednsCustomIp" gorm:"column:edns_custom_ip;size:255;not null;default:''"`
	EDNSClientSubnetPolicy string `json:"ednsClientSubnetPolicy" gorm:"column:edns_client_subnet_policy;size:32;not null;default:'client_ip'"`
	DisableIPv4Answer      bool   `json:"disableIpv4Answer" gorm:"column:disable_ipv4_answer;not null;default:false"`
	DisableIPv6Answer      bool   `json:"disableIpv6Answer" gorm:"column:disable_ipv6_answer;not null;default:false"`

	CertificateRecordID       uint   `json:"certificateRecordId" gorm:"not null;default:0"`
	CertificateRecordList     string `json:"certificateRecordList" gorm:"column:certificate_record_list;type:text;not null;default:''"`
	ListenHTTPVersionStrategy string `json:"listenHttpVersionStrategy" gorm:"column:listen_http_version_strategy;size:32;not null;default:''"`
	IPStrategy                string `json:"ipStrategy" gorm:"size:32;not null;default:'prefer_ipv4'"`
	HTTPVersionStrategy       string `json:"httpVersionStrategy" gorm:"size:32;not null;default:''"`
	UpstreamTLSVerify         bool   `json:"upstreamTlsVerify" gorm:"not null;default:true"`
	ApiPassthrough            bool   `json:"apiPassthrough" gorm:"not null;default:false"`

	Remark        string `json:"remark" gorm:"type:text;not null;default:''"`
	LastError     string `json:"lastError" gorm:"type:text;not null;default:''"`
	RuntimeStatus string `json:"runtimeStatus" gorm:"size:64;not null;default:''"`

	UpdatedAt time.Time `json:"updatedAt"`
	CreatedAt time.Time `json:"createdAt"`
}
