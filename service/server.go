package service

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"math/big"
	stdnet "net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/alireza0/s-ui/config"
	"github.com/alireza0/s-ui/logger"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/mem"
	"github.com/shirou/gopsutil/v4/net"
	"github.com/shirou/gopsutil/v4/process"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

type ServerService struct{}

type tlsCertificateUsage string

const (
	tlsCertificateUsageServer tlsCertificateUsage = "server"
	tlsCertificateUsageClient tlsCertificateUsage = "client"
)

type runtimeStats struct {
	MemoryBytes uint64
	Threads     uint64
	Uptime      uint64
}

type systemdUnitStats struct {
	Active    bool
	MainPID   int32
	Memory    uint64
	Tasks     uint64
	UptimeSec uint64
}

func (s *ServerService) GetStatus(request string) *map[string]interface{} {
	status := make(map[string]interface{}, 0)
	requests := strings.Split(request, ",")
	for _, req := range requests {
		switch req {
		case "cpu":
			status["cpu"] = s.GetCpuPercent()
		case "mem":
			status["mem"] = s.GetMemInfo()
		case "dsk":
			status["dsk"] = s.GetDiskInfo()
		case "dio":
			status["dio"] = s.GetDiskIO()
		case "swp":
			status["swp"] = s.GetSwapInfo()
		case "net":
			status["net"] = s.GetNetInfo()
		case "sys":
			status["uptime"] = s.GetUptime()
			status["sys"] = s.GetSystemInfo()
		case "sbd":
			status["sbd"] = s.GetSingboxInfo()
		}
	}
	return &status
}

func (s *ServerService) GetCpuPercent() float64 {
	percents, err := cpu.Percent(0, false)
	if err != nil {
		logger.Warning("get cpu percent failed:", err)
		return 0
	} else {
		return percents[0]
	}
}

func (s *ServerService) GetUptime() uint64 {
	upTime, err := host.Uptime()
	if err != nil {
		logger.Warning("get uptime failed:", err)
		return 0
	} else {
		return upTime
	}
}

func (s *ServerService) GetMemInfo() map[string]interface{} {
	info := make(map[string]interface{}, 0)
	memInfo, err := mem.VirtualMemory()
	if err != nil {
		logger.Warning("get virtual memory failed:", err)
	} else {
		info["current"] = memInfo.Used
		info["total"] = memInfo.Total
	}
	return info
}

func (s *ServerService) GetDiskInfo() map[string]interface{} {
	info := make(map[string]interface{}, 0)
	diskInfo, err := disk.Usage("/")
	if err != nil {
		logger.Warning("get disk usage failed:", err)
	} else {
		info["current"] = diskInfo.Used
		info["total"] = diskInfo.Total
	}
	return info
}

func (s *ServerService) GetDiskIO() map[string]interface{} {
	info := make(map[string]interface{}, 0)
	ioStats, err := disk.IOCounters()
	if err != nil {
		logger.Warning("get disk io counters failed:", err)
	} else if len(ioStats) > 0 {
		infoR, infoW := uint64(0), uint64(0)
		for _, ioStat := range ioStats {
			infoR += ioStat.ReadBytes
			infoW += ioStat.WriteBytes
		}
		info["read"] = infoR
		info["write"] = infoW
	} else {
		logger.Warning("can not find disk io counters")
	}
	return info
}

func (s *ServerService) GetSwapInfo() map[string]interface{} {
	info := make(map[string]interface{}, 0)
	swapInfo, err := mem.SwapMemory()
	if err != nil {
		logger.Warning("get swap memory failed:", err)
	} else {
		info["current"] = swapInfo.Used
		info["total"] = swapInfo.Total
	}
	return info
}

func (s *ServerService) GetNetInfo() map[string]interface{} {
	info := make(map[string]interface{}, 0)
	ioStats, err := net.IOCounters(false)
	if err != nil {
		logger.Warning("get io counters failed:", err)
	} else if len(ioStats) > 0 {
		ioStat := ioStats[0]
		info["sent"] = ioStat.BytesSent
		info["recv"] = ioStat.BytesRecv
		info["psent"] = ioStat.PacketsSent
		info["precv"] = ioStat.PacketsRecv
	} else {
		logger.Warning("can not find io counters")
	}
	return info
}

