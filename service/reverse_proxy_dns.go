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
	"sort"
	"strings"
	"sync"
	"time"

	dnsproxy "github.com/AdguardTeam/dnsproxy/proxy"
	dnsupstream "github.com/AdguardTeam/dnsproxy/upstream"
	aghnetutil "github.com/AdguardTeam/golibs/netutil"
	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
	"github.com/alireza0/s-ui/logger"
	"github.com/alireza0/s-ui/util/common"
	"github.com/miekg/dns"
	"github.com/quic-go/quic-go/http3"
)

const reverseProxyDNSShutdownTimeout = 5 * time.Second

type reverseProxyDNSRuntimeManager struct {
	mu      sync.Mutex
	running map[string]*reverseProxyDNSInstance
}

type reverseProxyDNSInstance struct {
	key             string
	ruleID          uint
	proxy           *dnsproxy.Proxy
	h3Server        *http3.Server
	h3PacketConns   []net.PacketConn
	rules           []model.ReverseProxyRule
	runtimeStateKey string
	cancel          context.CancelFunc
	doneCh          chan struct{}
	startErr        error
}

type reverseProxyDNSRuleHandler struct {
	defaultRoute *reverseProxyDNSRoute
	routes       map[string]*reverseProxyDNSRoute
	logger       *slog.Logger
}

type reverseProxyDNSRoute struct {
	rule      *model.ReverseProxyRule
	upstreams []dnsupstream.Upstream
}

type reverseProxyDNSIPStrategyResolver struct {
	base     dnsupstream.Resolver
	strategy string
}

var reverseProxyDNSRuntime = &reverseProxyDNSRuntimeManager{
	running: make(map[string]*reverseProxyDNSInstance),
}

func (m *reverseProxyDNSRuntimeManager) sync(service *ReverseProxyService, rows []model.ReverseProxyRule) error {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	want := make(map[string][]model.ReverseProxyRule)
	for i := range rows {
		row := rows[i]
		if !row.Enabled {
			continue
		}
		if !reverseProxyProtocolIsDNS(normalizeReverseProxyProtocolAlias(row.ListenProtocolAlias, row.ListenProtocol)) {
			continue
		}
		key := reverseProxyDNSInstanceKey(&row)
		want[key] = append(want[key], row)
	}
	for key := range want {
		sortReverseProxyDNSRules(want[key])
	}
	certificateState := loadReverseProxyCertificateRenderState(database.GetDB(), rows)

	nextRunning := make(map[string]*reverseProxyDNSInstance, len(want))
	created := make(map[string]*reverseProxyDNSInstance)
	stopped := make(map[string]*reverseProxyDNSInstance)
	for key, groupRows := range want {
		stateKey := reverseProxyDNSRuntimeStateKey(groupRows, certificateState)
		if instance, exists := m.running[key]; exists {
			if reverseProxyDNSInstanceMatchesRules(instance, groupRows, stateKey) {
				nextRunning[key] = instance
				continue
			}
			if err := instance.stop(); err != nil {
				return err
			}
			stopped[key] = instance
		}
		instance, err := newReverseProxyDNSInstance(service, key, groupRows, stateKey)
		if err != nil {
			for _, item := range created {
				_ = item.stop()
			}
			for oldKey, oldInstance := range stopped {
				if oldInstance == nil || len(oldInstance.rules) == 0 {
					continue
				}
				restored, restoreErr := newReverseProxyDNSInstance(service, oldInstance.key, oldInstance.rules, oldInstance.runtimeStateKey)
				if restoreErr != nil {
					logger.Warning("reverse proxy dns runtime rollback failed: ", restoreErr)
					continue
				}
				m.running[oldKey] = restored
			}
			return err
		}
		nextRunning[key] = instance
		created[key] = instance
	}
	for key, instance := range m.running {
		if _, exists := nextRunning[key]; exists {
			continue
		}
		_ = instance.stop()
	}
	m.running = nextRunning
	return nil
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

func reverseProxyDNSInstanceKey(row *model.ReverseProxyRule) string {
	if row == nil {
		return ""
	}
	alias := normalizeReverseProxyProtocolAlias(row.ListenProtocolAlias, row.ListenProtocol)
	listenIPs := decodeReverseProxyListenIPs(row)
	if len(listenIPs) == 0 {
		listenIPs = []string{"0.0.0.0"}
	}
	normalizedIPs := make([]string, 0, len(listenIPs))
	for _, item := range listenIPs {
		normalizedIPs = append(normalizedIPs, strings.ToLower(strings.TrimSpace(item)))
	}
	sort.Strings(normalizedIPs)
	keyParts := []string{
		alias,
		fmt.Sprintf("%d", row.ListenPort),
		strings.Join(normalizedIPs, ","),
	}
	if !reverseProxyDNSProtocolUsesPath(alias) {
		keyParts = append(keyParts, fmt.Sprintf("%d", row.Id))
	}
	return strings.Join(keyParts, "|")
}

func sortReverseProxyDNSRules(rows []model.ReverseProxyRule) {
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].ListOrder == rows[j].ListOrder {
			return rows[i].Id < rows[j].Id
		}
		return rows[i].ListOrder < rows[j].ListOrder
	})
}

