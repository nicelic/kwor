package service

import (
	"bufio"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
	"github.com/alireza0/s-ui/logger"
)

var (
	networkInterfaceMu       sync.RWMutex
	cachedNetworkInterfaces  []model.NetworkInterfaceInfo
	lastNetworkInterfaceSync time.Time
)

type NetworkInterfaceService struct{}

// SyncSystemInterfaces 探测系统当前网卡，持久化至数据库，并更新全局内存缓存。
func (s *NetworkInterfaceService) SyncSystemInterfaces() ([]model.NetworkInterfaceInfo, error) {
	networkInterfaceMu.Lock()
	defer networkInterfaceMu.Unlock()

	detected := detectSystemInterfacesInternal()
	now := time.Now()

	db := database.GetDB()
	if db != nil {
		var existing []model.NetworkInterfaceInfo
		if err := db.Find(&existing).Error; err == nil {
			existingMap := make(map[string]model.NetworkInterfaceInfo, len(existing))
			for _, item := range existing {
				existingMap[item.Name] = item
			}

			detectedNames := make(map[string]struct{}, len(detected))
			for idx := range detected {
				iface := &detected[idx]
				detectedNames[iface.Name] = struct{}{}
				if prev, ok := existingMap[iface.Name]; ok {
					iface.Id = prev.Id
					iface.FirstSeenAt = prev.FirstSeenAt
					if iface.FirstSeenAt.IsZero() {
						iface.FirstSeenAt = now
					}
					iface.UpdatedAt = now
					_ = db.Save(iface).Error
				} else {
					iface.FirstSeenAt = now
					iface.UpdatedAt = now
					_ = db.Create(iface).Error
				}
			}

			// 对于以前存在但当前未扫描到的网卡，标记为离线
			for _, prev := range existing {
				if _, ok := detectedNames[prev.Name]; !ok && prev.IsUp {
					prev.IsUp = false
					prev.IsDefaultRoute = false
					prev.UpdatedAt = now
					_ = db.Save(&prev).Error
				}
			}
		}
	}

	cachedNetworkInterfaces = cloneNetworkInterfaces(detected)
	lastNetworkInterfaceSync = now
	return cloneNetworkInterfaces(detected), nil
}

// GetSystemInterfaces 获取系统网卡列表，优先读取内存缓存；若缓存为空或过期则自动同步。
func (s *NetworkInterfaceService) GetSystemInterfaces(refresh bool) []model.NetworkInterfaceInfo {
	networkInterfaceMu.RLock()
	if !refresh && len(cachedNetworkInterfaces) > 0 && time.Since(lastNetworkInterfaceSync) < 30*time.Second {
		res := cloneNetworkInterfaces(cachedNetworkInterfaces)
		networkInterfaceMu.RUnlock()
		return res
	}
	networkInterfaceMu.RUnlock()

	res, err := s.SyncSystemInterfaces()
	if err != nil && len(cachedNetworkInterfaces) > 0 {
		networkInterfaceMu.RLock()
		defer networkInterfaceMu.RUnlock()
		return cloneNetworkInterfaces(cachedNetworkInterfaces)
	}
	return res
}

// GetDefaultTrafficInterfaces 获取推荐用于流量统计的网卡列表。
// 优先返回处于 UP 状态的物理网卡（physical）与模拟物理网卡（simulated）。
// 若均无，则回退至默认路由网卡或首个 UP 的非 lo 网卡。
func (s *NetworkInterfaceService) GetDefaultTrafficInterfaces() []string {
	ifaces := s.GetSystemInterfaces(false)
	var recommended []string
	var defaultRouteIface string

	for _, iface := range ifaces {
		if iface.IsDefaultRoute && defaultRouteIface == "" {
			defaultRouteIface = iface.Name
		}
		if iface.Category == model.NetworkInterfaceCategoryLoopback {
			continue
		}
		if iface.IsUp && (iface.Category == model.NetworkInterfaceCategoryPhysical || iface.Category == model.NetworkInterfaceCategorySimulated) {
			recommended = append(recommended, iface.Name)
		}
	}

	if len(recommended) > 0 {
		return recommended
	}

	if defaultRouteIface != "" && defaultRouteIface != "lo" {
		return []string{defaultRouteIface}
	}

	for _, iface := range ifaces {
		if iface.IsUp && iface.Category != model.NetworkInterfaceCategoryLoopback {
			return []string{iface.Name}
		}
	}

	return nil
}

func detectSystemInterfacesInternal() []model.NetworkInterfaceInfo {
	systemIfaces, err := net.Interfaces()
	if err != nil {
		logger.Warning("detect system interfaces failed:", err)
		return nil
	}

	defaultIface := detectDefaultRouteInterfaceName()
	isLXC := detectIsLXCEnvironment()

	var result []model.NetworkInterfaceInfo
	for _, rawIface := range systemIfaces {
		info := inspectSingleInterface(rawIface, defaultIface, isLXC)
		result = append(result, info)
	}

	// 排序：默认路由置顶，其次 physical/simulated，再次 virtual，最后 loopback
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].IsDefaultRoute != result[j].IsDefaultRoute {
			return result[i].IsDefaultRoute
		}
		priority := map[string]int{
			model.NetworkInterfaceCategoryPhysical:  1,
			model.NetworkInterfaceCategorySimulated: 2,
			model.NetworkInterfaceCategoryVirtual:   3,
			model.NetworkInterfaceCategoryLoopback:  4,
		}
		pi := priority[result[i].Category]
		pj := priority[result[j].Category]
		if pi != pj {
			return pi < pj
		}
		return result[i].Name < result[j].Name
	})

	return result
}