func (s *ServerService) GetSingboxInfo() map[string]interface{} {
	appProcessStats := s.getProcessRuntimeStats(int32(os.Getpid()))
	appStats := s.getAppRuntimeStats()
	singboxStats := s.getSingboxRuntimeStats()
	singboxProcessStats := runtimeStats{}
	if singboxStats.MainPID > 0 {
		singboxProcessStats = s.getProcessRuntimeStats(singboxStats.MainPID)
	}
	mihomoStats := s.getMihomoRuntimeStats()
	mihomoProcessStats := runtimeStats{}
	if mihomoStats.MainPID > 0 {
		mihomoProcessStats = s.getProcessRuntimeStats(mihomoStats.MainPID)
	}

	coreCombinedMemory := singboxStats.Memory + mihomoStats.Memory
	coreCombinedMemoryRSS := singboxProcessStats.MemoryBytes + mihomoProcessStats.MemoryBytes
	totalMemory := appStats.MemoryBytes + coreCombinedMemory
	totalMemoryRSS := appProcessStats.MemoryBytes + coreCombinedMemoryRSS

	return map[string]interface{}{
		"running": singboxStats.Active,
		"stats": map[string]interface{}{
			"AppMemory":             appStats.MemoryBytes,
			"CoreMemory":            singboxStats.Memory,
			"MihomoMemory":          mihomoStats.Memory,
			"CoreCombinedMemory":    coreCombinedMemory,
			"TotalMemory":           totalMemory,
			"AppMemoryRSS":          appProcessStats.MemoryBytes,
			"CoreMemoryRSS":         singboxProcessStats.MemoryBytes,
			"MihomoMemoryRSS":       mihomoProcessStats.MemoryBytes,
			"CoreCombinedMemoryRSS": coreCombinedMemoryRSS,
			"TotalMemoryRSS":        totalMemoryRSS,
			"AppThreads":            appStats.Threads,
			"CoreThreads":           singboxStats.Tasks,
			"AppUptime":             appStats.Uptime,
			"CoreUptime":            singboxStats.UptimeSec,

			// keep legacy keys for compatibility with older UI
			"NumGoroutine": appStats.Threads,
			"Alloc":        appStats.MemoryBytes,
			"Uptime":       appStats.Uptime,
		},
	}
}

func (s *ServerService) getAppRuntimeStats() runtimeStats {
	stats := s.getProcessRuntimeStats(int32(os.Getpid()))
	if runtime.GOOS != "linux" {
		return stats
	}

	unit := s.getCurrentServiceUnit()
	if unit == "" {
		return stats
	}

	unitStats, err := s.getSystemdUnitStats(unit)
	if err != nil {
		return stats
	}
	if unitStats.Memory > 0 {
		stats.MemoryBytes = unitStats.Memory
	}
	if unitStats.Tasks > 0 {
		stats.Threads = unitStats.Tasks
	}
	if unitStats.UptimeSec > 0 {
		stats.Uptime = unitStats.UptimeSec
	}
	return stats
}

func (s *ServerService) getSingboxRuntimeStats() systemdUnitStats {
	return s.getManagedCoreRuntimeStats(singboxSystemdName)
}

func (s *ServerService) getMihomoRuntimeStats() systemdUnitStats {
	return s.getManagedCoreRuntimeStats(mihomoSystemdName)
}

func (s *ServerService) getManagedCoreRuntimeStats(unit string) systemdUnitStats {
	stats := systemdUnitStats{}
	if runtime.GOOS != "linux" {
		return stats
	}

	unitStats, err := s.getSystemdUnitStats(unit)
	if err != nil {
		return stats
	}
	stats = unitStats

	if stats.MainPID > 0 {
		procStats := s.getProcessRuntimeStats(stats.MainPID)
		if stats.Memory == 0 {
			stats.Memory = procStats.MemoryBytes
		}
		if stats.Tasks == 0 {
			stats.Tasks = procStats.Threads
		}
		if stats.UptimeSec == 0 {
			stats.UptimeSec = procStats.Uptime
		}
	}

	return stats
}

func (s *ServerService) getProcessRuntimeStats(pid int32) runtimeStats {
	stats := runtimeStats{}
	p, err := process.NewProcess(pid)
	if err != nil {
		return stats
	}

	memInfo, err := p.MemoryInfo()
	if err == nil && memInfo != nil {
		stats.MemoryBytes = memInfo.RSS
	}

	numThreads, err := p.NumThreads()
	if err == nil && numThreads > 0 {
		stats.Threads = uint64(numThreads)
	}

	createTimeMs, err := p.CreateTime()
	if err == nil && createTimeMs > 0 {
		nowMs := time.Now().UnixMilli()
		if nowMs >= createTimeMs {
			stats.Uptime = uint64((nowMs - createTimeMs) / 1000)
		}
	}

	return stats
}

