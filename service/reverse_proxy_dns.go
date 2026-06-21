package service

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"sync"
	"time"

	dnsproxy "github.com/AdguardTeam/dnsproxy/proxy"
	dnsupstream "github.com/AdguardTeam/dnsproxy/upstream"
	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
	"github.com/alireza0/s-ui/logger"
	"github.com/alireza0/s-ui/util/common"
	"github.com/quic-go/quic-go/http3"
)

const reverseProxyDNSShutdownTimeout = 5 * time.Second

type reverseProxyDNSRuntimeManager struct {
	mu      sync.Mutex
	running map[uint]*reverseProxyDNSInstance
}

type reverseProxyDNSInstance struct {
	ruleID              uint
	proxy               *dnsproxy.Proxy
	h3Server            *http3.Server
	h3PacketConns       []net.PacketConn
	rule                *model.ReverseProxyRule
	certificateStateKey string
	cancel              context.CancelFunc
	doneCh              chan struct{}
	startErr            error
}

type reverseProxyDNSRuleHandler struct {
	rule      *model.ReverseProxyRule
	upstreams []dnsupstream.Upstream
	logger    *slog.Logger
}

var reverseProxyDNSRuntime = &reverseProxyDNSRuntimeManager{
	running: make(map[uint]*reverseProxyDNSInstance),
}

func (m *reverseProxyDNSRuntimeManager) sync(service *ReverseProxyService, rows []model.ReverseProxyRule) error {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	want := make(map[uint]model.ReverseProxyRule)
	for i := range rows {
		row := rows[i]
		if !row.Enabled {
			continue
		}
		if !reverseProxyProtocolIsDNS(normalizeReverseProxyProtocolAlias(row.ListenProtocolAlias, row.ListenProtocol)) {
			continue
		}
		want[row.Id] = row
	}
	certificateState := loadReverseProxyCertificateRenderState(database.GetDB(), rows)

	nextRunning := make(map[uint]*reverseProxyDNSInstance, len(want))
	created := make(map[uint]*reverseProxyDNSInstance)
	stopped := make(map[uint]*reverseProxyDNSInstance)
	for id, row := range want {
		certStateKey := reverseProxyDNSCertificateStateKey(&row, certificateState)
		if instance, exists := m.running[id]; exists {
			if reverseProxyDNSInstanceMatchesRule(instance, &row, certStateKey) {
				nextRunning[id] = instance
				continue
			}
			if reverseProxyDNSInstanceSharesListenerSocket(instance, &row) {
				if err := instance.stop(); err != nil {
					return err
				}
				stopped[id] = instance
			}
		}
		instance, err := newReverseProxyDNSInstance(service, &row, certStateKey)
		if err != nil {
			for _, item := range created {
				_ = item.stop()
			}
			for oldID, oldInstance := range stopped {
				if oldInstance == nil || oldInstance.rule == nil {
					continue
				}
				restored, restoreErr := newReverseProxyDNSInstance(service, oldInstance.rule, oldInstance.certificateStateKey)
				if restoreErr != nil {
					logger.Warning("reverse proxy dns runtime rollback failed: ", restoreErr)
					continue
				}
				m.running[oldID] = restored
			}
			return err
		}
		nextRunning[id] = instance
		created[id] = instance
	}
	for id, instance := range m.running {
		if _, exists := nextRunning[id]; exists {
			continue
		}
		_ = instance.stop()
	}
	m.running = nextRunning
	return nil
}

func reverseProxyDNSInstanceSharesListenerSocket(instance *reverseProxyDNSInstance, row *model.ReverseProxyRule) bool {
	if instance == nil || instance.rule == nil || row == nil {
		return false
	}
	current := instance.rule
	if current.ListenPort != row.ListenPort {
		return false
	}
	currentAlias := normalizeReverseProxyProtocolAlias(current.ListenProtocolAlias, current.ListenProtocol)
	nextAlias := normalizeReverseProxyProtocolAlias(row.ListenProtocolAlias, row.ListenProtocol)
	if !reverseProxyDNSProtocolSharesSocket(currentAlias, nextAlias) {
		return false
	}
	return reverseProxyListenIPSetsOverlap(decodeReverseProxyListenIPs(current), decodeReverseProxyListenIPs(row))
}

func reverseProxyListenIPSetsOverlap(a []string, b []string) bool {
	if len(a) == 0 {
		a = []string{"0.0.0.0"}
	}
	if len(b) == 0 {
		b = []string{"0.0.0.0"}
	}
	for _, left := range a {
		for _, right := range b {
			if reverseProxyListenIPsOverlap(left, right) {
				return true
			}
		}
	}
	return false
}

