package service

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"sort"
	"strings"
	"sync"

	dnsproxy "github.com/AdguardTeam/dnsproxy/proxy"
	dnsupstream "github.com/AdguardTeam/dnsproxy/upstream"
	"github.com/AdguardTeam/golibs/container"
	aghnetutil "github.com/AdguardTeam/golibs/netutil"
	compressionalgorithm "github.com/alireza0/s-ui/compression/Compression-algorithm"
	"github.com/miekg/dns"
	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
	"golang.org/x/net/http2"
)

// reverseProxyDNSCompressedHTTPUpstream is used for primary DoH/DoH3 targets.
// dnsproxy's built-in DoH client deliberately disables HTTP compression, while
// this project needs the target connection to negotiate and decode the same
// content codings as the ordinary reverse proxy.
type reverseProxyDNSCompressedHTTPUpstream struct {
	address *url.URL
	client  *http.Client
	closeFn func()
	once    sync.Once
}

func newReverseProxyDNSCompressedHTTPUpstream(address string, opts *dnsupstream.Options, http3Only bool) (*reverseProxyDNSCompressedHTTPUpstream, error) {
	parsed, err := url.Parse(address)
	if err != nil || parsed == nil || parsed.Hostname() == "" {
		return nil, fmt.Errorf("invalid dns http upstream address: %s", address)
	}
	if opts == nil {
		opts = &dnsupstream.Options{}
	}
	if parsed.Path == "" {
		parsed.Path = "/dns-query"
	}
	if http3Only && strings.EqualFold(parsed.Scheme, "h3") {
		// net/http URLs use https even when the selected RoundTripper is
		// HTTP/3. The h3 scheme is only the panel's upstream configuration
		// marker.
		parsed.Scheme = "https"
	}
	host := parsed.Hostname()
	port := parsed.Port()
	if port == "" {
		port = "443"
	}
	lookup := reverseProxyDNSHTTPLookupFunc(opts)
	tlsConfig := reverseProxyDNSHTTPUpstreamTLSConfig(parsed, opts, http3Only)
	result := &reverseProxyDNSCompressedHTTPUpstream{address: parsed}
	timeout := opts.Timeout
	if http3Only {
		transport := &http3.Transport{
			TLSClientConfig:    tlsConfig,
			DisableCompression: true,
			QUICConfig:         &quic.Config{KeepAlivePeriod: reverseProxyUpstreamQUICKeepAlivePeriod, MaxIdleTimeout: reverseProxyUpstreamIdleTimeout},
			Dial: func(ctx context.Context, _ string, config *tls.Config, quicConfig *quic.Config) (*quic.Conn, error) {
				addresses, lookupErr := lookup(ctx, "udp", host)
				if lookupErr != nil {
					return nil, lookupErr
				}
				var firstErr error
				for _, address := range addresses {
					conn, dialErr := quic.DialAddr(ctx, net.JoinHostPort(address.String(), port), config, quicConfig)
					if dialErr == nil {
						return conn, nil
					}
					if firstErr == nil {
						firstErr = dialErr
					}
				}
				if firstErr == nil {
					firstErr = errors.New("dns http3 upstream has no usable address")
				}
				return nil, firstErr
			},
		}
		result.client = &http.Client{Transport: transport, Timeout: timeout}
		result.closeFn = func() { _ = transport.Close() }
		return result, nil
	}

	dialer := &net.Dialer{Timeout: reverseProxyUpstreamResponseHeaderTimeout, KeepAlive: reverseProxyUpstreamTCPKeepAlive}
	transport := &http.Transport{
		TLSClientConfig:     tlsConfig,
		DialContext:         reverseProxyDNSHTTPDialContext(dialer, lookup),
		DisableCompression:  true,
		ForceAttemptHTTP2:   true,
		MaxConnsPerHost:     2,
		MaxIdleConns:        2,
		MaxIdleConnsPerHost: 2,
		IdleConnTimeout:     reverseProxyUpstreamIdleTimeout,
	}
	if h2Transport, configureErr := http2.ConfigureTransports(transport); configureErr == nil && h2Transport != nil {
		h2Transport.ReadIdleTimeout = reverseProxyUpstreamHTTP2ReadIdleTimeout
		h2Transport.PingTimeout = reverseProxyUpstreamHTTP2PingTimeout
	}
	result.client = &http.Client{Transport: transport, Timeout: timeout}
	result.closeFn = transport.CloseIdleConnections
	return result, nil
}

func reverseProxyDNSHTTPUpstreamTLSConfig(address *url.URL, opts *dnsupstream.Options, http3Only bool) *tls.Config {
	minVersion := uint16(tls.VersionTLS12)
	nextProtos := []string{"h2", "http/1.1"}
	if http3Only {
		minVersion = tls.VersionTLS13
		nextProtos = []string{"h3"}
	}
	return &tls.Config{
		ServerName:            address.Hostname(),
		RootCAs:               opts.RootCAs,
		CipherSuites:          opts.CipherSuites,
		MinVersion:            minVersion,
		NextProtos:            nextProtos,
		InsecureSkipVerify:    opts.InsecureSkipVerify,
		VerifyPeerCertificate: opts.VerifyServerCertificate,
		VerifyConnection:      opts.VerifyConnection,
	}
}