func (s *ServerService) getCurrentServiceUnit() string {
	cgroupData, err := os.ReadFile("/proc/self/cgroup")
	if err != nil {
		return ""
	}

	lines := strings.Split(string(cgroupData), "\n")
	for _, line := range lines {
		parts := strings.SplitN(line, ":", 3)
		if len(parts) != 3 {
			continue
		}
		cgroupPath := parts[2]
		pathParts := strings.Split(cgroupPath, "/")
		for i := len(pathParts) - 1; i >= 0; i-- {
			if strings.HasSuffix(pathParts[i], ".service") {
				return pathParts[i]
			}
		}
	}
	return ""
}

func (s *ServerService) getSystemdUnitStats(unit string) (systemdUnitStats, error) {
	stats := systemdUnitStats{}
	cmd := exec.Command("systemctl", "show", unit,
		"--property=ActiveState",
		"--property=MainPID",
		"--property=MemoryCurrent",
		"--property=TasksCurrent",
		"--property=ActiveEnterTimestampMonotonic",
	)
	output, err := cmd.Output()
	if err != nil {
		return stats, err
	}

	props := parseSystemdShowOutput(string(output))
	stats.Active = props["ActiveState"] == "active"

	if pid, err := strconv.ParseInt(props["MainPID"], 10, 32); err == nil && pid > 0 {
		stats.MainPID = int32(pid)
	}
	if memory, err := strconv.ParseUint(props["MemoryCurrent"], 10, 64); err == nil {
		stats.Memory = memory
	}
	if tasks, err := strconv.ParseUint(props["TasksCurrent"], 10, 64); err == nil {
		stats.Tasks = tasks
	}

	if activeEnterUsec, err := strconv.ParseUint(props["ActiveEnterTimestampMonotonic"], 10, 64); err == nil && activeEnterUsec > 0 {
		if upSec, err := host.Uptime(); err == nil {
			nowUsec := upSec * 1_000_000
			if nowUsec > activeEnterUsec {
				stats.UptimeSec = (nowUsec - activeEnterUsec) / 1_000_000
			}
		}
	}

	return stats, nil
}

func parseSystemdShowOutput(output string) map[string]string {
	props := make(map[string]string)
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		props[parts[0]] = parts[1]
	}
	return props
}

func (s *ServerService) GetSystemInfo() map[string]interface{} {
	info := make(map[string]interface{}, 0)
	var rtm runtime.MemStats
	runtime.ReadMemStats(&rtm)

	info["appMem"] = rtm.Sys
	info["appThreads"] = uint32(runtime.NumGoroutine())
	cpuInfo, err := cpu.Info()
	if err == nil {
		info["cpuType"] = cpuInfo[0].ModelName
	}
	info["cpuCount"] = runtime.NumCPU()
	info["hostName"], _ = os.Hostname()
	info["appVersion"] = config.GetVersion()
	ipv4 := make([]string, 0)
	ipv6 := make([]string, 0)
	// get ip address
	netInterfaces, _ := net.Interfaces()
	for i := 0; i < len(netInterfaces); i++ {
		if len(netInterfaces[i].Flags) > 2 && netInterfaces[i].Flags[0] == "up" && netInterfaces[i].Flags[1] != "loopback" {
			addrs := netInterfaces[i].Addrs

			for _, address := range addrs {
				if strings.Contains(address.Addr, ".") {
					ipv4 = append(ipv4, address.Addr)
				} else if address.Addr[0:6] != "fe80::" {
					ipv6 = append(ipv6, address.Addr)
				}
			}
		}
	}
	info["ipv4"] = ipv4
	info["ipv6"] = ipv6

	return info
}

func (s *ServerService) GetLogs(count string, level string) []string {
	c, err := strconv.Atoi(count)
	if err != nil {
		c = 10
	}
	return logger.GetLogs(c, level)
}

func (s *ServerService) GenKeypair(keyType string, options string) []string {
	return s.GenKeypairWithTemplate(keyType, options, "")
}

func (s *ServerService) GenKeypairWithTemplate(keyType string, options string, templateCode string) []string {
	if len(keyType) == 0 {
		return []string{"No keypair to generate"}
	}

	switch strings.ToLower(strings.TrimSpace(keyType)) {
	case "ech":
		return s.generateECHKeyPair(options)
	case "tls":
		return s.generateTLSKeyPairWithTemplate(options, templateCode)
	case "reality":
		return s.generateRealityKeyPair()
	case "wireguard":
		return s.generateWireGuardKey(options)
	case "vless_x25519", "vless-x25519":
		return s.generateVLESSX25519KeyPair()
	case "vless_mlkem768", "vless-mlkem768":
		return s.generateVLESSMLKEM768KeyPair()
	}

	return []string{"Failed to generate keypair"}
}

