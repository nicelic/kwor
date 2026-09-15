package model

import "time"

const (
	NetworkInterfaceCategoryPhysical  = "physical"  // 真实硬件物理网卡
	NetworkInterfaceCategorySimulated = "simulated" // 模拟物理网卡 (VirtIO, VMXNET3, Hyper-V, AWS ENA, GCP GVE, LXC 主网卡等)
	NetworkInterfaceCategoryVirtual   = "virtual"   // 应用/虚拟网卡 (TUN/TAP, WireGuard, Docker, veth, dummy, bridge 等)
	NetworkInterfaceCategoryLoopback  = "loopback"  // 回环网卡 (lo)
)

type NetworkInterfaceInfo struct {
	Id             uint      `json:"id" gorm:"primaryKey;autoIncrement"`
	Name           string    `json:"name" gorm:"uniqueIndex;not null;size:64"`
	DisplayName    string    `json:"displayName" gorm:"size:128"`
	Category       string    `json:"category" gorm:"index;size:32"`
	Driver         string    `json:"driver" gorm:"size:64"`
	MacAddress     string    `json:"macAddress" gorm:"size:32"`
	IPAddresses    string    `json:"ipAddresses" gorm:"type:text"` // JSON 数组存储 IPv4/IPv6 列表
	IsUp           bool      `json:"isUp" gorm:"index"`
	IsDefaultRoute bool      `json:"isDefaultRoute" gorm:"index"`
	SpeedMbps      int       `json:"speedMbps"`
	MTU            int       `json:"mtu"`
	Virtualization string    `json:"virtualization" gorm:"size:64"`
	FirstSeenAt    time.Time `json:"firstSeenAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
}