type reverseProxyDNSHTTPLookup func(context.Context, string, string) ([]netip.Addr, error)

func reverseProxyDNSHTTPLookupFunc(opts *dnsupstream.Options) reverseProxyDNSHTTPLookup {
	return func(ctx context.Context, network string, host string) ([]netip.Addr, error) {
		if parsed, err := netip.ParseAddr(strings.Trim(host, "[]")); err == nil {
			return []netip.Addr{parsed}, nil
		}
		resolver := opts.Bootstrap
		if resolver == nil {
			resolver = net.DefaultResolver
		}
		addrs, err := resolver.LookupNetIP(ctx, network, host)
		if err != nil {
			return nil, err
		}
		result := make([]netip.Addr, 0, len(addrs))
		for _, addr := range addrs {
			if addr.IsValid() {
				result = append(result, addr)
			}
		}
		if opts.PreferIPv6 {
			sort.SliceStable(result, func(i, j int) bool {
				return result[i].Is6() && result[j].Is4()
			})
		}
		if len(result) == 0 {
			return nil, errors.New("dns http upstream hostname has no usable address")
		}
		return result, nil
	}
}

func reverseProxyDNSHTTPDialContext(dialer *net.Dialer, lookup reverseProxyDNSHTTPLookup) func(context.Context, string, string) (net.Conn, error) {
	return func(ctx context.Context, network string, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, err
		}
		addresses, err := lookup(ctx, network, host)
		if err != nil {
			return nil, err
		}
		var firstErr error
		for _, item := range addresses {
			conn, dialErr := dialer.DialContext(ctx, network, net.JoinHostPort(item.String(), port))
			if dialErr == nil {
				return conn, nil
			}
			if firstErr == nil {
				firstErr = dialErr
			}
		}
		if firstErr == nil {
			firstErr = errors.New("dns http upstream dial failed")
		}
		return nil, firstErr
	}
}

func (u *reverseProxyDNSCompressedHTTPUpstream) Exchange(req *dns.Msg) (*dns.Msg, error) {
	if u == nil || u.client == nil || u.address == nil {
		return nil, errors.New("dns http upstream is unavailable")
	}
	if req == nil {
		return nil, errors.New("dns request is nil")
	}
	originalID := req.Id
	copyReq := req.Copy()
	copyReq.Id = 0
	packed, err := copyReq.Pack()
	if err != nil {
		return nil, fmt.Errorf("packing dns request: %w", err)
	}
	query := u.address.Query()
	query.Set("dns", base64.RawURLEncoding.EncodeToString(packed))
	target := *u.address
	target.RawQuery = query.Encode()
	httpReq, err := http.NewRequest(http.MethodGet, target.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("creating dns http request: %w", err)
	}
	httpReq.Header.Set("Accept", "application/dns-message")
	httpReq.Header.Set("Accept-Encoding", compressionalgorithm.UpstreamAcceptEncoding())
	httpReq.Header.Set("User-Agent", "")
	httpResp, err := u.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("requesting dns http upstream: %w", err)
	}
	defer httpResp.Body.Close()
	if httpResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("dns http upstream returned status %d", httpResp.StatusCode)
	}
	decoder, err := compressionalgorithm.NewDecoder(httpResp.Body, strings.Join(httpResp.Header.Values("Content-Encoding"), ","), reverseProxyDNSMaximumWireBytes+1)
	if err != nil {
		return nil, fmt.Errorf("decoding dns http upstream response: %w", err)
	}
	body, readErr := io.ReadAll(io.LimitReader(decoder, reverseProxyDNSMaximumWireBytes+1))
	_ = decoder.Close()
	if readErr != nil {
		return nil, fmt.Errorf("reading dns http upstream response: %w", readErr)
	}
	if len(body) == 0 || len(body) > reverseProxyDNSMaximumWireBytes {
		return nil, errors.New("dns http upstream response is too large or empty")
	}
	response := &dns.Msg{}
	if err := response.Unpack(body); err != nil {
		return nil, fmt.Errorf("unpacking dns http upstream response: %w", err)
	}
	response.Id = originalID
	return response, nil
}

func (u *reverseProxyDNSCompressedHTTPUpstream) Address() string {
	if u == nil || u.address == nil {
		return "reverse-proxy-dns-http"
	}
	return u.address.Redacted()
}

func (u *reverseProxyDNSCompressedHTTPUpstream) Close() error {
	if u == nil {
		return nil
	}
	u.once.Do(func() {
		if u.closeFn != nil {
			u.closeFn()
		}
	})
	return nil
}

var _ dnsupstream.Upstream = (*reverseProxyDNSCompressedHTTPUpstream)(nil)