// GenerateTLSPublicKeySHA256 calculates SHA-256 of certificate public key via openssl.
// sourceType supports "path" (certificate_path) and "pem" (certificate_pem).
func (s *ServerService) GenerateTLSPublicKeySHA256(sourceType string, certificatePath string, certificatePEM string) (string, error) {
	certInput, err := loadCertificateInput(sourceType, certificatePath, certificatePEM)
	if err != nil {
		return "", err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	pubKeyPEM, err := runOpenSSLCommand(ctx, []string{"x509", "-pubkey", "-noout"}, certInput)
	if err != nil {
		return "", err
	}
	pubKeyDER, err := runOpenSSLCommand(ctx, []string{"pkey", "-pubin", "-outform", "der"}, pubKeyPEM)
	if err != nil {
		return "", err
	}
	sha256Raw, err := runOpenSSLCommand(ctx, []string{"dgst", "-sha256", "-binary"}, pubKeyDER)
	if err != nil {
		return "", err
	}
	sha256Base64, err := runOpenSSLCommand(ctx, []string{"enc", "-base64", "-A"}, sha256Raw)
	if err != nil {
		return "", err
	}

	hash := strings.TrimSpace(string(sha256Base64))
	if hash == "" {
		return "", fmt.Errorf("failed to generate sha256: empty output")
	}
	if _, err := base64.StdEncoding.DecodeString(hash); err != nil {
		return "", fmt.Errorf("failed to parse sha256 output: %w", err)
	}
	return hash, nil
}

// GenerateTLSCertificateFingerprint calculates SHA-256 certificate fingerprint in OpenSSL style.
// sourceType supports "path" (certificate_path) and "pem" (certificate_pem).
func (s *ServerService) GenerateTLSCertificateFingerprint(sourceType string, certificatePath string, certificatePEM string) (string, error) {
	certInput, err := loadCertificateInput(sourceType, certificatePath, certificatePEM)
	if err != nil {
		return "", err
	}

	certs, err := parseCertificates(certInput)
	if err != nil {
		return "", err
	}

	sum := sha256.Sum256(certs[0].Raw)
	hexStr := strings.ToUpper(hex.EncodeToString(sum[:]))
	parts := make([]string, 0, len(hexStr)/2)
	for i := 0; i < len(hexStr); i += 2 {
		parts = append(parts, hexStr[i:i+2])
	}
	return strings.Join(parts, ":"), nil
}

// DetectTLSCertificateAlgorithm detects signature and key algorithm strength from a certificate.
// sourceType supports "path" (certificate_path) and "pem" (certificate_pem).
func (s *ServerService) DetectTLSCertificateAlgorithm(sourceType string, certificatePath string, certificatePEM string) (map[string]string, error) {
	certInput, err := loadCertificateInput(sourceType, certificatePath, certificatePEM)
	if err != nil {
		return nil, err
	}

	certs, err := parseCertificates(certInput)
	if err != nil {
		return nil, err
	}
	leafCert := certs[0]
	issuerCert := leafCert
	if len(certs) > 1 {
		issuerCert = certs[1]
	}

	keyAlgorithm := detectCertificateKeyAlgorithm(leafCert)
	signatureAlgorithm := detectCertificateSignatureAlgorithm(leafCert, issuerCert)
	if signatureAlgorithm == "" {
		signatureAlgorithm = keyAlgorithm
	}

	return map[string]string{
		"signature_algorithm": signatureAlgorithm,
		"key_algorithm":       keyAlgorithm,
	}, nil
}

func (s *ServerService) DetectTLSSelfSignedTemplate(sourceType string, certificatePath string, certificatePEM string) (string, error) {
	certInput, err := loadCertificateInput(sourceType, certificatePath, certificatePEM)
	if err != nil {
		return "", err
	}

	certs, err := parseCertificates(certInput)
	if err != nil {
		return "", err
	}

	return detectTLSSelfSignedTemplateCode(certs), nil
}

func loadCertificateInput(sourceType string, certificatePath string, certificatePEM string) ([]byte, error) {
	source := strings.ToLower(strings.TrimSpace(sourceType))
	if source == "" {
		return nil, fmt.Errorf("source_type is required")
	}

	var certInput []byte
	switch source {
	case "path":
		path := strings.TrimSpace(certificatePath)
		if path == "" {
			return nil, fmt.Errorf("certificate_path is required when source_type is path")
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("failed to read certificate file: %w", err)
		}
		certInput = content
	case "pem":
		content := strings.TrimSpace(certificatePEM)
		if content == "" {
			return nil, fmt.Errorf("certificate_pem is required when source_type is pem")
		}
		certInput = []byte(content + "\n")
	default:
		return nil, fmt.Errorf("unsupported source_type: %s", source)
	}

	if !strings.Contains(string(certInput), "BEGIN CERTIFICATE") {
		return nil, fmt.Errorf("invalid certificate content: BEGIN CERTIFICATE block not found")
	}

	return certInput, nil
}

func parseCertificates(certInput []byte) ([]*x509.Certificate, error) {
	rest := certInput
	certs := make([]*x509.Certificate, 0)
	for len(rest) > 0 {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			break
		}
		if block.Type != "CERTIFICATE" {
			continue
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("failed to parse certificate: %w", err)
		}
		certs = append(certs, cert)
	}
	if len(certs) == 0 {
		return nil, fmt.Errorf("invalid certificate content: BEGIN CERTIFICATE block not found")
	}
	return certs, nil
}