func inspectSingleInterface(raw net.Interface, defaultIfaceName string, isLXC bool) model.NetworkInterfaceInfo {
	name := raw.Name
	isUp := (raw.Flags & net.FlagUp) != 0
	isLoopback := (raw.Flags & net.FlagLoopback) != 0 || name == "lo"
	isDefault := name != "" && name == defaultIfaceName

	var ips []string
	if addrs, err := raw.Addrs(); err == nil {
		for _, addr := range addrs {
			if ipNet, ok := addr.(*net.IPNet); ok {
				ips = append(ips, ipNet.IP.String())
			}
		}
	}
	ipBytes, _ := json.Marshal(ips)

	info := model.NetworkInterfaceInfo{
		Name:           name,
		MacAddress:     raw.HardwareAddr.String(),
		IPAddresses:    string(ipBytes),
		IsUp:           isUp,
		IsDefaultRoute: isDefault,
		MTU:            raw.MTU,
	}

	if isLoopback {
		info.Category = model.NetworkInterfaceCategoryLoopback
		info.DisplayName = name + " (回环设备)"
		info.Virtualization = "none"
		return info
	}

	if runtime.GOOS != "linux" {
		// 非 Linux 系统（如 Windows 开发环境）
		lower := strings.ToLower(name)
		if strings.Contains(lower, "vethernet") || strings.Contains(lower, "tap") || strings.Contains(lower, "tun") || strings.Contains(lower, "loopback") {
			info.Category = model.NetworkInterfaceCategoryVirtual
			info.DisplayName = name + " (虚拟网卡)"
		} else {
			info.Category = model.NetworkInterfaceCategoryPhysical
			info.DisplayName = name + " (物理/主网卡)"
		}
		return info
	}

	// Linux 深度探测
	sysPath := filepath.Join("/sys/class/net", name)
	driver := readLinuxInterfaceDriver(sysPath)
	info.Driver = driver

	// 读取速率
	if speedContent, err := os.ReadFile(filepath.Join(sysPath, "speed")); err == nil {
		if sp, spErr := strconv.Atoi(strings.TrimSpace(string(speedContent))); spErr == nil && sp > 0 {
			info.SpeedMbps = sp
		}
	}

	// 识别分类与虚拟化类型
	category, virtType, desc := classifyLinuxInterface(name, sysPath, driver, isLXC, len(ips) > 0, isDefault)
	info.Category = category
	info.Virtualization = virtType
	info.DisplayName = name + " " + desc

	return info
}