func reverseProxyListenIPsOverlap(a string, b string) bool {
	left := net.ParseIP(strings.TrimSpace(a))
	right := net.ParseIP(strings.TrimSpace(b))
	if left == nil || right == nil {
		return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
	}
	left4 := left.To4() != nil
	right4 := right.To4() != nil
	if left4 != right4 {
		return false
	}
	if left.Equal(right) {
		return true
	}
	return left.IsUnspecified() || right.IsUnspecified()
}

func (m *reverseProxyDNSRuntimeManager) stopAll() error {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	var firstErr error
	for id, instance := range m.running {
		if err := instance.stop(); err != nil && firstErr == nil {
			firstErr = err
		}
		delete(m.running, id)
	}
	return firstErr
}

func reverseProxyDNSInstanceMatchesRule(instance *reverseProxyDNSInstance, row *model.ReverseProxyRule, certificateStateKey string) bool {
	if instance == nil || row == nil || instance.rule == nil {
		return false
	}
	current := instance.rule
	return current.ListenProtocol == row.ListenProtocol &&
		current.ListenProtocolAlias == row.ListenProtocolAlias &&
		current.ListenPort == row.ListenPort &&
		current.ListenIPList == row.ListenIPList &&
		current.ListenDNSPath == row.ListenDNSPath &&
		current.TargetProtocol == row.TargetProtocol &&
		current.TargetProtocolAlias == row.TargetProtocolAlias &&
		current.TargetAddresses == row.TargetAddresses &&
		current.TargetPort == row.TargetPort &&
		current.TargetDNSPath == row.TargetDNSPath &&
		current.UpstreamTLSVerify == row.UpstreamTLSVerify &&
		current.CertificateRecordList == row.CertificateRecordList &&
		current.CertificateRecordID == row.CertificateRecordID &&
		instance.certificateStateKey == certificateStateKey
}

func newReverseProxyDNSInstance(service *ReverseProxyService, row *model.ReverseProxyRule, certificateStateKey string) (*reverseProxyDNSInstance, error) {
	if service == nil || row == nil {
		return nil, errors.New("dns reverse proxy instance init failed: invalid rule")
	}

	handler, err := buildReverseProxyDNSRuleHandler(row)
	if err != nil {
		return nil, err
	}

	conf, err := buildReverseProxyDNSProxyConfig(service, row, handler)
	if err != nil {
		closeReverseProxyDNSUpstreams(handler.upstreams)
		return nil, err
	}

	if normalizeReverseProxyProtocolAlias(row.ListenProtocolAlias, row.ListenProtocol) == reverseProxyDNSProtocolDoHH3 {
		instance, err := buildReverseProxyDNSH3OnlyRuntime(row, conf.TLSConfig, handler, certificateStateKey)
		if err != nil {
			closeReverseProxyDNSUpstreams(handler.upstreams)
			return nil, err
		}
		return instance, nil
	}

	proxyInstance, err := dnsproxy.New(conf)
	if err != nil {
		closeReverseProxyDNSUpstreams(handler.upstreams)
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())
	instance := &reverseProxyDNSInstance{
		ruleID:              row.Id,
		proxy:               proxyInstance,
		rule:                cloneReverseProxyRule(row),
		certificateStateKey: certificateStateKey,
		cancel:              cancel,
		doneCh:              make(chan struct{}),
	}

	if err := proxyInstance.Start(ctx); err != nil {
		cancel()
		closeReverseProxyDNSUpstreams(handler.upstreams)
		return nil, translateReverseProxyDNSError(row, err)
	}

	_ = database.GetDB().Model(&model.ReverseProxyRule{}).Where("id = ?", row.Id).Updates(map[string]interface{}{
		"last_error":     "",
		"runtime_status": "running",
	}).Error

	go func() {
		<-ctx.Done()
		close(instance.doneCh)
	}()
	return instance, nil
}

