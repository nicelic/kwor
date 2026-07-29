package service

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"sync"
	"time"

	dnsproxy "github.com/AdguardTeam/dnsproxy/proxy"
	"github.com/alireza0/s-ui/database/model"
	"github.com/miekg/dns"
	"github.com/quic-go/quic-go"
)

const reverseProxyDNSMaximumWireBytes = 65535

// reverseProxyDNSLimitedListener applies the saved listener-group connection
// safety valve before a TCP or DoT connection reaches its protocol
// server.  Closing a connection always returns its lease, including a server
// shutdown path.
type reverseProxyDNSLimitedListener struct {
	net.Listener
	limiter *reverseProxyAdjustableLimiter
}

type reverseProxyDNSLimitedConn struct {
	net.Conn
	release func()
	once    sync.Once
}

func (c *reverseProxyDNSLimitedConn) Close() error {
	err := c.Conn.Close()
	c.once.Do(func() {
		if c.release != nil {
			c.release()
		}
	})
	return err
}

func (l *reverseProxyDNSLimitedListener) Accept() (net.Conn, error) {
	if l == nil || l.Listener == nil {
		return nil, net.ErrClosed
	}
	for {
		conn, err := l.Listener.Accept()
		if err != nil || conn == nil {
			return conn, err
		}
		if l.limiter == nil || l.limiter.TryAcquire() {
			return &reverseProxyDNSLimitedConn{
				Conn: conn,
				release: func() {
					if l.limiter != nil {
						l.limiter.Release()
					}
				},
			}, nil
		}
		// There is no protocol-neutral overload response at this level.  Close
		// immediately so a stalled client never consumes a goroutine or lease.
		_ = conn.Close()
	}
}

func newReverseProxyOwnedDNSListenerInstance(service *ReverseProxyService, key string, rows []model.ReverseProxyRule, handler *reverseProxyDNSRuleHandler, stateKey string, listenerStateKey string) (*reverseProxyDNSInstance, error) {
	if service == nil || len(rows) == 0 || handler == nil {
		return nil, errors.New("dns reverse proxy listener init failed")
	}
	row := &rows[0]
	alias := normalizeReverseProxyProtocolAlias(row.ListenProtocolAlias, row.ListenProtocol)
	ctx, cancel := context.WithCancel(context.Background())
	instance := &reverseProxyDNSInstance{
		key:               key,
		ruleID:            row.Id,
		handler:           handler,
		rules:             cloneReverseProxyRules(rows),
		runtimeStateKey:   stateKey,
		listenerStateKey:  listenerStateKey,
		connectionLimiter: newReverseProxyAdjustableLimiter(reverseProxyResources.current().ListenerConnectionLimit),
		cancel:            cancel,
		doneCh:            make(chan struct{}),
	}
	go func() {
		<-ctx.Done()
		close(instance.doneCh)
	}()

	var err error
	switch alias {
	case reverseProxyDNSProtocolUDP:
		err = instance.startPlainDNS(ctx, row, dnsproxy.ProtoUDP, nil)
	case reverseProxyDNSProtocolTCP:
		err = instance.startPlainDNS(ctx, row, dnsproxy.ProtoTCP, nil)
	case reverseProxyDNSProtocolDoT:
		var tlsConfig *tls.Config
		tlsConfig, err = buildReverseProxyDNSServerTLSConfig(service, rows, []string{"dot", "dns"})
		if err == nil {
			err = instance.startPlainDNS(ctx, row, dnsproxy.ProtoTLS, tlsConfig)
		}
	case reverseProxyDNSProtocolDoH, reverseProxyDNSProtocolDoHH3:
		err = errors.New("doh listeners are owned by the shared http runtime")
	case reverseProxyDNSProtocolDoQ:
		var tlsConfig *tls.Config
		tlsConfig, err = buildReverseProxyDNSServerTLSConfig(service, rows, []string{"doq", "doq-i02", "doq-i00", "dq"})
		if err == nil {
			tlsConfig.MinVersion = tls.VersionTLS13
			err = instance.startDoQ(ctx, row, tlsConfig)
		}
	default:
		err = fmt.Errorf("unsupported dns listen protocol: %s", alias)
	}
	if err != nil {
		_ = instance.stop()
		return nil, translateReverseProxyDNSError(row, err)
	}
	return instance, nil
}