func detectCertificateKeyAlgorithm(cert *x509.Certificate) string {
	switch pub := cert.PublicKey.(type) {
	case *ecdsa.PublicKey:
		if pub.Curve == nil || pub.Curve.Params() == nil {
			return ""
		}
		switch pub.Curve.Params().Name {
		case "P-224":
			return "ecc224"
		case "P-256":
			return "ecc256"
		case "P-384":
			return "ecc384"
		case "P-521":
			return "ecc521"
		default:
			return ""
		}
	case *rsa.PublicKey:
		return detectRSAAlgorithmByBits(pub.N.BitLen())
	default:
		return ""
	}
}

func detectCertificateSignatureAlgorithm(cert *x509.Certificate, issuerCert *x509.Certificate) string {
	switch cert.SignatureAlgorithm {
	case x509.ECDSAWithSHA1, x509.ECDSAWithSHA256, x509.ECDSAWithSHA384, x509.ECDSAWithSHA512:
		if issuerCert != nil {
			if issuerAlgorithm := detectCertificateKeyAlgorithm(issuerCert); strings.HasPrefix(issuerAlgorithm, "ecc") {
				return issuerAlgorithm
			}
		}
		switch cert.SignatureAlgorithm {
		case x509.ECDSAWithSHA512:
			return "ecc521"
		case x509.ECDSAWithSHA384:
			return "ecc384"
		default:
			return "ecc256"
		}
	case x509.SHA1WithRSA, x509.SHA256WithRSA, x509.SHA384WithRSA, x509.SHA512WithRSA,
		x509.SHA256WithRSAPSS, x509.SHA384WithRSAPSS, x509.SHA512WithRSAPSS:
		if issuerCert != nil {
			if pub, ok := issuerCert.PublicKey.(*rsa.PublicKey); ok {
				return detectRSAAlgorithmByBits(pub.N.BitLen())
			}
		}
		return "rsa2048"
	default:
		return ""
	}
}

func detectRSAAlgorithmByBits(bits int) string {
	if bits >= 8192 {
		return "rsa8192"
	}
	if bits >= 4096 {
		return "rsa4096"
	}
	if bits >= 3072 {
		return "rsa3072"
	}
	if bits >= 2048 {
		return "rsa2048"
	}
	if bits >= 1024 {
		return "rsa1024"
	}
	return ""
}

func runOpenSSLCommand(ctx context.Context, args []string, input []byte) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "openssl", args...)
	if len(input) > 0 {
		cmd.Stdin = bytes.NewReader(input)
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("openssl command timeout")
		}
		errText := strings.TrimSpace(stderr.String())
		if errText == "" {
			errText = err.Error()
		}
		return nil, fmt.Errorf("openssl %s failed: %s", strings.Join(args, " "), errText)
	}

	return stdout.Bytes(), nil
}

func (s *ServerService) generateECHKeyPair(serverName string) []string {
	_ = serverName
	return []string{"ECH keypair generation is disabled in this build to keep sing-box code fully external"}
}

// generateTLSKeyPair generates a certificate chain from UI options.
// options: serverName,durationValue,durationUnit,keyAlgorithm[,signatureAlgorithm[,usage]]
// algorithms: ecc224/ecc256/ecc384/ecc521/rsa1024/rsa2048/rsa3072/rsa4096/rsa8192
func (s *ServerService) generateTLSKeyPair(options string) []string {
	return s.generateTLSKeyPairWithTemplate(options, "")
}