func (i *reverseProxyDNSInstance) stop() error {
	if i == nil {
		return nil
	}
	if i.cancel != nil {
		i.cancel()
	}
	var firstErr error
	if i.proxy != nil {
		ctx, cancel := context.WithTimeout(context.Background(), reverseProxyDNSShutdownTimeout)
		err := i.proxy.Shutdown(ctx)
		cancel()
		if err != nil {
			firstErr = err
		}
	}
	if i.h3Server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), reverseProxyDNSShutdownTimeout)
		err := i.h3Server.Shutdown(ctx)
		cancel()
		if err != nil && !errors.Is(err, http.ErrServerClosed) && firstErr == nil {
			firstErr = err
		}
	}
	for _, conn := range i.h3PacketConns {
		if conn != nil {
			_ = conn.Close()
		}
	}
	if i.doneCh != nil {
		select {
		case <-i.doneCh:
		case <-time.After(reverseProxyDNSShutdownTimeout):
		}
	}
	return firstErr
}

func buildReverseProxyDNSProxyConfig(service *ReverseProxyService, row *model.ReverseProxyRule, handler *reverseProxyDNSRuleHandler) (*dnsproxy.Config, error) {
	if row == nil || handler == nil {
		return nil, errors.New("dns reverse proxy config failed: invalid rule")
	}
	listenAlias := normalizeReverseProxyProtocolAlias(row.ListenProtocolAlias, row.ListenProtocol)
	targetAlias := normalizeReverseProxyProtocolAlias(row.TargetProtocolAlias, row.TargetProtocol)

	conf := &dnsproxy.Config{
		RequestHandler: dnsproxy.HandlerFunc(handler.ServeDNS),
		UpstreamMode:   dnsproxy.UpstreamModeLoadBalance,
		UpstreamConfig: &dnsproxy.UpstreamConfig{
			Upstreams: append([]dnsupstream.Upstream(nil), handler.upstreams...),
		},
		Logger: slog.Default(),
	}

	listenIPs := decodeReverseProxyListenIPs(row)
	if len(listenIPs) == 0 {
		listenIPs = []string{"0.0.0.0"}
	}
	switch listenAlias {
	case reverseProxyDNSProtocolUDP:
		conf.UDPListenAddr = buildReverseProxyDNSUDPListenAddrs(listenIPs, row.ListenPort)
	case reverseProxyDNSProtocolTCP:
		conf.TCPListenAddr = buildReverseProxyDNSTCPListenAddrs(listenIPs, row.ListenPort)
	case reverseProxyDNSProtocolDoT:
		tlsConfig, err := buildReverseProxyDNSServerTLSConfig(service, row, []string{"dot", "dns"})
		if err != nil {
			return nil, err
		}
		conf.TLSConfig = tlsConfig
		conf.TLSListenAddr = buildReverseProxyDNSTCPListenAddrs(listenIPs, row.ListenPort)
	case reverseProxyDNSProtocolDoQ:
		tlsConfig, err := buildReverseProxyDNSServerTLSConfig(service, row, []string{"doq"})
		if err != nil {
			return nil, err
		}
		tlsConfig.NextProtos = []string{"doq", "doq-i02", "doq-i00", "dq"}
		conf.TLSConfig = tlsConfig
		conf.QUICListenAddr = buildReverseProxyDNSUDPListenAddrs(listenIPs, row.ListenPort)
	case reverseProxyDNSProtocolDoH, reverseProxyDNSProtocolDoHH3:
		tlsConfig, err := buildReverseProxyDNSServerTLSConfig(service, row, []string{"h2", "http/1.1", "h3"})
		if err != nil {
			return nil, err
		}
		conf.TLSConfig = tlsConfig
		routes := buildReverseProxyDNSDoHRoutes(strings.TrimSpace(row.ListenDNSPath))
		conf.HTTPConfig = &dnsproxy.HTTPConfig{
			ListenAddresses: buildReverseProxyDNSDoHListenAddrs(listenIPs, row.ListenPort),
			Routes:          routes,
			HTTP3Enabled:    listenAlias == reverseProxyDNSProtocolDoH,
		}
	default:
		return nil, fmt.Errorf("unsupported dns listen protocol: %s", listenAlias)
	}

	if reverseProxyDNSProtocolUsesPath(targetAlias) && strings.TrimSpace(row.TargetDNSPath) == "" {
		row.TargetDNSPath = "/dns-query"
	}

	return conf, nil
}