func (i *reverseProxyDNSInstance) startPlainDNS(ctx context.Context, row *model.ReverseProxyRule, proto dnsproxy.Proto, tlsConfig *tls.Config) error {
	if i == nil || row == nil || i.handler == nil {
		return errors.New("dns listener is unavailable")
	}
	handler := dns.HandlerFunc(func(writer dns.ResponseWriter, request *dns.Msg) {
		response, err := i.handler.resolveMessage(ctx, request, writer.RemoteAddr(), proto, nil)
		if response == nil {
			response = reverseProxyDNSServfailResponse(request)
		}
		if err != nil {
			reverseProxyRuntime.reportRuleState(row.Id, "upstream_error", err.Error())
		}
		_ = writer.WriteMsg(response)
	})
	started := 0
	if proto == dnsproxy.ProtoUDP {
		for _, bind := range reverseProxyUDPListenBinds(row.ListenPort) {
			packetConn, err := net.ListenPacket(bind.network, bind.address)
			if err != nil {
				if bind.optional && reverseProxyListenErrorAllowsOptionalBind(bind, err) {
					continue
				}
				return err
			}
			server := &dns.Server{
				PacketConn:    packetConn,
				Net:           "udp",
				Handler:       handler,
				UDPSize:       reverseProxyDNSMaximumWireBytes,
				ReadTimeout:   reverseProxyServerIdleTimeout,
				WriteTimeout:  reverseProxyServerIdleTimeout,
				MsgAcceptFunc: dns.DefaultMsgAcceptFunc,
			}
			i.dnsServers = append(i.dnsServers, server)
			go i.serveDNSServer(server, row.Id)
			started++
		}
	} else {
		for _, bind := range reverseProxyTCPListenBinds(row.ListenPort) {
			listener, err := net.Listen(bind.network, bind.address)
			if err != nil {
				if bind.optional && reverseProxyListenErrorAllowsOptionalBind(bind, err) {
					continue
				}
				return err
			}
			if proto == dnsproxy.ProtoTLS {
				listener = tls.NewListener(listener, tlsConfig.Clone())
			}
			limited := &reverseProxyDNSLimitedListener{Listener: listener, limiter: i.connectionLimiter}
			server := &dns.Server{
				Listener:     limited,
				Net:          "tcp",
				Handler:      handler,
				ReadTimeout:  reverseProxyServerIdleTimeout,
				WriteTimeout: reverseProxyServerIdleTimeout,
				IdleTimeout:  func() time.Duration { return reverseProxyServerIdleTimeout },
			}
			i.dnsServers = append(i.dnsServers, server)
			go i.serveDNSServer(server, row.Id)
			started++
		}
	}
	if started == 0 {
		return errors.New("dns listener failed: no wildcard bind started")
	}
	return nil
}

func (i *reverseProxyDNSInstance) serveDNSServer(server *dns.Server, ruleID uint) {
	if server == nil {
		return
	}
	err := server.ActivateAndServe()
	if err == nil || errors.Is(err, net.ErrClosed) {
		return
	}
	// Shutdown commonly returns a wrapped network-close error.  It is already
	// represented by the lifecycle state and must not overwrite it.
	if strings.Contains(strings.ToLower(err.Error()), "use of closed network connection") {
		return
	}
	reverseProxyRuntime.reportRuleState(ruleID, "listener_error", err.Error())
}

func (i *reverseProxyDNSInstance) startDoQ(ctx context.Context, row *model.ReverseProxyRule, tlsConfig *tls.Config) error {
	if i == nil || row == nil || i.handler == nil || tlsConfig == nil {
		return errors.New("doq listener is unavailable")
	}
	resources := reverseProxyResources.current()
	started := 0
	for _, bind := range reverseProxyUDPListenBinds(row.ListenPort) {
		packetConn, err := net.ListenPacket(bind.network, bind.address)
		if err != nil {
			if bind.optional && reverseProxyListenErrorAllowsOptionalBind(bind, err) {
				continue
			}
			return err
		}
		listenerTLS := tlsConfig.Clone()
		baseGetCertificate := listenerTLS.GetCertificate
		listenerTLS.GetCertificate = func(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
			return baseGetCertificate(reverseProxyClientHelloWithLocalIPHint(hello, bind.listenIP))
		}
		listener, err := quic.Listen(packetConn, listenerTLS, &quic.Config{
			MaxIncomingStreams:    resources.QUICMaxIncomingStreams,
			MaxIncomingUniStreams: resources.QUICMaxIncomingStreams,
			MaxIdleTimeout:        reverseProxyServerIdleTimeout,
		})
		if err != nil {
			_ = packetConn.Close()
			return err
		}
		i.doqListeners = append(i.doqListeners, listener)
		i.doqPacketConns = append(i.doqPacketConns, packetConn)
		go i.serveDoQListener(ctx, listener, row.Id)
		started++
	}
	if started == 0 {
		return errors.New("doq listener failed: no wildcard bind started")
	}
	return nil
}

