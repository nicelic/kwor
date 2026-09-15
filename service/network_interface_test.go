package service

import (
	"testing"

	"github.com/alireza0/s-ui/database/model"
)

func TestClassifyLinuxInterface(t *testing.T) {
	tests := []struct {
		name         string
		ifaceName    string
		driver       string
		isLXC        bool
		hasIP        bool
		isDefault    bool
		wantCategory string
	}{
		{
			name:         "VirtIO KVM NIC",
			ifaceName:    "eth0",
			driver:       "virtio_net",
			isLXC:        false,
			hasIP:        true,
			isDefault:    true,
			wantCategory: model.NetworkInterfaceCategorySimulated,
		},
		{
			name:         "Secondary VirtIO NIC",
			ifaceName:    "eth1",
			driver:       "virtio_net",
			isLXC:        false,
			hasIP:        true,
			isDefault:    false,
			wantCategory: model.NetworkInterfaceCategorySimulated,
		},
		{
			name:         "VMware VMXNET3",
			ifaceName:    "ens160",
			driver:       "vmxnet3",
			isLXC:        false,
			hasIP:        true,
			isDefault:    true,
			wantCategory: model.NetworkInterfaceCategorySimulated,
		},
		{
			name:         "LXC Container veth as main eth0",
			ifaceName:    "eth0",
			driver:       "veth",
			isLXC:        true,
			hasIP:        true,
			isDefault:    true,
			wantCategory: model.NetworkInterfaceCategorySimulated,
		},
		{
			name:         "Sing-box Tun interface",
			ifaceName:    "sing-tun",
			driver:       "tun",
			isLXC:        false,
			hasIP:        true,
			isDefault:    false,
			wantCategory: model.NetworkInterfaceCategoryVirtual,
		},
		{
			name:         "Docker bridge interface",
			ifaceName:    "docker0",
			driver:       "bridge",
			isLXC:        false,
			hasIP:        true,
			isDefault:    false,
			wantCategory: model.NetworkInterfaceCategoryVirtual,
		},
		{
			name:         "WireGuard interface",
			ifaceName:    "wg0",
			driver:       "wireguard",
			isLXC:        false,
			hasIP:        true,
			isDefault:    false,
			wantCategory: model.NetworkInterfaceCategoryVirtual,
		},
		{
			name:         "Physical Intel NIC",
			ifaceName:    "eth0",
			driver:       "e1000e",
			isLXC:        false,
			hasIP:        true,
			isDefault:    true,
			wantCategory: model.NetworkInterfaceCategoryPhysical,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			category, _, _ := classifyLinuxInterface(tc.ifaceName, "/tmp", tc.driver, tc.isLXC, tc.hasIP, tc.isDefault)
			if category != tc.wantCategory {
				t.Errorf("classifyLinuxInterface(%s, %s) got %s, want %s", tc.ifaceName, tc.driver, category, tc.wantCategory)
			}
		})
	}
}

func TestParseVnstatMultiInterfaceTrafficTotals(t *testing.T) {
	jsonSample := `{
		"vnstatversion": "2.13",
		"jsonversion": "2",
		"interfaces": [
			{
				"name": "eth0",
				"traffic": {
					"total": { "rx": 1000, "tx": 2000 },
					"alltime": { "rx": 1000, "tx": 2000 }
				}
			},
			{
				"name": "eth1",
				"traffic": {
					"total": { "rx": 3000, "tx": 4000 },
					"alltime": { "rx": 3000, "tx": 4000 }
				}
			},
			{
				"name": "docker0",
				"traffic": {
					"total": { "rx": 9999, "tx": 9999 },
					"alltime": { "rx": 9999, "tx": 9999 }
				}
			}
		]
	}`

	// 1. 提取单网卡 eth0
	tx0, rx0, err0 := parseVnstatMultiInterfaceTrafficTotals(jsonSample, []string{"eth0"})
	if err0 != nil {
		t.Fatalf("parse eth0 failed: %v", err0)
	}
	if tx0 != 2000 || rx0 != 1000 {
		t.Errorf("eth0 totals tx=%d rx=%d, want tx=2000 rx=1000", tx0, rx0)
	}

	// 2. 多网卡聚合 eth0 + eth1 (排除 docker0)
	txMulti, rxMulti, errMulti := parseVnstatMultiInterfaceTrafficTotals(jsonSample, []string{"eth0", "eth1"})
	if errMulti != nil {
		t.Fatalf("parse multi eth0+eth1 failed: %v", errMulti)
	}
	if txMulti != 6000 || rxMulti != 4000 {
		t.Errorf("multi totals tx=%d rx=%d, want tx=6000 rx=4000", txMulti, rxMulti)
	}
}

func TestJoinSortedInterfaces(t *testing.T) {
	got := joinSortedInterfaces([]string{"eth1", "eth0", " eth2 "})
	want := "eth0,eth1,eth2"
	if got != want {
		t.Errorf("joinSortedInterfaces got %q, want %q", got, want)
	}
}