func buildReverseProxyDNSH3OnlyRuntime(row *model.ReverseProxyRule, tlsConfig *tls.Config, handler *reverseProxyDNSRuleHandler, certificateStateKey string) (*reverseProxyDNSInstance, error) {
	if row == nil || tlsConfig == nil || handler == nil {
		return nil, errors.New("dns h3 runtime config failed")
	}
	conf := &dnsproxy.Config{
		RequestHandler: dnsproxy.HandlerFunc(handler.ServeDNS),
		UpstreamMode:   dnsproxy.UpstreamModeLoadBalance,
		UpstreamConfig: &dnsproxy.UpstreamConfig{
			Upstreams: append([]dnsupstream.Upstream(nil), handler.upstreams...),
		},
		HTTPConfig: &dnsproxy.HTTPConfig{},
		Logger:     slog.Default(),
	}
	proxyInstance, err := dnsproxy.New(conf)
	if err != nil {
		return nil, err
	}
	mux := http.NewServeMux()
	for _, route := range buildReverseProxyDNSDoHRoutes(strings.TrimSpace(row.ListenDNSPath)) {
		mux.Handle(route, proxyInstance)
	}
	h3Server := &http3.Server{
		Handler:   mux,
		TLSConfig: http3.ConfigureTLSConfig(tlsConfig),
		Port:      row.ListenPort,
	}
	listenIPs := decodeReverseProxyListenIPs(row)
	if len(listenIPs) == 0 {
		listenIPs = []string{"0.0.0.0"}
	}
	packetConns := make([]net.PacketConn, 0, len(listenIPs))
	for _, listenIP := range listenIPs {
		addr := net.JoinHostPort(strings.TrimSpace(listenIP), fmt.Sprintf("%d", row.ListenPort))
		packetConn, listenErr := net.ListenPacket("udp", addr)
		if listenErr != nil {
			for _, conn := range packetConns {
				_ = conn.Close()
			}
			return nil, listenErr
		}
		packetConns = append(packetConns, packetConn)
		go func(pc net.PacketConn) {
			if serveErr := h3Server.Serve(pc); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
				logger.Warning("reverse proxy dns h3 server serve failed: ", serveErr)
			}
		}(packetConn)
	}
	ctx, cancel := context.WithCancel(context.Background())
	instance := &reverseProxyDNSInstance{
		ruleID:              row.Id,
		proxy:               nil,
		h3Server:            h3Server,
		h3PacketConns:       packetConns,
		rule:                cloneReverseProxyRule(row),
		certificateStateKey: certificateStateKey,
		cancel:              cancel,
		doneCh:              make(chan struct{}),
	}
	go func() {
		<-ctx.Done()
		close(instance.doneCh)
	}()
	_ = database.GetDB().Model(&model.ReverseProxyRule{}).Where("id = ?", row.Id).Updates(map[string]interface{}{
		"last_error":     "",
		"runtime_status": "running",
	}).Error
	return instance, nil
}

func buildReverseProxyDNSRuleHandler(row *model.ReverseProxyRule) (*reverseProxyDNSRuleHandler, error) {
	targetAlias := normalizeReverseProxyProtocolAlias(row.TargetProtocolAlias, row.TargetProtocol)
	targets := decodeReverseProxyList(row.TargetAddresses)
	if len(targets) == 0 {
		return nil, errors.New("dns reverse proxy target is empty")
	}

	opts := &dnsupstream.Options{
		Timeout:            12 * time.Second,
		InsecureSkipVerify: !row.UpstreamTLSVerify,
		Logger:             slog.Default(),
	}
	if targetAlias == reverseProxyDNSProtocolDoH {
		opts.HTTPVersions = []dnsupstream.HTTPVersion{dnsupstream.HTTPVersion11, dnsupstream.HTTPVersion2}
	}
	if targetAlias == reverseProxyDNSProtocolDoHH3 {
		opts.HTTPVersions = []dnsupstream.HTTPVersion{dnsupstream.HTTPVersion3}
	}

	upstreams := make([]dnsupstream.Upstream, 0, len(targets))
	for _, target := range targets {
		address, err := buildReverseProxyDNSUpstreamAddress(targetAlias, target, row.TargetPort, row.TargetDNSPath)
		if err != nil {
			closeReverseProxyDNSUpstreams(upstreams)
			return nil, err
		}
		ups, err := dnsupstream.AddressToUpstream(address, opts.Clone())
		if err != nil {
			closeReverseProxyDNSUpstreams(upstreams)
			return nil, err
		}
		upstreams = append(upstreams, ups)
	}

	return &reverseProxyDNSRuleHandler{
		rule:      cloneReverseProxyRule(row),
		upstreams: upstreams,
		logger:    slog.Default(),
	}, nil
}