func (i *reverseProxyDNSInstance) serveDoQListener(ctx context.Context, listener *quic.Listener, ruleID uint) {
	if i == nil || listener == nil {
		return
	}
	for {
		connection, err := listener.Accept(ctx)
		if err != nil {
			if ctx.Err() == nil && !errors.Is(err, net.ErrClosed) {
				reverseProxyRuntime.reportRuleState(ruleID, "listener_error", err.Error())
			}
			return
		}
		if i.connectionLimiter != nil && !i.connectionLimiter.TryAcquire() {
			_ = connection.CloseWithError(0, "reverse proxy dns connection limit reached")
			continue
		}
		go func(conn *quic.Conn) {
			defer func() {
				if i.connectionLimiter != nil {
					i.connectionLimiter.Release()
				}
			}()
			i.serveDoQConnection(ctx, conn, ruleID)
		}(connection)
	}
}

func (i *reverseProxyDNSInstance) serveDoQConnection(ctx context.Context, connection *quic.Conn, ruleID uint) {
	if i == nil || connection == nil || i.handler == nil {
		return
	}
	for {
		stream, err := connection.AcceptStream(ctx)
		if err != nil {
			return
		}
		go i.serveDoQStream(ctx, connection, stream, ruleID)
	}
}

func (i *reverseProxyDNSInstance) serveDoQStream(ctx context.Context, connection *quic.Conn, stream *quic.Stream, ruleID uint) {
	if i == nil || connection == nil || stream == nil || i.handler == nil {
		return
	}
	defer stream.Close()
	_ = stream.SetDeadline(time.Now().Add(reverseProxyServerIdleTimeout))
	var sizeBytes [2]byte
	if _, err := io.ReadFull(stream, sizeBytes[:]); err != nil {
		return
	}
	size := int(binary.BigEndian.Uint16(sizeBytes[:]))
	if size <= 0 || size > reverseProxyDNSMaximumWireBytes {
		return
	}
	wire := make([]byte, size)
	if _, err := io.ReadFull(stream, wire); err != nil {
		return
	}
	request := &dns.Msg{}
	if err := request.Unpack(wire); err != nil {
		return
	}
	response, err := i.handler.resolveMessage(ctx, request, connection.RemoteAddr(), dnsproxy.ProtoQUIC, nil)
	if response == nil {
		response = reverseProxyDNSServfailResponse(request)
	}
	if err != nil {
		reverseProxyRuntime.reportRuleState(ruleID, "upstream_error", err.Error())
	}
	encoded, err := response.Pack()
	if err != nil || len(encoded) > reverseProxyDNSMaximumWireBytes {
		return
	}
	binary.BigEndian.PutUint16(sizeBytes[:], uint16(len(encoded)))
	_, _ = stream.Write(sizeBytes[:])
	_, _ = stream.Write(encoded)
}

func (h *reverseProxyDNSRuleHandler) resolveMessage(ctx context.Context, request *dns.Msg, remoteAddr net.Addr, proto dnsproxy.Proto, httpRequest *http.Request) (*dns.Msg, error) {
	if h == nil || request == nil {
		return nil, errors.New("dns request is unavailable")
	}
	dctx := &dnsproxy.DNSContext{
		Req:         request,
		Addr:        reverseProxyDNSAddrPort(remoteAddr),
		Proto:       proto,
		HTTPRequest: httpRequest,
	}
	err := h.ServeDNS(ctx, nil, dctx)
	return dctx.Res, err
}

func (h *reverseProxyDNSRuleHandler) resolveMessageForRule(ctx context.Context, request *dns.Msg, remoteAddr net.Addr, proto dnsproxy.Proto, httpRequest *http.Request, ruleID uint) (*dns.Msg, error) {
	if h == nil || request == nil || ruleID == 0 {
		return nil, errors.New("dns request rule is unavailable")
	}
	dctx := &dnsproxy.DNSContext{
		Req:         request,
		Addr:        reverseProxyDNSAddrPort(remoteAddr),
		Proto:       proto,
		HTTPRequest: httpRequest,
	}
	err := h.serveDNSRule(ctx, dctx, ruleID)
	return dctx.Res, err
}