func reverseProxyDNSRuntimeStateKey(rows []model.ReverseProxyRule, certificateState map[uint]model.CertificateRecord) string {
	parts := make([]string, 0, len(rows))
	for i := range rows {
		row := rows[i]
		parts = append(parts, strings.Join([]string{
			fmt.Sprintf("%d", row.Id),
			fmt.Sprintf("%d", row.ListOrder),
			row.ListenProtocol,
			row.ListenProtocolAlias,
			row.ListenIPList,
			fmt.Sprintf("%d", row.ListenPort),
			row.ListenDNSPath,
			row.TargetProtocol,
			row.TargetProtocolAlias,
			row.TargetAddresses,
			fmt.Sprintf("%d", row.TargetPort),
			row.TargetDNSPath,
			fmt.Sprintf("%t", row.EDNSEnabled),
			row.EDNSMode,
			row.EDNSCustomIP,
			row.EDNSClientSubnetPolicy,
			fmt.Sprintf("%t", row.DisableIPv4Answer),
			fmt.Sprintf("%t", row.DisableIPv6Answer),
			row.IPStrategy,
			fmt.Sprintf("%t", row.UpstreamTLSVerify),
			row.CertificateRecordList,
			fmt.Sprintf("%d", row.CertificateRecordID),
			reverseProxyDNSCertificateStateKey(&row, certificateState),
		}, "\x1f"))
	}
	return strings.Join(parts, "\x1e")
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

func reverseProxyDNSInstanceMatchesRules(instance *reverseProxyDNSInstance, rows []model.ReverseProxyRule, stateKey string) bool {
	if instance == nil {
		return false
	}
	return instance.runtimeStateKey == stateKey
}

func newReverseProxyDNSInstance(service *ReverseProxyService, key string, rows []model.ReverseProxyRule, stateKey string) (*reverseProxyDNSInstance, error) {
	if service == nil || len(rows) == 0 {
		return nil, errors.New("dns reverse proxy instance init failed: invalid rule")
	}
	row := &rows[0]

	handler, err := buildReverseProxyDNSRuleHandler(rows)
	if err != nil {
		return nil, err
	}

	conf, err := buildReverseProxyDNSProxyConfig(service, rows, handler)
	if err != nil {
		closeReverseProxyDNSHandler(handler)
		return nil, err
	}

	if normalizeReverseProxyProtocolAlias(row.ListenProtocolAlias, row.ListenProtocol) == reverseProxyDNSProtocolDoHH3 {
		instance, err := buildReverseProxyDNSH3OnlyRuntime(key, rows, conf.TLSConfig, handler, stateKey)
		if err != nil {
			closeReverseProxyDNSHandler(handler)
			return nil, err
		}
		return instance, nil
	}

	proxyInstance, err := dnsproxy.New(conf)
	if err != nil {
		closeReverseProxyDNSHandler(handler)
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())
	instance := &reverseProxyDNSInstance{
		key:             key,
		ruleID:          row.Id,
		proxy:           proxyInstance,
		rules:           cloneReverseProxyRules(rows),
		runtimeStateKey: stateKey,
		cancel:          cancel,
		doneCh:          make(chan struct{}),
	}

	if err := proxyInstance.Start(ctx); err != nil {
		cancel()
		closeReverseProxyDNSHandler(handler)
		return nil, translateReverseProxyDNSError(row, err)
	}

	_ = database.GetDB().Model(&model.ReverseProxyRule{}).Where("id IN ?", reverseProxyDNSRuleIDs(rows)).Updates(map[string]interface{}{
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

func buildReverseProxyDNSProxyConfig(service *ReverseProxyService, rows []model.ReverseProxyRule, handler *reverseProxyDNSRuleHandler) (*dnsproxy.Config, error) {
	if len(rows) == 0 || handler == nil {
		return nil, errors.New("dns reverse proxy config failed: invalid rule")
	}
	row := &rows[0]
	listenAlias := normalizeReverseProxyProtocolAlias(row.ListenProtocolAlias, row.ListenProtocol)
	targetAlias := normalizeReverseProxyProtocolAlias(row.TargetProtocolAlias, row.TargetProtocol)

	conf := &dnsproxy.Config{
		RequestHandler: dnsproxy.HandlerFunc(handler.ServeDNS),
		UpstreamMode:   dnsproxy.UpstreamModeLoadBalance,
		UpstreamConfig: &dnsproxy.UpstreamConfig{
			Upstreams: reverseProxyDNSHandlerUpstreams(handler),
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
		tlsConfig, err := buildReverseProxyDNSServerTLSConfig(service, rows, []string{"dot", "dns"})
		if err != nil {
			return nil, err
		}
		conf.TLSConfig = tlsConfig
		conf.TLSListenAddr = buildReverseProxyDNSTCPListenAddrs(listenIPs, row.ListenPort)
	case reverseProxyDNSProtocolDoQ:
		tlsConfig, err := buildReverseProxyDNSServerTLSConfig(service, rows, []string{"doq"})
		if err != nil {
			return nil, err
		}
		tlsConfig.NextProtos = []string{"doq", "doq-i02", "doq-i00", "dq"}
		conf.TLSConfig = tlsConfig
		conf.QUICListenAddr = buildReverseProxyDNSUDPListenAddrs(listenIPs, row.ListenPort)
	case reverseProxyDNSProtocolDoH, reverseProxyDNSProtocolDoHH3:
		nextProtos := []string{"h2", "http/1.1", "h3"}
		if listenAlias == reverseProxyDNSProtocolDoHH3 {
			nextProtos = []string{"h3"}
		}
		tlsConfig, err := buildReverseProxyDNSServerTLSConfig(service, rows, nextProtos)
		if err != nil {
			return nil, err
		}
		conf.TLSConfig = tlsConfig
		routes := make([]string, 0, len(rows)*2)
		routeSet := make(map[string]struct{}, len(rows)*2)
		for _, item := range rows {
			for _, route := range buildReverseProxyDNSDoHRoutes(strings.TrimSpace(item.ListenDNSPath)) {
				if _, exists := routeSet[route]; exists {
					continue
				}
				routeSet[route] = struct{}{}
				routes = append(routes, route)
			}
		}
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

func buildReverseProxyDNSH3OnlyRuntime(key string, rows []model.ReverseProxyRule, tlsConfig *tls.Config, handler *reverseProxyDNSRuleHandler, stateKey string) (*reverseProxyDNSInstance, error) {
	if len(rows) == 0 || tlsConfig == nil || handler == nil {
		return nil, errors.New("dns h3 runtime config failed")
	}
	row := &rows[0]
	conf := &dnsproxy.Config{
		RequestHandler: dnsproxy.HandlerFunc(handler.ServeDNS),
		UpstreamMode:   dnsproxy.UpstreamModeLoadBalance,
		UpstreamConfig: &dnsproxy.UpstreamConfig{
			Upstreams: reverseProxyDNSHandlerUpstreams(handler),
		},
		HTTPConfig: &dnsproxy.HTTPConfig{},
		Logger:     slog.Default(),
	}
	proxyInstance, err := dnsproxy.New(conf)
	if err != nil {
		return nil, err
	}
	mux := http.NewServeMux()
	registeredRoutes := make(map[string]struct{}, len(rows)*2)
	for _, item := range rows {
		for _, route := range buildReverseProxyDNSDoHRoutes(strings.TrimSpace(item.ListenDNSPath)) {
			if _, exists := registeredRoutes[route]; exists {
				continue
			}
			registeredRoutes[route] = struct{}{}
			mux.Handle(route, proxyInstance)
		}
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
		key:             key,
		ruleID:          row.Id,
		proxy:           nil,
		h3Server:        h3Server,
		h3PacketConns:   packetConns,
		rules:           cloneReverseProxyRules(rows),
		runtimeStateKey: stateKey,
		cancel:          cancel,
		doneCh:          make(chan struct{}),
	}
	go func() {
		<-ctx.Done()
		close(instance.doneCh)
	}()
	_ = database.GetDB().Model(&model.ReverseProxyRule{}).Where("id IN ?", reverseProxyDNSRuleIDs(rows)).Updates(map[string]interface{}{
		"last_error":     "",
		"runtime_status": "running",
	}).Error
	return instance, nil
}

func buildReverseProxyDNSRuleHandler(rows []model.ReverseProxyRule) (*reverseProxyDNSRuleHandler, error) {
	if len(rows) == 0 {
		return nil, errors.New("dns reverse proxy handler init failed: invalid rule")
	}
	handler := &reverseProxyDNSRuleHandler{
		routes: make(map[string]*reverseProxyDNSRoute),
		logger: slog.Default(),
	}
	for i := range rows {
		route, err := buildReverseProxyDNSRoute(&rows[i])
		if err != nil {
			closeReverseProxyDNSHandler(handler)
			return nil, err
		}
		if handler.defaultRoute == nil {
			handler.defaultRoute = route
		}
		alias := normalizeReverseProxyProtocolAlias(rows[i].ListenProtocolAlias, rows[i].ListenProtocol)
		if reverseProxyDNSProtocolUsesPath(alias) {
			path := normalizeReverseProxyDNSPath(rows[i].ListenDNSPath)
			if path == "" {
				path = "/dns-query"
			}
			if _, exists := handler.routes[path]; exists {
				closeReverseProxyDNSUpstreams(route.upstreams)
				closeReverseProxyDNSHandler(handler)
				return nil, fmt.Errorf("duplicate dns listener path: %s", path)
			}
			handler.routes[path] = route
		}
	}
	return handler, nil
}

func buildReverseProxyDNSRoute(row *model.ReverseProxyRule) (*reverseProxyDNSRoute, error) {
	targetAlias := normalizeReverseProxyProtocolAlias(row.TargetProtocolAlias, row.TargetProtocol)
	targets := decodeReverseProxyList(row.TargetAddresses)
	if len(targets) == 0 {
		return nil, errors.New("dns reverse proxy target is empty")
	}

	opts := buildReverseProxyDNSUpstreamOptions(row, targetAlias)

	upstreams := make([]dnsupstream.Upstream, 0, len(targets))
	for _, target := range targets {
		targetPath := row.TargetDNSPath
		if reverseProxyDNSProtocolUsesPath(targetAlias) && strings.TrimSpace(targetPath) == "" {
			targetPath = "/dns-query"
		}
		address, err := buildReverseProxyDNSUpstreamAddress(targetAlias, target, row.TargetPort, targetPath)
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

	return &reverseProxyDNSRoute{
		rule:      cloneReverseProxyRule(row),
		upstreams: upstreams,
	}, nil
}

func (h *reverseProxyDNSRuleHandler) ServeDNS(ctx context.Context, _ *dnsproxy.Proxy, dctx *dnsproxy.DNSContext) error {
	if h == nil || dctx == nil || dctx.Req == nil {
		return errors.New("dns reverse proxy handler received empty request")
	}
	route := h.defaultRoute
	if dctx.HTTPRequest != nil && dctx.HTTPRequest.URL != nil {
		if selected := h.routes[normalizeReverseProxyDNSPath(dctx.HTTPRequest.URL.Path)]; selected != nil {
			route = selected
		}
	}
	if route == nil || route.rule == nil {
		return errors.New("dns reverse proxy route is unavailable")
	}
	req := dctx.Req.Copy()
	reverseProxyDNSApplyEDNSPolicy(req, dctx, route.rule)
	var firstErr error
	for _, ups := range route.upstreams {
		if ups == nil {
			continue
		}
		resp, err := ups.Exchange(req.Copy())
		if err == nil && resp != nil {
			if route.rule.DisableIPv4Answer || route.rule.DisableIPv6Answer {
				reverseProxyDNSFilterResponse(resp, route.rule.DisableIPv4Answer, route.rule.DisableIPv6Answer)
			}
			dctx.Res = resp
			_ = database.GetDB().Model(&model.ReverseProxyRule{}).Where("id = ?", route.rule.Id).Updates(map[string]interface{}{
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
	_ = database.GetDB().Model(&model.ReverseProxyRule{}).Where("id = ?", route.rule.Id).Updates(map[string]interface{}{
		"last_error":     strings.TrimSpace(firstErr.Error()),
		"runtime_status": "upstream_error",
	}).Error
	return firstErr
}

func buildReverseProxyDNSUpstreamOptions(row *model.ReverseProxyRule, targetAlias string) *dnsupstream.Options {
	opts := &dnsupstream.Options{
		Timeout:            12 * time.Second,
		InsecureSkipVerify: !row.UpstreamTLSVerify,
		Logger:             slog.Default(),
		Bootstrap: reverseProxyDNSIPStrategyResolver{
			base:     net.DefaultResolver,
			strategy: row.IPStrategy,
		},
		PreferIPv6: strings.EqualFold(strings.TrimSpace(row.IPStrategy), reverseProxyIPStrategyPreferIPv6) ||
			strings.EqualFold(strings.TrimSpace(row.IPStrategy), reverseProxyIPStrategyIPv6Only),
	}
	if targetAlias == reverseProxyDNSProtocolDoH {
		opts.HTTPVersions = []dnsupstream.HTTPVersion{dnsupstream.HTTPVersion11, dnsupstream.HTTPVersion2}
	}
	if targetAlias == reverseProxyDNSProtocolDoHH3 {
		opts.HTTPVersions = []dnsupstream.HTTPVersion{dnsupstream.HTTPVersion3}
	}
	return opts
}

func (r reverseProxyDNSIPStrategyResolver) LookupNetIP(ctx context.Context, network string, host string) ([]netip.Addr, error) {
	base := r.base
	if base == nil {
		base = net.DefaultResolver
	}
	addrs, err := base.LookupNetIP(ctx, network, host)
	if err != nil {
		return nil, err
	}
	strategy := strings.ToLower(strings.TrimSpace(r.strategy))
	if strategy != reverseProxyIPStrategyIPv4Only && strategy != reverseProxyIPStrategyIPv6Only {
		return addrs, nil
	}
	filtered := make([]netip.Addr, 0, len(addrs))
	for _, addr := range addrs {
		if strategy == reverseProxyIPStrategyIPv4Only && addr.Is4() {
			filtered = append(filtered, addr)
			continue
		}
		if strategy == reverseProxyIPStrategyIPv6Only && addr.Is6() {
			filtered = append(filtered, addr)
		}
	}
	return filtered, nil
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

func buildReverseProxyDNSServerTLSConfig(service *ReverseProxyService, rows []model.ReverseProxyRule, nextProtos []string) (*tls.Config, error) {
	if service == nil || len(rows) == 0 {
		return nil, errors.New("dns reverse proxy tls config failed")
	}
	rulePtrs := make([]*model.ReverseProxyRule, 0, len(rows))
	hasCertificate := false
	for i := range rows {
		if len(reverseProxyRuleCertificateIDs(&rows[i])) > 0 {
			hasCertificate = true
		}
		rulePtrs = append(rulePtrs, &rows[i])
	}
	if !hasCertificate {
		return nil, errors.New("dns tls listener requires certificate")
	}
	_, orderedItems, err := service.loadRuleCertificates(rulePtrs)
	if err != nil {
		return nil, err
	}
	items := make([]*reverseProxyRuleCertificateBinding, 0, len(orderedItems))
	for _, item := range orderedItems {
		if item != nil {
			items = append(items, item)
		}
	}
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
	if normalizeReverseProxyProtocolAlias(rows[0].ListenProtocolAlias, rows[0].ListenProtocol) == reverseProxyDNSProtocolDoQ {
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

func cloneReverseProxyRules(rows []model.ReverseProxyRule) []model.ReverseProxyRule {
	out := make([]model.ReverseProxyRule, len(rows))
	copy(out, rows)
	return out
}

func reverseProxyDNSRuleIDs(rows []model.ReverseProxyRule) []uint {
	ids := make([]uint, 0, len(rows))
	for i := range rows {
		if rows[i].Id > 0 {
			ids = append(ids, rows[i].Id)
		}
	}
	return ids
}

func closeReverseProxyDNSUpstreams(items []dnsupstream.Upstream) {
	for _, item := range items {
		if item != nil {
			_ = item.Close()
		}
	}
}

func reverseProxyDNSHandlerUpstreams(handler *reverseProxyDNSRuleHandler) []dnsupstream.Upstream {
	if handler == nil {
		return nil
	}
	seen := make(map[dnsupstream.Upstream]struct{})
	out := make([]dnsupstream.Upstream, 0)
	for _, route := range handler.routes {
		if route == nil {
			continue
		}
		for _, ups := range route.upstreams {
			if ups == nil {
				continue
			}
			if _, exists := seen[ups]; exists {
				continue
			}
			seen[ups] = struct{}{}
			out = append(out, ups)
		}
	}
	if handler.defaultRoute != nil {
		for _, ups := range handler.defaultRoute.upstreams {
			if ups == nil {
				continue
			}
			if _, exists := seen[ups]; exists {
				continue
			}
			seen[ups] = struct{}{}
			out = append(out, ups)
		}
	}
	return out
}

func closeReverseProxyDNSHandler(handler *reverseProxyDNSRuleHandler) {
	closeReverseProxyDNSUpstreams(reverseProxyDNSHandlerUpstreams(handler))
}

func reverseProxyDNSApplyEDNSPolicy(req *dns.Msg, dctx *dnsproxy.DNSContext, rule *model.ReverseProxyRule) {
	if req == nil || rule == nil {
		return
	}
	if !rule.EDNSEnabled {
		reverseProxyDNSRemoveECS(req)
		return
	}

	switch normalizeReverseProxyEDNSMode(rule.EDNSMode) {
	case reverseProxyEDNSModeCustom:
		ip := net.ParseIP(strings.TrimSpace(rule.EDNSCustomIP))
		if ip == nil {
			reverseProxyDNSRemoveECS(req)
			return
		}
		reverseProxyDNSSetECS(req, ip)
	default:
		if normalizeReverseProxyEDNSClientSubnetPolicy(rule.EDNSClientSubnetPolicy) == reverseProxyEDNSClientSubnetPolicyPreferRequestPublic {
			if subnet, ok := reverseProxyDNSExtractUsableRequestECS(req); ok {
				reverseProxyDNSSetECSSubnet(req, subnet)
				return
			}
		}
		ip, ok := reverseProxyDNSResolveAutoEDNSIP(req, dctx, rule)
		if !ok {
			reverseProxyDNSRemoveECS(req)
			return
		}
		if ip4 := ip.To4(); ip4 != nil {
			ip = net.IPv4(ip4[0], ip4[1], ip4[2], 1)
		}
		reverseProxyDNSSetECS(req, ip)
	}
}

func reverseProxyDNSResolveAutoEDNSIP(req *dns.Msg, dctx *dnsproxy.DNSContext, rule *model.ReverseProxyRule) (net.IP, bool) {
	if dctx == nil || rule == nil {
		return nil, false
	}

	if normalizeReverseProxyEDNSClientSubnetPolicy(rule.EDNSClientSubnetPolicy) == reverseProxyEDNSClientSubnetPolicyPreferRequestPublic {
		if subnet, ok := reverseProxyDNSExtractUsableRequestECS(req); ok {
			return net.IP(append([]byte(nil), subnet.Address...)), true
		}
	}

	return reverseProxyDNSResolveClientEDNSIP(dctx)
}

func reverseProxyDNSResolveClientEDNSIP(dctx *dnsproxy.DNSContext) (net.IP, bool) {
	if dctx == nil {
		return nil, false
	}

	clientAddr := dctx.Addr.Addr()
	if !clientAddr.IsValid() || aghnetutil.IsSpecialPurpose(clientAddr) {
		return nil, false
	}

	clientIP := clientAddr.AsSlice()
	if len(clientIP) == 0 {
		return nil, false
	}

	return net.IP(append([]byte(nil), clientIP...)), true
}

func reverseProxyDNSExtractUsableRequestECS(req *dns.Msg) (*dns.EDNS0_SUBNET, bool) {
	if req == nil {
		return nil, false
	}

	opt := req.IsEdns0()
	if opt == nil {
		return nil, false
	}
	for _, option := range opt.Option {
		subnet, ok := option.(*dns.EDNS0_SUBNET)
		if !ok {
			continue
		}
		normalized, ok := reverseProxyDNSNormalizeUsableECS(subnet)
		if !ok {
			continue
		}
		return normalized, true
	}

	return nil, false
}

func reverseProxyDNSExtractUsableRequestECSIP(req *dns.Msg) (net.IP, bool) {
	subnet, ok := reverseProxyDNSExtractUsableRequestECS(req)
	if !ok || subnet == nil {
		return nil, false
	}

	return net.IP(append([]byte(nil), subnet.Address...)), true
}

func reverseProxyDNSNormalizeUsableECS(subnet *dns.EDNS0_SUBNET) (*dns.EDNS0_SUBNET, bool) {
	if subnet == nil {
		return nil, false
	}

	normalized := &dns.EDNS0_SUBNET{
		Code:          dns.EDNS0SUBNET,
		Family:        subnet.Family,
		SourceNetmask: subnet.SourceNetmask,
		SourceScope:   subnet.SourceScope,
	}

	switch subnet.Family {
	case 1:
		if subnet.SourceNetmask > net.IPv4len*8 {
			return nil, false
		}
		ip := subnet.Address.To4()
		if ip == nil {
			return nil, false
		}
		addr, ok := netip.AddrFromSlice(ip)
		if !ok || !addr.IsValid() || aghnetutil.IsSpecialPurpose(addr) {
			return nil, false
		}
		normalized.Address = net.IPv4(ip[0], ip[1], ip[2], ip[3])
	case 2:
		if subnet.SourceNetmask > net.IPv6len*8 {
			return nil, false
		}
		ip := subnet.Address.To16()
		if len(ip) != net.IPv6len {
			return nil, false
		}
		addr, ok := netip.AddrFromSlice(ip)
		if !ok || !addr.IsValid() || aghnetutil.IsSpecialPurpose(addr) {
			return nil, false
		}
		normalized.Address = append(net.IP(nil), ip...)
	default:
		return nil, false
	}

	return normalized, true
}

func reverseProxyDNSSetECS(req *dns.Msg, ip net.IP) {
	if req == nil || ip == nil {
		return
	}

	reverseProxyDNSRemoveECS(req)

	subnet := &dns.EDNS0_SUBNET{
		Code:        dns.EDNS0SUBNET,
		SourceScope: 0,
	}
	if ip4 := ip.To4(); ip4 != nil {
		subnet.Family = 1
		subnet.SourceNetmask = 32
		subnet.Address = net.IPv4(ip4[0], ip4[1], ip4[2], ip4[3])
	} else {
		subnet.Family = 2
		subnet.SourceNetmask = 128
		subnet.Address = append(net.IP(nil), ip...)
	}

	if opt := req.IsEdns0(); opt != nil {
		opt.Option = append(opt.Option, subnet)
		return
	}

	opt := &dns.OPT{
		Hdr: dns.RR_Header{
			Name:   ".",
			Rrtype: dns.TypeOPT,
		},
		Option: []dns.EDNS0{subnet},
	}
	opt.SetUDPSize(4096)
	req.Extra = append(req.Extra, opt)
}

func reverseProxyDNSSetECSSubnet(req *dns.Msg, subnet *dns.EDNS0_SUBNET) {
	if req == nil || subnet == nil {
		return
	}

	normalized, ok := reverseProxyDNSNormalizeUsableECS(subnet)
	if !ok {
		reverseProxyDNSRemoveECS(req)
		return
	}

	reverseProxyDNSRemoveECS(req)

	if opt := req.IsEdns0(); opt != nil {
		opt.Option = append(opt.Option, normalized)
		return
	}

	opt := &dns.OPT{
		Hdr: dns.RR_Header{
			Name:   ".",
			Rrtype: dns.TypeOPT,
		},
		Option: []dns.EDNS0{normalized},
	}
	opt.SetUDPSize(4096)
	req.Extra = append(req.Extra, opt)
}

func reverseProxyDNSRemoveECS(req *dns.Msg) {
	if req == nil {
		return
	}

	opt := req.IsEdns0()
	if opt == nil {
		return
	}

	filtered := opt.Option[:0]
	for _, option := range opt.Option {
		if _, ok := option.(*dns.EDNS0_SUBNET); ok {
			continue
		}
		filtered = append(filtered, option)
	}
	opt.Option = filtered
}

func reverseProxyDNSFilterResponse(resp *dns.Msg, disableIPv4 bool, disableIPv6 bool) {
	if resp == nil || (!disableIPv4 && !disableIPv6) {
		return
	}

	droppedKeys := make(map[string]struct{})
	reverseProxyDNSCollectDroppedKeys(resp.Answer, disableIPv4, disableIPv6, droppedKeys)
	reverseProxyDNSCollectDroppedKeys(resp.Ns, disableIPv4, disableIPv6, droppedKeys)
	reverseProxyDNSCollectDroppedKeys(resp.Extra, disableIPv4, disableIPv6, droppedKeys)

	answer, answerChanged := reverseProxyDNSFilterRRSection(resp.Answer, disableIPv4, disableIPv6, droppedKeys)
	ns, nsChanged := reverseProxyDNSFilterRRSection(resp.Ns, disableIPv4, disableIPv6, droppedKeys)
	extra, extraChanged := reverseProxyDNSFilterRRSection(resp.Extra, disableIPv4, disableIPv6, droppedKeys)

	resp.Answer = answer
	resp.Ns = ns
	resp.Extra = extra
	if answerChanged || nsChanged || extraChanged {
		resp.AuthenticatedData = false
	}
}

func reverseProxyDNSCollectDroppedKeys(items []dns.RR, disableIPv4 bool, disableIPv6 bool, droppedKeys map[string]struct{}) {
	if len(items) == 0 || droppedKeys == nil || (!disableIPv4 && !disableIPv6) {
		return
	}

	for _, rr := range items {
		reverseProxyDNSMarkDroppedKeys(rr, disableIPv4, disableIPv6, droppedKeys)
	}
}

func reverseProxyDNSFilterRRSection(items []dns.RR, disableIPv4 bool, disableIPv6 bool, droppedKeys map[string]struct{}) ([]dns.RR, bool) {
	if len(items) == 0 || (!disableIPv4 && !disableIPv6) {
		return items, false
	}

	filtered := make([]dns.RR, 0, len(items))
	changed := false
	for _, rr := range items {
		if rr == nil {
			changed = true
			continue
		}
		drop, rrChanged := reverseProxyDNSShouldDropRR(rr, disableIPv4, disableIPv6, droppedKeys)
		if rrChanged {
			changed = true
		}
		if drop {
			continue
		}
		if sig, ok := rr.(*dns.RRSIG); ok {
			if _, exists := droppedKeys[reverseProxyDNSRRSIGKey(sig.Hdr.Name, sig.TypeCovered)]; exists {
				changed = true
				continue
			}
		}
		filtered = append(filtered, rr)
	}

	return filtered, changed
}

func reverseProxyDNSMarkDroppedKeys(rr dns.RR, disableIPv4 bool, disableIPv6 bool, droppedKeys map[string]struct{}) {
	if rr == nil || droppedKeys == nil {
		return
	}

	switch record := rr.(type) {
	case *dns.A:
		if disableIPv4 {
			reverseProxyDNSMarkRRSIGForType(record.Header().Name, dns.TypeA, droppedKeys)
		}
	case *dns.AAAA:
		if disableIPv6 {
			reverseProxyDNSMarkRRSIGForType(record.Header().Name, dns.TypeAAAA, droppedKeys)
		}
	case *dns.NSEC:
		if (disableIPv4 && reverseProxyDNSNSECContainsType(record.TypeBitMap, dns.TypeA)) ||
			(disableIPv6 && reverseProxyDNSNSECContainsType(record.TypeBitMap, dns.TypeAAAA)) {
			reverseProxyDNSMarkRRSIGForType(record.Header().Name, dns.TypeNSEC, droppedKeys)
		}
	case *dns.NSEC3:
		if (disableIPv4 && reverseProxyDNSNSECContainsType(record.TypeBitMap, dns.TypeA)) ||
			(disableIPv6 && reverseProxyDNSNSECContainsType(record.TypeBitMap, dns.TypeAAAA)) {
			reverseProxyDNSMarkRRSIGForType(record.Header().Name, dns.TypeNSEC3, droppedKeys)
		}
	case *dns.HTTPS:
		if reverseProxyDNSHasBlockedSVCBHints(record.Value, disableIPv4, disableIPv6) {
			reverseProxyDNSMarkRRSIGForType(record.Header().Name, dns.TypeHTTPS, droppedKeys)
		}
	case *dns.SVCB:
		if reverseProxyDNSHasBlockedSVCBHints(record.Value, disableIPv4, disableIPv6) {
			reverseProxyDNSMarkRRSIGForType(record.Header().Name, dns.TypeSVCB, droppedKeys)
		}
	}
}

func reverseProxyDNSShouldDropRR(rr dns.RR, disableIPv4 bool, disableIPv6 bool, droppedKeys map[string]struct{}) (bool, bool) {
	switch record := rr.(type) {
	case *dns.A:
		if disableIPv4 {
			return true, true
		}
	case *dns.AAAA:
		if disableIPv6 {
			return true, true
		}
	case *dns.RRSIG:
		if disableIPv4 && record.TypeCovered == dns.TypeA {
			return true, true
		}
		if disableIPv6 && record.TypeCovered == dns.TypeAAAA {
			return true, true
		}
	case *dns.NSEC:
		if disableIPv4 && reverseProxyDNSNSECContainsType(record.TypeBitMap, dns.TypeA) {
			return true, true
		}
		if disableIPv6 && reverseProxyDNSNSECContainsType(record.TypeBitMap, dns.TypeAAAA) {
			return true, true
		}
	case *dns.NSEC3:
		if disableIPv4 && reverseProxyDNSNSECContainsType(record.TypeBitMap, dns.TypeA) {
			return true, true
		}
		if disableIPv6 && reverseProxyDNSNSECContainsType(record.TypeBitMap, dns.TypeAAAA) {
			return true, true
		}
	case *dns.HTTPS:
		if reverseProxyDNSFilterSVCBValueHints(&record.Value, disableIPv4, disableIPv6) {
			reverseProxyDNSMarkRRSIGForType(record.Header().Name, dns.TypeHTTPS, droppedKeys)
			return false, true
		}
	case *dns.SVCB:
		if reverseProxyDNSFilterSVCBValueHints(&record.Value, disableIPv4, disableIPv6) {
			reverseProxyDNSMarkRRSIGForType(record.Header().Name, dns.TypeSVCB, droppedKeys)
			return false, true
		}
	}

	return false, false
}

func reverseProxyDNSHasBlockedSVCBHints(values []dns.SVCBKeyValue, disableIPv4 bool, disableIPv6 bool) bool {
	if len(values) == 0 || (!disableIPv4 && !disableIPv6) {
		return false
	}

	for _, value := range values {
		switch value.(type) {
		case *dns.SVCBIPv4Hint:
			if disableIPv4 {
				return true
			}
		case *dns.SVCBIPv6Hint:
			if disableIPv6 {
				return true
			}
		}
	}

	return false
}

func reverseProxyDNSFilterSVCBValueHints(values *[]dns.SVCBKeyValue, disableIPv4 bool, disableIPv6 bool) bool {
	if values == nil || len(*values) == 0 || (!disableIPv4 && !disableIPv6) {
		return false
	}

	changed := false
	filtered := make([]dns.SVCBKeyValue, 0, len(*values))
	for _, value := range *values {
		switch value.(type) {
		case *dns.SVCBIPv4Hint:
			if disableIPv4 {
				changed = true
				continue
			}
			filtered = append(filtered, value)
		case *dns.SVCBIPv6Hint:
			if disableIPv6 {
				changed = true
				continue
			}
			filtered = append(filtered, value)
		default:
			filtered = append(filtered, value)
		}
	}
	if changed {
		*values = filtered
	}
	return changed
}

func reverseProxyDNSMarkRRSIGForType(name string, coveredType uint16, droppedKeys map[string]struct{}) {
	if droppedKeys == nil {
		return
	}
	droppedKeys[reverseProxyDNSRRSIGKey(name, coveredType)] = struct{}{}
}

func reverseProxyDNSRRSIGKey(name string, coveredType uint16) string {
	return strings.ToLower(strings.TrimSpace(name)) + "|" + fmt.Sprintf("%d", coveredType)
}

func reverseProxyDNSNSECContainsType(items []uint16, target uint16) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
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
		if instance == nil || len(instance.rules) == 0 {
			continue
		}
		rule := &instance.rules[0]
		alias := normalizeReverseProxyProtocolAlias(rule.ListenProtocolAlias, rule.ListenProtocol)
		if len(instance.h3PacketConns) > 0 {
			count += len(instance.h3PacketConns)
			continue
		}
		listenIPs := decodeReverseProxyListenIPs(rule)
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