// buildReverseProxyDNSCompressedFallbackUpstreamConfig mirrors dnsproxy's
// documented fallback syntax while replacing HTTPS/H3 entries with the
// compression-aware project upstream.  This keeps domain-specific fallback
// routing and exclusion rules intact.
func buildReverseProxyDNSCompressedFallbackUpstreamConfig(lines []string, opts *dnsupstream.Options) (*dnsproxy.UpstreamConfig, error) {
	if opts == nil {
		opts = &dnsupstream.Options{}
	}
	config := &dnsproxy.UpstreamConfig{
		DomainReservedUpstreams:  make(map[string][]dnsupstream.Upstream),
		SpecifiedDomainUpstreams: make(map[string][]dnsupstream.Upstream),
		SubdomainExclusions:      container.NewMapSet[string](),
		Upstreams:                make([]dnsupstream.Upstream, 0),
	}
	subdomainUpstreams := make(map[string][]dnsupstream.Upstream)
	upstreamIndex := make(map[string]dnsupstream.Upstream)
	closeConfig := func() {
		_ = config.Close()
	}

	for index, rawLine := range lines {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		addresses, domains, err := reverseProxyDNSSplitFallbackLine(line)
		if err != nil {
			closeConfig()
			return nil, fmt.Errorf("line %d: %w", index, err)
		}
		if len(addresses) == 0 {
			closeConfig()
			return nil, fmt.Errorf("line %d: no upstream address", index)
		}
		if len(addresses) == 1 && addresses[0] == "#" && len(domains) > 0 {
			for _, domain := range domains {
				if strings.HasPrefix(domain, "*.") {
					host := strings.TrimPrefix(domain, "*.")
					config.SubdomainExclusions.Add(host)
					subdomainUpstreams[host] = nil
					config.DomainReservedUpstreams[host] = nil
					continue
				}
				config.DomainReservedUpstreams[domain] = nil
				config.SpecifiedDomainUpstreams[domain] = nil
			}
			continue
		}

		for _, rawAddress := range addresses {
			address := strings.TrimSpace(rawAddress)
			if address == "" {
				continue
			}
			upstream := upstreamIndex[address]
			if upstream == nil {
				upstream, err = reverseProxyDNSAddressToUpstream(address, opts)
				if err != nil {
					closeConfig()
					return nil, fmt.Errorf("line %d upstream %q: %w", index, address, err)
				}
				upstreamIndex[address] = upstream
			}
			if len(domains) == 0 {
				config.Upstreams = append(config.Upstreams, upstream)
				continue
			}
			for _, domain := range domains {
				if strings.HasPrefix(domain, "*.") {
					host := strings.TrimPrefix(domain, "*.")
					config.SubdomainExclusions.Add(host)
					subdomainUpstreams[host] = append(subdomainUpstreams[host], upstream)
					config.DomainReservedUpstreams[host] = append(config.DomainReservedUpstreams[host], upstream)
					continue
				}
				config.SpecifiedDomainUpstreams[domain] = append(config.SpecifiedDomainUpstreams[domain], upstream)
				config.DomainReservedUpstreams[domain] = append(config.DomainReservedUpstreams[domain], upstream)
			}
		}
	}
	for host, upstreams := range subdomainUpstreams {
		config.DomainReservedUpstreams[host] = upstreams
	}
	if len(config.Upstreams) == 0 && len(config.DomainReservedUpstreams) == 0 && len(config.SpecifiedDomainUpstreams) == 0 {
		closeConfig()
		return nil, errors.New("no usable fallback upstreams")
	}
	return config, nil
}

func reverseProxyDNSAddressToUpstream(address string, opts *dnsupstream.Options) (dnsupstream.Upstream, error) {
	parsed, err := url.Parse(address)
	if err == nil && parsed != nil {
		switch strings.ToLower(parsed.Scheme) {
		case "https":
			return newReverseProxyDNSCompressedHTTPUpstream(address, opts.Clone(), false)
		case "h3":
			return newReverseProxyDNSCompressedHTTPUpstream(address, opts.Clone(), true)
		}
	}
	return dnsupstream.AddressToUpstream(address, opts.Clone())
}

func reverseProxyDNSSplitFallbackLine(line string) (addresses []string, domains []string, err error) {
	if !strings.HasPrefix(line, "[/") {
		return strings.Fields(line), nil, nil
	}
	domainLine, addressLine, found := strings.Cut(line[len("[/"):], "/]")
	if !found || strings.TrimSpace(addressLine) == "" {
		return nil, nil, errors.New("wrong upstream format")
	}
	for index, rawDomain := range strings.Split(domainLine, "/") {
		if rawDomain == "" {
			domains = append(domains, dnsproxy.UnqualifiedNames)
			continue
		}
		domain := strings.ToLower(strings.TrimSpace(rawDomain))
		if err := aghnetutil.ValidateDomainName(strings.TrimPrefix(domain, "*.")); err != nil {
			return nil, nil, fmt.Errorf("domain at index %d: %w", index, err)
		}
		domains = append(domains, domain+".")
	}
	return strings.Fields(addressLine), domains, nil
}