func (s *ServerService) generateTLSKeyPairWithTemplate(options string, templateCode string) []string {
	parts := strings.Split(options, ",")
	serverName := parts[0]
	durationValue := 1
	durationUnit := "y"
	keyAlgorithm := "ecc256"
	signatureAlgorithm := keyAlgorithm
	usage := tlsCertificateUsageServer

	if len(parts) > 1 && parts[1] != "" {
		if v, err := strconv.Atoi(parts[1]); err == nil && v > 0 {
			durationValue = v
		}
	}
	if len(parts) > 2 && parts[2] != "" {
		durationUnit = parts[2]
	}
	if len(parts) > 3 && parts[3] != "" {
		keyAlgorithm = strings.ToLower(strings.TrimSpace(parts[3]))
		signatureAlgorithm = keyAlgorithm
	}
	if len(parts) > 4 && parts[4] != "" {
		signatureAlgorithm = strings.ToLower(strings.TrimSpace(parts[4]))
	}
	if len(parts) > 5 && parts[5] != "" {
		usage = normalizeTLSCertificateUsage(parts[5])
	}

	notBefore := time.Now()
	var notAfter time.Time
	switch durationUnit {
	case "y":
		notAfter = notBefore.AddDate(durationValue, 0, 0)
	case "m":
		notAfter = notBefore.AddDate(0, durationValue, 0)
	case "d":
		notAfter = notBefore.AddDate(0, 0, durationValue)
	default:
		notAfter = notBefore.AddDate(durationValue, 0, 0)
	}

	templateProfile := resolveTLSSelfSignedTemplate(templateCode)
	if strings.TrimSpace(templateCode) != "" && templateProfile == nil {
		return []string{"Failed to generate TLS keypair: unknown tls self-signed template: " + strings.TrimSpace(templateCode)}
	}

	privateKeyPem, publicKeyPem, err := s.generateCertWithTemplate(serverName, keyAlgorithm, signatureAlgorithm, usage, notBefore, notAfter, templateProfile)
	if err != nil {
		return []string{"Failed to generate TLS keypair: " + err.Error()}
	}
	return append(strings.Split(string(privateKeyPem), "\n"), strings.Split(string(publicKeyPem), "\n")...)
}

func normalizeTLSCertificateUsage(raw string) tlsCertificateUsage {
	if strings.EqualFold(strings.TrimSpace(raw), string(tlsCertificateUsageClient)) {
		return tlsCertificateUsageClient
	}
	return tlsCertificateUsageServer
}

func (s *ServerService) generateKeyPairByAlgorithm(keyAlgorithm string) (interface{}, interface{}, error) {
	switch strings.ToLower(strings.TrimSpace(keyAlgorithm)) {
	case "ecc224":
		key, err := ecdsa.GenerateKey(elliptic.P224(), rand.Reader)
		if err != nil {
			return nil, nil, err
		}
		return key, &key.PublicKey, nil
	case "ecc256":
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			return nil, nil, err
		}
		return key, &key.PublicKey, nil
	case "ecc384":
		key, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
		if err != nil {
			return nil, nil, err
		}
		return key, &key.PublicKey, nil
	case "ecc521":
		key, err := ecdsa.GenerateKey(elliptic.P521(), rand.Reader)
		if err != nil {
			return nil, nil, err
		}
		return key, &key.PublicKey, nil
	case "rsa1024":
		key, err := rsa.GenerateKey(rand.Reader, 1024)
		if err != nil {
			return nil, nil, err
		}
		return key, &key.PublicKey, nil
	case "rsa2048":
		key, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			return nil, nil, err
		}
		return key, &key.PublicKey, nil
	case "rsa3072":
		key, err := rsa.GenerateKey(rand.Reader, 3072)
		if err != nil {
			return nil, nil, err
		}
		return key, &key.PublicKey, nil
	case "rsa4096":
		key, err := rsa.GenerateKey(rand.Reader, 4096)
		if err != nil {
			return nil, nil, err
		}
		return key, &key.PublicKey, nil
	case "rsa8192":
		key, err := rsa.GenerateKey(rand.Reader, 8192)
		if err != nil {
			return nil, nil, err
		}
		return key, &key.PublicKey, nil
	default:
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			return nil, nil, err
		}
		return key, &key.PublicKey, nil
	}
}

func signatureAlgorithmByCertAlgorithm(signatureAlgorithm string, signerPrivKey interface{}) x509.SignatureAlgorithm {
	normalized := strings.ToLower(strings.TrimSpace(signatureAlgorithm))
	switch signerPrivKey.(type) {
	case *ecdsa.PrivateKey:
		switch normalized {
		case "ecc521":
			return x509.ECDSAWithSHA512
		case "ecc384":
			return x509.ECDSAWithSHA384
		case "ecc224", "ecc256":
			return x509.ECDSAWithSHA256
		default:
			return x509.ECDSAWithSHA256
		}
	case *rsa.PrivateKey:
		switch normalized {
		case "rsa8192", "rsa4096":
			return x509.SHA512WithRSA
		case "rsa3072":
			return x509.SHA384WithRSA
		case "rsa1024", "rsa2048":
			return x509.SHA256WithRSA
		default:
			return x509.SHA256WithRSA
		}
	default:
		return x509.UnknownSignatureAlgorithm
	}
}