func reverseProxyDNSAddrPort(addr net.Addr) netip.AddrPort {
	if addr == nil {
		return netip.AddrPort{}
	}
	host, portText, err := net.SplitHostPort(addr.String())
	if err != nil {
		return netip.AddrPort{}
	}
	ip, err := netip.ParseAddr(strings.Trim(strings.TrimSpace(host), "[]"))
	if err != nil {
		return netip.AddrPort{}
	}
	var port uint64
	for _, char := range portText {
		if char < '0' || char > '9' {
			return netip.AddrPort{}
		}
		port = port*10 + uint64(char-'0')
		if port > 65535 {
			return netip.AddrPort{}
		}
	}
	return netip.AddrPortFrom(ip.Unmap(), uint16(port))
}

func reverseProxyDNSServfailResponse(request *dns.Msg) *dns.Msg {
	if request == nil {
		return nil
	}
	response := new(dns.Msg)
	response.SetReply(request)
	response.Rcode = dns.RcodeServerFailure
	return response
}

func (h *reverseProxyDNSRuleHandler) dohHTTPHandler() http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if h == nil || request.URL == nil {
			http.Error(writer, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
			return
		}
		path := normalizeReverseProxyDNSPath(request.URL.Path)
		h.mu.RLock()
		route := h.routes[path]
		h.mu.RUnlock()
		if route == nil {
			http.NotFound(writer, request)
			return
		}
		ruleID := uint(0)
		if route.rule != nil {
			ruleID = route.rule.Id
		}
		h.serveDoHRule(writer, request, ruleID)
	})
}

func (h *reverseProxyDNSRuleHandler) serveDoHRule(writer http.ResponseWriter, request *http.Request, ruleID uint) {
	if h == nil || request == nil || request.URL == nil || ruleID == 0 {
		http.Error(writer, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	if request.Method != http.MethodGet && request.Method != http.MethodPost {
		writer.Header().Set("Allow", http.MethodGet+", "+http.MethodPost)
		http.Error(writer, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}
	wire, err := reverseProxyDNSReadDoHMessage(writer, request)
	if err != nil {
		http.Error(writer, err.Error(), http.StatusBadRequest)
		return
	}
	query := &dns.Msg{}
	if err := query.Unpack(wire); err != nil {
		http.Error(writer, "invalid dns message", http.StatusBadRequest)
		return
	}
	response, resolveErr := h.resolveMessageForRule(request.Context(), query, reverseProxyHTTPRequestRemoteAddr(request), dnsproxy.ProtoHTTPS, request, ruleID)
	if response == nil {
		response = reverseProxyDNSServfailResponse(query)
	}
	encoded, err := response.Pack()
	if err != nil || len(encoded) > reverseProxyDNSMaximumWireBytes {
		http.Error(writer, "dns response is unavailable", http.StatusBadGateway)
		return
	}
	if resolveErr != nil {
		reverseProxyRuntime.reportRuleState(ruleID, "upstream_error", resolveErr.Error())
	}
	writer.Header().Set("Content-Type", "application/dns-message")
	writer.Header().Set("Cache-Control", "no-store")
	writer.WriteHeader(http.StatusOK)
	_, _ = writer.Write(encoded)
}

func reverseProxyDNSReadDoHMessage(writer http.ResponseWriter, request *http.Request) ([]byte, error) {
	if request == nil {
		return nil, errors.New("dns request is unavailable")
	}
	if request.Method == http.MethodGet {
		encoded := strings.TrimSpace(request.URL.Query().Get("dns"))
		if encoded == "" {
			return nil, errors.New("missing dns query parameter")
		}
		wire, err := base64.RawURLEncoding.DecodeString(encoded)
		if err != nil {
			wire, err = base64.URLEncoding.DecodeString(encoded)
		}
		if err != nil || len(wire) == 0 || len(wire) > reverseProxyDNSMaximumWireBytes {
			return nil, errors.New("invalid dns query parameter")
		}
		return wire, nil
	}
	if !strings.EqualFold(strings.TrimSpace(strings.Split(request.Header.Get("Content-Type"), ";")[0]), "application/dns-message") {
		return nil, errors.New("content-type must be application/dns-message")
	}
	defer request.Body.Close()
	wire, err := io.ReadAll(http.MaxBytesReader(writer, request.Body, reverseProxyDNSMaximumWireBytes+1))
	if err != nil || len(wire) == 0 || len(wire) > reverseProxyDNSMaximumWireBytes {
		return nil, errors.New("invalid dns message body")
	}
	return wire, nil
}

func reverseProxyHTTPRequestRemoteAddr(request *http.Request) net.Addr {
	if request == nil {
		return nil
	}
	address, err := net.ResolveTCPAddr("tcp", request.RemoteAddr)
	if err != nil {
		return nil
	}
	return address
}