func (h *reverseProxyDNSRuleHandler) ServeDNS(ctx context.Context, _ *dnsproxy.Proxy, dctx *dnsproxy.DNSContext) error {
	if h == nil || dctx == nil || dctx.Req == nil {
		return errors.New("dns reverse proxy handler received empty request")
	}
	var firstErr error
	for _, ups := range h.upstreams {
		if ups == nil {
			continue
		}
		resp, err := ups.Exchange(dctx.Req.Copy())
		if err == nil && resp != nil {
			dctx.Res = resp
			_ = database.GetDB().Model(&model.ReverseProxyRule{}).Where("id = ?", h.rule.Id).Updates(map[string]interface{}{
				"last_error":     "",
				"runtime_status": "running",
			}).Error
			return nil
		}
		if firstErr == nil {
			firstErr = err
		}
	}
	if firstErr == nil {
		firstErr = errors.New("all dns upstreams failed")
	}
	_ = database.GetDB().Model(&model.ReverseProxyRule{}).Where("id = ?", h.rule.Id).Updates(map[string]interface{}{
		"last_error":     strings.TrimSpace(firstErr.Error()),
		"runtime_status": "upstream_error",
	}).Error
	return firstErr
}

func buildReverseProxyDNSUpstreamAddress(alias string, target string, port int, path string) (string, error) {
	host := strings.TrimSpace(target)
	if host == "" {
		return "", errors.New("dns target host is empty")
	}
	switch alias {
	case reverseProxyDNSProtocolUDP:
		return "udp://" + net.JoinHostPort(host, fmt.Sprintf("%d", port)), nil
	case reverseProxyDNSProtocolTCP:
		return "tcp://" + net.JoinHostPort(host, fmt.Sprintf("%d", port)), nil
	case reverseProxyDNSProtocolDoT:
		return "tls://" + net.JoinHostPort(host, fmt.Sprintf("%d", port)), nil
	case reverseProxyDNSProtocolDoQ:
		return "quic://" + net.JoinHostPort(host, fmt.Sprintf("%d", port)), nil
	case reverseProxyDNSProtocolDoH:
		return (&url.URL{Scheme: "https", Host: net.JoinHostPort(host, fmt.Sprintf("%d", port)), Path: normalizeReverseProxyDNSPath(path)}).String(), nil
	case reverseProxyDNSProtocolDoHH3:
		return (&url.URL{Scheme: "h3", Host: net.JoinHostPort(host, fmt.Sprintf("%d", port)), Path: normalizeReverseProxyDNSPath(path)}).String(), nil
	default:
		return "", fmt.Errorf("unsupported dns target protocol: %s", alias)
	}
}

func buildReverseProxyDNSServerTLSConfig(service *ReverseProxyService, row *model.ReverseProxyRule, nextProtos []string) (*tls.Config, error) {
	if service == nil || row == nil {
		return nil, errors.New("dns reverse proxy tls config failed")
	}
	certIDs := reverseProxyRuleCertificateIDs(row)
	if len(certIDs) == 0 {
		return nil, errors.New("dns tls listener requires certificate")
	}
	bindings, _, err := service.loadRuleCertificates([]*model.ReverseProxyRule{row})
	if err != nil {
		return nil, err
	}
	items := bindings[row.Id]
	if len(items) == 0 {
		return nil, errors.New("dns tls listener certificate is unavailable")
	}
	config := &tls.Config{
		MinVersion: tls.VersionTLS12,
		GetCertificate: func(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
			serverName := ""
			if hello != nil {
				serverName = reverseProxyNormalizeServerName(hello.ServerName)
			}
			if serverName != "" {
				for _, item := range items {
					if item == nil || item.Certificate == nil {
						continue
					}
					if reverseProxyCertificateBindingMatchesServerName(item, serverName) {
						return item.Certificate, nil
					}
				}
				return nil, common.NewError("no certificate available for requested sni")
			}
			if selected := reverseProxyPickNoSNIBinding(items, ""); selected != nil && selected.Certificate != nil {
				return selected.Certificate, nil
			}
			for _, item := range items {
				if item != nil && item.Certificate != nil {
					return item.Certificate, nil
				}
			}
			return nil, errors.New("dns tls listener certificate is unavailable")
		},
	}
	if len(nextProtos) > 0 {
		config.NextProtos = append([]string(nil), nextProtos...)
	}
	if normalizeReverseProxyProtocolAlias(row.ListenProtocolAlias, row.ListenProtocol) == reverseProxyDNSProtocolDoQ {
		config.MinVersion = tls.VersionTLS13
	}
	return config, nil
}