func (s *ServerService) generateCertWithAlgorithm(serverName string, keyAlgorithm string, signatureAlgorithm string, usage tlsCertificateUsage, notBefore time.Time, notAfter time.Time) ([]byte, []byte, error) {
	return s.generateCertWithTemplate(serverName, keyAlgorithm, signatureAlgorithm, usage, notBefore, notAfter, nil)
}

func (s *ServerService) generateCertWithTemplate(serverName string, keyAlgorithm string, signatureAlgorithm string, usage tlsCertificateUsage, notBefore time.Time, notAfter time.Time, templateProfile *tlsSelfSignedTemplateProfile) ([]byte, []byte, error) {
	rootPrivKey, rootPubKey, err := s.generateKeyPairByAlgorithm(signatureAlgorithm)
	if err != nil {
		return nil, nil, err
	}
	rootSigAlg := signatureAlgorithmByCertAlgorithm(signatureAlgorithm, rootPrivKey)

	rootSerialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, nil, err
	}

	rootTemplate := x509.Certificate{
		SerialNumber: rootSerialNumber,
		Subject: pkix.Name{
			CommonName:   "USERTrust ECC Certification Authority",
			Organization: []string{"The USERTRUST Network"},
			Country:      []string{"US"},
		},
		NotBefore:             notBefore.AddDate(-5, 0, 0),
		NotAfter:              notAfter.AddDate(10, 0, 0),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLen:            2,
		SignatureAlgorithm:    rootSigAlg,
	}
	if templateProfile != nil {
		applyTLSTemplateCertificateDetails(&rootTemplate, templateProfile.Root, nil)
	}

	rootCertDER, err := x509.CreateCertificate(rand.Reader, &rootTemplate, &rootTemplate, rootPubKey, rootPrivKey)
	if err != nil {
		return nil, nil, err
	}

	rootCert, err := x509.ParseCertificate(rootCertDER)
	if err != nil {
		return nil, nil, err
	}

	interPrivKey, interPubKey, err := s.generateKeyPairByAlgorithm(signatureAlgorithm)
	if err != nil {
		return nil, nil, err
	}
	interSigAlg := signatureAlgorithmByCertAlgorithm(signatureAlgorithm, rootPrivKey)

	interSerialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, nil, err
	}

	interTemplate := x509.Certificate{
		SerialNumber: interSerialNumber,
		Subject: pkix.Name{
			CommonName:   "ZeroSSL ECC Domain Secure Site CA",
			Organization: []string{"ZeroSSL"},
			Country:      []string{"AT"},
		},
		NotBefore:             notBefore.AddDate(-2, 0, 0),
		NotAfter:              notAfter.AddDate(5, 0, 0),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLen:            0,
		MaxPathLenZero:        true,
		SignatureAlgorithm:    interSigAlg,
	}
	if templateProfile != nil {
		applyTLSTemplateCertificateDetails(&interTemplate, templateProfile.Intermediate, templateProfile)
	}

	interCertDER, err := x509.CreateCertificate(rand.Reader, &interTemplate, rootCert, interPubKey, rootPrivKey)
	if err != nil {
		return nil, nil, err
	}

	interCert, err := x509.ParseCertificate(interCertDER)
	if err != nil {
		return nil, nil, err
	}

	leafPrivKey, leafPubKey, err := s.generateKeyPairByAlgorithm(keyAlgorithm)
	if err != nil {
		return nil, nil, err
	}
	leafSigAlg := signatureAlgorithmByCertAlgorithm(signatureAlgorithm, interPrivKey)

	leafSerialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, nil, err
	}

	leafTemplate := x509.Certificate{
		SerialNumber: leafSerialNumber,
		Subject: pkix.Name{
			CommonName: "kwor-self-signed",
		},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           tlsLeafExtKeyUsage(usage),
		BasicConstraintsValid: true,
		IsCA:                  false,
		SignatureAlgorithm:    leafSigAlg,
	}

	serverName = strings.TrimSpace(serverName)
	if serverName != "" {
		leafTemplate.Subject.CommonName = serverName
	}

	if usage == tlsCertificateUsageServer && serverName != "" {
		if parsedIP := stdnet.ParseIP(serverName); parsedIP != nil {
			leafTemplate.IPAddresses = []stdnet.IP{parsedIP}
		} else {
			leafTemplate.DNSNames = []string{serverName}
		}
	}

	leafCertDER, err := x509.CreateCertificate(rand.Reader, &leafTemplate, interCert, leafPubKey, interPrivKey)
	if err != nil {
		return nil, nil, err
	}

	leafPrivKeyBytes, err := x509.MarshalPKCS8PrivateKey(leafPrivKey)
	if err != nil {
		return nil, nil, err
	}
	privateKeyPem := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: leafPrivKeyBytes})

	leafCertPem := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: leafCertDER})
	interCertPem := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: interCertDER})
	rootCertPem := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: rootCertDER})

	fullChainPem := append(leafCertPem, interCertPem...)
	fullChainPem = append(fullChainPem, rootCertPem...)

	return privateKeyPem, fullChainPem, nil
}