func classifyLinuxInterface(name, sysPath, driver string, isLXC bool, hasIP bool, isDefault bool) (category string, virtType string, desc string) {
	lowerName := strings.ToLower(name)
	lowerDriver := strings.ToLower(driver)

	// 1. 明确的应用/代理/VPN/容器虚拟网卡
	if isTunOrTap(lowerName, lowerDriver, sysPath) {
		return model.NetworkInterfaceCategoryVirtual, "proxy/tunnel", "(代理/VPN 虚拟网卡)"
	}
	if strings.HasPrefix(lowerName, "wg") || lowerDriver == "wireguard" || strings.HasPrefix(lowerName, "tailscale") || strings.HasPrefix(lowerName, "zt") {
		return model.NetworkInterfaceCategoryVirtual, "vpn", "(VPN 虚拟网卡)"
	}
	if strings.HasPrefix(lowerName, "docker") || strings.HasPrefix(lowerName, "br-") || strings.HasPrefix(lowerName, "cni") || strings.HasPrefix(lowerName, "flannel") {
		return model.NetworkInterfaceCategoryVirtual, "container-bridge", "(容器桥接网卡)"
	}
	if strings.HasPrefix(lowerName, "veth") || lowerDriver == "veth" {
		// 如果在 LXC 容器内且为主网卡（有 IP 且名称为 eth*），则归为模拟物理网卡
		if isLXC && (strings.HasPrefix(lowerName, "eth") || strings.HasPrefix(lowerName, "ens") || isDefault) {
			return model.NetworkInterfaceCategorySimulated, "lxc", "(LXC 容器主网卡)"
		}
		return model.NetworkInterfaceCategoryVirtual, "veth", "(容器虚拟对端)"
	}
	if strings.HasPrefix(lowerName, "dummy") || strings.HasPrefix(lowerName, "sit") || strings.HasPrefix(lowerName, "ip6tnl") || strings.HasPrefix(lowerName, "gre") {
		return model.NetworkInterfaceCategoryVirtual, "tunnel", "(内核虚拟隧道)"
	}

	// 2. 模拟物理网卡 / 云平台与虚拟机驱动
	switch lowerDriver {
	case "virtio_net":
		return model.NetworkInterfaceCategorySimulated, "kvm/qemu", "(VirtIO 模拟物理网卡)"
	case "vmxnet3", "vmxnet":
		return model.NetworkInterfaceCategorySimulated, "vmware", "(VMware 虚拟网卡)"
	case "hv_netvsc", "netvsc":
		return model.NetworkInterfaceCategorySimulated, "hyper-v/azure", "(Hyper-V 虚拟网卡)"
	case "ena":
		return model.NetworkInterfaceCategorySimulated, "aws", "(AWS ENA 网卡)"
	case "gve":
		return model.NetworkInterfaceCategorySimulated, "gcp", "(GCP GVE 网卡)"
	case "xen-netfront":
		return model.NetworkInterfaceCategorySimulated, "xen", "(Xen 虚拟网卡)"
	}

	// 3. LXC 容器环境下的主以太网卡
	if isLXC && (strings.HasPrefix(lowerName, "eth") || strings.HasPrefix(lowerName, "ens") || strings.HasPrefix(lowerName, "enp") || isDefault) {
		return model.NetworkInterfaceCategorySimulated, "lxc", "(LXC 容器主网卡)"
	}

	// 4. 检查 PCI/USB 物理硬件设备路径
	devicePath := filepath.Join(sysPath, "device")
	if target, err := os.Readlink(devicePath); err == nil {
		lowerTarget := strings.ToLower(target)
		if strings.Contains(lowerTarget, "virtio") {
			return model.NetworkInterfaceCategorySimulated, "kvm/virtio", "(VirtIO 模拟物理网卡)"
		}
		if strings.Contains(lowerTarget, "pci") || strings.Contains(lowerTarget, "usb") {
			return model.NetworkInterfaceCategoryPhysical, "hardware", "(物理网卡)"
		}
	}

	// 5. 常见物理以太网驱动
	if lowerDriver == "e1000e" || lowerDriver == "igb" || lowerDriver == "ixgbe" || lowerDriver == "r8169" || lowerDriver == "tg3" || lowerDriver == "mlx5_core" || lowerDriver == "bnxt_en" {
		return model.NetworkInterfaceCategoryPhysical, "hardware", "(物理网卡)"
	}

	// 6. 兜底判定：如果是以太网命名规范（eth*, ens*, enp*, eno*）
	if strings.HasPrefix(lowerName, "eth") || strings.HasPrefix(lowerName, "ens") || strings.HasPrefix(lowerName, "enp") || strings.HasPrefix(lowerName, "eno") {
		return model.NetworkInterfaceCategorySimulated, "ethernet", "(物理/主以太网卡)"
	}

	return model.NetworkInterfaceCategoryVirtual, "unknown", "(虚拟网卡)"
}

func isTunOrTap(name, driver, sysPath string) bool {
	if driver == "tun" || driver == "tap" {
		return true
	}
	if _, err := os.Stat(filepath.Join(sysPath, "tun_flags")); err == nil {
		return true
	}
	if strings.HasPrefix(name, "tun") || strings.HasPrefix(name, "tap") ||
		strings.HasPrefix(name, "sing-tun") || strings.HasPrefix(name, "xray-tun") ||
		strings.HasPrefix(name, "meta-tun") || strings.HasPrefix(name, "utun") {
		return true
	}
	return false
}

func readLinuxInterfaceDriver(sysPath string) string {
	driverPath := filepath.Join(sysPath, "device", "driver")
	if linkTarget, err := os.Readlink(driverPath); err == nil {
		return filepath.Base(linkTarget)
	}

	ueventPath := filepath.Join(sysPath, "device", "uevent")
	if content, err := os.ReadFile(ueventPath); err == nil {
		scanner := bufio.NewScanner(strings.NewReader(string(content)))
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "DRIVER=") {
				return strings.TrimPrefix(line, "DRIVER=")
			}
		}
	}
	return ""
}

func detectIsLXCEnvironment() bool {
	if runtime.GOOS != "linux" {
		return false
	}
	if envBytes, err := os.ReadFile("/proc/1/environ"); err == nil {
		if strings.Contains(string(envBytes), "container=lxc") || strings.Contains(string(envBytes), "container=podman") {
			return true
		}
	}
	if _, err := os.Stat("/dev/lxd"); err == nil {
		return true
	}
	return false
}

func detectDefaultRouteInterfaceName() string {
	if iface := parseDefaultInterfaceFromProcRoute("/proc/net/route"); iface != "" {
		return iface
	}
	if iface := parseDefaultInterfaceFromIPRouteCommand(); iface != "" {
		return iface
	}
	return fallbackFirstActiveInterface()
}

func cloneNetworkInterfaces(items []model.NetworkInterfaceInfo) []model.NetworkInterfaceInfo {
	if items == nil {
		return nil
	}
	cloned := make([]model.NetworkInterfaceInfo, len(items))
	copy(cloned, items)
	return cloned
}