func buildReverseProxyDNSUDPListenAddrs(items []string, port int) []*net.UDPAddr {
	out := make([]*net.UDPAddr, 0, len(items))
	for _, item := range items {
		addr, err := net.ResolveUDPAddr("udp", net.JoinHostPort(strings.TrimSpace(item), fmt.Sprintf("%d", port)))
		if err == nil && addr != nil {
			out = append(out, addr)
		}
	}
	return out
}

func buildReverseProxyDNSTCPListenAddrs(items []string, port int) []*net.TCPAddr {
	out := make([]*net.TCPAddr, 0, len(items))
	for _, item := range items {
		addr, err := net.ResolveTCPAddr("tcp", net.JoinHostPort(strings.TrimSpace(item), fmt.Sprintf("%d", port)))
		if err == nil && addr != nil {
			out = append(out, addr)
		}
	}
	return out
}

func buildReverseProxyDNSDoHListenAddrs(items []string, port int) []netip.AddrPort {
	out := make([]netip.AddrPort, 0, len(items))
	for _, item := range items {
		ip := net.ParseIP(strings.TrimSpace(item))
		if ip == nil {
			continue
		}
		addr, ok := netip.AddrFromSlice(ip)
		if !ok {
			continue
		}
		out = append(out, netip.AddrPortFrom(addr, uint16(port)))
	}
	return out
}

func buildReverseProxyDNSDoHRoutes(path string) []string {
	path = normalizeReverseProxyDNSPath(path)
	if path == "" {
		path = "/dns-query"
	}
	return []string{
		http.MethodGet + " " + path,
		http.MethodPost + " " + path,
	}
}

func cloneReverseProxyRule(row *model.ReverseProxyRule) *model.ReverseProxyRule {
	if row == nil {
		return nil
	}
	clone := *row
	return &clone
}

func closeReverseProxyDNSUpstreams(items []dnsupstream.Upstream) {
	for _, item := range items {
		if item != nil {
			_ = item.Close()
		}
	}
}

func translateReverseProxyDNSError(row *model.ReverseProxyRule, err error) error {
	if row == nil || err == nil {
		return err
	}
	message := strings.TrimSpace(err.Error())
	if strings.Contains(strings.ToLower(message), "address already in use") {
		return common.NewError(fmt.Sprintf("dns reverse proxy listen :%d failed: address already in use", row.ListenPort))
	}
	return common.NewError(message)
}

func syncReverseProxyDNSRuntime(service *ReverseProxyService, rows []model.ReverseProxyRule) error {
	if err := reverseProxyDNSRuntime.sync(service, rows); err != nil {
		return err
	}
	return nil
}

func stopReverseProxyDNSRuntime() error {
	return reverseProxyDNSRuntime.stopAll()
}

func (m *reverseProxyDNSRuntimeManager) listenerCount() int {
	if m == nil {
		return 0
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	count := 0
	for _, instance := range m.running {
		if instance == nil || instance.rule == nil {
			continue
		}
		alias := normalizeReverseProxyProtocolAlias(instance.rule.ListenProtocolAlias, instance.rule.ListenProtocol)
		if len(instance.h3PacketConns) > 0 {
			count += len(instance.h3PacketConns)
			continue
		}
		listenIPs := decodeReverseProxyListenIPs(instance.rule)
		if len(listenIPs) == 0 {
			listenIPs = []string{"0.0.0.0"}
		}
		if alias == reverseProxyDNSProtocolDoH {
			count += len(listenIPs) * 2
			continue
		}
		if alias == reverseProxyDNSProtocolDoHH3 {
			count += len(listenIPs)
			continue
		}
		count += len(listenIPs)
	}
	return count
}

func reverseProxyDNSCertificateStateKey(row *model.ReverseProxyRule, certificateState map[uint]model.CertificateRecord) string {
	if row == nil {
		return ""
	}
	certIDs := reverseProxyRuleCertificateIDs(row)
	if len(certIDs) == 0 {
		return ""
	}
	parts := make([]string, 0, len(certIDs))
	for _, certID := range certIDs {
		record := certificateState[certID]
		updatedAt := int64(0)
		if !record.UpdatedAt.IsZero() {
			updatedAt = record.UpdatedAt.Unix()
		}
		parts = append(parts, fmt.Sprintf("%d:%s:%d", certID, strings.TrimSpace(record.Fingerprint), updatedAt))
	}
	return strings.Join(parts, "|")
}