func tlsLeafExtKeyUsage(usage tlsCertificateUsage) []x509.ExtKeyUsage {
	if usage == tlsCertificateUsageClient {
		return []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}
	}
	return []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}
}

func (s *ServerService) generateRealityKeyPair() []string {
	privateKey, err := wgtypes.GeneratePrivateKey()
	if err != nil {
		return []string{"Failed to generate Reality keypair: ", err.Error()}
	}
	publicKey := privateKey.PublicKey()
	return []string{"PrivateKey: " + base64.RawURLEncoding.EncodeToString(privateKey[:]), "PublicKey: " + base64.RawURLEncoding.EncodeToString(publicKey[:])}
}

func (s *ServerService) generateWireGuardKey(pk string) []string {
	if len(pk) > 0 {
		key, _ := wgtypes.ParseKey(pk)
		return []string{key.PublicKey().String()}
	}
	wgKeys, err := wgtypes.GeneratePrivateKey()
	if err != nil {
		return []string{"Failed to generate wireguard keypair: ", err.Error()}
	}
	return []string{"PrivateKey: " + wgKeys.String(), "PublicKey: " + wgKeys.PublicKey().String()}
}

func (s *ServerService) generateVLESSX25519KeyPair() []string {
	lines, err := s.generateMihomoKeyPairBySubcommand("vless-x25519", []string{"PrivateKey:", "Password:", "Hash32:"})
	if err != nil {
		return []string{"Failed to generate VLESS X25519 keypair: " + err.Error()}
	}
	return lines
}

func (s *ServerService) generateVLESSMLKEM768KeyPair() []string {
	lines, err := s.generateMihomoKeyPairBySubcommand("vless-mlkem768", []string{"Seed:", "Client:", "Hash32:"})
	if err != nil {
		return []string{"Failed to generate VLESS ML-KEM-768 keypair: " + err.Error()}
	}
	return lines
}

func (s *ServerService) generateMihomoKeyPairBySubcommand(subcommand string, prefixes []string) ([]string, error) {
	binaryPath, err := s.resolveMihomoBinaryPath()
	if err != nil {
		return nil, err
	}

	cmd := exec.Command(binaryPath, "generate", subcommand)
	output, err := cmd.CombinedOutput()
	if err != nil {
		details := strings.TrimSpace(string(output))
		if details == "" {
			details = err.Error()
		}
		return nil, fmt.Errorf("%s", details)
	}

	lines := splitNonEmptyLines(string(output))
	if len(lines) == 0 {
		return nil, fmt.Errorf("empty output")
	}

	filtered := pickLinesByPrefix(lines, prefixes)
	if len(filtered) > 0 {
		return filtered, nil
	}
	return lines, nil
}

func (s *ServerService) resolveMihomoBinaryPath() (string, error) {
	if err := EnsureManagedCoreLayout(); err != nil {
		return "", err
	}

	binName := "mihomo"
	if runtime.GOOS == "windows" {
		binName = "mihomo.exe"
	}

	managedPath := filepath.Join(GetMihomoCoreDir(), binName)
	if info, err := os.Stat(managedPath); err == nil && info != nil && !info.IsDir() {
		return managedPath, nil
	}

	if fromPath, err := exec.LookPath("mihomo"); err == nil && fromPath != "" {
		return fromPath, nil
	}

	return "", fmt.Errorf("mihomo binary not found (expected %s or command in PATH)", managedPath)
}

func splitNonEmptyLines(raw string) []string {
	raw = strings.ReplaceAll(raw, "\r\n", "\n")
	parts := strings.Split(raw, "\n")
	lines := make([]string, 0, len(parts))
	for _, line := range parts {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		lines = append(lines, line)
	}
	return lines
}

func pickLinesByPrefix(lines []string, prefixes []string) []string {
	if len(lines) == 0 || len(prefixes) == 0 {
		return nil
	}

	filtered := make([]string, 0, len(prefixes))
	for _, line := range lines {
		for _, prefix := range prefixes {
			if strings.HasPrefix(line, prefix) {
				filtered = append(filtered, line)
				break
			}
		}
	}
	return filtered
}
