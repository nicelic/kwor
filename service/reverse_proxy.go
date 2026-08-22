package service

import (
	"bytes"
	"container/list"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"mime"
	"net"
	"net/http"
	"net/http/httputil"
	"net/netip"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	compressionalgorithm "github.com/alireza0/s-ui/compression/Compression-algorithm"
	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
	"github.com/alireza0/s-ui/logger"
	"github.com/alireza0/s-ui/network"
	"github.com/alireza0/s-ui/util/common"
	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
	"golang.org/x/net/http2"
	"gorm.io/gorm"
)

const (
	reverseProxyDisplayIDMin uint64 = 1
	reverseProxyDisplayIDMax uint64 = 1000000

	reverseProxyProtocolHTTP  = "http"
	reverseProxyProtocolHTTPS = "https"
	reverseProxyProtocolDNS   = "dns"

	reverseProxyDNSProtocolDoH   = "dns_doh"
	reverseProxyDNSProtocolDoHH3 = "dns_doh3"
	reverseProxyDNSProtocolDoQ   = "dns_doq"
	reverseProxyDNSProtocolDoT   = "dns_dot"
	reverseProxyDNSProtocolUDP   = "dns_udp"
	reverseProxyDNSProtocolTCP   = "dns_tcp"

	reverseProxyIPStrategyIPv4Only   = "ipv4_only"
	reverseProxyIPStrategyIPv6Only   = "ipv6_only"
	reverseProxyIPStrategyPreferIPv4 = "prefer_ipv4"
	reverseProxyIPStrategyPreferIPv6 = "prefer_ipv6"

	reverseProxyListenHTTPVersionH2H3   = "h2_h3"
	reverseProxyListenHTTPVersionH2Only = "h2_only"
	reverseProxyListenHTTPVersionH3Only = "h3_only"

	reverseProxyHTTPVersionH2Only               = "h2_only"
	reverseProxyHTTPVersionH3Only               = "h3_only"
	reverseProxyHTTPVersionPreferH2             = "prefer_h2"
	reverseProxyHTTPVersionPreferH3             = "prefer_h3"
	reverseProxyHTTPVersionDualRequiredPreferH3 = "dual_required_prefer_h3"

	reverseProxyMismatchFreeLimit             = 5
	reverseProxyMismatchDelay                 = 30 * time.Second
	reverseProxyMismatchCooldown              = 5 * time.Minute
	reverseProxyMismatchMaxEntries            = 16384
	reverseProxyRuntimeTableTTL               = 5 * time.Minute
	reverseProxyRuntimeTableMaxEntries        = 16384
	reverseProxyDialFallbackGap               = 20 * time.Millisecond
	reverseProxyReadHeaderTimeout             = 15 * time.Second
	reverseProxyServerIdleTimeout             = 10 * time.Minute
	reverseProxyRequestTimeout                = 120 * time.Second
	reverseProxyShutdownTimeout               = 5 * time.Second
	reverseProxyRuntimeRetryBaseDelay         = time.Second
	reverseProxyRuntimeRetryMaxDelay          = time.Minute
	reverseProxyAltSvcMaxAgeSeconds           = 300
	reverseProxyUpstreamResolveCacheTTL       = time.Minute
	reverseProxyUpstreamIdleTimeout           = 10 * time.Minute
	reverseProxyUpstreamTCPKeepAlive          = 30 * time.Second
	reverseProxyUpstreamHTTP2ReadIdleTimeout  = 30 * time.Second
	reverseProxyUpstreamHTTP2PingTimeout      = 15 * time.Second
	reverseProxyUpstreamQUICKeepAlivePeriod   = 30 * time.Second
	reverseProxyServerReadTimeout             = 120 * time.Second
	reverseProxyUpstreamResponseHeaderTimeout = 30 * time.Second
	reverseProxyMaxHeaderBytes                = 1 * 1024 * 1024
	reverseProxyDelayReasonMismatch           = "url_mismatch_penalty"

	reverseProxyUpstreamModeHTTP    = "http"
	reverseProxyUpstreamModeHTTPS   = "https"
	reverseProxyUpstreamModeHTTPSH2 = "https_h2"
	reverseProxyUpstreamModeHTTPSH3 = "https_h3"

	reverseProxyEDNSModeAuto   = "auto"
	reverseProxyEDNSModeCustom = "custom"

	reverseProxyEDNSClientSubnetPolicyClientIP            = "client_ip"
	reverseProxyEDNSClientSubnetPolicyPreferRequestPublic = "prefer_request_public"

	reverseProxyDNSDefaultUpstreamTimeoutSeconds = 12
	reverseProxyDNSMaxUpstreamTimeoutSeconds     = 120
	reverseProxyDNSDefaultCacheSizeBytes         = 4 * 1024 * 1024
	reverseProxyDNSMaxCacheTTLSeconds            = 4294967295
	reverseProxyDNSDefaultRateLimitQPS           = 50
	reverseProxyDNSDefaultMaxConcurrentQueries   = 128
	reverseProxyDNSMaxRateLimitQPS               = 10000
	reverseProxyDNSMaxConcurrentQueryLimit       = 4096
	reverseProxyDefaultMaxConcurrentRequests     = 128
	reverseProxyMaxConcurrentRequestLimit        = 10000
)

var reverseProxyHostTokenRe = regexp.MustCompile(`^[A-Za-z0-9\.\-:\[\]]+$`)

type ReverseProxyService struct {
	CertificateInventoryService
}

type ReverseProxyRulePayload struct {
	ExpectedRevision            *uint64  `json:"expectedRevision"`
	ID                          uint     `json:"id"`
	Name                        string   `json:"name"`
	Enabled                     bool     `json:"enabled"`
	ListenProtocol              string   `json:"listenProtocol"`
	ListenProtocolAlias         string   `json:"listenProtocolAlias"`
	ListenPort                  int      `json:"listenPort"`
	ListenCompressionEnabled    *bool    `json:"listenCompressionEnabled"`
	ListenCompressionAlgorithms []string `json:"listenCompressionAlgorithms"`
	Hosts                       string   `json:"hosts"`
	PathPrefix                  string   `json:"pathPrefix"`
	ListenDNSPath               string   `json:"listenDnsPath"`
	TargetProtocol              string   `json:"targetProtocol"`
	TargetProtocolAlias         string   `json:"targetProtocolAlias"`
	TargetAddresses             string   `json:"targetAddresses"`
	TargetPort                  int      `json:"targetPort"`
	TargetCompressionEnabled    *bool    `json:"targetCompressionEnabled"`
	TargetCompressionAlgorithms []string `json:"targetCompressionAlgorithms"`
	TargetPath                  string   `json:"targetPath"`
	TargetDNSPath               string   `json:"targetDnsPath"`
	FallbackDNSUpstreams        string   `json:"fallbackDnsUpstreams"`
	DNSUpstreamTimeoutSeconds   *int     `json:"dnsUpstreamTimeoutSeconds"`
	DNSCacheEnabled             bool     `json:"dnsCacheEnabled"`
	DNSCacheSizeBytes           *int     `json:"dnsCacheSizeBytes"`
	DNSCacheMinTTL              int      `json:"dnsCacheMinTtl"`
	DNSCacheMaxTTL              int      `json:"dnsCacheMaxTtl"`
	EDNSEnabled                 bool     `json:"ednsEnabled"`
	EDNSMode                    string   `json:"ednsMode"`
	EDNSCustomIP                string   `json:"ednsCustomIp"`
	EDNSClientSubnetPolicy      string   `json:"ednsClientSubnetPolicy"`
	DisableIPv4Answer           bool     `json:"disableIpv4Answer"`
	DisableIPv6Answer           bool     `json:"disableIpv6Answer"`
	CertificateRecordIDs        []uint   `json:"certificateRecordIds"`
	CertificateRecordID         uint     `json:"certificateRecordId"`
	ListenHTTPVersionStrategy   string   `json:"listenHttpVersionStrategy"`
	IPStrategy                  string   `json:"ipStrategy"`
	HTTPVersionStrategy         string   `json:"httpVersionStrategy"`
	UpstreamTLSVerify           bool     `json:"upstreamTlsVerify"`
	DNSAllowedCIDRs             string   `json:"dnsAllowedCidrs"`
	DNSRateLimitQPS             *int     `json:"dnsRateLimitQps"`
	DNSMaxConcurrentQueries     *int     `json:"dnsMaxConcurrentQueries"`
	MaxConcurrentConnections    *int     `json:"maxConcurrentConnections"`
	MaxConcurrentRequests       *int     `json:"maxConcurrentRequests"`
	UpstreamMaxConnections      *int     `json:"upstreamMaxConnections"`
	UpstreamMaxIdleConnections  *int     `json:"upstreamMaxIdleConnections"`
	MemoryLimitBytes            *int64   `json:"memoryLimitBytes"`
	ApiPassthrough              bool     `json:"apiPassthrough"`
	AdvertiseHTTP3              bool     `json:"advertiseHttp3"`
	Remark                      string   `json:"remark"`

	// These flags preserve JSON omission semantics while keeping direct Go
	// callers compatible with the established bool payload field.
	upstreamTLSVerifySet     bool
	upstreamTLSVerifyDecoded bool
}

type reverseProxyRulePayloadAlias ReverseProxyRulePayload

func (p *ReverseProxyRulePayload) UnmarshalJSON(data []byte) error {
	if p == nil {
		return errors.New("reverse proxy payload is nil")
	}
	var decoded reverseProxyRulePayloadAlias
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*p = ReverseProxyRulePayload(decoded)
	fields := make(map[string]json.RawMessage)
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	p.upstreamTLSVerifyDecoded = true
	_, p.upstreamTLSVerifySet = fields["upstreamTlsVerify"]
	return nil
}

func reverseProxyPayloadCompressionEnabled(value *bool) bool {
	// Omitted fields are legacy payloads and must retain the historical
	// all-algorithm behavior. An explicit false is the new "关闭" choice.
	return value == nil || *value
}

type ReverseProxyRuleReorderPayload struct {
	ExpectedRevision *uint64 `json:"expectedRevision"`
	IDs              []uint  `json:"ids"`
}

type ReverseProxyRuleStatusPayload struct {
	ExpectedRevision *uint64 `json:"expectedRevision"`
	ID               uint    `json:"id"`
	Enabled          bool    `json:"enabled"`
}

type ReverseProxyRuleMovePayload struct {
	ExpectedRevision *uint64 `json:"expectedRevision"`
	ID               uint    `json:"id"`
	Direction        int     `json:"direction"`
}

type ReverseProxyRuleStatusResult struct {
	Revision uint64 `json:"revision"`
	ID       uint   `json:"id"`
	Enabled  bool   `json:"enabled"`
}

type ReverseProxyRuleMoveResult struct {
	Revision   uint64 `json:"revision"`
	ID         uint   `json:"id"`
	AdjacentID uint   `json:"adjacentId"`
}

type ReverseProxyRuleDeletePayload struct {
	ExpectedRevision *uint64 `json:"expectedRevision"`
	ID               uint    `json:"id"`
}

type ReverseProxyCertificateOption struct {
	ID         uint     `json:"id"`
	DisplayID  uint64   `json:"displayId"`
	MainDomain string   `json:"mainDomain"`
	Domains    []string `json:"domains"`
	NotAfter   int64    `json:"notAfter"`
	Status     string   `json:"status"`
}

type ReverseProxyRuleView struct {
	ID                          uint                                       `json:"id"`
	DisplayID                   uint64                                     `json:"displayId"`
	ListOrder                   int64                                      `json:"listOrder"`
	Name                        string                                     `json:"name"`
	Enabled                     bool                                       `json:"enabled"`
	ListenProtocol              string                                     `json:"listenProtocol"`
	ListenProtocolAlias         string                                     `json:"listenProtocolAlias"`
	ListenPort                  int                                        `json:"listenPort"`
	ListenCompressionEnabled    bool                                       `json:"listenCompressionEnabled"`
	ListenCompressionAlgorithms []string                                   `json:"listenCompressionAlgorithms"`
	Hosts                       []string                                   `json:"hosts"`
	PathPrefix                  string                                     `json:"pathPrefix"`
	ListenDNSPath               string                                     `json:"listenDnsPath"`
	TargetProtocol              string                                     `json:"targetProtocol"`
	TargetProtocolAlias         string                                     `json:"targetProtocolAlias"`
	TargetAddresses             []string                                   `json:"targetAddresses"`
	TargetPort                  int                                        `json:"targetPort"`
	TargetCompressionEnabled    bool                                       `json:"targetCompressionEnabled"`
	TargetCompressionAlgorithms []string                                   `json:"targetCompressionAlgorithms"`
	TargetPath                  string                                     `json:"targetPath"`
	TargetDNSPath               string                                     `json:"targetDnsPath"`
	FallbackDNSUpstreams        string                                     `json:"fallbackDnsUpstreams"`
	DNSUpstreamTimeoutSeconds   int                                        `json:"dnsUpstreamTimeoutSeconds"`
	DNSCacheEnabled             bool                                       `json:"dnsCacheEnabled"`
	DNSCacheSizeBytes           int                                        `json:"dnsCacheSizeBytes"`
	DNSCacheMinTTL              int                                        `json:"dnsCacheMinTtl"`
	DNSCacheMaxTTL              int                                        `json:"dnsCacheMaxTtl"`
	DNSAllowedCIDRs             []string                                   `json:"dnsAllowedCidrs"`
	DNSRateLimitQPS             int                                        `json:"dnsRateLimitQps"`
	DNSMaxConcurrentQueries     int                                        `json:"dnsMaxConcurrentQueries"`
	EDNSEnabled                 bool                                       `json:"ednsEnabled"`
	EDNSMode                    string                                     `json:"ednsMode"`
	EDNSCustomIP                string                                     `json:"ednsCustomIp"`
	EDNSClientSubnetPolicy      string                                     `json:"ednsClientSubnetPolicy"`
	DisableIPv4Answer           bool                                       `json:"disableIpv4Answer"`
	DisableIPv6Answer           bool                                       `json:"disableIpv6Answer"`
	CertificateRecordIDs        []uint                                     `json:"certificateRecordIds"`
	CertificateRecordID         uint                                       `json:"certificateRecordId"`
	CertificateLabel            string                                     `json:"certificateLabel"`
	CertificateLabels           []string                                   `json:"certificateLabels"`
	ListenHTTPVersionStrategy   string                                     `json:"listenHttpVersionStrategy"`
	IPStrategy                  string                                     `json:"ipStrategy"`
	HTTPVersionStrategy         string                                     `json:"httpVersionStrategy"`
	UpstreamTLSVerify           bool                                       `json:"upstreamTlsVerify"`
	MaxConcurrentConnections    int                                        `json:"maxConcurrentConnections"`
	MaxConcurrentRequests       int                                        `json:"maxConcurrentRequests"`
	UpstreamMaxConnections      int                                        `json:"upstreamMaxConnections"`
	UpstreamMaxIdleConnections  int                                        `json:"upstreamMaxIdleConnections"`
	MemoryLimitBytes            int64                                      `json:"memoryLimitBytes"`
	ApiPassthrough              bool                                       `json:"apiPassthrough"`
	AdvertiseHTTP3              bool                                       `json:"advertiseHttp3"`
	Remark                      string                                     `json:"remark"`
	LastError                   string                                     `json:"lastError"`
	RuntimeStatus               string                                     `json:"runtimeStatus"`
	LocalConnectionCount        int                                        `json:"localConnectionCount"`
	UpstreamConnectionCount     int                                        `json:"upstreamConnectionCount"`
	CertificateHints            []string                                   `json:"certificateHints,omitempty"`
	CertificateBalance          []ReverseProxyCertificateBalanceDiagnostic `json:"certificateBalance,omitempty"`
	UpdatedAt                   int64                                      `json:"updatedAt"`
	CreatedAt                   int64                                      `json:"createdAt"`
}

type ReverseProxyOverview struct {
	Revision         uint64                          `json:"revision"`
	ResourceSettings ReverseProxyResourceSettings    `json:"resourceSettings"`
	Available        bool                            `json:"available"`
	Started          bool                            `json:"started"`
	ListenerCount    int                             `json:"listenerCount"`
	EnabledCount     int                             `json:"enabledCount"`
	RuleCount        int                             `json:"ruleCount"`
	CertificateCount int                             `json:"certificateCount"`
	LastSyncAt       int64                           `json:"lastSyncAt"`
	Certificates     []ReverseProxyCertificateOption `json:"certificates"`
	Rules            []ReverseProxyRuleView          `json:"rules"`
	Warnings         []string                        `json:"warnings,omitempty"`
	Error            string                          `json:"error,omitempty"`
}

type reverseProxyLoadedConfiguration struct {
	Settings model.ReverseProxySettings
	Rules    []model.ReverseProxyRule
}

type reverseProxyNormalizedRule struct {
	id                          uint
	name                        string
	enabled                     bool
	listenProtocol              string
	listenProtocolAlias         string
	listenPort                  int
	listenCompressionEnabled    bool
	listenCompressionAlgorithms []string
	hosts                       []string
	pathPrefix                  string
	listenDNSPath               string
	targetProtocol              string
	targetProtocolAlias         string
	targetAddresses             []string
	targetPort                  int
	targetCompressionEnabled    bool
	targetCompressionAlgorithms []string
	targetPath                  string
	targetDNSPath               string
	fallbackDNSUpstreams        string
	dnsUpstreamTimeoutSeconds   int
	dnsCacheEnabled             bool
	dnsCacheSizeBytes           int
	dnsCacheMinTTL              int
	dnsCacheMaxTTL              int
	dnsAllowedCIDRs             []string
	dnsRateLimitQPS             int
	dnsMaxConcurrentQueries     int
	ednsEnabled                 bool
	ednsMode                    string
	ednsCustomIP                string
	ednsClientSubnetPolicy      string
	disableIPv4Answer           bool
	disableIPv6Answer           bool
	certificateRecordIDs        []uint
	certificateRecordID         uint
	listenHTTPVersionStrategy   string
	ipStrategy                  string
	httpVersionStrategy         string
	upstreamTLSVerify           bool
	maxConcurrentConnections    int
	maxConcurrentRequests       int
	upstreamMaxConnections      int
	upstreamMaxIdleConnections  int
	memoryLimitBytes            int64
	apiPassthrough              bool
	advertiseHTTP3              bool
	remark                      string
}

type reverseProxyMismatchEntry struct {
	Count        int
	LastAttempt  time.Time
	DelayedUntil time.Time
	LastReason   string
	element      *list.Element
}

const reverseProxyRuntimeTableShardCount = 16

const (
	reverseProxyTLSClientDomain = "domain"
	reverseProxyTLSClientIP     = "ip_sni"
	reverseProxyTLSClientNoSNI  = "no_sni"
)

type reverseProxyMismatchShard struct {
	mu      sync.Mutex
	entries map[string]*reverseProxyMismatchEntry
	lru     *list.List
}

type reverseProxyTargetCandidate struct {
	address    string
	serverName string
	hostHeader string
	family     string
}

func reverseProxyIsWebSocketAlias(alias string) bool {
	switch strings.ToLower(strings.TrimSpace(alias)) {
	case "ws", "wss":
		return true
	default:
		return false
	}
}

func reverseProxyIsHTTP3WebSocketRequest(r *http.Request) bool {
	return r != nil &&
		r.ProtoMajor == 3 &&
		r.Method == http.MethodConnect &&
		strings.EqualFold(strings.TrimSpace(r.Proto), "websocket")
}

func reverseProxyIsWebSocketUpgradeRequest(r *http.Request) bool {
	if r == nil || r.ProtoMajor != 1 || !strings.EqualFold(strings.TrimSpace(r.Header.Get("Upgrade")), "websocket") {
		return false
	}
	for _, value := range strings.Split(r.Header.Get("Connection"), ",") {
		if strings.EqualFold(strings.TrimSpace(value), "upgrade") {
			return true
		}
	}
	return false
}

type reverseProxyListenBind struct {
	network  string
	listenIP string
	address  string
	optional bool
}

func normalizeReverseProxyEDNSCustomIPv4(raw string) (string, bool) {
	parsedIP := net.ParseIP(strings.TrimSpace(raw))
	if parsedIP == nil {
		return "", false
	}

	ip4 := parsedIP.To4()
	if ip4 == nil {
		return "", false
	}

	return net.IPv4(ip4[0], ip4[1], ip4[2], 1).String(), true
}

type reverseProxyRuntimeState struct {
	lastSyncAt                  time.Time
	lastCertificateBalancePrune time.Time
	lastRenderKey               string
	warnings                    []string
	revision                    uint64
	certificateGeneration       uint64
	nextRetryAt                 time.Time
	retryDelay                  time.Duration
}

type reverseProxyRuleRuntimeState struct {
	status    string
	lastError string
	updatedAt time.Time
}

type reverseProxyRenderRule struct {
	ID                          uint                                 `json:"id"`
	ListOrder                   int64                                `json:"listOrder"`
	Enabled                     bool                                 `json:"enabled"`
	ListenProtocol              string                               `json:"listenProtocol"`
	ListenProtocolAlias         string                               `json:"listenProtocolAlias"`
	ListenHTTPVersionStrategy   string                               `json:"listenHttpVersionStrategy"`
	ListenPort                  int                                  `json:"listenPort"`
	ListenCompressionEnabled    bool                                 `json:"listenCompressionEnabled"`
	ListenCompressionAlgorithms []string                             `json:"listenCompressionAlgorithms"`
	Hosts                       []string                             `json:"hosts"`
	PathPrefix                  string                               `json:"pathPrefix"`
	ListenDNSPath               string                               `json:"listenDnsPath"`
	TargetProtocol              string                               `json:"targetProtocol"`
	TargetProtocolAlias         string                               `json:"targetProtocolAlias"`
	TargetAddresses             []string                             `json:"targetAddresses"`
	TargetPort                  int                                  `json:"targetPort"`
	TargetCompressionEnabled    bool                                 `json:"targetCompressionEnabled"`
	TargetCompressionAlgorithms []string                             `json:"targetCompressionAlgorithms"`
	TargetPath                  string                               `json:"targetPath"`
	TargetDNSPath               string                               `json:"targetDnsPath"`
	AdvertiseHTTP3              bool                                 `json:"advertiseHttp3"`
	EDNSEnabled                 bool                                 `json:"ednsEnabled"`
	EDNSMode                    string                               `json:"ednsMode"`
	EDNSCustomIP                string                               `json:"ednsCustomIp"`
	EDNSClientSubnetPolicy      string                               `json:"ednsClientSubnetPolicy"`
	DisableIPv4Answer           bool                                 `json:"disableIpv4Answer"`
	DisableIPv6Answer           bool                                 `json:"disableIpv6Answer"`
	DNSAllowedCIDRs             []string                             `json:"dnsAllowedCidrs"`
	DNSRateLimitQPS             int                                  `json:"dnsRateLimitQps"`
	DNSMaxConcurrentQueries     int                                  `json:"dnsMaxConcurrentQueries"`
	DNSRuntimeState             string                               `json:"dnsRuntimeState,omitempty"`
	CertificateRecordIDs        []uint                               `json:"certificateRecordIds,omitempty"`
	CertificateStates           []reverseProxyRenderCertificateState `json:"certificateStates,omitempty"`
	IPStrategy                  string                               `json:"ipStrategy"`
	HTTPVersionStrategy         string                               `json:"httpVersionStrategy"`
	UpstreamTLSVerify           bool                                 `json:"upstreamTlsVerify"`
	MaxConcurrentConnections    int                                  `json:"maxConcurrentConnections"`
	MaxConcurrentRequests       int                                  `json:"maxConcurrentRequests"`
	UpstreamMaxConnections      int                                  `json:"upstreamMaxConnections"`
	UpstreamMaxIdleConnections  int                                  `json:"upstreamMaxIdleConnections"`
	MemoryLimitBytes            int64                                `json:"memoryLimitBytes"`
	ApiPassthrough              bool                                 `json:"apiPassthrough"`
}

type reverseProxyRenderCertificateState struct {
	ID          uint   `json:"id"`
	Fingerprint string `json:"fingerprint,omitempty"`
	UpdatedAt   int64  `json:"updatedAt,omitempty"`
}

type reverseProxyRuleCertificateBinding struct {
	RuleID              uint
	CertificateRecordID uint
	Certificate         *tls.Certificate
	Leaf                *x509LeafState
}

type reverseProxyParsedCertificateMaterial struct {
	Certificate *tls.Certificate
	Leaf        *x509LeafState
	Err         error
}

var reverseProxyParsedCertificateMaterials = struct {
	sync.RWMutex
	databaseGeneration uint64
	generation         uint64
	items              map[uint]reverseProxyParsedCertificateMaterial
}{items: make(map[uint]reverseProxyParsedCertificateMaterial)}

var reverseProxyDatabaseGeneration atomic.Uint64

func init() {
	database.RegisterDBResetHook(func() {
		databaseGeneration := reverseProxyDatabaseGeneration.Add(1)
		reverseProxyParsedCertificateMaterials.Lock()
		reverseProxyParsedCertificateMaterials.databaseGeneration = databaseGeneration
		reverseProxyParsedCertificateMaterials.generation = 0
		reverseProxyParsedCertificateMaterials.items = make(map[uint]reverseProxyParsedCertificateMaterial)
		reverseProxyParsedCertificateMaterials.Unlock()
		noteReverseProxyCertificateInventoryChanged()
	})
}

type reverseProxyCertificateSelection struct {
	ListenerKey         string
	SNIBucket           string
	CertificateRecordID uint
	ClientKind          string
	RequestedIP         string
}

type reverseProxyLocalConnectionState struct {
	RuleConnectionLimiters map[uint]*reverseProxyAdjustableLimiter
	HijackedRuleID         uint
	Selection              reverseProxyCertificateSelection
	HasSelection           bool
}

type reverseProxyPendingCertificateSelection struct {
	Selection reverseProxyCertificateSelection
	CreatedAt time.Time
	element   *list.Element
}

// Pending TLS selections bridge the short interval between certificate choice
// and connection registration.  They are partitioned by connection address
// so a burst of unrelated handshakes cannot turn expiry or eviction into one
// listener-wide LRU scan.
type reverseProxyPendingCertificateShard struct {
	selections map[string]*reverseProxyPendingCertificateSelection
	lru        *list.List
}

const reverseProxyPendingCertificateShardLimit = reverseProxyRuntimeTableMaxEntries / reverseProxyRuntimeTableShardCount

type reverseProxyListenerGroup struct {
	mu                         sync.RWMutex
	statsMu                    sync.Mutex
	closed                     bool
	key                        string
	renderKey                  string
	listenIP                   string
	listenIPs                  []string
	listenPort                 int
	protocol                   string
	socketKind                 string
	listenHTTPVersionStrategy  string
	h3ListenerAvailable        bool
	h3AvailabilityKnown        bool
	h3ServingCount             int
	server                     *http.Server
	listener                   net.Listener
	h3Server                   *http3.Server
	packetConn                 net.PacketConn
	servers                    []*http.Server
	listeners                  []net.Listener
	h3Servers                  []*http3.Server
	packetConns                []net.PacketConn
	rules                      []*model.ReverseProxyRule
	ruleMatchData              map[uint]reverseProxyRuleMatchData
	service                    *ReverseProxyService
	certBindingsByRule         map[uint][]*reverseProxyRuleCertificateBinding
	orderedCertBindings        []*reverseProxyRuleCertificateBinding
	ipCertBindings             map[string][]*reverseProxyRuleCertificateBinding
	ipCertificateUniverse      map[string]struct{}
	natFallbackCertBindings    map[string][]*reverseProxyRuleCertificateBinding
	dnsHandler                 *reverseProxyDNSRuleHandler
	warnings                   []string
	upstreamByRule             map[uint]*reverseProxyCachedUpstream
	connectionCounts           map[uint]reverseProxyConnectionCounts
	localConnIDs               map[net.Conn]string
	localConnByID              map[string]net.Conn
	localConnStates            map[string]reverseProxyLocalConnectionState
	localConnAddrToID          map[string]string
	localConnAddrByID          map[string]string
	hijackedConnections        map[string]net.Conn
	pendingConnSelectionShards [reverseProxyRuntimeTableShardCount]reverseProxyPendingCertificateShard
	connectionSlotIDs          map[string]struct{}
	certificateBalanceShards   [reverseProxyRuntimeTableShardCount]reverseProxyCertificateBalanceShard
	resources                  ReverseProxyResourceSettings
	listenerConnectionLimiter  *reverseProxyAdjustableLimiter
	ruleConnectionLimiters     map[uint]*reverseProxyAdjustableLimiter
	requestLimiters            map[uint]*reverseProxyAdjustableLimiter
	upstreamLimiters           map[uint]*reverseProxyAdjustableLimiter
	nextConnID                 uint64
}

type reverseProxyRuleMatchData struct {
	serverNames []string
	listenAlias string
	pathPrefix  string
	dnsPath     string
}

type reverseProxyCertificateHint struct {
	ruleID   uint
	messages []string
}

type reverseProxyRuntimeManager struct {
	mu                  sync.Mutex
	groups              map[string]*reverseProxyListenerGroup
	mismatchShards      [reverseProxyRuntimeTableShardCount]reverseProxyMismatchShard
	ruleStateMu         sync.RWMutex
	ruleStates          map[uint]reverseProxyRuleRuntimeState
	state               reverseProxyRuntimeState
	reconcileError      string
	overviewMu          sync.RWMutex
	configuration       *ReverseProxyOverview
	loadedConfiguration *reverseProxyLoadedConfiguration
}

type x509LeafState struct {
	Certificate *tls.Certificate
	Leaf        *x509.Certificate
	Fingerprint string
	NotAfter    time.Time
	HasIPSAN    bool
}

type reverseProxyTransportBundle struct {
	RoundTripper http.RoundTripper
	Cleanup      func()
}

type reverseProxyCachedUpstream struct {
	ResolvedAddress string
	ServerName      string
	HostHeader      string
	TransportMode   string
	ResolvedAt      time.Time
	RoundTripper    http.RoundTripper
	Cleanup         func()
	refs            int
	closing         bool
}

type reverseProxyCleanupReadCloser struct {
	io.ReadCloser
	onClose func()
	once    sync.Once
}

func (c *reverseProxyCleanupReadCloser) Close() error {
	err := c.ReadCloser.Close()
	c.once.Do(func() {
		if c.onClose != nil {
			c.onClose()
		}
	})
	return err
}

type reverseProxyCleanupReadWriteCloser struct {
	io.ReadWriteCloser
	onClose func()
	once    sync.Once
}

func (c *reverseProxyCleanupReadWriteCloser) Close() error {
	err := c.ReadWriteCloser.Close()
	c.once.Do(func() {
		if c.onClose != nil {
			c.onClose()
		}
	})
	return err
}

type reverseProxyResponseHeaderTimeoutTransport struct {
	base    http.RoundTripper
	timeout time.Duration
}

func (t reverseProxyResponseHeaderTimeoutTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if t.base == nil {
		return nil, errors.New("upstream transport is nil")
	}
	if req == nil || t.timeout <= 0 {
		return t.base.RoundTrip(req)
	}
	ctx, cancel := context.WithCancel(req.Context())
	var stateMu sync.Mutex
	timedOut := false
	timer := time.AfterFunc(t.timeout, func() {
		stateMu.Lock()
		timedOut = true
		stateMu.Unlock()
		cancel()
	})
	response, err := t.base.RoundTrip(req.WithContext(ctx))
	timer.Stop()
	if err != nil {
		cancel()
		stateMu.Lock()
		didTimeout := timedOut
		stateMu.Unlock()
		if didTimeout {
			return nil, fmt.Errorf("upstream response header timeout: %w", context.DeadlineExceeded)
		}
		return nil, err
	}
	stateMu.Lock()
	didTimeout := timedOut
	stateMu.Unlock()
	if didTimeout {
		cancel()
		if response != nil && response.Body != nil {
			_ = response.Body.Close()
		}
		return nil, fmt.Errorf("upstream response header timeout: %w", context.DeadlineExceeded)
	}
	if response == nil || response.Body == nil {
		cancel()
		return response, nil
	}
	if responseBody, ok := response.Body.(io.ReadWriteCloser); ok {
		response.Body = &reverseProxyCleanupReadWriteCloser{ReadWriteCloser: responseBody, onClose: cancel}
	} else {
		response.Body = &reverseProxyCleanupReadCloser{ReadCloser: response.Body, onClose: cancel}
	}
	return response, nil
}

func reverseProxyWithResponseHeaderTimeout(base http.RoundTripper) http.RoundTripper {
	return reverseProxyResponseHeaderTimeoutTransport{
		base:    base,
		timeout: reverseProxyUpstreamResponseHeaderTimeout,
	}
}

type reverseProxyConnectionCounts struct {
	LocalOpen    int
	UpstreamOpen int
}

type reverseProxyConnContextKey struct{}

type reverseProxyCountedConn struct {
	net.Conn
	onClose func()
	once    sync.Once
}

// reverseProxyTrackedClientConn keeps the connection lifecycle visible after
// net/http marks a WebSocket connection as StateHijacked.  The standard
// ConnState callback does not receive StateClosed for a hijacked tunnel, so
// relying on it alone leaked connection counters and TLS balance leases.
type reverseProxyTrackedClientConn struct {
	net.Conn
	onClose func()
	once    sync.Once
}

type reverseProxyClientHelloLocalHintConn struct {
	local net.Addr
}

func (c *reverseProxyClientHelloLocalHintConn) Read([]byte) (int, error)         { return 0, net.ErrClosed }
func (c *reverseProxyClientHelloLocalHintConn) Write([]byte) (int, error)        { return 0, net.ErrClosed }
func (c *reverseProxyClientHelloLocalHintConn) Close() error                     { return nil }
func (c *reverseProxyClientHelloLocalHintConn) LocalAddr() net.Addr              { return c.local }
func (c *reverseProxyClientHelloLocalHintConn) RemoteAddr() net.Addr             { return &net.UDPAddr{} }
func (c *reverseProxyClientHelloLocalHintConn) SetDeadline(time.Time) error      { return nil }
func (c *reverseProxyClientHelloLocalHintConn) SetReadDeadline(time.Time) error  { return nil }
func (c *reverseProxyClientHelloLocalHintConn) SetWriteDeadline(time.Time) error { return nil }

func reverseProxyClientHelloWithLocalIPHint(hello *tls.ClientHelloInfo, listenIP string) *tls.ClientHelloInfo {
	if hello == nil || hello.Conn != nil {
		return hello
	}
	ip := net.ParseIP(strings.Trim(strings.TrimSpace(listenIP), "[]"))
	if ip == nil {
		return hello
	}
	copyHello := *hello
	copyHello.Conn = &reverseProxyClientHelloLocalHintConn{local: &net.UDPAddr{IP: ip}}
	return &copyHello
}

func (c *reverseProxyTrackedClientConn) Close() error {
	err := c.Conn.Close()
	c.once.Do(func() {
		if c.onClose != nil {
			c.onClose()
		}
	})
	return err
}

type reverseProxyTrackedClientListener struct {
	net.Listener
	onClose func(net.Conn)
}

func (l *reverseProxyTrackedClientListener) Accept() (net.Conn, error) {
	conn, err := l.Listener.Accept()
	if err != nil || conn == nil {
		return conn, err
	}
	return &reverseProxyTrackedClientConn{
		Conn: conn,
		onClose: func() {
			if l.onClose != nil {
				l.onClose(conn)
			}
		},
	}, nil
}

func (c *reverseProxyCountedConn) Close() error {
	err := c.Conn.Close()
	c.once.Do(func() {
		if c.onClose != nil {
			c.onClose()
		}
	})
	return err
}

type reverseProxyStringReplacement struct {
	Old string
	New string
}

type reverseProxyResponseRewritePlan struct {
	Enabled              bool
	Replacements         []reverseProxyStringReplacement
	UpstreamCookieDomain string
	ExternalCookieDomain string
	UpstreamPathPrefix   string
	ExternalPathPrefix   string
}

var reverseProxyRuntime = &reverseProxyRuntimeManager{
	groups:     make(map[string]*reverseProxyListenerGroup),
	ruleStates: make(map[uint]reverseProxyRuleRuntimeState),
}

var reverseProxyCertificateGeneration atomic.Uint64

func currentReverseProxyCertificateGeneration() uint64 {
	return reverseProxyCertificateGeneration.Load()
}

func noteReverseProxyCertificateInventoryChanged() uint64 {
	generation := reverseProxyCertificateGeneration.Add(1)
	reverseProxyRuntime.overviewMu.Lock()
	reverseProxyRuntime.configuration = nil
	reverseProxyRuntime.overviewMu.Unlock()
	// Certificate changes are already committed before this notification. Wake
	// the independent monitor so listener bindings do not wait for its 30s tick.
	WakeReverseProxyRuntime()
	return generation
}

func (r *reverseProxyRuntimeManager) reportRuleState(ruleID uint, status string, lastError string) {
	if r == nil || ruleID == 0 {
		return
	}
	status = strings.TrimSpace(status)
	lastError = strings.TrimSpace(lastError)
	if status == "running" {
		lastError = ""
	}
	r.ruleStateMu.Lock()
	if r.ruleStates == nil {
		r.ruleStates = make(map[uint]reverseProxyRuleRuntimeState)
	}
	if current, exists := r.ruleStates[ruleID]; exists && current.status == status && current.lastError == lastError {
		r.ruleStateMu.Unlock()
		return
	}
	r.ruleStates[ruleID] = reverseProxyRuleRuntimeState{status: status, lastError: lastError, updatedAt: time.Now()}
	r.ruleStateMu.Unlock()
}

func (r *reverseProxyRuntimeManager) clearRuleState(ruleID uint) {
	if r == nil || ruleID == 0 {
		return
	}
	r.ruleStateMu.Lock()
	delete(r.ruleStates, ruleID)
	r.ruleStateMu.Unlock()
}

func (r *reverseProxyRuntimeManager) resetRuleStates() {
	if r == nil {
		return
	}
	r.ruleStateMu.Lock()
	r.ruleStates = make(map[uint]reverseProxyRuleRuntimeState)
	r.ruleStateMu.Unlock()
}

func (r *reverseProxyRuntimeManager) reconcileRuleStates(rows []model.ReverseProxyRule) {
	if r == nil {
		return
	}
	valid := make(map[uint]bool, len(rows))
	for i := range rows {
		if rows[i].Id != 0 {
			valid[rows[i].Id] = rows[i].Enabled
		}
	}
	now := time.Now()
	r.ruleStateMu.Lock()
	if r.ruleStates == nil {
		r.ruleStates = make(map[uint]reverseProxyRuleRuntimeState)
	}
	for ruleID := range r.ruleStates {
		if _, exists := valid[ruleID]; !exists {
			delete(r.ruleStates, ruleID)
		}
	}
	for ruleID, enabled := range valid {
		if !enabled {
			r.ruleStates[ruleID] = reverseProxyRuleRuntimeState{status: "disabled", updatedAt: now}
			continue
		}
		if _, exists := r.ruleStates[ruleID]; !exists {
			r.ruleStates[ruleID] = reverseProxyRuleRuntimeState{status: "pending", updatedAt: now}
		}
	}
	r.ruleStateMu.Unlock()
}

func (r *reverseProxyRuntimeManager) snapshotRuleStates() map[uint]reverseProxyRuleRuntimeState {
	result := make(map[uint]reverseProxyRuleRuntimeState)
	if r == nil {
		return result
	}
	r.ruleStateMu.RLock()
	for ruleID, state := range r.ruleStates {
		result[ruleID] = state
	}
	r.ruleStateMu.RUnlock()
	return result
}

func reverseProxySupported() bool {
	return true
}

func reverseProxyListenerCount(groups map[string]*reverseProxyListenerGroup) int {
	count := 0
	for _, group := range groups {
		if group == nil {
			continue
		}
		if len(group.listeners) > 0 || len(group.packetConns) > 0 {
			count += len(group.listeners)
			count += len(group.packetConns)
			continue
		}
		if group.listener != nil {
			count++
		}
		if group.packetConn != nil {
			count++
		}
	}
	return count
}

func reverseProxySnapshotConnectionCounts(groups map[string]*reverseProxyListenerGroup) map[uint]reverseProxyConnectionCounts {
	if len(groups) == 0 {
		return map[uint]reverseProxyConnectionCounts{}
	}
	result := make(map[uint]reverseProxyConnectionCounts)
	for _, group := range groups {
		if group == nil {
			continue
		}
		counts := group.snapshotConnectionCounts()
		for ruleID, item := range counts {
			current := result[ruleID]
			current.LocalOpen += item.LocalOpen
			current.UpstreamOpen += item.UpstreamOpen
			result[ruleID] = current
		}
	}
	return result
}

func (s *ReverseProxyService) GetOverview() (*ReverseProxyOverview, error) {
	configuration, err := s.reverseProxyConfigurationSnapshot()
	if err != nil {
		return nil, err
	}
	runtime := s.reverseProxyRuntimeSnapshot(configuration.Revision)
	overview := cloneReverseProxyOverview(configuration)
	overview.Available = runtime.Available
	overview.Started = runtime.Started
	overview.ListenerCount = runtime.ListenerCount
	overview.LastSyncAt = runtime.LastSyncAt
	overview.Warnings = append([]string(nil), runtime.Warnings...)
	overview.Error = runtime.Error
	runtimeByRule := make(map[uint]ReverseProxyRuntimeRuleStateView, len(runtime.Rules))
	for _, state := range runtime.Rules {
		runtimeByRule[state.ID] = state
	}
	for index := range overview.Rules {
		if state, exists := runtimeByRule[overview.Rules[index].ID]; exists {
			overview.Rules[index].RuntimeStatus = state.RuntimeStatus
			overview.Rules[index].LastError = state.LastError
			overview.Rules[index].LocalConnectionCount = state.LocalConnectionCount
			overview.Rules[index].UpstreamConnectionCount = state.UpstreamConnectionCount
		}
	}
	return overview, nil
}

// GetRuntimeOverview never reads rules, certificate material, or the complete
// SQLite configuration. It is the endpoint used by the successful 10-second
// UI runtime poll.
func (s *ReverseProxyService) GetRuntimeOverview() (*ReverseProxyRuntimeOverview, error) {
	revision := uint64(0)
	reverseProxyRuntime.overviewMu.RLock()
	if reverseProxyRuntime.configuration != nil {
		revision = reverseProxyRuntime.configuration.Revision
	}
	reverseProxyRuntime.overviewMu.RUnlock()
	if revision == 0 {
		reverseProxyRuntime.mu.Lock()
		revision = reverseProxyRuntime.state.revision
		reverseProxyRuntime.mu.Unlock()
	}
	runtime := s.reverseProxyRuntimeSnapshot(revision)
	return &runtime, nil
}

func reverseProxyRulePayloadFromModel(row *model.ReverseProxyRule, enabled bool) ReverseProxyRulePayload {
	if row == nil {
		return ReverseProxyRulePayload{}
	}
	dnsTimeout := row.DNSUpstreamTimeoutSeconds
	dnsCacheSize := row.DNSCacheSizeBytes
	dnsRateLimit := row.DNSRateLimitQPS
	dnsConcurrent := row.DNSMaxConcurrentQueries
	maxConnections := row.MaxConcurrentConnections
	maxRequests := row.MaxConcurrentRequests
	upstreamConnections := row.UpstreamMaxConnections
	upstreamIdleConnections := row.UpstreamMaxIdleConnections
	memoryLimit := row.MemoryLimitBytes
	listenCompressionEnabled, listenCompressionAlgorithms := reverseProxyCompressionSettingsFromModel(row.ListenCompressionEnabled, row.ListenCompressionAlgorithms)
	targetCompressionEnabled, targetCompressionAlgorithms := reverseProxyCompressionSettingsFromModel(row.TargetCompressionEnabled, row.TargetCompressionAlgorithms)
	payload := ReverseProxyRulePayload{
		ID:                          row.Id,
		Name:                        row.Name,
		Enabled:                     enabled,
		ListenProtocol:              row.ListenProtocol,
		ListenProtocolAlias:         row.ListenProtocolAlias,
		ListenPort:                  row.ListenPort,
		ListenCompressionEnabled:    &listenCompressionEnabled,
		ListenCompressionAlgorithms: listenCompressionAlgorithms,
		Hosts:                       strings.Join(reverseProxyRuleServerNames(row), ", "),
		PathPrefix:                  row.PathPrefix,
		ListenDNSPath:               row.ListenDNSPath,
		TargetProtocol:              row.TargetProtocol,
		TargetProtocolAlias:         row.TargetProtocolAlias,
		TargetAddresses:             strings.Join(decodeReverseProxyList(row.TargetAddresses), ", "),
		TargetPort:                  row.TargetPort,
		TargetCompressionEnabled:    &targetCompressionEnabled,
		TargetCompressionAlgorithms: targetCompressionAlgorithms,
		TargetPath:                  row.TargetPath,
		TargetDNSPath:               row.TargetDNSPath,
		FallbackDNSUpstreams:        row.FallbackDNSUpstreams,
		DNSUpstreamTimeoutSeconds:   &dnsTimeout,
		DNSCacheEnabled:             row.DNSCacheEnabled,
		DNSCacheSizeBytes:           &dnsCacheSize,
		DNSCacheMinTTL:              row.DNSCacheMinTTL,
		DNSCacheMaxTTL:              row.DNSCacheMaxTTL,
		EDNSEnabled:                 row.EDNSEnabled,
		EDNSMode:                    row.EDNSMode,
		EDNSCustomIP:                row.EDNSCustomIP,
		EDNSClientSubnetPolicy:      row.EDNSClientSubnetPolicy,
		DisableIPv4Answer:           row.DisableIPv4Answer,
		DisableIPv6Answer:           row.DisableIPv6Answer,
		CertificateRecordIDs:        reverseProxyRuleCertificateIDs(row),
		CertificateRecordID:         row.CertificateRecordID,
		ListenHTTPVersionStrategy:   row.ListenHTTPVersionStrategy,
		IPStrategy:                  row.IPStrategy,
		HTTPVersionStrategy:         row.HTTPVersionStrategy,
		UpstreamTLSVerify:           row.UpstreamTLSVerify,
		DNSAllowedCIDRs:             strings.Join(decodeReverseProxyList(row.DNSAllowedCIDRs), ", "),
		DNSRateLimitQPS:             &dnsRateLimit,
		DNSMaxConcurrentQueries:     &dnsConcurrent,
		MaxConcurrentConnections:    &maxConnections,
		MaxConcurrentRequests:       &maxRequests,
		UpstreamMaxConnections:      &upstreamConnections,
		UpstreamMaxIdleConnections:  &upstreamIdleConnections,
		MemoryLimitBytes:            &memoryLimit,
		ApiPassthrough:              row.ApiPassthrough,
		AdvertiseHTTP3:              row.AdvertiseHTTP3,
		Remark:                      row.Remark,
		upstreamTLSVerifySet:        true,
		upstreamTLSVerifyDecoded:    true,
	}
	return payload
}

func (s *ReverseProxyService) reverseProxyConfigurationSnapshot() (*ReverseProxyOverview, error) {
	reverseProxyRuntime.overviewMu.RLock()
	existing := reverseProxyRuntime.configuration
	reverseProxyRuntime.overviewMu.RUnlock()
	if existing != nil {
		return cloneReverseProxyOverview(existing), nil
	}
	if err := s.refreshReverseProxyConfigurationSnapshot(); err != nil {
		return nil, err
	}
	reverseProxyRuntime.overviewMu.RLock()
	current := reverseProxyRuntime.configuration
	reverseProxyRuntime.overviewMu.RUnlock()
	if current == nil {
		return nil, common.NewError("reverse proxy configuration snapshot is unavailable")
	}
	return cloneReverseProxyOverview(current), nil
}

func (s *ReverseProxyService) refreshReverseProxyConfigurationSnapshot() error {
	settings, err := s.loadReverseProxySettings()
	if err != nil {
		return err
	}
	rules, err := s.loadRulesLocked(database.GetDB())
	if err != nil {
		return err
	}
	reverseProxyRuntime.mu.Lock()
	reverseProxyRuntime.loadedConfiguration = &reverseProxyLoadedConfiguration{
		Settings: *settings,
		Rules:    append([]model.ReverseProxyRule(nil), rules...),
	}
	reverseProxyRuntime.mu.Unlock()
	return s.publishReverseProxyConfigurationSnapshot(settings, rules)
}

func (s *ReverseProxyService) publishReverseProxyConfigurationSnapshot(settings *model.ReverseProxySettings, rules []model.ReverseProxyRule) error {
	if settings == nil {
		return common.NewError("reverse proxy settings are unavailable")
	}
	certOptions, certMap, err := s.listCertificateOptions()
	if err != nil {
		return err
	}
	views := make([]ReverseProxyRuleView, 0, len(rules))
	enabledCount := 0
	for index := range rules {
		if rules[index].Enabled {
			enabledCount++
		}
		views = append(views, buildReverseProxyRuleView(&rules[index], certMap, reverseProxyConnectionCounts{}, nil))
	}
	resources := reverseProxySettingsView(settings)
	snapshot := &ReverseProxyOverview{
		Revision:         settings.Revision,
		ResourceSettings: resources,
		Available:        reverseProxySupported(),
		EnabledCount:     enabledCount,
		RuleCount:        len(rules),
		CertificateCount: len(certOptions),
		Certificates:     certOptions,
		Rules:            views,
	}
	reverseProxyRuntime.overviewMu.Lock()
	reverseProxyRuntime.configuration = snapshot
	reverseProxyRuntime.overviewMu.Unlock()
	return nil
}

func cloneReverseProxyOverview(source *ReverseProxyOverview) *ReverseProxyOverview {
	if source == nil {
		return nil
	}
	copyOverview := *source
	copyOverview.Warnings = append([]string(nil), source.Warnings...)
	copyOverview.Certificates = make([]ReverseProxyCertificateOption, len(source.Certificates))
	for index := range source.Certificates {
		copyOverview.Certificates[index] = source.Certificates[index]
		copyOverview.Certificates[index].Domains = append([]string(nil), source.Certificates[index].Domains...)
	}
	copyOverview.Rules = make([]ReverseProxyRuleView, len(source.Rules))
	for index := range source.Rules {
		copyOverview.Rules[index] = source.Rules[index]
		copyOverview.Rules[index].Hosts = append([]string(nil), source.Rules[index].Hosts...)
		copyOverview.Rules[index].TargetAddresses = append([]string(nil), source.Rules[index].TargetAddresses...)
		copyOverview.Rules[index].DNSAllowedCIDRs = append([]string(nil), source.Rules[index].DNSAllowedCIDRs...)
		copyOverview.Rules[index].CertificateRecordIDs = append([]uint(nil), source.Rules[index].CertificateRecordIDs...)
		copyOverview.Rules[index].CertificateLabels = append([]string(nil), source.Rules[index].CertificateLabels...)
		copyOverview.Rules[index].CertificateHints = append([]string(nil), source.Rules[index].CertificateHints...)
		copyOverview.Rules[index].CertificateBalance = append([]ReverseProxyCertificateBalanceDiagnostic(nil), source.Rules[index].CertificateBalance...)
	}
	return &copyOverview
}

func cloneReverseProxyLoadedConfiguration(source *reverseProxyLoadedConfiguration) *reverseProxyLoadedConfiguration {
	if source == nil {
		return nil
	}
	clone := &reverseProxyLoadedConfiguration{Settings: source.Settings}
	clone.Rules = append([]model.ReverseProxyRule(nil), source.Rules...)
	return clone
}

func (r *reverseProxyRuntimeManager) loadedConfigurationSnapshot() *reverseProxyLoadedConfiguration {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	loaded := cloneReverseProxyLoadedConfiguration(r.loadedConfiguration)
	r.mu.Unlock()
	return loaded
}

func (s *ReverseProxyService) loadReverseProxyLoadedConfiguration() (*reverseProxyLoadedConfiguration, error) {
	settings, err := s.loadReverseProxySettings()
	if err != nil {
		return nil, err
	}
	rules, err := s.loadRulesLocked(database.GetDB())
	if err != nil {
		return nil, err
	}
	if err := prepareReverseProxyParsedCertificateMaterials(rules); err != nil {
		return nil, err
	}
	return &reverseProxyLoadedConfiguration{
		Settings: *settings,
		Rules:    rules,
	}, nil
}

func (s *ReverseProxyService) reverseProxyRuntimeSnapshot(revision uint64) ReverseProxyRuntimeOverview {
	reverseProxyRuntime.mu.Lock()
	groups := make(map[string]*reverseProxyListenerGroup, len(reverseProxyRuntime.groups))
	for key, group := range reverseProxyRuntime.groups {
		groups[key] = group
	}
	warnings := append([]string(nil), reverseProxyRuntime.state.warnings...)
	runtimeError := reverseProxyRuntime.reconcileError
	lastSyncAt := int64(0)
	if !reverseProxyRuntime.state.lastSyncAt.IsZero() {
		lastSyncAt = reverseProxyRuntime.state.lastSyncAt.Unix()
	}
	reverseProxyRuntime.mu.Unlock()
	connectionCounts := reverseProxySnapshotConnectionCounts(groups)
	runtimeStates := reverseProxyRuntime.snapshotRuleStates()
	views := make([]ReverseProxyRuntimeRuleStateView, 0, len(runtimeStates)+len(connectionCounts))
	ids := make(map[uint]struct{}, len(runtimeStates)+len(connectionCounts))
	for ruleID := range runtimeStates {
		ids[ruleID] = struct{}{}
	}
	for ruleID := range connectionCounts {
		ids[ruleID] = struct{}{}
	}
	for ruleID := range ids {
		state := runtimeStates[ruleID]
		counts := connectionCounts[ruleID]
		views = append(views, ReverseProxyRuntimeRuleStateView{
			ID:                      ruleID,
			RuntimeStatus:           state.status,
			LastError:               state.lastError,
			LocalConnectionCount:    counts.LocalOpen,
			UpstreamConnectionCount: counts.UpstreamOpen,
		})
	}
	sort.Slice(views, func(i, j int) bool { return views[i].ID < views[j].ID })
	listenerCount := reverseProxyListenerCount(groups) + reverseProxyDNSRuntime.listenerCount()
	overview := ReverseProxyRuntimeOverview{
		Revision:      revision,
		Available:     reverseProxySupported(),
		Started:       listenerCount > 0,
		ListenerCount: listenerCount,
		LastSyncAt:    lastSyncAt,
		Rules:         views,
		Resources:     reverseProxyResources.runtimeUsage(),
		Warnings:      warnings,
	}
	if runtimeError != "" {
		overview.Error = runtimeError
	}
	if !overview.Available {
		overview.Error = "reverse proxy runtime is unavailable on this system"
	}
	return overview
}

func (s *ReverseProxyService) UpsertRule(payload ReverseProxyRulePayload) error {
	db := database.GetDB()
	if db == nil {
		return common.NewError("database is not ready")
	}

	normalized, err := s.normalizeRulePayload(payload)
	if err != nil {
		return err
	}
	// Resolution can take seconds and must never hold the single SQLite
	// connection inside a transaction.  Runtime resolution remains a second
	// safety net for targets whose DNS answer changes after this preflight.
	if err := s.preflightNormalizedRule(normalized); err != nil {
		return err
	}

	err = db.Transaction(func(tx *gorm.DB) error {
		settings, err := reverseProxyExpectedRevision(tx, payload.ExpectedRevision)
		if err != nil {
			return err
		}
		existing := &model.ReverseProxyRule{}
		isNew := normalized.id == 0
		if !isNew {
			if err := tx.Where("id = ?", normalized.id).First(existing).Error; err != nil {
				return err
			}
		}
		if err := s.validateNormalizedRule(tx, normalized); err != nil {
			return err
		}

		row := existing
		if isNew {
			row = &model.ReverseProxyRule{}
			nextDisplayID, allocErr := s.allocateNextDisplayIDTx(tx)
			if allocErr != nil {
				return allocErr
			}
			row.DisplayID = nextDisplayID
			maxOrder, orderErr := s.nextListOrderTx(tx)
			if orderErr != nil {
				return orderErr
			}
			row.ListOrder = maxOrder
		}

		row.Name = normalized.name
		row.Enabled = normalized.enabled
		row.ListenProtocol = normalized.listenProtocol
		row.ListenProtocolAlias = normalized.listenProtocolAlias
		row.ListenPort = normalized.listenPort
		row.ListenCompressionEnabled = normalized.listenCompressionEnabled
		row.ListenCompressionAlgorithms = reverseProxyCompressionStorageValue(normalized.listenCompressionEnabled, normalized.listenCompressionAlgorithms)
		row.HostList = encodeReverseProxyList(normalized.hosts)
		row.PathPrefix = normalized.pathPrefix
		row.ListenDNSPath = normalized.listenDNSPath
		row.TargetProtocol = normalized.targetProtocol
		row.TargetProtocolAlias = normalized.targetProtocolAlias
		row.TargetAddresses = encodeReverseProxyList(normalized.targetAddresses)
		row.TargetPort = normalized.targetPort
		row.TargetCompressionEnabled = normalized.targetCompressionEnabled
		row.TargetCompressionAlgorithms = reverseProxyCompressionStorageValue(normalized.targetCompressionEnabled, normalized.targetCompressionAlgorithms)
		row.TargetPath = normalized.targetPath
		row.TargetDNSPath = normalized.targetDNSPath
		row.FallbackDNSUpstreams = normalized.fallbackDNSUpstreams
		row.DNSUpstreamTimeoutSeconds = normalized.dnsUpstreamTimeoutSeconds
		row.DNSCacheEnabled = normalized.dnsCacheEnabled
		row.DNSCacheSizeBytes = normalized.dnsCacheSizeBytes
		row.DNSCacheMinTTL = normalized.dnsCacheMinTTL
		row.DNSCacheMaxTTL = normalized.dnsCacheMaxTTL
		row.DNSAllowedCIDRs = encodeReverseProxyList(normalized.dnsAllowedCIDRs)
		row.DNSRateLimitQPS = normalized.dnsRateLimitQPS
		row.DNSMaxConcurrentQueries = normalized.dnsMaxConcurrentQueries
		row.EDNSEnabled = normalized.ednsEnabled
		row.EDNSMode = normalized.ednsMode
		row.EDNSCustomIP = normalized.ednsCustomIP
		row.EDNSClientSubnetPolicy = normalized.ednsClientSubnetPolicy
		row.DisableIPv4Answer = normalized.disableIPv4Answer
		row.DisableIPv6Answer = normalized.disableIPv6Answer
		row.CertificateRecordList = encodeReverseProxyUintList(normalized.certificateRecordIDs)
		row.CertificateRecordID = normalized.certificateRecordID
		row.ListenHTTPVersionStrategy = normalized.listenHTTPVersionStrategy
		row.IPStrategy = normalized.ipStrategy
		row.HTTPVersionStrategy = normalized.httpVersionStrategy
		row.UpstreamTLSVerify = normalized.upstreamTLSVerify
		row.MaxConcurrentConnections = normalized.maxConcurrentConnections
		row.MaxConcurrentRequests = normalized.maxConcurrentRequests
		row.UpstreamMaxConnections = normalized.upstreamMaxConnections
		row.UpstreamMaxIdleConnections = normalized.upstreamMaxIdleConnections
		row.MemoryLimitBytes = normalized.memoryLimitBytes
		row.ApiPassthrough = normalized.apiPassthrough
		row.AdvertiseHTTP3 = normalized.advertiseHTTP3
		row.Remark = normalized.remark
		if row.Enabled {
			row.RuntimeStatus = "pending"
			row.LastError = ""
		} else {
			row.RuntimeStatus = "disabled"
			row.LastError = ""
		}
		if isNew {
			createValues := reverseProxyRulePersistenceMap(row)
			if err := tx.Create(row).Error; err != nil {
				return err
			}
			if err := tx.Model(&model.ReverseProxyRule{}).Where("id = ?", row.Id).Updates(createValues).Error; err != nil {
				return err
			}
			row.Enabled = normalized.enabled
			row.UpstreamTLSVerify = normalized.upstreamTLSVerify
		} else {
			if err := tx.Save(row).Error; err != nil {
				return err
			}
		}
		if err := validatePortForwardListenerClaimsAgainstActiveRules(tx); err != nil {
			return err
		}
		return reverseProxyBumpRevisionTx(tx, settings)
	})
	if err != nil {
		return err
	}
	if err := s.syncAllRuntimeNow(); err != nil {
		logger.Warning("reverse proxy runtime apply after rule save failed: ", err)
	}
	return nil
}

func (s *ReverseProxyService) SetRuleEnabled(payload ReverseProxyRuleStatusPayload) (*ReverseProxyRuleStatusResult, error) {
	if payload.ID == 0 {
		return nil, common.NewError("id is required")
	}
	if payload.ExpectedRevision == nil {
		return nil, common.NewError("expectedRevision is required")
	}
	db := database.GetDB()
	if db == nil {
		return nil, common.NewError("database is not ready")
	}

	var normalized reverseProxyNormalizedRule
	if payload.Enabled {
		row := &model.ReverseProxyRule{}
		if err := db.Where("id = ?", payload.ID).First(row).Error; err != nil {
			return nil, err
		}
		var err error
		normalized, err = s.normalizeRulePayload(reverseProxyRulePayloadFromModel(row, true))
		if err != nil {
			return nil, err
		}
		if err := s.preflightNormalizedRule(normalized); err != nil {
			return nil, err
		}
	}

	result := &ReverseProxyRuleStatusResult{ID: payload.ID, Enabled: payload.Enabled}
	changed := false
	err := db.Transaction(func(tx *gorm.DB) error {
		settings, err := reverseProxyExpectedRevision(tx, payload.ExpectedRevision)
		if err != nil {
			return err
		}
		row := &model.ReverseProxyRule{}
		if err := tx.Where("id = ?", payload.ID).First(row).Error; err != nil {
			return err
		}
		if row.Enabled == payload.Enabled {
			result.Revision = settings.Revision
			return nil
		}
		if payload.Enabled {
			if err := s.validateNormalizedRule(tx, normalized); err != nil {
				return err
			}
		}
		status := "disabled"
		if payload.Enabled {
			status = "pending"
		}
		if err := tx.Model(&model.ReverseProxyRule{}).Where("id = ?", payload.ID).Updates(map[string]interface{}{
			"enabled":        payload.Enabled,
			"runtime_status": status,
			"last_error":     "",
		}).Error; err != nil {
			return err
		}
		if payload.Enabled {
			if err := validatePortForwardListenerClaimsAgainstActiveRules(tx); err != nil {
				return err
			}
		}
		if err := reverseProxyBumpRevisionTx(tx, settings); err != nil {
			return err
		}
		changed = true
		result.Revision = settings.Revision
		return nil
	})
	if err != nil {
		return nil, err
	}
	if changed {
		if err := s.syncAllRuntimeNow(); err != nil {
			logger.Warning("reverse proxy runtime apply after rule status change failed: ", err)
		}
	}
	return result, nil
}

func (s *ReverseProxyService) DeleteRule(id uint) error {
	return s.deleteRule(id, nil)
}

func (s *ReverseProxyService) DeleteRuleWithRevision(payload ReverseProxyRuleDeletePayload) error {
	return s.deleteRule(payload.ID, payload.ExpectedRevision)
}

func (s *ReverseProxyService) deleteRule(id uint, expectedRevision *uint64) error {
	if id == 0 {
		return common.NewError("id is required")
	}
	db := database.GetDB()
	if db == nil {
		return common.NewError("database is not ready")
	}
	err := db.Transaction(func(tx *gorm.DB) error {
		settings, err := reverseProxyExpectedRevision(tx, expectedRevision)
		if err != nil {
			return err
		}
		row := &model.ReverseProxyRule{}
		if err := tx.Where("id = ?", id).First(row).Error; err != nil {
			return err
		}
		if err := tx.Delete(row).Error; err != nil {
			return err
		}
		return reverseProxyBumpRevisionTx(tx, settings)
	})
	if err != nil {
		return err
	}
	if err := s.syncAllRuntimeNow(); err != nil {
		logger.Warning("reverse proxy runtime apply after rule delete failed: ", err)
	}
	reverseProxyRuntime.clearRuleState(id)
	return nil
}

func updateReverseProxyListOrdersTx(tx *gorm.DB, orders map[uint]int64) error {
	if tx == nil || len(orders) == 0 {
		return nil
	}
	ids := make([]uint, 0, len(orders))
	for id := range orders {
		if id != 0 {
			ids = append(ids, id)
		}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	const chunkSize = 300
	for start := 0; start < len(ids); start += chunkSize {
		end := start + chunkSize
		if end > len(ids) {
			end = len(ids)
		}
		chunk := ids[start:end]
		query := "CASE id"
		args := make([]interface{}, 0, len(chunk)*2)
		for _, id := range chunk {
			query += " WHEN ? THEN ?"
			args = append(args, id, orders[id])
		}
		query += " ELSE list_order END"
		result := tx.Model(&model.ReverseProxyRule{}).
			Where("id IN ?", chunk).
			UpdateColumn("list_order", gorm.Expr(query, args...))
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != int64(len(chunk)) {
			return common.NewError("reorder ids mismatch current rules")
		}
	}
	return nil
}

func (s *ReverseProxyService) MoveRule(payload ReverseProxyRuleMovePayload) (*ReverseProxyRuleMoveResult, error) {
	if payload.ID == 0 {
		return nil, common.NewError("id is required")
	}
	if payload.Direction != -1 && payload.Direction != 1 {
		return nil, common.NewError("direction must be -1 or 1")
	}
	if payload.ExpectedRevision == nil {
		return nil, common.NewError("expectedRevision is required")
	}
	db := database.GetDB()
	if db == nil {
		return nil, common.NewError("database is not ready")
	}
	result := &ReverseProxyRuleMoveResult{ID: payload.ID}
	changed := false
	err := db.Transaction(func(tx *gorm.DB) error {
		settings, err := reverseProxyExpectedRevision(tx, payload.ExpectedRevision)
		if err != nil {
			return err
		}
		current := &model.ReverseProxyRule{}
		if err := tx.Where("id = ?", payload.ID).First(current).Error; err != nil {
			return err
		}
		neighbor := &model.ReverseProxyRule{}
		query := tx.Model(&model.ReverseProxyRule{})
		if payload.Direction < 0 {
			query = query.Where("list_order < ? OR (list_order = ? AND id < ?)", current.ListOrder, current.ListOrder, current.Id).
				Order("list_order DESC, id DESC")
		} else {
			query = query.Where("list_order > ? OR (list_order = ? AND id > ?)", current.ListOrder, current.ListOrder, current.Id).
				Order("list_order ASC, id ASC")
		}
		findErr := query.First(neighbor).Error
		if database.IsNotFound(findErr) {
			result.Revision = settings.Revision
			return nil
		}
		if findErr != nil {
			return findErr
		}
		orders := map[uint]int64{
			current.Id:  neighbor.ListOrder,
			neighbor.Id: current.ListOrder,
		}
		if current.ListOrder == neighbor.ListOrder {
			rules := make([]model.ReverseProxyRule, 0)
			if err := tx.Order("list_order ASC, id ASC").Find(&rules).Error; err != nil {
				return err
			}
			index := -1
			for i := range rules {
				if rules[i].Id == current.Id {
					index = i
					break
				}
			}
			nextIndex := index + payload.Direction
			if index < 0 || nextIndex < 0 || nextIndex >= len(rules) {
				result.Revision = settings.Revision
				return nil
			}
			rules[index], rules[nextIndex] = rules[nextIndex], rules[index]
			orders = make(map[uint]int64, len(rules))
			for i := range rules {
				nextOrder := int64(i + 1)
				if rules[i].ListOrder != nextOrder {
					orders[rules[i].Id] = nextOrder
				}
			}
			neighbor = &rules[index]
		}
		if err := updateReverseProxyListOrdersTx(tx, orders); err != nil {
			return err
		}
		if err := reverseProxyBumpRevisionTx(tx, settings); err != nil {
			return err
		}
		changed = true
		result.Revision = settings.Revision
		result.AdjacentID = neighbor.Id
		return nil
	})
	if err != nil {
		return nil, err
	}
	if changed {
		if err := s.syncAllRuntimeNow(); err != nil {
			logger.Warning("reverse proxy runtime apply after rule move failed: ", err)
		}
	}
	return result, nil
}

func (s *ReverseProxyService) ReorderRules(payload ReverseProxyRuleReorderPayload) error {
	if len(payload.IDs) == 0 {
		return common.NewError("ids are required")
	}
	db := database.GetDB()
	if db == nil {
		return common.NewError("database is not ready")
	}
	err := db.Transaction(func(tx *gorm.DB) error {
		settings, err := reverseProxyExpectedRevision(tx, payload.ExpectedRevision)
		if err != nil {
			return err
		}
		rules, err := s.loadRulesLocked(tx)
		if err != nil {
			return err
		}
		if len(rules) != len(payload.IDs) {
			return common.NewError("reorder ids must include all rules")
		}
		orderMap := make(map[uint]int64, len(payload.IDs))
		for idx, id := range payload.IDs {
			if id == 0 {
				return common.NewError("reorder ids contain zero")
			}
			if _, exists := orderMap[id]; exists {
				return common.NewError("reorder ids contain duplicates")
			}
			orderMap[id] = int64(idx + 1)
		}
		changedOrders := make(map[uint]int64)
		for i := range rules {
			nextOrder, ok := orderMap[rules[i].Id]
			if !ok {
				return common.NewError("reorder ids mismatch current rules")
			}
			if rules[i].ListOrder != nextOrder {
				changedOrders[rules[i].Id] = nextOrder
			}
		}
		if err := updateReverseProxyListOrdersTx(tx, changedOrders); err != nil {
			return err
		}
		return reverseProxyBumpRevisionTx(tx, settings)
	})
	if err != nil {
		return err
	}
	if err := s.syncAllRuntimeNow(); err != nil {
		logger.Warning("reverse proxy runtime apply after reorder failed: ", err)
	}
	return nil
}

func (s *ReverseProxyService) SyncIfNeeded(minGap time.Duration) (syncErr error) {
	// A cron reconciliation can observe a revision written outside this API
	// process (for example during a controlled database restore).  Refresh the
	// heavyweight configuration snapshot only when its in-memory revision is
	// behind the reconciled runtime; normal cron ticks remain SQLite-free after
	// the tiny revision probe.
	defer func() {
		if snapshotErr := s.refreshReverseProxyConfigurationSnapshotIfStale(); snapshotErr != nil && syncErr == nil {
			syncErr = snapshotErr
		}
	}()
	if err := reverseProxyRuntime.SyncIfNeeded(s, minGap); err != nil {
		return err
	}
	revision := reverseProxyRuntime.currentRevision()
	now := time.Now()
	if reverseProxyRuntime.maintainCertificateBalance(now) {
		reverseProxyDNSRuntime.pruneAdmissionClients(now)
	}
	if minGap > 0 {
		if !reverseProxyDNSRuntime.needsSync(revision, time.Now()) {
			return nil
		}
	}
	loaded := reverseProxyRuntime.loadedConfigurationSnapshot()
	if loaded == nil {
		var err error
		loaded, err = s.loadReverseProxyLoadedConfiguration()
		if err != nil {
			return err
		}
	}
	return syncReverseProxyDNSRuntimeAtRevision(s, loaded.Rules, revision)
}

func (s *ReverseProxyService) refreshReverseProxyConfigurationSnapshotIfStale() error {
	if s == nil {
		return nil
	}
	reverseProxyRuntime.mu.Lock()
	runtimeRevision := reverseProxyRuntime.state.revision
	reverseProxyRuntime.mu.Unlock()
	if runtimeRevision == 0 {
		return nil
	}
	reverseProxyRuntime.overviewMu.RLock()
	snapshotRevision := uint64(0)
	if reverseProxyRuntime.configuration != nil {
		snapshotRevision = reverseProxyRuntime.configuration.Revision
	}
	reverseProxyRuntime.overviewMu.RUnlock()
	if snapshotRevision == runtimeRevision {
		return nil
	}
	loaded := reverseProxyRuntime.loadedConfigurationSnapshot()
	if loaded == nil || loaded.Settings.Revision != runtimeRevision {
		return s.refreshReverseProxyConfigurationSnapshot()
	}
	return s.publishReverseProxyConfigurationSnapshot(&loaded.Settings, loaded.Rules)
}

func (s *ReverseProxyService) StartRuntime() error {
	defer func() {
		reverseProxyRuntimeMonitor.AllowAfterDatabaseRestore()
		reverseProxyRuntimeMonitor.Start(s)
	}()
	if err := s.CertificateInventoryService.BackfillIssuedAlgorithms(100); err != nil {
		return err
	}
	if err := s.resetRuntimeStateForStartup(); err != nil {
		return err
	}
	if err := reverseProxyRuntime.SyncNow(s); err != nil {
		return err
	}
	loaded := reverseProxyRuntime.loadedConfigurationSnapshot()
	if loaded == nil {
		var err error
		loaded, err = s.loadReverseProxyLoadedConfiguration()
		if err != nil {
			return err
		}
	}
	if err := syncReverseProxyDNSRuntimeAtRevision(s, loaded.Rules, loaded.Settings.Revision); err != nil {
		return err
	}
	if err := s.publishReverseProxyConfigurationSnapshot(&loaded.Settings, loaded.Rules); err != nil {
		return err
	}
	return nil
}

func (s *ReverseProxyService) StopRuntime() error {
	reverseProxyRuntimeMonitor.StopAndWait()
	httpErr := reverseProxyRuntime.Stop()
	dnsErr := stopReverseProxyDNSRuntime()
	reverseProxyRuntime.resetRuleStates()
	reverseProxyRuntime.overviewMu.Lock()
	reverseProxyRuntime.configuration = nil
	reverseProxyRuntime.overviewMu.Unlock()
	if httpErr != nil {
		return httpErr
	}
	return dnsErr
}

func (s *ReverseProxyService) syncAllRuntimeNow() error {
	httpErr := reverseProxyRuntime.SyncNow(s)
	loaded := reverseProxyRuntime.loadedConfigurationSnapshot()
	if loaded == nil || httpErr != nil {
		var loadErr error
		loaded, loadErr = s.loadReverseProxyLoadedConfiguration()
		if loadErr != nil {
			return errors.Join(httpErr, loadErr)
		}
	}
	dnsErr := syncReverseProxyDNSRuntimeAtRevision(s, loaded.Rules, loaded.Settings.Revision)
	var retryHTTPErr error
	// HTTP reconciliation runs first so HTTP -> DNS changes can release their
	// TCP socket before DNS binds it. The inverse transition releases the old
	// DNS listener during the DNS pass, so retry a failed HTTP bind once now
	// instead of leaving a saved protocol switch pending until the cron retry.
	if httpErr == nil && reverseProxyRuntime.hasPendingReconcile() {
		retryHTTPErr = reverseProxyRuntime.SyncNow(s)
	}
	snapshotErr := s.publishReverseProxyConfigurationSnapshot(&loaded.Settings, loaded.Rules)
	return errors.Join(httpErr, dnsErr, retryHTTPErr, snapshotErr)
}

func (s *ReverseProxyService) SyncCertificateInventoryNow() error {
	if s == nil {
		s = &ReverseProxyService{}
	}
	return s.syncAllRuntimeNow()
}

func (s *ReverseProxyService) resetRuntimeStateForStartup() error {
	reverseProxyRuntime.resetRuleStates()
	db := database.GetDB()
	if db == nil {
		return nil
	}
	if err := db.Session(&gormSessionAllowAll).Model(&model.ReverseProxyRule{}).Updates(map[string]interface{}{
		"last_error":     "",
		"runtime_status": "",
	}).Error; err != nil {
		return err
	}
	return db.Model(&model.ReverseProxyRule{}).Where("enabled = ?", true).Update("runtime_status", "pending").Error
}

func (s *ReverseProxyService) loadRulesLocked(db *gorm.DB) ([]model.ReverseProxyRule, error) {
	rows := make([]model.ReverseProxyRule, 0)
	if db == nil {
		return rows, nil
	}
	if err := db.Order("id asc").Find(&rows).Error; err != nil {
		return nil, err
	}
	if err := repairReverseProxyRowsTx(db, rows); err != nil {
		return nil, err
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].ListOrder == rows[j].ListOrder {
			return rows[i].Id < rows[j].Id
		}
		return rows[i].ListOrder < rows[j].ListOrder
	})
	return rows, nil
}

func (s *ReverseProxyService) listCertificateOptions() ([]ReverseProxyCertificateOption, map[uint]ReverseProxyCertificateOption, error) {
	rows := make([]model.CertificateRecord, 0)
	if err := database.GetDB().
		Select("id", "display_id", "main_domain", "domain_set", "not_after", "last_error", "list_order_at").
		Order("list_order_at DESC, id DESC").
		Find(&rows).Error; err != nil {
		return nil, nil, err
	}
	options := make([]ReverseProxyCertificateOption, 0, len(rows))
	byID := make(map[uint]ReverseProxyCertificateOption, len(rows))
	for i := range rows {
		if rows[i].Id == 0 {
			continue
		}
		option := ReverseProxyCertificateOption{
			ID:         rows[i].Id,
			DisplayID:  rows[i].DisplayID,
			MainDomain: strings.TrimSpace(rows[i].MainDomain),
			Domains:    decodeCertificateDomains(rows[i].DomainSet),
			NotAfter:   rows[i].NotAfter,
			Status:     certificateStatus(&rows[i]),
		}
		options = append(options, option)
		byID[option.ID] = option
	}
	return options, byID, nil
}

func buildReverseProxyRuleView(row *model.ReverseProxyRule, certMap map[uint]ReverseProxyCertificateOption, counts reverseProxyConnectionCounts, balance []ReverseProxyCertificateBalanceDiagnostic) ReverseProxyRuleView {
	view := ReverseProxyRuleView{}
	if row == nil {
		return view
	}
	hosts := reverseProxyRuleServerNames(row)
	certIDs := reverseProxyRuleCertificateIDs(row)
	view = ReverseProxyRuleView{
		ID:                  row.Id,
		DisplayID:           row.DisplayID,
		ListOrder:           row.ListOrder,
		Name:                strings.TrimSpace(row.Name),
		Enabled:             row.Enabled,
		ListenProtocol:      strings.TrimSpace(row.ListenProtocol),
		ListenProtocolAlias: strings.TrimSpace(row.ListenProtocolAlias),
		ListenPort:          row.ListenPort,
		ListenCompressionEnabled: func() bool {
			enabled, _ := reverseProxyCompressionSettingsFromModel(row.ListenCompressionEnabled, row.ListenCompressionAlgorithms)
			return enabled
		}(),
		ListenCompressionAlgorithms: func() []string {
			_, values := reverseProxyCompressionSettingsFromModel(row.ListenCompressionEnabled, row.ListenCompressionAlgorithms)
			return values
		}(),
		Hosts:               hosts,
		PathPrefix:          strings.TrimSpace(row.PathPrefix),
		ListenDNSPath:       strings.TrimSpace(row.ListenDNSPath),
		TargetProtocol:      strings.TrimSpace(row.TargetProtocol),
		TargetProtocolAlias: strings.TrimSpace(row.TargetProtocolAlias),
		TargetAddresses:     decodeReverseProxyList(row.TargetAddresses),
		TargetPort:          row.TargetPort,
		TargetCompressionEnabled: func() bool {
			enabled, _ := reverseProxyCompressionSettingsFromModel(row.TargetCompressionEnabled, row.TargetCompressionAlgorithms)
			return enabled
		}(),
		TargetCompressionAlgorithms: func() []string {
			_, values := reverseProxyCompressionSettingsFromModel(row.TargetCompressionEnabled, row.TargetCompressionAlgorithms)
			return values
		}(),
		TargetPath:                 strings.TrimSpace(row.TargetPath),
		TargetDNSPath:              strings.TrimSpace(row.TargetDNSPath),
		FallbackDNSUpstreams:       strings.TrimSpace(row.FallbackDNSUpstreams),
		DNSUpstreamTimeoutSeconds:  reverseProxyDNSUpstreamTimeoutSeconds(row.DNSUpstreamTimeoutSeconds),
		DNSCacheEnabled:            row.DNSCacheEnabled,
		DNSCacheSizeBytes:          reverseProxyDNSCacheSizeBytes(row.DNSCacheSizeBytes),
		DNSCacheMinTTL:             row.DNSCacheMinTTL,
		DNSCacheMaxTTL:             row.DNSCacheMaxTTL,
		DNSAllowedCIDRs:            decodeReverseProxyList(row.DNSAllowedCIDRs),
		DNSRateLimitQPS:            reverseProxyDNSRateLimitQPS(row.DNSRateLimitQPS),
		DNSMaxConcurrentQueries:    reverseProxyDNSMaxConcurrentQueries(row.DNSMaxConcurrentQueries),
		EDNSEnabled:                row.EDNSEnabled,
		EDNSMode:                   strings.TrimSpace(row.EDNSMode),
		EDNSCustomIP:               strings.TrimSpace(row.EDNSCustomIP),
		EDNSClientSubnetPolicy:     strings.TrimSpace(row.EDNSClientSubnetPolicy),
		DisableIPv4Answer:          row.DisableIPv4Answer,
		DisableIPv6Answer:          row.DisableIPv6Answer,
		CertificateRecordIDs:       append([]uint(nil), certIDs...),
		ListenHTTPVersionStrategy:  strings.TrimSpace(row.ListenHTTPVersionStrategy),
		IPStrategy:                 strings.TrimSpace(row.IPStrategy),
		HTTPVersionStrategy:        strings.TrimSpace(row.HTTPVersionStrategy),
		UpstreamTLSVerify:          row.UpstreamTLSVerify,
		MaxConcurrentConnections:   reverseProxyRuleLimit(row.MaxConcurrentConnections),
		MaxConcurrentRequests:      reverseProxyMaxConcurrentRequests(row.MaxConcurrentRequests),
		UpstreamMaxConnections:     reverseProxyRuleLimit(row.UpstreamMaxConnections),
		UpstreamMaxIdleConnections: reverseProxyRuleLimit(row.UpstreamMaxIdleConnections),
		MemoryLimitBytes:           row.MemoryLimitBytes,
		ApiPassthrough:             row.ApiPassthrough,
		AdvertiseHTTP3:             row.AdvertiseHTTP3,
		Remark:                     strings.TrimSpace(row.Remark),
		LastError:                  strings.TrimSpace(row.LastError),
		RuntimeStatus:              strings.TrimSpace(row.RuntimeStatus),
		LocalConnectionCount:       counts.LocalOpen,
		UpstreamConnectionCount:    counts.UpstreamOpen,
		CertificateBalance:         append([]ReverseProxyCertificateBalanceDiagnostic(nil), balance...),
		UpdatedAt:                  row.UpdatedAt.Unix(),
		CreatedAt:                  row.CreatedAt.Unix(),
	}
	if !row.Enabled {
		view.RuntimeStatus = "disabled"
		view.LastError = ""
		view.LocalConnectionCount = 0
		view.UpstreamConnectionCount = 0
	} else if view.RuntimeStatus == "" {
		view.RuntimeStatus = "pending"
	}
	if normalizedListenStrategy, err := normalizeReverseProxyListenHTTPVersionStrategy(row.ListenHTTPVersionStrategy, row.ListenProtocol); err == nil {
		view.ListenHTTPVersionStrategy = normalizedListenStrategy
	}
	if len(certIDs) > 0 {
		view.CertificateRecordID = certIDs[0]
	}
	hintCerts := make([]ReverseProxyCertificateOption, 0, len(certIDs))
	certLabels := make([]string, 0, len(certIDs))
	for _, certID := range certIDs {
		cert, ok := certMap[certID]
		if !ok {
			continue
		}
		certLabel := strconv.FormatUint(cert.DisplayID, 10) + " / " + strings.TrimSpace(cert.MainDomain)
		certLabels = append(certLabels, certLabel)
		hintCerts = append(hintCerts, cert)
	}
	view.CertificateLabels = certLabels
	if len(certLabels) > 0 {
		view.CertificateLabel = strings.Join(certLabels, ", ")
		view.CertificateHints = buildReverseProxyCertificateHints(view.Hosts, hintCerts)
	}
	return view
}

func reverseProxyRulePersistenceMap(row *model.ReverseProxyRule) map[string]interface{} {
	if row == nil {
		return map[string]interface{}{}
	}
	return map[string]interface{}{
		"display_id":                    row.DisplayID,
		"list_order":                    row.ListOrder,
		"name":                          row.Name,
		"enabled":                       row.Enabled,
		"listen_protocol":               row.ListenProtocol,
		"listen_protocol_alias":         row.ListenProtocolAlias,
		"listen_port":                   row.ListenPort,
		"listen_compression_enabled":    row.ListenCompressionEnabled,
		"listen_compression_algorithms": row.ListenCompressionAlgorithms,
		"host_list":                     row.HostList,
		"path_prefix":                   row.PathPrefix,
		"listen_dns_path":               row.ListenDNSPath,
		"target_protocol":               row.TargetProtocol,
		"target_protocol_alias":         row.TargetProtocolAlias,
		"target_addresses":              row.TargetAddresses,
		"target_port":                   row.TargetPort,
		"target_compression_enabled":    row.TargetCompressionEnabled,
		"target_compression_algorithms": row.TargetCompressionAlgorithms,
		"target_path":                   row.TargetPath,
		"target_dns_path":               row.TargetDNSPath,
		"fallback_dns_upstreams":        row.FallbackDNSUpstreams,
		"dns_upstream_timeout_seconds":  row.DNSUpstreamTimeoutSeconds,
		"dns_cache_enabled":             row.DNSCacheEnabled,
		"dns_cache_size_bytes":          row.DNSCacheSizeBytes,
		"dns_cache_min_ttl":             row.DNSCacheMinTTL,
		"dns_cache_max_ttl":             row.DNSCacheMaxTTL,
		"dns_allowed_cidrs":             row.DNSAllowedCIDRs,
		"dns_rate_limit_qps":            row.DNSRateLimitQPS,
		"dns_max_concurrent_queries":    row.DNSMaxConcurrentQueries,
		"edns_enabled":                  row.EDNSEnabled,
		"edns_mode":                     row.EDNSMode,
		"edns_custom_ip":                row.EDNSCustomIP,
		"edns_client_subnet_policy":     row.EDNSClientSubnetPolicy,
		"disable_ipv4_answer":           row.DisableIPv4Answer,
		"disable_ipv6_answer":           row.DisableIPv6Answer,
		"certificate_record_list":       row.CertificateRecordList,
		"certificate_record_id":         row.CertificateRecordID,
		"listen_http_version_strategy":  row.ListenHTTPVersionStrategy,
		"ip_strategy":                   row.IPStrategy,
		"http_version_strategy":         row.HTTPVersionStrategy,
		"upstream_tls_verify":           row.UpstreamTLSVerify,
		"max_concurrent_connections":    row.MaxConcurrentConnections,
		"max_concurrent_requests":       row.MaxConcurrentRequests,
		"upstream_max_connections":      row.UpstreamMaxConnections,
		"upstream_max_idle_connections": row.UpstreamMaxIdleConnections,
		"memory_limit_bytes":            row.MemoryLimitBytes,
		"api_passthrough":               row.ApiPassthrough,
		"advertise_http3":               row.AdvertiseHTTP3,
		"remark":                        row.Remark,
		"last_error":                    row.LastError,
		"runtime_status":                row.RuntimeStatus,
	}
}

func reverseProxyDNSUpstreamTimeoutSeconds(value int) int {
	if value == 0 {
		return reverseProxyDNSDefaultUpstreamTimeoutSeconds
	}
	return value
}

func reverseProxyDNSCacheSizeBytes(value int) int {
	if value == 0 {
		return reverseProxyDNSDefaultCacheSizeBytes
	}
	return value
}

func reverseProxyDNSRateLimitQPS(value int) int {
	if value <= 0 {
		return reverseProxyDNSDefaultRateLimitQPS
	}
	return value
}

func reverseProxyDNSMaxConcurrentQueries(value int) int {
	return reverseProxyRuleLimit(value)
}

func reverseProxyMaxConcurrentRequests(value int) int {
	return reverseProxyRuleLimit(value)
}

func reverseProxyRuleLimit(value int) int {
	if value < 0 {
		return 0
	}
	return value
}

func normalizeReverseProxyCIDRs(raw string) ([]string, error) {
	parts := strings.FieldsFunc(strings.TrimSpace(raw), func(r rune) bool {
		return r == ',' || r == '\n' || r == '\r' || r == '\t' || r == ' '
	})
	result := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, item := range parts {
		prefix, err := netip.ParsePrefix(strings.TrimSpace(item))
		if err != nil {
			return nil, common.NewError("invalid dns allowed cidr: ", strings.TrimSpace(item))
		}
		prefix = prefix.Masked()
		if prefix.Bits() == 0 {
			return nil, common.NewError("dns allowed cidr must not allow the entire internet")
		}
		value := prefix.String()
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result, nil
}

func validateReverseProxyDNSAdmissionSettings(row reverseProxyNormalizedRule) error {
	if row.dnsRateLimitQPS < 1 || row.dnsRateLimitQPS > reverseProxyDNSMaxRateLimitQPS {
		return common.NewError("dns rate limit qps must be between 1 and ", reverseProxyDNSMaxRateLimitQPS)
	}
	if row.dnsMaxConcurrentQueries < 0 || row.dnsMaxConcurrentQueries > reverseProxyDNSMaxConcurrentQueryLimit {
		return common.NewError("dns max concurrent queries must be between 0 and ", reverseProxyDNSMaxConcurrentQueryLimit)
	}
	if row.maxConcurrentRequests < 0 || row.maxConcurrentRequests > reverseProxyMaxConcurrentRequestLimit {
		return common.NewError("max concurrent requests must be between 0 and ", reverseProxyMaxConcurrentRequestLimit)
	}
	if len(row.dnsAllowedCIDRs) == 0 {
		return common.NewError("dns wildcard listeners require at least one non-global allowed cidr")
	}
	return nil
}

func normalizeReverseProxyDNSUpstreamsText(value string) string {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.ReplaceAll(value, "\r", "\n")
	lines := strings.Split(value, "\n")
	for i := range lines {
		lines[i] = strings.TrimSpace(lines[i])
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func (s *ReverseProxyService) normalizeRulePayload(payload ReverseProxyRulePayload) (reverseProxyNormalizedRule, error) {
	dnsUpstreamTimeoutSeconds := reverseProxyDNSDefaultUpstreamTimeoutSeconds
	if payload.DNSUpstreamTimeoutSeconds != nil {
		dnsUpstreamTimeoutSeconds = *payload.DNSUpstreamTimeoutSeconds
	}
	dnsCacheSizeBytes := reverseProxyDNSDefaultCacheSizeBytes
	if payload.DNSCacheSizeBytes != nil {
		dnsCacheSizeBytes = *payload.DNSCacheSizeBytes
	}
	dnsRateLimitQPS := reverseProxyDNSDefaultRateLimitQPS
	if payload.DNSRateLimitQPS != nil {
		dnsRateLimitQPS = *payload.DNSRateLimitQPS
	}
	dnsMaxConcurrentQueries := 0
	if payload.DNSMaxConcurrentQueries != nil {
		dnsMaxConcurrentQueries = *payload.DNSMaxConcurrentQueries
	}
	maxConcurrentConnections := 0
	if payload.MaxConcurrentConnections != nil {
		maxConcurrentConnections = *payload.MaxConcurrentConnections
	}
	maxConcurrentRequests := 0
	if payload.MaxConcurrentRequests != nil {
		maxConcurrentRequests = *payload.MaxConcurrentRequests
	}
	upstreamMaxConnections := 0
	if payload.UpstreamMaxConnections != nil {
		upstreamMaxConnections = *payload.UpstreamMaxConnections
	}
	upstreamMaxIdleConnections := 0
	if payload.UpstreamMaxIdleConnections != nil {
		upstreamMaxIdleConnections = *payload.UpstreamMaxIdleConnections
	}
	memoryLimitBytes := int64(0)
	if payload.MemoryLimitBytes != nil {
		memoryLimitBytes = *payload.MemoryLimitBytes
	}
	listenNameInput := strings.TrimSpace(payload.Hosts)
	listenProtocolAliasInput := strings.ToLower(strings.TrimSpace(payload.ListenProtocolAlias))
	targetProtocolAliasInput := strings.ToLower(strings.TrimSpace(payload.TargetProtocolAlias))
	listenCompressionEnabled := reverseProxyPayloadCompressionEnabled(payload.ListenCompressionEnabled)
	listenCompressionAlgorithms, compressionErr := normalizeReverseProxyCompressionAlgorithms(payload.ListenCompressionAlgorithms)
	if compressionErr != nil {
		return reverseProxyNormalizedRule{}, compressionErr
	}
	targetCompressionEnabled := reverseProxyPayloadCompressionEnabled(payload.TargetCompressionEnabled)
	targetCompressionAlgorithms, compressionErr := normalizeReverseProxyCompressionAlgorithms(payload.TargetCompressionAlgorithms)
	if compressionErr != nil {
		return reverseProxyNormalizedRule{}, compressionErr
	}
	if !listenCompressionEnabled {
		listenCompressionAlgorithms = []string{}
	}
	if !targetCompressionEnabled {
		targetCompressionAlgorithms = []string{}
	}
	normalized := reverseProxyNormalizedRule{
		id:                          payload.ID,
		name:                        strings.TrimSpace(payload.Name),
		enabled:                     payload.Enabled,
		listenPort:                  payload.ListenPort,
		listenCompressionEnabled:    listenCompressionEnabled,
		listenCompressionAlgorithms: listenCompressionAlgorithms,
		targetPort:                  payload.TargetPort,
		targetCompressionEnabled:    targetCompressionEnabled,
		targetCompressionAlgorithms: targetCompressionAlgorithms,
		maxConcurrentConnections:    maxConcurrentConnections,
		maxConcurrentRequests:       maxConcurrentRequests,
		upstreamMaxConnections:      upstreamMaxConnections,
		upstreamMaxIdleConnections:  upstreamMaxIdleConnections,
		memoryLimitBytes:            memoryLimitBytes,
		apiPassthrough:              payload.ApiPassthrough,
		advertiseHTTP3:              payload.AdvertiseHTTP3,
		remark:                      strings.TrimSpace(payload.Remark),
		fallbackDNSUpstreams:        normalizeReverseProxyDNSUpstreamsText(payload.FallbackDNSUpstreams),
		dnsUpstreamTimeoutSeconds:   dnsUpstreamTimeoutSeconds,
		dnsCacheEnabled:             payload.DNSCacheEnabled,
		dnsCacheSizeBytes:           dnsCacheSizeBytes,
		dnsCacheMinTTL:              payload.DNSCacheMinTTL,
		dnsCacheMaxTTL:              payload.DNSCacheMaxTTL,
		dnsRateLimitQPS:             dnsRateLimitQPS,
		dnsMaxConcurrentQueries:     dnsMaxConcurrentQueries,
		ednsEnabled:                 payload.EDNSEnabled,
		disableIPv4Answer:           payload.DisableIPv4Answer,
		disableIPv6Answer:           payload.DisableIPv6Answer,
	}
	if normalized.name == "" {
		normalized.name = buildReverseProxyDefaultName(payload.ListenProtocol, listenNameInput, payload.ListenPort, payload.PathPrefix)
	}

	listenProtocolInput := strings.TrimSpace(payload.ListenProtocol)
	targetProtocolInput := strings.TrimSpace(payload.TargetProtocol)

	listenProtocol, err := normalizeReverseProxyProtocol(listenProtocolInput)
	if err != nil {
		return reverseProxyNormalizedRule{}, err
	}
	targetProtocol, err := normalizeReverseProxyProtocol(targetProtocolInput)
	if err != nil {
		return reverseProxyNormalizedRule{}, err
	}
	normalized.listenProtocol = listenProtocol
	normalized.targetProtocol = targetProtocol
	normalized.listenProtocolAlias = normalizeReverseProxyProtocolAlias(listenProtocolAliasInput, listenProtocolInput)
	normalized.targetProtocolAlias = normalizeReverseProxyProtocolAlias(targetProtocolAliasInput, targetProtocolInput)
	if !reverseProxyProtocolSupportsCompression(normalized.listenProtocol, normalized.listenProtocolAlias) {
		normalized.listenCompressionEnabled = false
		normalized.listenCompressionAlgorithms = []string{}
	}
	if !reverseProxyProtocolSupportsCompression(normalized.targetProtocol, normalized.targetProtocolAlias) {
		normalized.targetCompressionEnabled = false
		normalized.targetCompressionAlgorithms = []string{}
	}
	normalized.listenDNSPath = normalizeReverseProxyDNSPath(payload.ListenDNSPath)
	normalized.targetDNSPath = normalizeReverseProxyDNSPath(payload.TargetDNSPath)
	if normalized.listenDNSPath == "" {
		normalized.listenDNSPath = normalizeReverseProxyDNSPath(payload.PathPrefix)
	}
	if normalized.targetDNSPath == "" {
		normalized.targetDNSPath = normalizeReverseProxyDNSPath(payload.TargetPath)
	}

	if normalized.listenPort < 1 || normalized.listenPort > 65535 {
		return reverseProxyNormalizedRule{}, common.NewError("listen port must be between 1 and 65535")
	}
	if normalized.targetPort < 1 || normalized.targetPort > 65535 {
		return reverseProxyNormalizedRule{}, common.NewError("target port must be between 1 and 65535")
	}

	hosts, err := normalizeReverseProxyTokens(listenNameInput, reverseProxyTokenModeListenName)
	if err != nil {
		return reverseProxyNormalizedRule{}, err
	}
	normalized.hosts = hosts
	normalized.pathPrefix = normalizeReverseProxyPath(payload.PathPrefix, false)

	targetAddresses, err := normalizeReverseProxyTokens(payload.TargetAddresses, reverseProxyTokenModeTarget)
	if err != nil {
		return reverseProxyNormalizedRule{}, err
	}
	if len(targetAddresses) == 0 {
		return reverseProxyNormalizedRule{}, common.NewError("target addresses are required")
	}
	normalized.targetAddresses = targetAddresses
	normalized.targetPath = normalizeReverseProxyPath(payload.TargetPath, false)
	ipStrategy, err := normalizeReverseProxyIPStrategy(payload.IPStrategy)
	if err != nil {
		return reverseProxyNormalizedRule{}, err
	}
	normalized.ipStrategy = ipStrategy
	rawEDNSCustomIP := strings.TrimSpace(payload.EDNSCustomIP)
	normalized.ednsMode = normalizeReverseProxyEDNSMode(payload.EDNSMode)
	normalized.ednsClientSubnetPolicy = normalizeReverseProxyEDNSClientSubnetPolicy(payload.EDNSClientSubnetPolicy)

	if normalized.listenProtocol == reverseProxyProtocolDNS ||
		normalized.targetProtocol == reverseProxyProtocolDNS ||
		reverseProxyProtocolIsDNS(normalized.listenProtocolAlias) ||
		reverseProxyProtocolIsDNS(normalized.targetProtocolAlias) {
		if !reverseProxyProtocolIsDNS(normalized.listenProtocolAlias) || !reverseProxyProtocolIsDNS(normalized.targetProtocolAlias) {
			return reverseProxyNormalizedRule{}, common.NewError("dns reverse proxy requires both local protocol and target protocol to be dns")
		}
		if !reverseProxyDNSProtocolUsesTLS(normalized.listenProtocolAlias) {
			normalized.hosts = []string{}
		}
		normalized.pathPrefix = ""
		normalized.targetPath = ""
		normalized.listenHTTPVersionStrategy = ""
		normalized.httpVersionStrategy = ""
		normalized.apiPassthrough = true
		normalized.advertiseHTTP3 = false
		normalized.maxConcurrentConnections = 0
		normalized.maxConcurrentRequests = 0
		normalized.upstreamMaxConnections = 0
		normalized.upstreamMaxIdleConnections = 0
		normalized.upstreamTLSVerify = reverseProxyPayloadTLSVerify(payload.UpstreamTLSVerify, payload.upstreamTLSVerifyDecoded, payload.upstreamTLSVerifySet, reverseProxyDNSProtocolUsesTLS(normalized.targetProtocolAlias))
		allowedCIDRs, cidrErr := normalizeReverseProxyCIDRs(payload.DNSAllowedCIDRs)
		if cidrErr != nil {
			return reverseProxyNormalizedRule{}, cidrErr
		}
		normalized.dnsAllowedCIDRs = allowedCIDRs
		if err := validateReverseProxyDNSAdmissionSettings(normalized); err != nil {
			return reverseProxyNormalizedRule{}, err
		}
		if reverseProxyDNSProtocolUsesPath(normalized.listenProtocolAlias) && normalized.listenDNSPath == "" {
			normalized.listenDNSPath = "/dns-query"
		}
		if reverseProxyDNSProtocolUsesPath(normalized.targetProtocolAlias) && normalized.targetDNSPath == "" {
			normalized.targetDNSPath = "/dns-query"
		}
		if !reverseProxyDNSProtocolUsesPath(normalized.listenProtocolAlias) {
			normalized.listenDNSPath = ""
		}
		if !reverseProxyDNSProtocolUsesPath(normalized.targetProtocolAlias) {
			normalized.targetDNSPath = ""
		}
		if !normalized.ednsEnabled {
			normalized.ednsMode = reverseProxyEDNSModeAuto
			normalized.ednsCustomIP = ""
			normalized.ednsClientSubnetPolicy = reverseProxyEDNSClientSubnetPolicyClientIP
		} else {
			if normalized.ednsMode == reverseProxyEDNSModeCustom {
				if rawEDNSCustomIP == "" {
					return reverseProxyNormalizedRule{}, common.NewError("edns custom ip is required")
				}
				normalizedIP, ok := normalizeReverseProxyEDNSCustomIPv4(rawEDNSCustomIP)
				if !ok {
					return reverseProxyNormalizedRule{}, common.NewError("invalid edns custom ip: only ipv4 is supported")
				}
				normalized.ednsCustomIP = normalizedIP
			} else {
				normalized.ednsCustomIP = ""
			}
		}
		if err := validateReverseProxyDNSCacheSettings(normalized); err != nil {
			return reverseProxyNormalizedRule{}, err
		}
		certIDs := normalizeReverseProxyCertificateIDList(payload.CertificateRecordIDs, payload.CertificateRecordID)
		if reverseProxyDNSProtocolUsesTLS(normalized.listenProtocolAlias) {
			if len(certIDs) == 0 {
				return reverseProxyNormalizedRule{}, common.NewError("dns tls listener requires certificate")
			}
			normalized.certificateRecordIDs = certIDs
			normalized.certificateRecordID = certIDs[0]
		} else {
			normalized.certificateRecordIDs = []uint{}
			normalized.certificateRecordID = 0
		}
		if err := validateReverseProxyRuleResourceSettings(normalized); err != nil {
			return reverseProxyNormalizedRule{}, err
		}
		return normalized, nil
	}

	normalized.ednsEnabled = false
	normalized.ednsMode = reverseProxyEDNSModeAuto
	normalized.ednsCustomIP = ""
	normalized.ednsClientSubnetPolicy = reverseProxyEDNSClientSubnetPolicyClientIP
	normalized.disableIPv4Answer = false
	normalized.disableIPv6Answer = false
	normalized.listenDNSPath = ""
	normalized.targetDNSPath = ""
	normalized.fallbackDNSUpstreams = ""
	normalized.dnsUpstreamTimeoutSeconds = reverseProxyDNSDefaultUpstreamTimeoutSeconds
	normalized.dnsCacheEnabled = false
	normalized.dnsCacheSizeBytes = reverseProxyDNSDefaultCacheSizeBytes
	normalized.dnsCacheMinTTL = 0
	normalized.dnsCacheMaxTTL = 0
	normalized.dnsAllowedCIDRs = []string{}
	normalized.dnsRateLimitQPS = reverseProxyDNSDefaultRateLimitQPS
	normalized.dnsMaxConcurrentQueries = 0

	listenHTTPVersionInput := payload.ListenHTTPVersionStrategy
	if strings.EqualFold(normalized.listenProtocolAlias, "wss") {
		listenHTTPVersionInput = reverseProxyListenHTTPVersionH2Only
	}
	if implied := reverseProxyListenProtocolAliasStrategy(listenProtocolInput); implied != "" {
		explicit := strings.ToLower(strings.TrimSpace(payload.ListenHTTPVersionStrategy))
		if explicit != "" && explicit != implied {
			return reverseProxyNormalizedRule{}, common.NewError("listen protocol alias conflicts with listen http version strategy")
		}
		listenHTTPVersionInput = implied
	}
	listenHTTPVersionStrategy, err := normalizeReverseProxyListenHTTPVersionStrategy(listenHTTPVersionInput, normalized.listenProtocol)
	if err != nil {
		return reverseProxyNormalizedRule{}, err
	}
	normalized.listenHTTPVersionStrategy = listenHTTPVersionStrategy
	normalized.advertiseHTTP3 = normalized.advertiseHTTP3 &&
		normalized.listenProtocol == reverseProxyProtocolHTTPS &&
		normalized.listenHTTPVersionStrategy == reverseProxyListenHTTPVersionH2H3 &&
		!reverseProxyIsWebSocketAlias(normalized.listenProtocolAlias)

	httpVersionInput := payload.HTTPVersionStrategy
	if implied := reverseProxyTargetProtocolAliasStrategy(targetProtocolInput); implied != "" {
		explicit := strings.ToLower(strings.TrimSpace(payload.HTTPVersionStrategy))
		if explicit != "" && explicit != implied {
			return reverseProxyNormalizedRule{}, common.NewError("target protocol alias conflicts with http version strategy")
		}
		httpVersionInput = implied
	}
	httpVersionStrategy, err := normalizeReverseProxyHTTPVersionStrategy(httpVersionInput, normalized.targetProtocol)
	if err != nil {
		return reverseProxyNormalizedRule{}, err
	}
	normalized.httpVersionStrategy = httpVersionStrategy
	if normalized.listenProtocol == reverseProxyProtocolHTTPS &&
		!reverseProxyIsWebSocketAlias(normalized.listenProtocolAlias) &&
		reverseProxyIsWebSocketAlias(normalized.targetProtocolAlias) {
		return reverseProxyNormalizedRule{}, common.NewError("strict HTTPS H2/H3 listener cannot proxy a websocket target; use WSS or HTTP listener")
	}
	normalized.upstreamTLSVerify = reverseProxyPayloadTLSVerify(payload.UpstreamTLSVerify, payload.upstreamTLSVerifyDecoded, payload.upstreamTLSVerifySet, normalized.targetProtocol == reverseProxyProtocolHTTPS)

	if normalized.listenProtocol == reverseProxyProtocolHTTPS {
		certIDs := normalizeReverseProxyCertificateIDList(payload.CertificateRecordIDs, payload.CertificateRecordID)
		if len(certIDs) == 0 {
			return reverseProxyNormalizedRule{}, common.NewError("https listener requires certificate")
		}
		normalized.certificateRecordIDs = certIDs
		normalized.certificateRecordID = certIDs[0]
	} else {
		normalized.certificateRecordIDs = []uint{}
		normalized.certificateRecordID = 0
	}

	if normalized.targetProtocol == reverseProxyProtocolHTTP {
		normalized.httpVersionStrategy = ""
		normalized.upstreamTLSVerify = false
	}
	if err := validateReverseProxyRuleResourceSettings(normalized); err != nil {
		return reverseProxyNormalizedRule{}, err
	}
	return normalized, nil
}

func validateReverseProxyRuleResourceSettings(row reverseProxyNormalizedRule) error {
	if row.maxConcurrentConnections < 0 || row.maxConcurrentConnections > reverseProxyMaximumConfiguredLimit {
		return common.NewError("max concurrent connections must be between 0 and ", reverseProxyMaximumConfiguredLimit)
	}
	if row.maxConcurrentRequests < 0 || row.maxConcurrentRequests > reverseProxyMaxConcurrentRequestLimit {
		return common.NewError("max concurrent requests must be between 0 and ", reverseProxyMaxConcurrentRequestLimit)
	}
	if row.dnsMaxConcurrentQueries < 0 || row.dnsMaxConcurrentQueries > reverseProxyDNSMaxConcurrentQueryLimit {
		return common.NewError("dns max concurrent queries must be between 0 and ", reverseProxyDNSMaxConcurrentQueryLimit)
	}
	if row.upstreamMaxConnections < 0 || row.upstreamMaxConnections > reverseProxyMaximumConfiguredLimit {
		return common.NewError("upstream max connections must be between 0 and ", reverseProxyMaximumConfiguredLimit)
	}
	if row.upstreamMaxIdleConnections < 0 || row.upstreamMaxIdleConnections > reverseProxyMaximumConfiguredLimit {
		return common.NewError("upstream max idle connections must be between 0 and ", reverseProxyMaximumConfiguredLimit)
	}
	if row.memoryLimitBytes < 0 || row.memoryLimitBytes > reverseProxyMaximumMemoryPoolBytes {
		return common.NewError("memory limit is invalid")
	}
	return nil
}

func reverseProxyPayloadTLSVerify(value bool, decodedFromJSON bool, explicitlySet bool, tlsTarget bool) bool {
	if !tlsTarget {
		return false
	}
	if decodedFromJSON && !explicitlySet {
		return true
	}
	return value
}

func (s *ReverseProxyService) validateNormalizedRule(db *gorm.DB, row reverseProxyNormalizedRule) error {
	if db == nil {
		return nil
	}
	if err := validateReverseProxyNoObviousLoop(row); err != nil {
		return err
	}
	settings, err := loadReverseProxySettingsTx(db)
	if err != nil {
		return err
	}
	if err := validateReverseProxyRuleMemoryAgainstSettings(row, settings); err != nil {
		return err
	}
	certificateIDs, err := validateReverseProxyCertificateRequirements(row)
	if err != nil {
		return err
	}
	if err := validateReverseProxyCertificateReferences(db, certificateIDs); err != nil {
		return err
	}
	if reverseProxyProtocolIsDNS(row.listenProtocolAlias) {
		return s.validateNormalizedDNSRuleMetadata(db, row)
	}

	rows := make([]model.ReverseProxyRule, 0)
	if err := db.Select(
		"id",
		"listen_port",
		"listen_protocol",
		"listen_protocol_alias",
		"listen_http_version_strategy",
		"host_list",
		"path_prefix",
		"listen_dns_path",
	).Where("id <> ?", row.id).Find(&rows).Error; err != nil {
		return err
	}
	for _, existing := range rows {
		if existing.ListenPort != row.listenPort {
			continue
		}
		existingListenAlias := normalizeReverseProxyProtocolAlias(existing.ListenProtocolAlias, existing.ListenProtocol)
		if !reverseProxyProtocolsShareUnderlyingSocket(existing.ListenProtocol, existing.ListenHTTPVersionStrategy, row.listenProtocol, row.listenHTTPVersionStrategy, existingListenAlias, row.listenProtocolAlias) {
			continue
		}
		if reverseProxyProtocolIsDNS(existingListenAlias) {
			if !reverseProxyListenIPSetsOverlap(reverseProxyDNSRuntimeListenIPs(&existing), reverseProxyNormalizedHTTPRuntimeListenIPs(row)) {
				continue
			}
			if reverseProxyIsHTTPDNSAlias(existingListenAlias) && row.listenProtocol == reverseProxyProtocolHTTPS &&
				!reverseProxyExistingNormalizedHTTPConditionsOverlap(&existing, row) {
				continue
			}
			return common.NewError("reverse proxy listener conflicts with existing dns listener on the same port")
		}
		if !reverseProxyListenIPSetsOverlap(reverseProxyHTTPRuntimeListenIPs(&existing), reverseProxyNormalizedHTTPRuntimeListenIPs(row)) {
			continue
		}
		if existing.ListenProtocol != row.listenProtocol {
			return common.NewError("reverse proxy listener conflicts with existing reverse proxy listener on the same port")
		}
		if !reverseProxyExistingNormalizedHTTPConditionsOverlap(&existing, row) {
			continue
		}
		return common.NewError("reverse proxy rule conflicts with existing host/path on the same listener")
	}
	return nil
}

func validateReverseProxyRuleMemoryAgainstSettings(row reverseProxyNormalizedRule, settings *model.ReverseProxySettings) error {
	resources := reverseProxySettingsView(settings)
	effective := row.memoryLimitBytes
	if effective == 0 {
		effective = resources.DefaultRuleMemoryLimitBytes
	}
	if effective < reverseProxyMinimumRewriteReservationBytes || effective > resources.MemoryPoolBytes {
		return common.NewError("rule memory limit must fit in the shared reverse proxy memory pool")
	}
	if reverseProxyProtocolIsDNS(row.listenProtocolAlias) && row.dnsCacheEnabled && int64(row.dnsCacheSizeBytes) > effective {
		return common.NewError("dns cache size must not exceed the effective rule memory limit")
	}
	return nil
}

func reverseProxyNormalizedCertificateIDs(row reverseProxyNormalizedRule) []uint {
	ids := append([]uint(nil), row.certificateRecordIDs...)
	if len(ids) == 0 && row.certificateRecordID > 0 {
		ids = []uint{row.certificateRecordID}
	}
	if len(ids) < 2 {
		return ids
	}
	seen := make(map[uint]struct{}, len(ids))
	result := make([]uint, 0, len(ids))
	for _, id := range ids {
		if id == 0 {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	return result
}

// validateReverseProxyCertificateRequirements performs only deterministic
// shape checks. It is safe to run both before and inside the SQLite write
// transaction; loading and parsing PEM material is intentionally separate.
func validateReverseProxyCertificateRequirements(row reverseProxyNormalizedRule) ([]uint, error) {
	certificateIDs := reverseProxyNormalizedCertificateIDs(row)
	if reverseProxyProtocolIsDNS(row.listenProtocolAlias) {
		if reverseProxyDNSProtocolUsesTLS(row.listenProtocolAlias) {
			if len(certificateIDs) == 0 {
				return nil, common.NewError("dns tls listener requires certificate")
			}
			return certificateIDs, nil
		}
		if len(certificateIDs) > 0 {
			return nil, common.NewError("plain dns listener cannot bind certificate")
		}
		return nil, nil
	}
	if row.listenProtocol == reverseProxyProtocolHTTP {
		if len(certificateIDs) > 0 {
			return nil, common.NewError("http listener cannot bind certificate")
		}
		return nil, nil
	}
	if row.listenProtocol == reverseProxyProtocolHTTPS {
		if len(certificateIDs) == 0 {
			return nil, common.NewError("https listener requires certificate")
		}
		return certificateIDs, nil
	}
	return certificateIDs, nil
}

// validateReverseProxyCertificateReferences keeps the transaction short: it
// reads only primary keys, never certificate BLOBs or X.509 structures.
func validateReverseProxyCertificateReferences(db *gorm.DB, certificateIDs []uint) error {
	if len(certificateIDs) == 0 {
		return nil
	}
	if db == nil {
		db = database.GetDB()
	}
	if db == nil {
		return common.NewError("database is not ready")
	}
	requested := make(map[uint]struct{}, len(certificateIDs))
	for _, certificateID := range certificateIDs {
		if certificateID == 0 {
			return common.NewError("certificate id is required")
		}
		requested[certificateID] = struct{}{}
	}
	ids := make([]uint, 0, len(requested))
	for certificateID := range requested {
		ids = append(ids, certificateID)
	}
	type certificateIDRow struct {
		ID uint `gorm:"column:id"`
	}
	rows := make([]certificateIDRow, 0, len(ids))
	if err := db.Model(&model.CertificateRecord{}).Select("id").Where("id IN ?", ids).Find(&rows).Error; err != nil {
		return err
	}
	found := make(map[uint]struct{}, len(rows))
	for _, row := range rows {
		found[row.ID] = struct{}{}
	}
	for certificateID := range requested {
		if _, exists := found[certificateID]; !exists {
			return common.NewError("certificate not found")
		}
	}
	return nil
}

// preflightReverseProxyCertificateMaterial runs before UpsertRule opens its
// SQLite write transaction. Parsing the key pair here also catches corrupt
// material early without holding the single SQLite connection during CPU- and
// allocation-heavy X.509 work.
func preflightReverseProxyCertificateMaterial(certificateIDs []uint) error {
	materials, err := loadReverseProxyParsedCertificateMaterials(certificateIDs, false)
	if err != nil {
		return err
	}
	for _, certificateID := range certificateIDs {
		material, exists := materials[certificateID]
		if !exists {
			return common.NewError("certificate not found")
		}
		if material.Err != nil {
			return common.NewError("certificate material is invalid: ", material.Err)
		}
	}
	return nil
}

func (s *ReverseProxyService) preflightNormalizedRule(row reverseProxyNormalizedRule) error {
	if err := validateReverseProxyNoObviousLoop(row); err != nil {
		return err
	}
	certificateIDs, err := validateReverseProxyCertificateRequirements(row)
	if err != nil {
		return err
	}
	if err := preflightReverseProxyCertificateMaterial(certificateIDs); err != nil {
		return err
	}
	if reverseProxyProtocolIsDNS(row.listenProtocolAlias) {
		// dnsproxy parses fallback upstream syntax and may construct resolver
		// helpers. Keep that work outside the later SQLite write transaction.
		if err := validateReverseProxyDNSFallbackUpstreams(row); err != nil {
			return err
		}
	}
	return s.validateReverseProxyResolvedLoop(row)
}

func (s *ReverseProxyService) validateNormalizedDNSRule(db *gorm.DB, row reverseProxyNormalizedRule) error {
	certificateIDs, err := validateReverseProxyCertificateRequirements(row)
	if err != nil {
		return err
	}
	if err := validateReverseProxyCertificateReferences(db, certificateIDs); err != nil {
		return err
	}
	return s.validateNormalizedDNSRuleMetadata(db, row)
}

// validateNormalizedDNSRuleMetadata is the SQLite transaction-safe portion of
// DNS validation. Certificate material is deliberately loaded and parsed by
// preflightNormalizedRule before the transaction starts, while this function
// only checks rule metadata and listener conflicts.
func (s *ReverseProxyService) validateNormalizedDNSRuleMetadata(db *gorm.DB, row reverseProxyNormalizedRule) error {
	if !reverseProxyProtocolIsDNS(row.targetProtocolAlias) {
		return common.NewError("dns reverse proxy target protocol is invalid")
	}
	if err := validateReverseProxyDNSCacheSettings(row); err != nil {
		return err
	}
	if reverseProxyDNSProtocolUsesPath(row.listenProtocolAlias) && row.listenDNSPath == "" {
		return common.NewError("doh listener requires url path")
	}
	if reverseProxyDNSProtocolUsesPath(row.targetProtocolAlias) && row.targetDNSPath == "" {
		return common.NewError("doh target requires url path")
	}
	if !reverseProxyDNSProtocolUsesPath(row.listenProtocolAlias) && row.listenDNSPath != "" {
		return common.NewError("only doh / doh3 listener supports url path")
	}
	if !reverseProxyDNSProtocolUsesPath(row.targetProtocolAlias) && row.targetDNSPath != "" {
		return common.NewError("only doh / doh3 target supports url path")
	}
	if row.listenProtocol != reverseProxyProtocolDNS || row.targetProtocol != reverseProxyProtocolDNS {
		return common.NewError("dns reverse proxy must use dns protocol")
	}

	rows := make([]model.ReverseProxyRule, 0)
	if err := db.Select(
		"id",
		"listen_port",
		"listen_protocol",
		"listen_protocol_alias",
		"listen_http_version_strategy",
		"host_list",
		"path_prefix",
		"listen_dns_path",
	).Where("id <> ?", row.id).Find(&rows).Error; err != nil {
		return err
	}
	for _, existing := range rows {
		if existing.ListenPort != row.listenPort {
			continue
		}
		existingListenAlias := normalizeReverseProxyProtocolAlias(existing.ListenProtocolAlias, existing.ListenProtocol)
		if reverseProxyProtocolIsDNS(existingListenAlias) {
			if !reverseProxyDNSProtocolSharesSocket(existingListenAlias, row.listenProtocolAlias) {
				continue
			}
			if !reverseProxyListenIPSetsOverlap(reverseProxyDNSRuntimeListenIPs(&existing), reverseProxyNormalizedDNSRuntimeListenIPs(row)) {
				continue
			}
			if reverseProxyIsHTTPDNSAlias(existingListenAlias) && reverseProxyIsHTTPDNSAlias(row.listenProtocolAlias) &&
				!reverseProxyExistingNormalizedHTTPConditionsOverlap(&existing, row) {
				continue
			}
			return common.NewError("dns reverse proxy listener conflicts with existing dns listener on the same port")
		}
		if !reverseProxyProtocolsShareUnderlyingSocket(existing.ListenProtocol, existing.ListenHTTPVersionStrategy, row.listenProtocol, row.listenHTTPVersionStrategy, existingListenAlias, row.listenProtocolAlias) ||
			!reverseProxyListenIPSetsOverlap(reverseProxyHTTPRuntimeListenIPs(&existing), reverseProxyNormalizedDNSRuntimeListenIPs(row)) {
			continue
		}
		if reverseProxyIsHTTPDNSAlias(row.listenProtocolAlias) && strings.EqualFold(existing.ListenProtocol, reverseProxyProtocolHTTPS) &&
			!reverseProxyExistingNormalizedHTTPConditionsOverlap(&existing, row) {
			continue
		}
		return common.NewError("dns reverse proxy listener conflicts with existing reverse proxy listener on the same port")
	}
	return nil
}

func reverseProxyDNSListenersCanSharePathSocket(existing *model.ReverseProxyRule, row reverseProxyNormalizedRule, existingAlias string) bool {
	if existing == nil {
		return false
	}
	if existingAlias != row.listenProtocolAlias {
		return false
	}
	if !reverseProxyDNSProtocolUsesPath(row.listenProtocolAlias) {
		return false
	}
	if !reverseProxyListenIPSetsEqual(reverseProxyDNSRuntimeListenIPs(existing), reverseProxyNormalizedDNSRuntimeListenIPs(row)) {
		return false
	}
	existingPath := normalizeReverseProxyDNSPath(existing.ListenDNSPath)
	newPath := normalizeReverseProxyDNSPath(row.listenDNSPath)
	if existingPath == "" {
		existingPath = "/dns-query"
	}
	if newPath == "" {
		newPath = "/dns-query"
	}
	return existingPath != newPath
}

func reverseProxyListenIPSetsEqual(a []string, b []string) bool {
	normalize := func(items []string) []string {
		if len(items) == 0 {
			items = []string{"0.0.0.0"}
		}
		out := make([]string, 0, len(items))
		seen := make(map[string]struct{}, len(items))
		for _, item := range items {
			value := strings.ToLower(strings.TrimSpace(item))
			if value == "" {
				continue
			}
			if _, exists := seen[value]; exists {
				continue
			}
			seen[value] = struct{}{}
			out = append(out, value)
		}
		if len(out) == 0 {
			out = append(out, "0.0.0.0")
		}
		sort.Strings(out)
		return out
	}
	left := normalize(a)
	right := normalize(b)
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func loadReverseProxyCertificateRecord(db *gorm.DB, id uint) (*model.CertificateRecord, error) {
	if id == 0 {
		return nil, common.NewError("certificate id is required")
	}
	if db == nil {
		db = database.GetDB()
	}
	if db == nil {
		return nil, common.NewError("database is not ready")
	}
	row := &model.CertificateRecord{}
	if err := db.Where("id = ?", id).First(row).Error; err != nil {
		return nil, err
	}
	return row, nil
}

func validateReverseProxyNoObviousLoop(row reverseProxyNormalizedRule) error {
	return nil
}

func (s *ReverseProxyService) validateReverseProxyResolvedLoop(row reverseProxyNormalizedRule) error {
	if row.listenPort <= 0 || row.targetPort <= 0 || row.listenPort != row.targetPort || len(row.targetAddresses) == 0 {
		return nil
	}
	listenIPs := reverseProxyNormalizedHTTPRuntimeListenIPs(row)
	if reverseProxyProtocolIsDNS(row.listenProtocolAlias) {
		listenIPs = reverseProxyNormalizedDNSRuntimeListenIPs(row)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	for _, target := range row.targetAddresses {
		candidates, err := s.resolveTargetCandidates(ctx, target, row.targetPort, row.ipStrategy)
		if err != nil {
			// A temporary DNS failure must not make an otherwise valid rule
			// unsavable. Runtime resolution applies the same guard before dialing.
			continue
		}
		for _, candidate := range candidates {
			if reverseProxyResolvedTargetLoopsToListener(listenIPs, row.listenPort, row.targetPort, candidate.address) {
				return common.NewError("target address resolves back to the local listener")
			}
		}
	}
	return nil
}

func reverseProxyResolvedTargetLoopsToListener(listenIPs []string, listenPort int, targetPort int, targetAddress string) bool {
	if listenPort <= 0 || targetPort <= 0 || listenPort != targetPort {
		return false
	}
	target, err := netip.ParseAddr(strings.Trim(strings.TrimSpace(targetAddress), "[]"))
	if err != nil {
		return false
	}
	target = target.Unmap()
	for _, item := range listenIPs {
		listenAddr, parseErr := netip.ParseAddr(strings.Trim(strings.TrimSpace(item), "[]"))
		if parseErr != nil {
			continue
		}
		listenAddr = listenAddr.Unmap()
		if listenAddr == target {
			return true
		}
		if listenAddr.IsUnspecified() && sameReverseProxyIPFamily(listenAddr, target) && reverseProxyAddressIsLocal(target) {
			return true
		}
	}
	return false
}

func sameReverseProxyIPFamily(left netip.Addr, right netip.Addr) bool {
	return left.Is4() == right.Is4()
}

func reverseProxyAddressIsLocal(address netip.Addr) bool {
	if !address.IsValid() {
		return false
	}
	if address.IsLoopback() || address.IsUnspecified() {
		return true
	}
	interfaces, err := net.InterfaceAddrs()
	if err != nil {
		return false
	}
	for _, item := range interfaces {
		var candidate netip.Addr
		switch value := item.(type) {
		case *net.IPNet:
			candidate, _ = netip.AddrFromSlice(value.IP)
		case *net.IPAddr:
			candidate, _ = netip.AddrFromSlice(value.IP)
		}
		if candidate.IsValid() && candidate.Unmap() == address.Unmap() {
			return true
		}
	}
	return false
}

func (s *ReverseProxyService) repairDisplayIDsTx(db *gorm.DB) error {
	rows := make([]model.ReverseProxyRule, 0)
	if err := db.Order("id asc").Find(&rows).Error; err != nil {
		return err
	}
	return repairReverseProxyRowsTx(db, rows)
}

func repairReverseProxyRowsTx(db *gorm.DB, rows []model.ReverseProxyRule) error {
	if db == nil {
		return nil
	}
	usedDisplayIDs := make(map[uint64]struct{}, len(rows))
	needsRepair := false
	for i := range rows {
		if rows[i].DisplayID < reverseProxyDisplayIDMin || rows[i].DisplayID > reverseProxyDisplayIDMax {
			needsRepair = true
			break
		}
		if _, exists := usedDisplayIDs[rows[i].DisplayID]; exists {
			needsRepair = true
			break
		}
		usedDisplayIDs[rows[i].DisplayID] = struct{}{}
		if rows[i].ListOrder <= 0 {
			needsRepair = true
			break
		}
	}
	if !needsRepair {
		return nil
	}

	usedDisplayIDs = make(map[uint64]struct{}, len(rows))
	for i := range rows {
		if rows[i].ListOrder <= 0 {
			rows[i].ListOrder = int64(i + 1)
		}
		if rows[i].DisplayID >= reverseProxyDisplayIDMin && rows[i].DisplayID <= reverseProxyDisplayIDMax {
			if _, exists := usedDisplayIDs[rows[i].DisplayID]; !exists {
				usedDisplayIDs[rows[i].DisplayID] = struct{}{}
				continue
			}
		}
		nextID, err := allocateReverseProxyDisplayID(usedDisplayIDs)
		if err != nil {
			return err
		}
		rows[i].DisplayID = nextID
		usedDisplayIDs[nextID] = struct{}{}
	}

	for i := range rows {
		if err := db.Model(&model.ReverseProxyRule{}).
			Where("id = ?", rows[i].Id).
			Updates(map[string]interface{}{
				"display_id": rows[i].DisplayID,
				"list_order": rows[i].ListOrder,
			}).Error; err != nil {
			return err
		}
	}
	return nil
}

func (s *ReverseProxyService) allocateNextDisplayIDTx(db *gorm.DB) (uint64, error) {
	type displayIDRow struct {
		DisplayID uint64 `gorm:"column:display_id"`
	}
	rows := make([]displayIDRow, 0)
	if err := db.Model(&model.ReverseProxyRule{}).Select("display_id").Where("display_id > 0").Order("display_id asc").Find(&rows).Error; err != nil {
		return 0, err
	}
	used := make(map[uint64]struct{}, len(rows))
	for i := range rows {
		used[rows[i].DisplayID] = struct{}{}
	}
	return allocateReverseProxyDisplayID(used)
}

func (s *ReverseProxyService) nextListOrderTx(db *gorm.DB) (int64, error) {
	type result struct {
		Max int64 `gorm:"column:max_order"`
	}
	out := result{}
	if err := db.Model(&model.ReverseProxyRule{}).Select("COALESCE(MAX(list_order), 0) AS max_order").Scan(&out).Error; err != nil {
		return 0, err
	}
	return out.Max + 1, nil
}

func allocateReverseProxyDisplayID(used map[uint64]struct{}) (uint64, error) {
	for candidate := reverseProxyDisplayIDMin; candidate <= reverseProxyDisplayIDMax; candidate++ {
		if _, exists := used[candidate]; exists {
			continue
		}
		return candidate, nil
	}
	return 0, common.NewError("reverse proxy display id exhausted")
}

func normalizeReverseProxyProtocol(raw string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case reverseProxyProtocolHTTP, "ws":
		return reverseProxyProtocolHTTP, nil
	case reverseProxyProtocolHTTPS, "h2", "h3", "wss":
		return reverseProxyProtocolHTTPS, nil
	case reverseProxyProtocolDNS, reverseProxyDNSProtocolDoH, reverseProxyDNSProtocolDoHH3, reverseProxyDNSProtocolDoQ, reverseProxyDNSProtocolDoT, reverseProxyDNSProtocolUDP, reverseProxyDNSProtocolTCP:
		return reverseProxyProtocolDNS, nil
	default:
		return "", common.NewError("protocol must be http, https, h2, h3, ws, wss, dns, dns_doh, dns_doh3, dns_doq, dns_dot, dns_udp, or dns_tcp")
	}
}

func normalizeReverseProxyProtocolAlias(alias string, protocolRaw string) string {
	rawAlias := strings.ToLower(strings.TrimSpace(alias))
	switch rawAlias {
	case "ws", "wss", reverseProxyDNSProtocolDoH, reverseProxyDNSProtocolDoHH3, reverseProxyDNSProtocolDoQ, reverseProxyDNSProtocolDoT, reverseProxyDNSProtocolUDP, reverseProxyDNSProtocolTCP:
		return rawAlias
	}

	rawProtocol := strings.ToLower(strings.TrimSpace(protocolRaw))
	switch rawProtocol {
	case "ws", "wss", reverseProxyDNSProtocolDoH, reverseProxyDNSProtocolDoHH3, reverseProxyDNSProtocolDoQ, reverseProxyDNSProtocolDoT, reverseProxyDNSProtocolUDP, reverseProxyDNSProtocolTCP:
		return rawProtocol
	default:
		return ""
	}
}

func reverseProxyListenProtocolAliasStrategy(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "h2":
		return reverseProxyListenHTTPVersionH2Only
	case "h3":
		return reverseProxyListenHTTPVersionH3Only
	default:
		return ""
	}
}

func reverseProxyTargetProtocolAliasStrategy(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "h2":
		return reverseProxyHTTPVersionH2Only
	case "h3":
		return reverseProxyHTTPVersionH3Only
	default:
		return ""
	}
}

func normalizeReverseProxyIPStrategy(raw string) (string, error) {
	value := strings.ToLower(strings.TrimSpace(raw))
	switch value {
	case "", reverseProxyIPStrategyPreferIPv4:
		return reverseProxyIPStrategyPreferIPv4, nil
	case reverseProxyIPStrategyIPv4Only, reverseProxyIPStrategyIPv6Only, reverseProxyIPStrategyPreferIPv6:
		return value, nil
	default:
		return "", common.NewError("invalid ip strategy")
	}
}

func normalizeReverseProxyEDNSMode(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case reverseProxyEDNSModeCustom:
		return reverseProxyEDNSModeCustom
	default:
		return reverseProxyEDNSModeAuto
	}
}

func normalizeReverseProxyEDNSClientSubnetPolicy(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case reverseProxyEDNSClientSubnetPolicyPreferRequestPublic:
		return reverseProxyEDNSClientSubnetPolicyPreferRequestPublic
	default:
		return reverseProxyEDNSClientSubnetPolicyClientIP
	}
}

func normalizeReverseProxyListenHTTPVersionStrategy(raw string, listenProtocol string) (string, error) {
	if listenProtocol != reverseProxyProtocolHTTPS {
		return "", nil
	}
	value := strings.ToLower(strings.TrimSpace(raw))
	switch value {
	case "", reverseProxyListenHTTPVersionH2H3:
		return reverseProxyListenHTTPVersionH2H3, nil
	case reverseProxyListenHTTPVersionH2Only, reverseProxyListenHTTPVersionH3Only:
		return value, nil
	default:
		return "", common.NewError("invalid listen http version strategy")
	}
}

func normalizeReverseProxyHTTPVersionStrategy(raw string, targetProtocol string) (string, error) {
	if targetProtocol != reverseProxyProtocolHTTPS {
		return "", nil
	}
	value := strings.ToLower(strings.TrimSpace(raw))
	switch value {
	case "", reverseProxyHTTPVersionPreferH2:
		return reverseProxyHTTPVersionPreferH2, nil
	case reverseProxyHTTPVersionH2Only, reverseProxyHTTPVersionH3Only, reverseProxyHTTPVersionPreferH3, reverseProxyHTTPVersionDualRequiredPreferH3:
		return value, nil
	default:
		return "", common.NewError("invalid http version strategy")
	}
}

func reverseProxyProtocolIsDNS(alias string) bool {
	switch strings.ToLower(strings.TrimSpace(alias)) {
	case reverseProxyDNSProtocolDoH,
		reverseProxyDNSProtocolDoHH3,
		reverseProxyDNSProtocolDoQ,
		reverseProxyDNSProtocolDoT,
		reverseProxyDNSProtocolUDP,
		reverseProxyDNSProtocolTCP:
		return true
	default:
		return false
	}
}

func reverseProxyIsHTTPDNSAlias(alias string) bool {
	switch strings.ToLower(strings.TrimSpace(alias)) {
	case reverseProxyDNSProtocolDoH, reverseProxyDNSProtocolDoHH3:
		return true
	default:
		return false
	}
}

func reverseProxyListenerGroupProtocol(row *model.ReverseProxyRule) string {
	if row == nil {
		return ""
	}
	alias := normalizeReverseProxyProtocolAlias(row.ListenProtocolAlias, row.ListenProtocol)
	if reverseProxyIsHTTPDNSAlias(alias) {
		return reverseProxyProtocolHTTPS
	}
	return strings.ToLower(strings.TrimSpace(row.ListenProtocol))
}

func reverseProxyDNSProtocolUsesPath(alias string) bool {
	switch strings.ToLower(strings.TrimSpace(alias)) {
	case reverseProxyDNSProtocolDoH, reverseProxyDNSProtocolDoHH3:
		return true
	default:
		return false
	}
}

func reverseProxyDNSProtocolUsesTLS(alias string) bool {
	switch strings.ToLower(strings.TrimSpace(alias)) {
	case reverseProxyDNSProtocolDoH, reverseProxyDNSProtocolDoHH3, reverseProxyDNSProtocolDoQ, reverseProxyDNSProtocolDoT:
		return true
	default:
		return false
	}
}

func normalizeReverseProxyDNSPath(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return ""
	}
	if !strings.HasPrefix(value, "/") {
		value = "/" + value
	}
	for len(value) > 1 && strings.HasSuffix(value, "/") {
		value = strings.TrimSuffix(value, "/")
	}
	return value
}

func reverseProxyDNSProtocolUsesTCP(alias string) bool {
	switch strings.ToLower(strings.TrimSpace(alias)) {
	case reverseProxyDNSProtocolDoH, reverseProxyDNSProtocolDoT, reverseProxyDNSProtocolTCP:
		return true
	default:
		return false
	}
}

func reverseProxyDNSProtocolUsesUDP(alias string) bool {
	switch strings.ToLower(strings.TrimSpace(alias)) {
	case reverseProxyDNSProtocolDoHH3, reverseProxyDNSProtocolDoQ, reverseProxyDNSProtocolUDP:
		return true
	default:
		return false
	}
}

func reverseProxyDNSProtocolSharesSocket(a string, b string) bool {
	return (reverseProxyDNSProtocolUsesTCP(a) && reverseProxyDNSProtocolUsesTCP(b)) ||
		(reverseProxyDNSProtocolUsesUDP(a) && reverseProxyDNSProtocolUsesUDP(b))
}

func reverseProxyListenerUsesUnderlyingSockets(protocol string, listenStrategy string, alias string) (bool, bool) {
	if reverseProxyProtocolIsDNS(alias) {
		return reverseProxyDNSProtocolUsesTCP(alias), reverseProxyDNSProtocolUsesUDP(alias)
	}
	if reverseProxyIsWebSocketAlias(alias) {
		return true, false
	}
	return reverseProxyHTTPListenerUsesSockets(protocol, listenStrategy)
}

func reverseProxyProtocolsShareUnderlyingSocket(existingProtocol string, existingListenStrategy string, newProtocol string, newListenStrategy string, existingAlias string, newAlias string) bool {
	existingTCP, existingUDP := reverseProxyListenerUsesUnderlyingSockets(existingProtocol, existingListenStrategy, existingAlias)
	newTCP, newUDP := reverseProxyListenerUsesUnderlyingSockets(newProtocol, newListenStrategy, newAlias)
	return (existingTCP && newTCP) || (existingUDP && newUDP)
}

func reverseProxyHTTPListenerUsesSockets(protocol string, listenStrategy string) (bool, bool) {
	if strings.EqualFold(strings.TrimSpace(protocol), reverseProxyProtocolHTTPS) {
		normalized, err := normalizeReverseProxyListenHTTPVersionStrategy(listenStrategy, reverseProxyProtocolHTTPS)
		if err != nil {
			normalized = reverseProxyListenHTTPVersionH2H3
		}
		switch normalized {
		case reverseProxyListenHTTPVersionH2Only:
			return true, false
		case reverseProxyListenHTTPVersionH3Only:
			return false, true
		default:
			return true, true
		}
	}
	return true, false
}

// reverseProxyHTTPSListenerNextProtos describes the protocols that the TCP
// side of a TLS listener may negotiate.  HTTP/1.1 is deliberately limited to
// WSS routes because ordinary HTTPS, H2-only, and H2+H3 routes must not expose
// an HTTP/1.1 service on the secure listener.
func reverseProxyHTTPSListenerNextProtos(rules []*model.ReverseProxyRule) []string {
	hasWebSocket := false
	hasHTTP2 := false
	for _, rule := range rules {
		if rule == nil {
			continue
		}
		alias := normalizeReverseProxyProtocolAlias(rule.ListenProtocolAlias, rule.ListenProtocol)
		if reverseProxyIsWebSocketAlias(alias) {
			hasWebSocket = true
			continue
		}
		hasHTTP2 = true
	}
	if !hasHTTP2 && hasWebSocket {
		return []string{"http/1.1"}
	}
	if hasWebSocket {
		return []string{"h2", "http/1.1"}
	}
	return []string{"h2"}
}

// reverseProxyRequireHTTP2ALPN rejects TLS clients that do not offer H2.  A
// TLS client without ALPN can otherwise be accepted by net/http and handled
// as HTTP/1.1 even when NextProtos contains only "h2".
func reverseProxyRequireHTTP2ALPN(hello *tls.ClientHelloInfo) (*tls.Config, error) {
	if hello != nil {
		for _, protocol := range hello.SupportedProtos {
			if protocol == "h2" {
				return nil, nil
			}
		}
	}
	return nil, errors.New("reverse proxy HTTPS listener requires ALPN h2")
}

const (
	reverseProxyTokenModeServerName = iota + 1
	reverseProxyTokenModeListenName
	reverseProxyTokenModeHost
	reverseProxyTokenModeTarget
)

func normalizeReverseProxyTokenInput(token string, mode int) string {
	token = strings.TrimSpace(token)
	switch mode {
	case reverseProxyTokenModeServerName, reverseProxyTokenModeListenName, reverseProxyTokenModeHost, reverseProxyTokenModeTarget:
		lower := strings.ToLower(token)
		switch {
		case strings.HasPrefix(lower, "http://"):
			token = token[len("http://"):]
		case strings.HasPrefix(lower, "https://"):
			token = token[len("https://"):]
		}
	}
	return strings.TrimSpace(token)
}

func normalizeReverseProxyTokens(raw string, mode int) ([]string, error) {
	fields := splitReverseProxyTokenFields(raw)
	seen := make(map[string]struct{}, len(fields))
	result := make([]string, 0, len(fields))
	for _, field := range fields {
		token := normalizeReverseProxyTokenInput(field, mode)
		if reverseProxyTokenHasExplicitPort(token) {
			switch mode {
			case reverseProxyTokenModeTarget:
				return nil, common.NewError("target addresses must not include port; use the target port field")
			default:
				return nil, common.NewError("listen names must not include port; use the listen port field")
			}
		}
		token = strings.Trim(token, "[]")
		if token == "" {
			continue
		}
		lower := strings.ToLower(token)
		switch mode {
		case reverseProxyTokenModeServerName:
			if strings.Contains(lower, "*") {
				if !reverseProxyIsStandardWildcardHost(lower) {
					return nil, common.NewError("sni wildcard must follow *.example.com format")
				}
			} else if reverseProxyParseIPLiteral(lower) == nil && (!reverseProxyHostTokenRe.MatchString(lower) || !reverseProxyLooksLikeHost(lower)) {
				return nil, common.NewError("sni names must be domain or ip")
			}
		case reverseProxyTokenModeListenName:
			if strings.Contains(lower, "*") {
				if !reverseProxyIsStandardWildcardHost(lower) {
					return nil, common.NewError("listen wildcard must follow *.example.com format")
				}
			} else if reverseProxyParseIPLiteral(lower) != nil {
				return nil, common.NewError("listen names must be domain")
			} else if !reverseProxyHostTokenRe.MatchString(lower) || !reverseProxyLooksLikeHost(lower) {
				return nil, common.NewError("listen names must be domain")
			}
		case reverseProxyTokenModeHost:
			if strings.Contains(lower, "*") {
				if !reverseProxyIsStandardWildcardHost(lower) {
					return nil, common.NewError("hosts wildcard must follow *.example.com format")
				}
			} else if reverseProxyParseIPLiteral(lower) != nil {
				return nil, common.NewError("hosts must be domain")
			} else if !reverseProxyHostTokenRe.MatchString(lower) || !reverseProxyLooksLikeHost(lower) {
				return nil, common.NewError("hosts must be domain")
			}
		case reverseProxyTokenModeTarget:
			if strings.Contains(lower, "*") {
				return nil, common.NewError("target addresses do not support wildcards")
			}
			if !reverseProxyHostTokenRe.MatchString(lower) || !reverseProxyLooksLikeHost(lower) {
				return nil, common.NewError("target addresses must be domain or ip")
			}
		default:
			return nil, common.NewError("invalid token mode")
		}
		if _, exists := seen[lower]; exists {
			continue
		}
		seen[lower] = struct{}{}
		result = append(result, lower)
	}
	return result, nil
}

func splitReverseProxyTokenFields(raw string) []string {
	return strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\n' || r == '\r' || r == '\t'
	})
}

func collectReverseProxyLegacyListenNames(values []string, strict bool) ([]string, error) {
	result := make([]string, 0)
	seen := make(map[string]struct{})
	for _, raw := range values {
		for _, field := range splitReverseProxyTokenFields(raw) {
			token := strings.TrimSpace(field)
			if reverseProxyTokenHasExplicitPort(token) {
				if strict {
					return nil, common.NewError("listen names must not include port; use the listen port field")
				}
				continue
			}
			token = strings.Trim(token, "[]")
			if token == "" {
				continue
			}
			lower := strings.ToLower(token)
			if reverseProxyParseIPLiteral(lower) != nil {
				continue
			}
			if strings.Contains(lower, "*") {
				if !reverseProxyIsStandardWildcardHost(lower) {
					if strict {
						return nil, common.NewError("listen wildcard must follow *.example.com format")
					}
					continue
				}
			} else if !reverseProxyHostTokenRe.MatchString(lower) || !reverseProxyLooksLikeHost(lower) {
				if strict {
					return nil, common.NewError("listen names must be domain")
				}
				continue
			}
			if _, exists := seen[lower]; exists {
				continue
			}
			seen[lower] = struct{}{}
			result = append(result, lower)
		}
	}
	return result, nil
}

func normalizeReverseProxyLegacyListenNames(raw string) ([]string, error) {
	return collectReverseProxyLegacyListenNames([]string{raw}, true)
}

func reverseProxyTokenHasExplicitPort(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	if ip := net.ParseIP(strings.Trim(value, "[]")); ip != nil {
		return false
	}
	host, port, err := net.SplitHostPort(value)
	return err == nil && strings.TrimSpace(host) != "" && strings.TrimSpace(port) != ""
}

func normalizeReverseProxyPath(raw string, required bool) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		if required {
			return ""
		}
		return ""
	}
	if !strings.HasPrefix(value, "/") {
		value = "/" + value
	}
	return value
}

func reverseProxyLooksLikeHost(value string) bool {
	if net.ParseIP(value) != nil {
		return true
	}
	return strings.Contains(value, ".") || strings.EqualFold(value, "localhost")
}

func reverseProxyIsStandardWildcardHost(value string) bool {
	if !strings.HasPrefix(value, "*.") {
		return false
	}
	if strings.Count(value, "*") != 1 {
		return false
	}
	suffix := strings.TrimPrefix(value, "*.")
	if suffix == "" || !strings.Contains(suffix, ".") {
		return false
	}
	return reverseProxyHostTokenRe.MatchString(suffix)
}

func reverseProxyNormalizedServerNames(row reverseProxyNormalizedRule) []string {
	values := make([]string, 0, len(row.hosts))
	values = append(values, row.hosts...)
	return reverseProxyCleanServerNames(values)
}

func reverseProxyRuleServerNames(row *model.ReverseProxyRule) []string {
	if row == nil {
		return []string{}
	}
	return reverseProxyCleanServerNames(decodeReverseProxyList(row.HostList))
}

func reverseProxyCleanServerNames(values []string) []string {
	cleaned := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = reverseProxyNormalizeServerName(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		cleaned = append(cleaned, value)
	}
	return cleaned
}

func reverseProxyNormalizeServerName(value string) string {
	value = strings.TrimSpace(value)
	value = strings.Trim(value, "[]")
	value = strings.TrimSuffix(value, ".")
	return strings.ToLower(value)
}

func reverseProxyRuleNamesOverlap(a []string, b []string) bool {
	if len(a) == 0 || len(b) == 0 {
		return len(a) == 0 && len(b) == 0
	}
	for _, item := range a {
		for _, candidate := range b {
			if reverseProxyHostPatternMatches(item, candidate) || reverseProxyHostPatternMatches(candidate, item) {
				return true
			}
		}
	}
	return false
}

func reverseProxyRuleNameSetsAreSNIDisjoint(a []string, b []string) bool {
	return !reverseProxyRuleNamesOverlap(a, b)
}

func reverseProxyRulePathsOverlap(a string, b string) bool {
	a = reverseProxyNormalizePathPrefix(a)
	b = reverseProxyNormalizePathPrefix(b)
	if a == "" || b == "" {
		return true
	}
	if a == b {
		return true
	}
	return strings.HasPrefix(a, b+"/") || strings.HasPrefix(b, a+"/")
}

func reverseProxyHTTPPathConditionsOverlap(a string, aExact bool, b string, bExact bool) bool {
	if aExact {
		a = normalizeReverseProxyDNSPath(a)
		if a == "" {
			a = "/dns-query"
		}
	} else {
		a = reverseProxyNormalizePathPrefix(a)
	}
	if bExact {
		b = normalizeReverseProxyDNSPath(b)
		if b == "" {
			b = "/dns-query"
		}
	} else {
		b = reverseProxyNormalizePathPrefix(b)
	}
	if aExact && bExact {
		return a == b
	}
	if aExact {
		return b == "" || a == b || strings.HasPrefix(a, b+"/")
	}
	if bExact {
		return a == "" || b == a || strings.HasPrefix(b, a+"/")
	}
	return reverseProxyRulePathsOverlap(a, b)
}

func reverseProxyExistingNormalizedHTTPConditionsOverlap(existing *model.ReverseProxyRule, row reverseProxyNormalizedRule) bool {
	if existing == nil {
		return false
	}
	existingAlias := normalizeReverseProxyProtocolAlias(existing.ListenProtocolAlias, existing.ListenProtocol)
	existingDNSHTTP := reverseProxyIsHTTPDNSAlias(existingAlias)
	newDNSHTTP := reverseProxyIsHTTPDNSAlias(row.listenProtocolAlias)
	existingPath := existing.PathPrefix
	if existingDNSHTTP {
		existingPath = existing.ListenDNSPath
	}
	newPath := row.pathPrefix
	if newDNSHTTP {
		newPath = row.listenDNSPath
	}
	return reverseProxyRuleNamesOverlap(reverseProxyRuleServerNames(existing), reverseProxyNormalizedServerNames(row)) &&
		reverseProxyHTTPPathConditionsOverlap(existingPath, existingDNSHTTP, newPath, newDNSHTTP)
}

func encodeReverseProxyList(values []string) string {
	cleaned := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		cleaned = append(cleaned, value)
	}
	raw, _ := json.Marshal(cleaned)
	return string(raw)
}

func decodeReverseProxyList(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return []string{}
	}
	result := make([]string, 0)
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		return []string{}
	}
	cleaned := make([]string, 0, len(result))
	seen := make(map[string]struct{}, len(result))
	for _, item := range result {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		lower := strings.ToLower(item)
		if _, exists := seen[lower]; exists {
			continue
		}
		seen[lower] = struct{}{}
		cleaned = append(cleaned, lower)
	}
	return cleaned
}

func encodeReverseProxyUintList(values []uint) string {
	if len(values) == 0 {
		return ""
	}
	cleaned := make([]uint, 0, len(values))
	seen := make(map[uint]struct{}, len(values))
	for _, value := range values {
		if value == 0 {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		cleaned = append(cleaned, value)
	}
	if len(cleaned) == 0 {
		return ""
	}
	raw, _ := json.Marshal(cleaned)
	return string(raw)
}

func decodeReverseProxyUintList(raw string) []uint {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return []uint{}
	}
	values := make([]uint, 0)
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		return []uint{}
	}
	cleaned := make([]uint, 0, len(values))
	seen := make(map[uint]struct{}, len(values))
	for _, value := range values {
		if value == 0 {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		cleaned = append(cleaned, value)
	}
	return cleaned
}

func normalizeReverseProxyCertificateIDList(values []uint, legacy uint) []uint {
	source := make([]uint, 0, len(values)+1)
	source = append(source, values...)
	if legacy > 0 {
		source = append(source, legacy)
	}
	if len(source) == 0 {
		return []uint{}
	}
	cleaned := make([]uint, 0, len(source))
	seen := make(map[uint]struct{}, len(source))
	for _, value := range source {
		if value == 0 {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		cleaned = append(cleaned, value)
	}
	return cleaned
}

func reverseProxyRuleCertificateIDs(row *model.ReverseProxyRule) []uint {
	if row == nil {
		return []uint{}
	}
	ids := decodeReverseProxyUintList(row.CertificateRecordList)
	if len(ids) > 0 {
		return ids
	}
	if row.CertificateRecordID > 0 {
		return []uint{row.CertificateRecordID}
	}
	return []uint{}
}

func buildReverseProxyCertificateHints(hosts []string, certs []ReverseProxyCertificateOption) []string {
	hints := make([]string, 0)
	if len(certs) == 0 || len(hosts) == 0 {
		return hints
	}
	certDomains := make([]string, 0, len(certs)*3)
	for _, cert := range certs {
		mainDomain := strings.ToLower(strings.TrimSpace(cert.MainDomain))
		if mainDomain != "" {
			certDomains = append(certDomains, mainDomain)
		}
		for _, item := range cert.Domains {
			value := strings.ToLower(strings.TrimSpace(item))
			if value == "" {
				continue
			}
			certDomains = append(certDomains, value)
		}
	}
	if len(certDomains) == 0 {
		for _, host := range hosts {
			if reverseProxyParseIPLiteral(host) != nil {
				hints = append(hints, "证书未覆盖 IP: "+host)
				continue
			}
			hints = append(hints, "证书未覆盖域名: "+host)
		}
		return hints
	}
	for _, host := range hosts {
		if reverseProxyParseIPLiteral(host) != nil {
			if !reverseProxyCertificateDomainsCoverIP(certDomains, host) {
				hints = append(hints, "证书未覆盖 IP: "+host)
			}
			continue
		}
		if !reverseProxyCertificateDomainsCoverHost(certDomains, host) {
			hints = append(hints, "证书未覆盖域名: "+host)
		}
	}
	return hints
}

func reverseProxyLeafMatchesServerName(leaf *x509.Certificate, serverName string) bool {
	if leaf == nil {
		return false
	}
	serverName = reverseProxyNormalizeServerName(serverName)
	if serverName == "" {
		return false
	}
	return leaf.VerifyHostname(serverName) == nil
}

func reverseProxyCertificateBindingHasIPSAN(binding *reverseProxyRuleCertificateBinding) bool {
	if binding == nil || binding.Leaf == nil {
		return false
	}
	return binding.Leaf.HasIPSAN
}

func reverseProxyCertificateBindingMatchesServerName(binding *reverseProxyRuleCertificateBinding, serverName string) bool {
	if !reverseProxyCertificateBindingUsable(binding, time.Now()) {
		return false
	}
	return reverseProxyLeafMatchesServerName(binding.Leaf.Leaf, serverName)
}

type reverseProxySNIMatchCategory int

const (
	reverseProxySNIMatchNone reverseProxySNIMatchCategory = iota
	reverseProxySNIMatchExact
	reverseProxySNIMatchWildcard
)

func reverseProxyCertificateBindingSNIMatchType(binding *reverseProxyRuleCertificateBinding, serverName string) reverseProxySNIMatchCategory {
	if !reverseProxyCertificateBindingUsable(binding, time.Now()) || binding == nil || binding.Leaf == nil || binding.Leaf.Leaf == nil {
		return reverseProxySNIMatchNone
	}
	serverName = reverseProxyNormalizeServerName(serverName)
	if serverName == "" {
		return reverseProxySNIMatchNone
	}
	if binding.Leaf.Leaf.VerifyHostname(serverName) != nil {
		return reverseProxySNIMatchNone
	}
	if reverseProxyParseIPLiteral(serverName) != nil {
		return reverseProxySNIMatchExact
	}
	for _, dnsName := range binding.Leaf.Leaf.DNSNames {
		candidate := reverseProxyNormalizeServerName(dnsName)
		if candidate == "" {
			continue
		}
		if candidate == serverName {
			return reverseProxySNIMatchExact
		}
	}
	return reverseProxySNIMatchWildcard
}

func reverseProxySplitSNICertificateCandidates(bindings []*reverseProxyRuleCertificateBinding, serverName string) ([]*reverseProxyRuleCertificateBinding, []*reverseProxyRuleCertificateBinding) {
	serverName = reverseProxyNormalizeServerName(serverName)
	if serverName == "" {
		return nil, nil
	}
	exact := make([]*reverseProxyRuleCertificateBinding, 0, len(bindings))
	wildcard := make([]*reverseProxyRuleCertificateBinding, 0, len(bindings))
	for _, binding := range bindings {
		switch reverseProxyCertificateBindingSNIMatchType(binding, serverName) {
		case reverseProxySNIMatchExact:
			exact = append(exact, binding)
		case reverseProxySNIMatchWildcard:
			wildcard = append(wildcard, binding)
		}
	}
	return exact, wildcard
}

func reverseProxyCertificateDomainsCoverHost(domains []string, host string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	for _, domain := range domains {
		if reverseProxyHostPatternMatches(domain, host) || reverseProxyHostPatternMatches(host, domain) {
			return true
		}
	}
	return false
}

func reverseProxyCertificateDomainsCoverIP(domains []string, ip string) bool {
	ip = strings.ToLower(strings.TrimSpace(ip))
	for _, domain := range domains {
		if reverseProxyIPLiteralEqual(domain, ip) || strings.EqualFold(strings.TrimSpace(domain), ip) {
			return true
		}
	}
	return false
}

func reverseProxyParseIPLiteral(value string) net.IP {
	value = strings.TrimSpace(strings.Trim(value, "[]"))
	if value == "" {
		return nil
	}
	return net.ParseIP(value)
}

func reverseProxyIPLiteralEqual(a string, b string) bool {
	ipA := reverseProxyParseIPLiteral(a)
	ipB := reverseProxyParseIPLiteral(b)
	if ipA == nil || ipB == nil {
		return false
	}
	return ipA.Equal(ipB)
}

func reverseProxyLocalAddressMayHidePublicTarget(ip net.IP) bool {
	if ip == nil {
		return true
	}
	addr, ok := netip.AddrFromSlice(ip)
	if !ok {
		return true
	}
	addr = addr.Unmap()
	if addr.IsUnspecified() || addr.IsLoopback() || addr.IsPrivate() || addr.IsLinkLocalUnicast() {
		return true
	}
	if prefix, err := netip.ParsePrefix("100.64.0.0/10"); err == nil && prefix.Contains(addr) {
		return true
	}
	return false
}

func reverseProxyHostPatternMatches(pattern string, host string) bool {
	pattern = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(pattern), "."))
	host = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(host), "."))
	if pattern == "" || host == "" {
		return false
	}
	if reverseProxyIPLiteralEqual(pattern, host) {
		return true
	}
	if pattern == host {
		return true
	}
	if !reverseProxyIsStandardWildcardHost(pattern) {
		return false
	}
	suffix := strings.TrimPrefix(pattern, "*.")
	if !strings.HasSuffix(host, "."+suffix) {
		return false
	}
	remainder := strings.TrimSuffix(host, "."+suffix)
	return remainder != "" && !strings.Contains(remainder, ".")
}

func buildReverseProxyDefaultName(protocol string, listenIP string, listenPort int, pathPrefix string) string {
	host := strings.TrimSpace(listenIP)
	if host == "" {
		host = "*"
	}
	path := normalizeReverseProxyPath(pathPrefix, false)
	if path == "" {
		path = "*"
	}
	return strings.ToUpper(strings.TrimSpace(protocol)) + " " + host + ":" + strconv.Itoa(listenPort) + " " + path
}

func (r *reverseProxyRuntimeManager) SyncIfNeeded(service *ReverseProxyService, minGap time.Duration) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.reconcileLocked(service, minGap, false)
}

func (r *reverseProxyRuntimeManager) currentRevision() uint64 {
	if r == nil {
		return 0
	}
	r.mu.Lock()
	revision := r.state.revision
	r.mu.Unlock()
	return revision
}

func (r *reverseProxyRuntimeManager) maintainCertificateBalance(now time.Time) bool {
	if r == nil {
		return false
	}
	r.mu.Lock()
	if !r.state.lastCertificateBalancePrune.IsZero() && now.Sub(r.state.lastCertificateBalancePrune) < reverseProxyCertBalanceCleanupGap {
		r.mu.Unlock()
		return false
	}
	groups := make([]*reverseProxyListenerGroup, 0, len(r.groups))
	for _, group := range r.groups {
		if group != nil {
			groups = append(groups, group)
		}
	}
	r.state.lastCertificateBalancePrune = now
	r.mu.Unlock()
	for _, group := range groups {
		group.pruneCertificateBalanceStates(now)
		group.pruneExpiredCachedUpstreams(now)
	}
	return true
}

func (r *reverseProxyRuntimeManager) SyncNow(service *ReverseProxyService) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.reconcileLocked(service, 0, true)
}

func (r *reverseProxyRuntimeManager) hasPendingReconcile() bool {
	if r == nil {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.state.lastRenderKey == "" && strings.TrimSpace(r.reconcileError) != ""
}

func (r *reverseProxyRuntimeManager) Stop() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	firstErr := shutdownReverseProxyListenerGroups(r.groups)
	r.groups = make(map[string]*reverseProxyListenerGroup)
	r.loadedConfiguration = nil
	r.resetMismatchTable()
	r.state.lastRenderKey = ""
	r.state.lastSyncAt = time.Now()
	r.state.lastCertificateBalancePrune = time.Time{}
	r.state.warnings = nil
	r.state.revision = 0
	r.state.certificateGeneration = 0
	r.state.nextRetryAt = time.Time{}
	r.state.retryDelay = 0
	r.reconcileError = ""
	return firstErr
}

func (r *reverseProxyRuntimeManager) reconcileLocked(service *ReverseProxyService, minGap time.Duration, force bool) error {
	if service == nil {
		return nil
	}
	now := time.Now()
	certificateGeneration := currentReverseProxyCertificateGeneration()
	if !force && minGap > 0 {
		if !r.state.lastSyncAt.IsZero() && now.Sub(r.state.lastSyncAt) < minGap {
			return nil
		}
		revision, err := service.peekReverseProxyRevision()
		if err != nil {
			return err
		}
		if revision != 0 && revision == r.state.revision && certificateGeneration == r.state.certificateGeneration {
			if !r.state.nextRetryAt.IsZero() && now.Before(r.state.nextRetryAt) {
				r.state.lastSyncAt = now
				return nil
			}
			if r.state.lastRenderKey != "" {
				r.state.lastSyncAt = now
				return nil
			}
		}
	}
	db := database.GetDB()
	settings, err := service.loadReverseProxySettings()
	if err != nil {
		return err
	}
	if settings != nil {
		reverseProxyResources.apply(reverseProxySettingsView(settings))
	}
	rows, err := service.loadRulesLocked(db)
	if err != nil {
		return err
	}
	if err := prepareReverseProxyParsedCertificateMaterials(rows); err != nil {
		return err
	}
	if settings != nil {
		r.loadedConfiguration = &reverseProxyLoadedConfiguration{
			Settings: *settings,
			Rules:    append([]model.ReverseProxyRule(nil), rows...),
		}
	}
	certificateState := loadReverseProxyCertificateRenderState(db, rows)
	renderKey := computeReverseProxyRenderKeyWithCertificateState(rows, certificateState)
	resourcesChanged := false
	currentResources := reverseProxyResources.current()
	for _, group := range r.groups {
		if group == nil {
			continue
		}
		group.mu.RLock()
		changed := group.resources != currentResources
		group.mu.RUnlock()
		if changed {
			resourcesChanged = true
			break
		}
	}
	configurationChanged := renderKey != r.state.lastRenderKey
	http3HealthNeedsReconcile := reverseProxyHTTP3HealthNeedsReconcile(r.groups)
	if !configurationChanged && !resourcesChanged && !http3HealthNeedsReconcile {
		r.state.lastSyncAt = now
		if settings != nil {
			r.state.revision = settings.Revision
		}
		r.state.certificateGeneration = certificateGeneration
		return nil
	}
	grouped := reverseProxyGroupRules(rows)
	nextGroups := make(map[string]*reverseProxyListenerGroup, len(grouped)+len(r.groups))
	warnings := make([]string, 0)
	type failedGroup struct {
		key   string
		rules []*model.ReverseProxyRule
	}
	failedGroups := make([]failedGroup, 0)
	stoppedGroups := make(map[*reverseProxyListenerGroup]struct{})
	reportGroupState := func(groupRows []*model.ReverseProxyRule, status string, listenerErr error) {
		message := ""
		if listenerErr != nil {
			message = strings.TrimSpace(listenerErr.Error())
		}
		for _, rule := range groupRows {
			if rule == nil {
				continue
			}
			reverseProxyRuntime.reportRuleState(rule.Id, status, message)
		}
	}
	for key, groupRows := range grouped {
		groupRenderKey := computeReverseProxyRenderKeyWithCertificateState(reverseProxyRuleValues(groupRows), certificateState)
		if existing, ok := r.groups[key]; ok && existing != nil {
			if reverseProxyListenerGroupNeedsRestart(existing, groupRows) {
				if reverseProxyListenerGroupRestartNeedsClosingExisting(existing, groupRows) {
					restorePoint := snapshotReverseProxyListenerGroup(existing)
					// Stream limits are negotiated when the listener is created. The
					// existing endpoint must therefore be closed before rebinding;
					// keeping its stopped object would make every retry fail against
					// the same socket while reporting a healthy listener.
					if stopErr := existing.shutdown(); stopErr != nil {
						stoppedGroups[existing] = struct{}{}
						if restored, restoreErr := restoreReverseProxyListenerGroup(service, restorePoint); restoreErr == nil && restored != nil {
							nextGroups[restorePoint.key] = restored
						} else if restoreErr != nil {
							warnings = append(warnings, "reverse proxy listener "+key+" rollback failed: "+strings.TrimSpace(restoreErr.Error()))
						}
						failedGroups = append(failedGroups, failedGroup{key: key, rules: groupRows})
						reportGroupState(groupRows, "listener_error", stopErr)
						warnings = append(warnings, "reverse proxy listener "+key+" stopped during rebuild: "+strings.TrimSpace(stopErr.Error()))
						continue
					}
					stoppedGroups[existing] = struct{}{}
					group, err := service.newListenerGroup(key, groupRows)
					if err != nil {
						if restored, restoreErr := restoreReverseProxyListenerGroup(service, restorePoint); restoreErr == nil && restored != nil {
							nextGroups[restorePoint.key] = restored
						} else if restoreErr != nil {
							warnings = append(warnings, "reverse proxy listener "+key+" rollback failed: "+strings.TrimSpace(restoreErr.Error()))
						}
						failedGroups = append(failedGroups, failedGroup{key: key, rules: groupRows})
						reportGroupState(groupRows, "listener_error", err)
						warnings = append(warnings, "reverse proxy listener "+key+" rebuild failed after stop: "+strings.TrimSpace(err.Error()))
						continue
					}
					nextGroups[key] = group
					group.mu.Lock()
					group.renderKey = groupRenderKey
					group.mu.Unlock()
					reportGroupState(groupRows, "running", nil)
					continue
				}
				group, err := service.newListenerGroup(key, groupRows)
				if err != nil {
					nextGroups[key] = existing
					failedGroups = append(failedGroups, failedGroup{key: key, rules: groupRows})
					reportGroupState(groupRows, "listener_error", err)
					warnings = append(warnings, "reverse proxy listener "+key+" retained previous instance: "+strings.TrimSpace(err.Error()))
					continue
				}
				if err := existing.shutdown(); err != nil {
					_ = group.shutdown()
					stoppedGroups[existing] = struct{}{}
					failedGroups = append(failedGroups, failedGroup{key: key, rules: groupRows})
					reportGroupState(groupRows, "listener_error", err)
					warnings = append(warnings, "reverse proxy listener "+key+" stopped during rebuild: "+strings.TrimSpace(err.Error()))
					continue
				}
				stoppedGroups[existing] = struct{}{}
				nextGroups[key] = group
				group.mu.Lock()
				group.renderKey = groupRenderKey
				group.mu.Unlock()
				reportGroupState(groupRows, "running", nil)
				continue
			}
			existing.mu.RLock()
			groupConfigurationChanged := existing.renderKey != groupRenderKey
			existing.mu.RUnlock()
			if !groupConfigurationChanged {
				// All non-stream resource guards are adjustable.  Keep the
				// listener, certificate bindings and healthy upstream pools in
				// place; only a changed inherited idle-pool policy retires the
				// affected transport cache below.
				existing.applyResourceSettings(currentResources)
				nextGroups[key] = existing
				reportGroupState(groupRows, "running", nil)
				continue
			}
			if err := service.refreshListenerGroup(existing, groupRows); err != nil {
				nextGroups[key] = existing
				failedGroups = append(failedGroups, failedGroup{key: key, rules: groupRows})
				reportGroupState(groupRows, "listener_error", err)
				warnings = append(warnings, "reverse proxy listener "+key+" retained previous instance: "+strings.TrimSpace(err.Error()))
				continue
			}
			existing.mu.Lock()
			existing.renderKey = groupRenderKey
			existing.mu.Unlock()
			nextGroups[key] = existing
			reportGroupState(groupRows, "running", nil)
			continue
		}
		blockers := reverseProxyBlockingListenerGroups(r.groups, grouped, key, groupRows, stoppedGroups)
		restorePoints := make([]reverseProxyListenerGroupRestorePoint, 0, len(blockers))
		stopErrors := make([]error, 0)
		for _, blocker := range blockers {
			if blocker.group == nil {
				continue
			}
			restorePoints = append(restorePoints, snapshotReverseProxyListenerGroup(blocker.group))
			if stopErr := blocker.group.shutdown(); stopErr != nil {
				stopErrors = append(stopErrors, fmt.Errorf("%s: %w", blocker.key, stopErr))
			}
			stoppedGroups[blocker.group] = struct{}{}
		}
		if stopErr := errors.Join(stopErrors...); stopErr != nil {
			restoreErrors := restoreReverseProxyListenerGroups(service, restorePoints, nextGroups)
			if restoreErr := errors.Join(restoreErrors...); restoreErr != nil {
				warnings = append(warnings, "reverse proxy listener "+key+" rollback failed: "+strings.TrimSpace(restoreErr.Error()))
			}
			failedGroups = append(failedGroups, failedGroup{key: key, rules: groupRows})
			reportGroupState(groupRows, "listener_error", stopErr)
			warnings = append(warnings, "reverse proxy listener "+key+" could not release the previous binding: "+strings.TrimSpace(stopErr.Error()))
			continue
		}
		group, err := service.newListenerGroup(key, groupRows)
		if err != nil {
			restoreErrors := restoreReverseProxyListenerGroups(service, restorePoints, nextGroups)
			if restoreErr := errors.Join(restoreErrors...); restoreErr != nil {
				warnings = append(warnings, "reverse proxy listener "+key+" rollback failed: "+strings.TrimSpace(restoreErr.Error()))
			}
			failedGroups = append(failedGroups, failedGroup{key: key, rules: groupRows})
			reportGroupState(groupRows, "listener_error", err)
			warnings = append(warnings, "reverse proxy listener "+key+" failed: "+strings.TrimSpace(err.Error()))
			continue
		}
		nextGroups[key] = group
		group.mu.Lock()
		group.renderKey = groupRenderKey
		group.mu.Unlock()
		reportGroupState(groupRows, "running", nil)
	}
	for key, group := range r.groups {
		if _, exists := nextGroups[key]; exists {
			continue
		}
		if _, wasStopped := stoppedGroups[group]; wasStopped {
			continue
		}
		keepPrevious := false
		for _, failed := range failedGroups {
			if reverseProxyListenerGroupOverlapsRules(group, failed.rules) || reverseProxyListenerGroupSharesRuleIDs(group, failed.rules) {
				keepPrevious = true
				break
			}
		}
		if keepPrevious {
			nextGroups[key] = group
			continue
		}
		if group != nil {
			if err := group.shutdown(); err != nil {
				warnings = append(warnings, "reverse proxy listener "+key+" shutdown failed: "+strings.TrimSpace(err.Error()))
			}
		}
	}
	for _, group := range nextGroups {
		if group == nil || len(group.warnings) == 0 {
			continue
		}
		warnings = append(warnings, group.warnings...)
	}
	r.groups = nextGroups
	refreshReverseProxyHTTP3AvailabilityLocked(r.groups)
	if len(failedGroups) == 0 {
		r.state.lastRenderKey = renderKey
		r.state.nextRetryAt = time.Time{}
		r.state.retryDelay = 0
	} else {
		// A failed group is retried with a bounded exponential backoff.  This
		// avoids a permanently occupied port consuming CPU on every cron tick.
		r.state.lastRenderKey = ""
		if r.state.retryDelay <= 0 {
			r.state.retryDelay = reverseProxyRuntimeRetryBaseDelay
		} else {
			r.state.retryDelay *= 2
			if r.state.retryDelay > reverseProxyRuntimeRetryMaxDelay {
				r.state.retryDelay = reverseProxyRuntimeRetryMaxDelay
			}
		}
		r.state.nextRetryAt = now.Add(r.state.retryDelay)
	}
	r.state.lastSyncAt = now
	if settings != nil {
		r.state.revision = settings.Revision
	}
	r.state.certificateGeneration = certificateGeneration
	r.state.warnings = warnings
	if len(failedGroups) == 0 {
		r.reconcileError = ""
	} else {
		r.reconcileError = "reverse proxy listener rebuild is waiting for retry"
	}
	return nil
}

func reverseProxyListenerGroupOverlapsRules(group *reverseProxyListenerGroup, rules []*model.ReverseProxyRule) bool {
	if group == nil || len(rules) == 0 {
		return false
	}
	group.mu.RLock()
	port := group.listenPort
	socketKind := group.socketKind
	listenIPs := append([]string(nil), group.listenIPs...)
	group.mu.RUnlock()
	for _, rule := range rules {
		if rule == nil || rule.ListenPort != port {
			continue
		}
		alias := normalizeReverseProxyProtocolAlias(rule.ListenProtocolAlias, rule.ListenProtocol)
		for _, desiredSocket := range reverseProxyListenerSocketKinds(rule.ListenProtocol, rule.ListenHTTPVersionStrategy, alias) {
			if desiredSocket != socketKind {
				continue
			}
			if reverseProxyListenIPSetsOverlap(listenIPs, reverseProxyHTTPRuntimeListenIPs(rule)) {
				return true
			}
		}
	}
	return false
}

type reverseProxyListenerGroupRef struct {
	key   string
	group *reverseProxyListenerGroup
}

type reverseProxyListenerGroupRestorePoint struct {
	key       string
	renderKey string
	rules     []model.ReverseProxyRule
}

func reverseProxyListenerGroupSharesRuleIDs(group *reverseProxyListenerGroup, rules []*model.ReverseProxyRule) bool {
	if group == nil || len(rules) == 0 {
		return false
	}
	desiredIDs := make(map[uint]struct{}, len(rules))
	for _, rule := range rules {
		if rule != nil && rule.Id != 0 {
			desiredIDs[rule.Id] = struct{}{}
		}
	}
	if len(desiredIDs) == 0 {
		return false
	}
	group.mu.RLock()
	defer group.mu.RUnlock()
	for _, rule := range group.rules {
		if rule == nil {
			continue
		}
		if _, exists := desiredIDs[rule.Id]; exists {
			return true
		}
	}
	return false
}

func reverseProxyBlockingListenerGroups(groups map[string]*reverseProxyListenerGroup, desired map[string][]*model.ReverseProxyRule, desiredKey string, rules []*model.ReverseProxyRule, stopped map[*reverseProxyListenerGroup]struct{}) []reverseProxyListenerGroupRef {
	keys := make([]string, 0)
	for key, group := range groups {
		if key == desiredKey || group == nil {
			continue
		}
		if _, stillDesired := desired[key]; stillDesired {
			continue
		}
		if _, alreadyStopped := stopped[group]; alreadyStopped {
			continue
		}
		if reverseProxyListenerGroupOverlapsRules(group, rules) {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	result := make([]reverseProxyListenerGroupRef, 0, len(keys))
	for _, key := range keys {
		result = append(result, reverseProxyListenerGroupRef{key: key, group: groups[key]})
	}
	return result
}

func snapshotReverseProxyListenerGroup(group *reverseProxyListenerGroup) reverseProxyListenerGroupRestorePoint {
	if group == nil {
		return reverseProxyListenerGroupRestorePoint{}
	}
	group.mu.RLock()
	defer group.mu.RUnlock()
	point := reverseProxyListenerGroupRestorePoint{
		key:       group.key,
		renderKey: group.renderKey,
		rules:     make([]model.ReverseProxyRule, 0, len(group.rules)),
	}
	for _, rule := range group.rules {
		if rule != nil {
			point.rules = append(point.rules, *rule)
		}
	}
	return point
}

func restoreReverseProxyListenerGroup(service *ReverseProxyService, point reverseProxyListenerGroupRestorePoint) (*reverseProxyListenerGroup, error) {
	if service == nil || strings.TrimSpace(point.key) == "" || len(point.rules) == 0 {
		return nil, nil
	}
	rules := make([]*model.ReverseProxyRule, len(point.rules))
	for i := range point.rules {
		rules[i] = &point.rules[i]
	}
	group, err := service.newListenerGroup(point.key, rules)
	if err != nil {
		return nil, err
	}
	group.mu.Lock()
	group.renderKey = point.renderKey
	group.mu.Unlock()
	return group, nil
}

func restoreReverseProxyListenerGroups(service *ReverseProxyService, points []reverseProxyListenerGroupRestorePoint, target map[string]*reverseProxyListenerGroup) []error {
	errs := make([]error, 0)
	for _, point := range points {
		group, err := restoreReverseProxyListenerGroup(service, point)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", point.key, err))
			continue
		}
		if group != nil {
			target[point.key] = group
		}
	}
	return errs
}

const (
	reverseProxySocketKindTCP = "tcp"
	reverseProxySocketKindUDP = "udp"
)

func reverseProxyCanonicalListenIPs(items []string, fallback []string) []string {
	if len(items) == 0 {
		items = fallback
	}
	result := make([]string, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		value := strings.Trim(strings.TrimSpace(item), "[]")
		if value == "" {
			continue
		}
		addr, err := netip.ParseAddr(value)
		if err != nil {
			continue
		}
		value = addr.Unmap().String()
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	if len(result) == 0 {
		for _, item := range fallback {
			value := strings.Trim(strings.TrimSpace(item), "[]")
			if value == "" {
				continue
			}
			if _, exists := seen[value]; exists {
				continue
			}
			seen[value] = struct{}{}
			result = append(result, value)
		}
	}
	sort.Strings(result)
	return result
}

func reverseProxyHTTPRuntimeListenIPs(row *model.ReverseProxyRule) []string {
	return []string{"0.0.0.0", "::"}
}

func reverseProxyNormalizedHTTPRuntimeListenIPs(row reverseProxyNormalizedRule) []string {
	return []string{"0.0.0.0", "::"}
}

func reverseProxyNormalizedDNSRuntimeListenIPs(row reverseProxyNormalizedRule) []string {
	return []string{"0.0.0.0", "::"}
}

func reverseProxyListenerSocketKinds(protocol string, listenStrategy string, alias string) []string {
	tcp, udp := reverseProxyListenerUsesUnderlyingSockets(protocol, listenStrategy, alias)
	items := make([]string, 0, 2)
	if tcp {
		items = append(items, reverseProxySocketKindTCP)
	}
	if udp {
		items = append(items, reverseProxySocketKindUDP)
	}
	return items
}

func reverseProxyListenerGroupKey(row *model.ReverseProxyRule, socketKind string) string {
	if row == nil {
		return ""
	}
	listenIPs := strings.Join(reverseProxyHTTPRuntimeListenIPs(row), ",")
	return reverseProxyListenerKey(reverseProxyListenerGroupProtocol(row), row.ListenPort, socketKind, listenIPs)
}

func reverseProxyHTTP3ListenerGroupKey(protocol string, port int, listenIPs []string) string {
	return reverseProxyListenerKey(protocol, port, reverseProxySocketKindUDP, strings.Join(listenIPs, ","))
}

func reverseProxyListenerGroupIsHTTP3(group *reverseProxyListenerGroup) bool {
	if group == nil {
		return false
	}
	group.mu.RLock()
	defer group.mu.RUnlock()
	return strings.EqualFold(group.protocol, reverseProxyProtocolHTTPS) && group.socketKind == reverseProxySocketKindUDP
}

// refreshReverseProxyHTTP3AvailabilityLocked links the TCP H2 listener to the
// matching UDP H3 listener. H2 and H3 are separate listener groups, so the
// presence of an H2 group alone must never be enough to advertise Alt-Svc.
// The caller holds reverseProxyRuntime.mu.
func refreshReverseProxyHTTP3AvailabilityLocked(groups map[string]*reverseProxyListenerGroup) {
	for _, tcpGroup := range groups {
		if tcpGroup == nil {
			continue
		}
		tcpGroup.mu.RLock()
		protocol := tcpGroup.protocol
		socketKind := tcpGroup.socketKind
		strategy := tcpGroup.listenHTTPVersionStrategy
		port := tcpGroup.listenPort
		listenIPs := append([]string(nil), tcpGroup.listenIPs...)
		tcpGroup.mu.RUnlock()
		if !strings.EqualFold(protocol, reverseProxyProtocolHTTPS) ||
			socketKind != reverseProxySocketKindTCP ||
			strategy != reverseProxyListenHTTPVersionH2H3 {
			continue
		}

		available := false
		udpGroup := groups[reverseProxyHTTP3ListenerGroupKey(protocol, port, listenIPs)]
		if udpGroup != nil {
			udpGroup.mu.RLock()
			available = !udpGroup.closed && udpGroup.h3AvailabilityKnown && udpGroup.h3ListenerAvailable && len(udpGroup.packetConns) > 0
			udpGroup.mu.RUnlock()
		}
		tcpGroup.mu.Lock()
		tcpGroup.h3AvailabilityKnown = true
		tcpGroup.h3ListenerAvailable = available
		tcpGroup.mu.Unlock()
	}
}

func reverseProxyHTTP3HealthNeedsReconcile(groups map[string]*reverseProxyListenerGroup) bool {
	for _, group := range groups {
		if group == nil || !reverseProxyListenerGroupIsHTTP3(group) {
			continue
		}
		group.mu.RLock()
		unhealthy := !group.closed && group.h3AvailabilityKnown && !group.h3ListenerAvailable
		group.mu.RUnlock()
		if unhealthy {
			return true
		}
	}
	return false
}

func reverseProxyListenerSocketKindFromKey(key string) string {
	parts := strings.SplitN(strings.TrimSpace(key), "|", 4)
	if len(parts) >= 3 && (parts[2] == reverseProxySocketKindTCP || parts[2] == reverseProxySocketKindUDP) {
		return parts[2]
	}
	return reverseProxySocketKindTCP
}

func reverseProxyGroupListenHTTPVersionStrategy(protocol string, socketKind string, rules []*model.ReverseProxyRule) string {
	if !strings.EqualFold(strings.TrimSpace(protocol), reverseProxyProtocolHTTPS) {
		return ""
	}
	for _, rule := range rules {
		if rule == nil {
			continue
		}
		strategy, err := normalizeReverseProxyListenHTTPVersionStrategy(rule.ListenHTTPVersionStrategy, rule.ListenProtocol)
		if err == nil && strategy == reverseProxyListenHTTPVersionH2H3 {
			return reverseProxyListenHTTPVersionH2H3
		}
	}
	if socketKind == reverseProxySocketKindUDP {
		return reverseProxyListenHTTPVersionH3Only
	}
	return reverseProxyListenHTTPVersionH2Only
}

// reverseProxyListenerGroupNeedsRestart reports changes that alter the bound
// socket topology or stream settings negotiated during a new H2/H3 connection.
// Adjustable connection, request, upstream and memory guards refresh in place.
func reverseProxyListenerGroupNeedsRestart(group *reverseProxyListenerGroup, rules []*model.ReverseProxyRule) bool {
	if group == nil || len(rules) == 0 {
		return true
	}
	first := rules[0]
	desiredProtocol := reverseProxyListenerGroupProtocol(first)
	desiredIPs := reverseProxyHTTPRuntimeListenIPs(first)
	group.mu.RLock()
	defer group.mu.RUnlock()
	return group.closed ||
		group.listenPort != first.ListenPort ||
		!strings.EqualFold(group.protocol, desiredProtocol) ||
		!reverseProxyListenIPSetsEqual(group.listenIPs, desiredIPs) ||
		(group.socketKind == reverseProxySocketKindUDP && group.h3AvailabilityKnown && !group.h3ListenerAvailable) ||
		reverseProxyListenerGroupStaticResourceRestartRequired(group.protocol, group.socketKind, group.resources, reverseProxyResources.current())
}

func reverseProxyListenerGroupRestartNeedsClosingExisting(group *reverseProxyListenerGroup, rules []*model.ReverseProxyRule) bool {
	if group == nil || len(rules) == 0 {
		return false
	}
	first := rules[0]
	desiredIPs := reverseProxyHTTPRuntimeListenIPs(first)
	group.mu.RLock()
	defer group.mu.RUnlock()
	if group.closed ||
		group.listenPort != first.ListenPort ||
		!strings.EqualFold(group.protocol, reverseProxyListenerGroupProtocol(first)) ||
		!reverseProxyListenIPSetsEqual(group.listenIPs, desiredIPs) {
		return false
	}
	return (group.socketKind == reverseProxySocketKindUDP && group.h3AvailabilityKnown && !group.h3ListenerAvailable) ||
		reverseProxyListenerGroupStaticResourceRestartRequired(group.protocol, group.socketKind, group.resources, reverseProxyResources.current())
}

func reverseProxyListenerGroupStaticResourceRestartRequired(protocol string, socketKind string, previous ReverseProxyResourceSettings, current ReverseProxyResourceSettings) bool {
	if !strings.EqualFold(strings.TrimSpace(protocol), reverseProxyProtocolHTTPS) {
		return false
	}
	switch socketKind {
	case reverseProxySocketKindTCP:
		return previous.HTTP2MaxConcurrentStreams != current.HTTP2MaxConcurrentStreams
	case reverseProxySocketKindUDP:
		return previous.QUICMaxIncomingStreams != current.QUICMaxIncomingStreams
	default:
		return false
	}
}

func reverseProxyGroupRules(rows []model.ReverseProxyRule) map[string][]*model.ReverseProxyRule {
	grouped := make(map[string][]*model.ReverseProxyRule)
	for i := range rows {
		row := &rows[i]
		if !row.Enabled {
			continue
		}
		listenAlias := normalizeReverseProxyProtocolAlias(row.ListenProtocolAlias, row.ListenProtocol)
		if reverseProxyProtocolIsDNS(listenAlias) && !reverseProxyIsHTTPDNSAlias(listenAlias) {
			continue
		}
		for _, socketKind := range reverseProxyListenerSocketKinds(row.ListenProtocol, row.ListenHTTPVersionStrategy, listenAlias) {
			key := reverseProxyListenerGroupKey(row, socketKind)
			grouped[key] = append(grouped[key], row)
		}
	}
	for key := range grouped {
		sort.SliceStable(grouped[key], func(i, j int) bool {
			if grouped[key][i].ListOrder == grouped[key][j].ListOrder {
				return grouped[key][i].Id < grouped[key][j].Id
			}
			return grouped[key][i].ListOrder < grouped[key][j].ListOrder
		})
	}
	return grouped
}

func reverseProxyHTTPDNSRuleValues(rules []*model.ReverseProxyRule) []model.ReverseProxyRule {
	result := make([]model.ReverseProxyRule, 0)
	for _, rule := range rules {
		if rule == nil {
			continue
		}
		alias := normalizeReverseProxyProtocolAlias(rule.ListenProtocolAlias, rule.ListenProtocol)
		if reverseProxyIsHTTPDNSAlias(alias) {
			result = append(result, *rule)
		}
	}
	return result
}

func (r *reverseProxyRuntimeManager) swapGroupsLocked(next map[string]*reverseProxyListenerGroup) error {
	if next == nil {
		next = map[string]*reverseProxyListenerGroup{}
	}
	oldGroups := r.groups
	r.groups = next
	var firstErr error
	for key, group := range oldGroups {
		if _, exists := next[key]; exists {
			continue
		}
		if group == nil {
			continue
		}
		if err := group.shutdown(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func loadReverseProxyCertificateRenderState(db *gorm.DB, rows []model.ReverseProxyRule) map[uint]model.CertificateRecord {
	result := make(map[uint]model.CertificateRecord)
	if db == nil {
		return result
	}
	certIDs := make([]uint, 0)
	seen := make(map[uint]struct{})
	for i := range rows {
		row := rows[i]
		listenAlias := normalizeReverseProxyProtocolAlias(row.ListenProtocolAlias, row.ListenProtocol)
		if !row.Enabled || !reverseProxyListenerUsesManagedCertificates(strings.TrimSpace(row.ListenProtocol), listenAlias) {
			continue
		}
		for _, certID := range reverseProxyRuleCertificateIDs(&row) {
			if _, exists := seen[certID]; exists {
				continue
			}
			seen[certID] = struct{}{}
			certIDs = append(certIDs, certID)
		}
	}
	if len(certIDs) == 0 {
		return result
	}
	records := make([]model.CertificateRecord, 0, len(certIDs))
	if err := db.Select("id", "fingerprint", "updated_at").Where("id IN ?", certIDs).Find(&records).Error; err != nil {
		return result
	}
	for i := range records {
		result[records[i].Id] = records[i]
	}
	return result
}

func computeReverseProxyRenderKey(db *gorm.DB, rows []model.ReverseProxyRule) string {
	return computeReverseProxyRenderKeyWithCertificateState(rows, loadReverseProxyCertificateRenderState(db, rows))
}

func reverseProxyRuleValues(rows []*model.ReverseProxyRule) []model.ReverseProxyRule {
	result := make([]model.ReverseProxyRule, 0, len(rows))
	for _, row := range rows {
		if row != nil {
			result = append(result, *row)
		}
	}
	return result
}

func computeReverseProxyRenderKeyWithCertificateState(rows []model.ReverseProxyRule, certState map[uint]model.CertificateRecord) string {
	httpRows := make([]model.ReverseProxyRule, 0, len(rows))
	for i := range rows {
		listenAlias := normalizeReverseProxyProtocolAlias(rows[i].ListenProtocolAlias, rows[i].ListenProtocol)
		if reverseProxyProtocolIsDNS(listenAlias) && !reverseProxyIsHTTPDNSAlias(listenAlias) {
			continue
		}
		httpRows = append(httpRows, rows[i])
	}
	snapshot := make([]reverseProxyRenderRule, 0, len(httpRows))
	for i := range httpRows {
		row := httpRows[i]
		listenProtocol := strings.ToLower(strings.TrimSpace(row.ListenProtocol))
		listenAlias := normalizeReverseProxyProtocolAlias(row.ListenProtocolAlias, row.ListenProtocol)
		targetProtocol := strings.ToLower(strings.TrimSpace(row.TargetProtocol))
		targetAlias := normalizeReverseProxyProtocolAlias(row.TargetProtocolAlias, row.TargetProtocol)
		listenHTTPVersionStrategy := strings.ToLower(strings.TrimSpace(row.ListenHTTPVersionStrategy))
		certificateRecordIDs := []uint{}
		certificateStates := []reverseProxyRenderCertificateState{}
		if reverseProxyListenerUsesManagedCertificates(listenProtocol, listenAlias) {
			certificateRecordIDs = reverseProxyRuleCertificateIDs(&row)
			certificateStates = make([]reverseProxyRenderCertificateState, 0, len(certificateRecordIDs))
			for _, certID := range certificateRecordIDs {
				state := reverseProxyRenderCertificateState{ID: certID}
				if cert, ok := certState[certID]; ok {
					state.Fingerprint = strings.TrimSpace(cert.Fingerprint)
					if !cert.UpdatedAt.IsZero() {
						state.UpdatedAt = cert.UpdatedAt.Unix()
					}
				}
				certificateStates = append(certificateStates, state)
			}
		}
		snapshot = append(snapshot, reverseProxyRenderRule{
			ID:                  row.Id,
			ListOrder:           row.ListOrder,
			Enabled:             row.Enabled,
			ListenProtocol:      listenProtocol,
			ListenProtocolAlias: listenAlias,
			ListenPort:          row.ListenPort,
			ListenCompressionEnabled: func() bool {
				enabled, _ := reverseProxyCompressionSettingsFromModel(row.ListenCompressionEnabled, row.ListenCompressionAlgorithms)
				return enabled
			}(),
			ListenCompressionAlgorithms: func() []string {
				_, values := reverseProxyCompressionSettingsFromModel(row.ListenCompressionEnabled, row.ListenCompressionAlgorithms)
				return values
			}(),
			Hosts:               reverseProxyRuleServerNames(&row),
			PathPrefix:          normalizeReverseProxyPath(row.PathPrefix, false),
			ListenDNSPath:       normalizeReverseProxyDNSPath(row.ListenDNSPath),
			TargetProtocol:      targetProtocol,
			TargetProtocolAlias: targetAlias,
			TargetAddresses:     decodeReverseProxyList(row.TargetAddresses),
			TargetPort:          row.TargetPort,
			TargetCompressionEnabled: func() bool {
				enabled, _ := reverseProxyCompressionSettingsFromModel(row.TargetCompressionEnabled, row.TargetCompressionAlgorithms)
				return enabled
			}(),
			TargetCompressionAlgorithms: func() []string {
				_, values := reverseProxyCompressionSettingsFromModel(row.TargetCompressionEnabled, row.TargetCompressionAlgorithms)
				return values
			}(),
			TargetPath:             normalizeReverseProxyPath(row.TargetPath, false),
			TargetDNSPath:          normalizeReverseProxyDNSPath(row.TargetDNSPath),
			AdvertiseHTTP3:         row.AdvertiseHTTP3,
			EDNSEnabled:            false,
			EDNSMode:               "",
			EDNSCustomIP:           "",
			EDNSClientSubnetPolicy: "",
			DisableIPv4Answer:      false,
			DisableIPv6Answer:      false,
			DNSRuntimeState: func() string {
				if reverseProxyIsHTTPDNSAlias(listenAlias) {
					return reverseProxyDNSRouteRuntimeStateKey(&row)
				}
				return ""
			}(),
			CertificateRecordIDs: certificateRecordIDs,
			CertificateStates:    certificateStates,
			ListenHTTPVersionStrategy: func() string {
				if listenProtocol != reverseProxyProtocolHTTPS {
					return ""
				}
				value, err := normalizeReverseProxyListenHTTPVersionStrategy(listenHTTPVersionStrategy, listenProtocol)
				if err != nil {
					return reverseProxyListenHTTPVersionH2H3
				}
				return value
			}(),
			IPStrategy: strings.ToLower(strings.TrimSpace(row.IPStrategy)),
			HTTPVersionStrategy: func() string {
				if targetProtocol == reverseProxyProtocolHTTPS {
					return strings.ToLower(strings.TrimSpace(row.HTTPVersionStrategy))
				}
				return ""
			}(),
			UpstreamTLSVerify: func() bool {
				if targetProtocol == reverseProxyProtocolHTTPS {
					return row.UpstreamTLSVerify
				}
				return false
			}(),
			MaxConcurrentConnections:   reverseProxyRuleLimit(row.MaxConcurrentConnections),
			MaxConcurrentRequests:      reverseProxyMaxConcurrentRequests(row.MaxConcurrentRequests),
			UpstreamMaxConnections:     reverseProxyRuleLimit(row.UpstreamMaxConnections),
			UpstreamMaxIdleConnections: reverseProxyRuleLimit(row.UpstreamMaxIdleConnections),
			MemoryLimitBytes:           row.MemoryLimitBytes,
			ApiPassthrough:             row.ApiPassthrough,
		})
	}
	raw, _ := json.Marshal(snapshot)
	return string(raw)
}

func shutdownReverseProxyListenerGroups(groups map[string]*reverseProxyListenerGroup) error {
	var firstErr error
	for _, group := range groups {
		if group == nil {
			continue
		}
		if err := group.shutdown(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func shutdownReverseProxyHTTPServer(server *http.Server) error {
	if server == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), reverseProxyShutdownTimeout)
	defer cancel()
	err := server.Shutdown(ctx)
	if err == nil || errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	closeErr := server.Close()
	if closeErr != nil && !errors.Is(closeErr, http.ErrServerClosed) && !errors.Is(closeErr, net.ErrClosed) {
		return closeErr
	}
	if errors.Is(err, context.DeadlineExceeded) {
		logger.Warning("reverse proxy graceful shutdown exceeded deadline; forced close applied")
		return nil
	}
	return err
}

func shutdownReverseProxyHTTP3Server(server *http3.Server) error {
	if server == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), reverseProxyShutdownTimeout)
	err := server.Shutdown(ctx)
	cancel()
	if err == nil || errors.Is(err, http.ErrServerClosed) || errors.Is(err, net.ErrClosed) {
		return nil
	}
	closeErr := server.Close()
	if closeErr != nil && !errors.Is(closeErr, http.ErrServerClosed) && !errors.Is(closeErr, net.ErrClosed) {
		return closeErr
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		logger.Warning("reverse proxy http3 graceful shutdown exceeded deadline; forced close applied")
		return nil
	}
	return err
}

func (s *ReverseProxyService) buildListenerGroups(rows []model.ReverseProxyRule) (map[string]*reverseProxyListenerGroup, []string, error) {
	grouped := reverseProxyGroupRules(rows)
	nextGroups := make(map[string]*reverseProxyListenerGroup, len(grouped))
	warnings := make([]string, 0)
	for key, groupRows := range grouped {
		group, err := s.newListenerGroup(key, groupRows)
		if err != nil {
			return nil, nil, err
		}
		nextGroups[key] = group
		if len(group.rules) == 0 {
			warnings = append(warnings, "empty reverse proxy listener group skipped: "+key)
		}
	}
	return nextGroups, warnings, nil
}

func reverseProxyListenerKey(protocol string, port int, parts ...string) string {
	keyParts := []string{strings.TrimSpace(protocol), strconv.Itoa(port)}
	for _, part := range parts {
		keyParts = append(keyParts, strings.TrimSpace(part))
	}
	return strings.Join(keyParts, "|")
}

func newReverseProxyAdjustableLimiter(max int) *reverseProxyAdjustableLimiter {
	limiter := &reverseProxyAdjustableLimiter{}
	limiter.SetMax(max)
	return limiter
}

// configureRuleLimitersLocked refreshes the lookup maps for future requests.
// A limiter belonging to a still-present rule is deliberately reused: active
// requests, local connections and upstream sockets retain that same pointer,
// so lowering a limit takes effect immediately without forgetting in-flight
// leases.  Limiters for removed rules may outlive the map briefly; their
// holders release them normally when the request or connection ends.
func (g *reverseProxyListenerGroup) configureRuleLimitersLocked(rules []*model.ReverseProxyRule, resources ReverseProxyResourceSettings) {
	if g == nil {
		return
	}
	g.resources = resources
	if g.listenerConnectionLimiter == nil {
		g.listenerConnectionLimiter = newReverseProxyAdjustableLimiter(resources.ListenerConnectionLimit)
	} else {
		g.listenerConnectionLimiter.SetMax(resources.ListenerConnectionLimit)
	}
	previousRuleConnectionLimiters := g.ruleConnectionLimiters
	previousRequestLimiters := g.requestLimiters
	previousUpstreamLimiters := g.upstreamLimiters
	nextRuleConnectionLimiters := make(map[uint]*reverseProxyAdjustableLimiter, len(rules))
	nextRequestLimiters := make(map[uint]*reverseProxyAdjustableLimiter, len(rules))
	nextUpstreamLimiters := make(map[uint]*reverseProxyAdjustableLimiter, len(rules))
	for _, rule := range rules {
		if rule == nil || rule.Id == 0 {
			continue
		}
		ruleConnectionLimit := reverseProxyRuleLimit(rule.MaxConcurrentConnections)
		requestLimit := reverseProxyMaxConcurrentRequests(rule.MaxConcurrentRequests)
		upstreamLimit := reverseProxyRuleLimit(rule.UpstreamMaxConnections)

		ruleConnectionLimiter := previousRuleConnectionLimiters[rule.Id]
		if ruleConnectionLimiter == nil {
			ruleConnectionLimiter = newReverseProxyAdjustableLimiter(ruleConnectionLimit)
		} else {
			ruleConnectionLimiter.SetMax(ruleConnectionLimit)
		}
		nextRuleConnectionLimiters[rule.Id] = ruleConnectionLimiter

		requestLimiter := previousRequestLimiters[rule.Id]
		if requestLimiter == nil {
			requestLimiter = newReverseProxyAdjustableLimiter(requestLimit)
		} else {
			requestLimiter.SetMax(requestLimit)
		}
		nextRequestLimiters[rule.Id] = requestLimiter

		upstreamLimiter := previousUpstreamLimiters[rule.Id]
		if upstreamLimiter == nil {
			upstreamLimiter = newReverseProxyAdjustableLimiter(upstreamLimit)
		} else {
			upstreamLimiter.SetMax(upstreamLimit)
		}
		nextUpstreamLimiters[rule.Id] = upstreamLimiter
	}
	g.ruleConnectionLimiters = nextRuleConnectionLimiters
	g.requestLimiters = nextRequestLimiters
	g.upstreamLimiters = nextUpstreamLimiters
}

// applyResourceSettings updates guards that can change while a listener is
// live.  H2/QUIC stream limits are handled by the restart path because they
// are negotiated at connection creation.  Changing the inherited idle-pool
// setting is the lone dynamic field that must retire existing transports: the
// net/http transport fields are immutable once a request has used them.
func (g *reverseProxyListenerGroup) applyResourceSettings(resources ReverseProxyResourceSettings) {
	if g == nil {
		return
	}
	staleUpstreams := make([]*reverseProxyCachedUpstream, 0)
	g.mu.Lock()
	previous := g.resources
	g.configureRuleLimitersLocked(g.rules, resources)
	if previous.DefaultUpstreamMaxIdleConnections != resources.DefaultUpstreamMaxIdleConnections {
		for ruleID, upstream := range g.upstreamByRule {
			usesDefault := true
			for _, rule := range g.rules {
				if rule != nil && rule.Id == ruleID {
					usesDefault = reverseProxyRuleLimit(rule.UpstreamMaxIdleConnections) == 0
					break
				}
			}
			if !usesDefault {
				continue
			}
			delete(g.upstreamByRule, ruleID)
			if upstream != nil {
				staleUpstreams = append(staleUpstreams, upstream)
			}
		}
	}
	g.mu.Unlock()
	for _, upstream := range staleUpstreams {
		g.disposeCachedUpstream(upstream)
	}
}

func (s *ReverseProxyService) newListenerGroup(key string, rules []*model.ReverseProxyRule) (*reverseProxyListenerGroup, error) {
	resources := reverseProxyResources.current()
	if len(rules) == 0 {
		return &reverseProxyListenerGroup{
			key:                 key,
			service:             s,
			ruleMatchData:       make(map[uint]reverseProxyRuleMatchData),
			upstreamByRule:      make(map[uint]*reverseProxyCachedUpstream),
			connectionCounts:    make(map[uint]reverseProxyConnectionCounts),
			localConnIDs:        make(map[net.Conn]string),
			localConnByID:       make(map[string]net.Conn),
			localConnStates:     make(map[string]reverseProxyLocalConnectionState),
			localConnAddrToID:   make(map[string]string),
			localConnAddrByID:   make(map[string]string),
			hijackedConnections: make(map[string]net.Conn),
			connectionSlotIDs:   make(map[string]struct{}),
			resources:           resources,
			listenerConnectionLimiter: func() *reverseProxyAdjustableLimiter {
				limiter := &reverseProxyAdjustableLimiter{}
				limiter.SetMax(resources.ListenerConnectionLimit)
				return limiter
			}(),
			ruleConnectionLimiters: make(map[uint]*reverseProxyAdjustableLimiter),
			requestLimiters:        make(map[uint]*reverseProxyAdjustableLimiter),
			upstreamLimiters:       make(map[uint]*reverseProxyAdjustableLimiter),
		}, nil
	}
	first := rules[0]
	socketKind := reverseProxyListenerSocketKindFromKey(key)
	listenIPs := reverseProxyHTTPRuntimeListenIPs(first)

	group := &reverseProxyListenerGroup{
		key:                 key,
		listenIPs:           listenIPs,
		listenPort:          first.ListenPort,
		protocol:            reverseProxyListenerGroupProtocol(first),
		socketKind:          socketKind,
		h3AvailabilityKnown: socketKind == reverseProxySocketKindUDP && strings.EqualFold(reverseProxyListenerGroupProtocol(first), reverseProxyProtocolHTTPS),
		rules:               rules,
		ruleMatchData:       buildReverseProxyRuleMatchData(rules),
		service:             s,
		certBindingsByRule:  make(map[uint][]*reverseProxyRuleCertificateBinding),
		orderedCertBindings: make([]*reverseProxyRuleCertificateBinding, 0),
		warnings:            make([]string, 0),
		upstreamByRule:      make(map[uint]*reverseProxyCachedUpstream),
		connectionCounts:    make(map[uint]reverseProxyConnectionCounts),
		localConnIDs:        make(map[net.Conn]string),
		localConnByID:       make(map[string]net.Conn),
		localConnStates:     make(map[string]reverseProxyLocalConnectionState),
		localConnAddrToID:   make(map[string]string),
		localConnAddrByID:   make(map[string]string),
		hijackedConnections: make(map[string]net.Conn),
		connectionSlotIDs:   make(map[string]struct{}),
		resources:           resources,
		listenerConnectionLimiter: func() *reverseProxyAdjustableLimiter {
			limiter := &reverseProxyAdjustableLimiter{}
			limiter.SetMax(resources.ListenerConnectionLimit)
			return limiter
		}(),
		ruleConnectionLimiters: make(map[uint]*reverseProxyAdjustableLimiter),
		requestLimiters:        make(map[uint]*reverseProxyAdjustableLimiter),
		upstreamLimiters:       make(map[uint]*reverseProxyAdjustableLimiter),
	}

	certBindingsByRule, orderedCertBindings, err := s.loadRuleCertificates(rules)
	if err != nil {
		return nil, err
	}
	group.certBindingsByRule = certBindingsByRule
	group.orderedCertBindings = orderedCertBindings
	group.configureIPCertificateIndexesLocked()
	if dnsRows := reverseProxyHTTPDNSRuleValues(rules); len(dnsRows) > 0 {
		group.dnsHandler, err = buildReverseProxyDNSRuleHandler(dnsRows)
		if err != nil {
			return nil, err
		}
	}

	group.listenHTTPVersionStrategy = reverseProxyGroupListenHTTPVersionStrategy(group.protocol, socketKind, rules)
	enableTCP := socketKind == reverseProxySocketKindTCP
	enableUDP := socketKind == reverseProxySocketKindUDP
	if enableTCP && group.protocol == reverseProxyProtocolHTTPS && group.listenHTTPVersionStrategy == reverseProxyListenHTTPVersionH2H3 {
		// Keep advertising disabled until reconciliation links this TCP group to
		// a real UDP/H3 group. This closes the short startup window where the H2
		// server can accept requests before the paired UDP group is registered.
		group.h3AvailabilityKnown = true
	}
	group.configureRuleLimitersLocked(rules, resources)

	handler := group.newHandler()
	var firstErr error
	if enableTCP {
		binds := reverseProxyTCPListenBinds(first.ListenPort, group.listenIPs)
		for _, bind := range binds {
			listener, listenErr := net.Listen(bind.network, bind.address)
			if listenErr != nil {
				if firstErr == nil {
					firstErr = reverseProxyExplainListenError(bind.listenIP, first.ListenPort, listenErr)
				}
				if bind.optional && reverseProxyListenErrorAllowsOptionalBind(bind, listenErr) {
					group.warnings = append(group.warnings, "optional reverse proxy listener skipped: "+reverseProxyExplainListenError(bind.listenIP, first.ListenPort, listenErr).Error())
					continue
				}
				_ = group.shutdown()
				return nil, firstErr
			}
			server := &http.Server{
				Handler:           handler,
				ReadHeaderTimeout: reverseProxyReadHeaderTimeout,
				ReadTimeout:       reverseProxyServerReadTimeout,
				IdleTimeout:       reverseProxyServerIdleTimeout,
				MaxHeaderBytes:    reverseProxyMaxHeaderBytes,
				ConnContext: func(ctx context.Context, conn net.Conn) context.Context {
					return group.registerTCPConnectionContext(ctx, conn)
				},
				ConnState: func(conn net.Conn, state http.ConnState) {
					if state == http.StateClosed {
						group.releaseLocalConnectionByConn(conn)
					} else if state == http.StateHijacked {
						group.markHijackedConnection(conn)
					}
				},
			}
			listener = &reverseProxyTrackedClientListener{
				Listener: listener,
				onClose: func(conn net.Conn) {
					group.releaseLocalConnectionByAddrKey(reverseProxyConnectionAddrKey(conn))
				},
			}
			if group.protocol == reverseProxyProtocolHTTPS {
				nextProtos := reverseProxyHTTPSListenerNextProtos(rules)
				tlsConfig := &tls.Config{
					GetCertificate: group.getCertificate,
					MinVersion:     tls.VersionTLS12,
					NextProtos:     nextProtos,
				}
				requiresHTTP2 := false
				for _, protocol := range nextProtos {
					if protocol == "h2" {
						requiresHTTP2 = true
					}
					if protocol == "http/1.1" {
						requiresHTTP2 = false
						break
					}
				}
				if requiresHTTP2 {
					tlsConfig.GetConfigForClient = reverseProxyRequireHTTP2ALPN
				}
				if err := http2.ConfigureServer(server, &http2.Server{MaxConcurrentStreams: resources.HTTP2MaxConcurrentStreams}); err != nil {
					_ = listener.Close()
					_ = group.shutdown()
					return nil, err
				}
				listener = network.NewAutoHttpsListener(listener)
				listener = tls.NewListener(listener, tlsConfig)
			} else {
				listener = network.NewAutoHttpListener(listener)
			}
			group.listeners = append(group.listeners, listener)
			group.servers = append(group.servers, server)
			go func(srv *http.Server, ln net.Listener) {
				if serveErr := srv.Serve(ln); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
					logger.Warning("reverse proxy server serve failed: ", serveErr)
				}
			}(server, listener)
		}
	}
	if group.protocol == reverseProxyProtocolHTTPS && enableUDP {
		udpBinds := reverseProxyUDPListenBinds(first.ListenPort, group.listenIPs)
		for _, bind := range udpBinds {
			packetConn, listenErr := net.ListenPacket(bind.network, bind.address)
			if listenErr != nil {
				if firstErr == nil {
					firstErr = reverseProxyExplainListenError(bind.listenIP, first.ListenPort, listenErr)
				}
				if bind.optional && reverseProxyListenErrorAllowsOptionalBind(bind, listenErr) {
					group.warnings = append(group.warnings, "optional reverse proxy listener skipped: "+reverseProxyExplainListenError(bind.listenIP, first.ListenPort, listenErr).Error())
					continue
				}
				_ = group.shutdown()
				return nil, firstErr
			}
			h3TLS := &tls.Config{
				GetCertificate: func(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
					return group.getCertificate(reverseProxyClientHelloWithLocalIPHint(hello, bind.listenIP))
				},
				MinVersion: tls.VersionTLS13,
			}
			h3Server := &http3.Server{
				Handler:        handler,
				TLSConfig:      h3TLS,
				Port:           first.ListenPort,
				MaxHeaderBytes: reverseProxyMaxHeaderBytes,
				QUICConfig: &quic.Config{
					KeepAlivePeriod:       reverseProxyUpstreamQUICKeepAlivePeriod,
					MaxIdleTimeout:        reverseProxyServerIdleTimeout,
					MaxIncomingStreams:    resources.QUICMaxIncomingStreams,
					MaxIncomingUniStreams: resources.QUICMaxIncomingStreams,
				},
				ConnContext: func(ctx context.Context, conn *quic.Conn) context.Context {
					return group.registerQUICConnectionContext(ctx, conn)
				},
			}
			group.packetConns = append(group.packetConns, packetConn)
			group.h3Servers = append(group.h3Servers, h3Server)
			group.h3ServingCount++
			group.h3ListenerAvailable = true
			go func(srv *http3.Server, conn net.PacketConn) {
				serveErr := srv.Serve(conn)
				if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
					logger.Warning("reverse proxy http3 server serve failed: ", serveErr)
				}
				group.noteHTTP3ServerStopped()
			}(h3Server, packetConn)
		}
	}
	if enableTCP && len(group.listeners) == 0 {
		if firstErr == nil {
			firstErr = common.NewError("reverse proxy listen failed: no tcp listener started")
		}
		return nil, firstErr
	}
	if enableUDP && len(group.packetConns) == 0 {
		if firstErr == nil {
			firstErr = common.NewError("reverse proxy listen failed: no udp listener started")
		}
		_ = group.shutdown()
		return nil, firstErr
	}
	if len(group.listeners) == 0 && len(group.packetConns) == 0 {
		if firstErr == nil {
			firstErr = common.NewError("reverse proxy listen failed: no listener started")
		}
		return nil, firstErr
	}
	if len(group.servers) > 0 {
		group.server = group.servers[0]
	}
	if len(group.listeners) > 0 {
		group.listener = group.listeners[0]
	}
	if len(group.h3Servers) > 0 {
		group.h3Server = group.h3Servers[0]
	}
	if len(group.packetConns) > 0 {
		group.packetConn = group.packetConns[0]
	}
	return group, nil
}

func reverseProxyTCPListenBinds(port int, requestedIPs ...[]string) []reverseProxyListenBind {
	listenIPs := []string{"0.0.0.0", "::"}
	if len(requestedIPs) > 0 {
		listenIPs = reverseProxyCanonicalListenIPs(requestedIPs[0], listenIPs)
	}
	return reverseProxyListenBindsForNetwork("tcp", port, listenIPs)
}

func reverseProxyUDPListenBinds(port int, requestedIPs ...[]string) []reverseProxyListenBind {
	listenIPs := []string{"0.0.0.0", "::"}
	if len(requestedIPs) > 0 {
		listenIPs = reverseProxyCanonicalListenIPs(requestedIPs[0], listenIPs)
	}
	return reverseProxyListenBindsForNetwork("udp", port, listenIPs)
}

func reverseProxyListenBindsForNetwork(protocol string, port int, listenIPs []string) []reverseProxyListenBind {
	binds := make([]reverseProxyListenBind, 0, len(listenIPs))
	for _, listenIP := range listenIPs {
		addr, err := netip.ParseAddr(strings.Trim(strings.TrimSpace(listenIP), "[]"))
		if err != nil {
			continue
		}
		value := addr.Unmap().String()
		networkName := protocol + "6"
		if addr.Is4() || addr.Is4In6() {
			networkName = protocol + "4"
		}
		binds = append(binds, reverseProxyListenBind{
			network:  networkName,
			listenIP: value,
			address:  net.JoinHostPort(value, strconv.Itoa(port)),
			// Keep the historic dual-stack fallback for an unavailable IPv6 stack.
			optional: addr.Is6() && addr.IsUnspecified(),
		})
	}
	return binds
}

func reverseProxyAltSvcValue(port int) string {
	if port <= 0 || port > 65535 {
		port = 443
	}
	return fmt.Sprintf(`h3=":%d"; ma=%d`, port, reverseProxyAltSvcMaxAgeSeconds)
}

func reverseProxyHTTP3ExternalPort(rawHost string) int {
	rawHost = strings.TrimSpace(rawHost)
	if rawHost != "" {
		if _, portText, err := net.SplitHostPort(rawHost); err == nil {
			if port, parseErr := strconv.Atoi(strings.TrimSpace(portText)); parseErr == nil && port > 0 && port <= 65535 {
				return port
			}
		}
	}
	return 443
}

func (g *reverseProxyListenerGroup) http3AdvertisementHeader(host string, sni string, protoMajor int, externalPort int) string {
	if g == nil {
		return ""
	}
	g.mu.RLock()
	protocol := g.protocol
	strategy, err := normalizeReverseProxyListenHTTPVersionStrategy(g.listenHTTPVersionStrategy, protocol)
	if err != nil || strategy == reverseProxyListenHTTPVersionH3Only {
		g.mu.RUnlock()
		return ""
	}
	if !strings.EqualFold(strings.TrimSpace(protocol), reverseProxyProtocolHTTPS) {
		g.mu.RUnlock()
		return ""
	}

	requireSNI := strings.EqualFold(protocol, reverseProxyProtocolHTTPS)
	h3AvailabilityKnown := g.h3AvailabilityKnown
	h3ListenerAvailable := g.h3ListenerAvailable
	originMatched := false
	webSocketMatched := false
	advertiseHTTP3 := false
	for _, rule := range g.rules {
		matched, _ := g.ruleRequestNameMatchLocked(rule, host, sni, requireSNI)
		if !matched {
			continue
		}
		originMatched = true
		listenAlias := normalizeReverseProxyProtocolAlias(rule.ListenProtocolAlias, rule.ListenProtocol)
		targetAlias := normalizeReverseProxyProtocolAlias(rule.TargetProtocolAlias, rule.TargetProtocol)
		if reverseProxyIsWebSocketAlias(listenAlias) || reverseProxyIsWebSocketAlias(targetAlias) {
			webSocketMatched = true
			continue
		}
		if rule.AdvertiseHTTP3 && strategy == reverseProxyListenHTTPVersionH2H3 {
			advertiseHTTP3 = true
		}
	}
	g.mu.RUnlock()
	if !originMatched {
		return ""
	}
	if h3AvailabilityKnown && !h3ListenerAvailable {
		return "clear"
	}
	if webSocketMatched {
		return "clear"
	}
	if advertiseHTTP3 && protoMajor < 3 {
		return reverseProxyAltSvcValue(externalPort)
	}
	if advertiseHTTP3 {
		return ""
	}
	// Clear a cached alternative service even on an existing H3 connection.
	// Otherwise a browser can keep retrying UDP after every rule for this
	// origin disables H3 advertising or the listener switches to H2-only.
	return "clear"
}

func reverseProxyListenErrorAllowsOptionalBind(bind reverseProxyListenBind, err error) bool {
	if err == nil || !bind.optional {
		return false
	}
	lower := strings.ToLower(strings.TrimSpace(err.Error()))
	return strings.Contains(lower, "address family not supported") ||
		strings.Contains(lower, "protocol not available") ||
		strings.Contains(lower, "can't assign requested address") ||
		strings.Contains(lower, "cannot assign requested address")
}

func reverseProxyExplainListenError(listenIP string, port int, err error) error {
	if err == nil {
		return nil
	}

	addr := net.JoinHostPort(strings.TrimSpace(listenIP), strconv.Itoa(port))
	if strings.TrimSpace(listenIP) == "" {
		addr = net.JoinHostPort("0.0.0.0", strconv.Itoa(port))
	}

	var opErr *net.OpError
	if errors.As(err, &opErr) && opErr != nil {
		if errors.Is(opErr.Err, os.ErrPermission) {
			return common.NewError(fmt.Sprintf("reverse proxy listen %s failed: permission denied; linux usually requires root or CAP_NET_BIND_SERVICE for privileged ports", addr))
		}
	}

	lower := strings.ToLower(strings.TrimSpace(err.Error()))
	if strings.Contains(lower, "address already in use") {
		return common.NewError(fmt.Sprintf("reverse proxy listen %s failed: address already in use", addr))
	}
	if strings.Contains(lower, "cannot assign requested address") {
		return common.NewError(fmt.Sprintf("reverse proxy listen %s failed: listen ip is not assigned to this linux host", addr))
	}

	return common.NewError(fmt.Sprintf("reverse proxy listen %s failed: %v", addr, err))
}

func (s *ReverseProxyService) refreshListenerGroup(group *reverseProxyListenerGroup, rules []*model.ReverseProxyRule) error {
	if group == nil {
		return common.NewError("listener group is nil")
	}
	certBindingsByRule, orderedCertBindings, err := s.loadRuleCertificates(rules)
	if err != nil {
		return err
	}
	var nextDNSHandler *reverseProxyDNSRuleHandler
	if dnsRows := reverseProxyHTTPDNSRuleValues(rules); len(dnsRows) > 0 {
		nextDNSHandler, err = buildReverseProxyDNSRuleHandler(dnsRows)
		if err != nil {
			return err
		}
	}
	group.mu.Lock()
	previousRules := append([]*model.ReverseProxyRule(nil), group.rules...)
	group.rules = rules
	group.ruleMatchData = buildReverseProxyRuleMatchData(rules)
	if len(rules) > 0 {
		group.listenIPs = reverseProxyHTTPRuntimeListenIPs(rules[0])
		group.listenHTTPVersionStrategy = reverseProxyGroupListenHTTPVersionStrategy(group.protocol, group.socketKind, rules)
	}
	group.certBindingsByRule = certBindingsByRule
	group.orderedCertBindings = orderedCertBindings
	group.configureIPCertificateIndexesLocked()
	oldDNSHandler := group.dnsHandler
	group.dnsHandler = nextDNSHandler
	group.configureRuleLimitersLocked(rules, reverseProxyResources.current())
	oldUpstreams := group.upstreamByRule
	group.upstreamByRule = make(map[uint]*reverseProxyCachedUpstream)
	group.mu.Unlock()
	_ = closeReverseProxyDNSHandler(oldDNSHandler)
	removedRuleIDs := make(map[uint]struct{})
	activeRuleIDs := make(map[uint]struct{}, len(rules))
	for _, rule := range rules {
		if rule != nil && rule.Id != 0 {
			activeRuleIDs[rule.Id] = struct{}{}
		}
	}
	for _, rule := range previousRules {
		if rule == nil || rule.Id == 0 {
			continue
		}
		if _, stillActive := activeRuleIDs[rule.Id]; !stillActive {
			removedRuleIDs[rule.Id] = struct{}{}
		}
	}
	group.closeHijackedConnectionsForRules(removedRuleIDs)
	for _, upstream := range oldUpstreams {
		group.disposeCachedUpstream(upstream)
	}
	return nil
}

func (g *reverseProxyListenerGroup) acquireCachedUpstream(ruleID uint) *reverseProxyCachedUpstream {
	if g == nil || ruleID == 0 {
		return nil
	}
	g.mu.Lock()
	upstream := g.upstreamByRule[ruleID]
	if upstream == nil || upstream.closing || upstream.RoundTripper == nil {
		g.mu.Unlock()
		return nil
	}
	if !upstream.ResolvedAt.IsZero() && time.Since(upstream.ResolvedAt) >= reverseProxyUpstreamResolveCacheTTL {
		delete(g.upstreamByRule, ruleID)
		g.mu.Unlock()
		g.disposeCachedUpstream(upstream)
		return nil
	}
	upstream.refs++
	g.mu.Unlock()
	return upstream
}

// pruneExpiredCachedUpstreams gives idle transports a bounded lifetime even
// when a rule receives no further requests. Request-time lookup still expires
// entries promptly; this maintenance path releases abandoned idle pools.
func (g *reverseProxyListenerGroup) pruneExpiredCachedUpstreams(now time.Time) {
	if g == nil {
		return
	}
	expired := make([]*reverseProxyCachedUpstream, 0)
	g.mu.Lock()
	for ruleID, upstream := range g.upstreamByRule {
		if upstream == nil || upstream.ResolvedAt.IsZero() || now.Sub(upstream.ResolvedAt) < reverseProxyUpstreamResolveCacheTTL {
			continue
		}
		delete(g.upstreamByRule, ruleID)
		expired = append(expired, upstream)
	}
	g.mu.Unlock()
	for _, upstream := range expired {
		g.disposeCachedUpstream(upstream)
	}
}

func (g *reverseProxyListenerGroup) storeCachedUpstream(ruleID uint, upstream *reverseProxyCachedUpstream) {
	if g == nil || ruleID == 0 || upstream == nil || upstream.RoundTripper == nil {
		return
	}
	g.mu.Lock()
	if g.upstreamByRule == nil {
		g.upstreamByRule = make(map[uint]*reverseProxyCachedUpstream)
	}
	previous := g.upstreamByRule[ruleID]
	upstream.refs++
	g.upstreamByRule[ruleID] = upstream
	g.mu.Unlock()
	g.disposeCachedUpstream(previous)
}

func (g *reverseProxyListenerGroup) releaseCachedUpstream(upstream *reverseProxyCachedUpstream) {
	if g == nil || upstream == nil {
		return
	}
	var cleanup func()
	g.mu.Lock()
	if upstream.refs > 0 {
		upstream.refs--
	}
	if upstream.refs == 0 && upstream.closing && upstream.Cleanup != nil {
		cleanup = upstream.Cleanup
		upstream.Cleanup = nil
	}
	g.mu.Unlock()
	if cleanup != nil {
		cleanup()
	}
}

func (g *reverseProxyListenerGroup) invalidateCachedUpstream(ruleID uint) {
	if g == nil || ruleID == 0 {
		return
	}
	g.mu.Lock()
	upstream := g.upstreamByRule[ruleID]
	delete(g.upstreamByRule, ruleID)
	g.mu.Unlock()
	g.disposeCachedUpstream(upstream)
}

func (g *reverseProxyListenerGroup) disposeCachedUpstream(upstream *reverseProxyCachedUpstream) {
	if g == nil || upstream == nil {
		return
	}
	var cleanup func()
	g.mu.Lock()
	upstream.closing = true
	if upstream.refs == 0 && upstream.Cleanup != nil {
		cleanup = upstream.Cleanup
		upstream.Cleanup = nil
	}
	g.mu.Unlock()
	if cleanup != nil {
		cleanup()
	}
}

func buildReverseProxyTargetURL(rule *model.ReverseProxyRule, hostHeader string) *url.URL {
	if rule == nil {
		return nil
	}
	return &url.URL{
		Scheme: strings.TrimSpace(rule.TargetProtocol),
		Host:   net.JoinHostPort(hostHeader, strconv.Itoa(rule.TargetPort)),
		Path:   normalizeReverseProxyPath(rule.TargetPath, false),
	}
}

func reverseProxyReferencedCertificateIDs(rows []model.ReverseProxyRule) []uint {
	seen := make(map[uint]struct{})
	ids := make([]uint, 0)
	for i := range rows {
		listenAlias := normalizeReverseProxyProtocolAlias(rows[i].ListenProtocolAlias, rows[i].ListenProtocol)
		if !rows[i].Enabled || !reverseProxyListenerUsesManagedCertificates(rows[i].ListenProtocol, listenAlias) {
			continue
		}
		for _, id := range reverseProxyRuleCertificateIDs(&rows[i]) {
			if id == 0 {
				continue
			}
			if _, exists := seen[id]; exists {
				continue
			}
			seen[id] = struct{}{}
			ids = append(ids, id)
		}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

func loadReverseProxyParsedCertificateMaterials(ids []uint, replace bool) (map[uint]reverseProxyParsedCertificateMaterial, error) {
	ids = normalizeReverseProxyCertificateIDList(ids, 0)
	databaseGeneration := reverseProxyDatabaseGeneration.Load()
	generation := currentReverseProxyCertificateGeneration()
	if len(ids) == 0 {
		if replace {
			reverseProxyParsedCertificateMaterials.Lock()
			reverseProxyParsedCertificateMaterials.databaseGeneration = databaseGeneration
			reverseProxyParsedCertificateMaterials.generation = generation
			reverseProxyParsedCertificateMaterials.items = make(map[uint]reverseProxyParsedCertificateMaterial)
			reverseProxyParsedCertificateMaterials.Unlock()
		}
		return map[uint]reverseProxyParsedCertificateMaterial{}, nil
	}
	if !replace {
		reverseProxyParsedCertificateMaterials.RLock()
		allPresent := reverseProxyParsedCertificateMaterials.databaseGeneration == databaseGeneration &&
			reverseProxyParsedCertificateMaterials.generation == generation
		result := make(map[uint]reverseProxyParsedCertificateMaterial, len(ids))
		if allPresent {
			for _, id := range ids {
				item, exists := reverseProxyParsedCertificateMaterials.items[id]
				if !exists {
					allPresent = false
					break
				}
				result[id] = item
			}
		}
		reverseProxyParsedCertificateMaterials.RUnlock()
		if allPresent {
			return result, nil
		}
	}

	db := database.GetDB()
	if db == nil {
		return nil, common.NewError("database is not ready")
	}
	records := make([]model.CertificateRecord, 0, len(ids))
	if err := db.Select("id", "fullchain_pem", "key_pem", "fingerprint", "updated_at").Where("id IN ?", ids).Find(&records).Error; err != nil {
		return nil, err
	}
	byID := make(map[uint]model.CertificateRecord, len(records))
	for i := range records {
		byID[records[i].Id] = records[i]
	}
	loaded := make(map[uint]reverseProxyParsedCertificateMaterial, len(ids))
	for _, id := range ids {
		record, exists := byID[id]
		if !exists {
			loaded[id] = reverseProxyParsedCertificateMaterial{Err: common.NewError("certificate not found")}
			continue
		}
		certificate, leaf, err := reverseProxyLoadCertificate(&record)
		loaded[id] = reverseProxyParsedCertificateMaterial{Certificate: certificate, Leaf: leaf, Err: err}
	}
	if reverseProxyDatabaseGeneration.Load() != databaseGeneration || currentReverseProxyCertificateGeneration() != generation {
		return nil, common.NewError("certificate inventory changed during reverse proxy refresh")
	}
	reverseProxyParsedCertificateMaterials.Lock()
	if replace ||
		reverseProxyParsedCertificateMaterials.databaseGeneration != databaseGeneration ||
		reverseProxyParsedCertificateMaterials.generation != generation {
		reverseProxyParsedCertificateMaterials.items = make(map[uint]reverseProxyParsedCertificateMaterial, len(loaded))
	}
	for id, item := range loaded {
		reverseProxyParsedCertificateMaterials.items[id] = item
	}
	reverseProxyParsedCertificateMaterials.databaseGeneration = databaseGeneration
	reverseProxyParsedCertificateMaterials.generation = generation
	reverseProxyParsedCertificateMaterials.Unlock()
	return loaded, nil
}

func prepareReverseProxyParsedCertificateMaterials(rows []model.ReverseProxyRule) error {
	_, err := loadReverseProxyParsedCertificateMaterials(reverseProxyReferencedCertificateIDs(rows), true)
	return err
}

func (s *ReverseProxyService) loadRuleCertificates(rules []*model.ReverseProxyRule) (map[uint][]*reverseProxyRuleCertificateBinding, []*reverseProxyRuleCertificateBinding, error) {
	certBindingsByRule := make(map[uint][]*reverseProxyRuleCertificateBinding)
	orderedCertBindings := make([]*reverseProxyRuleCertificateBinding, 0)
	ids := make([]uint, 0)
	for _, rule := range rules {
		if rule != nil && reverseProxyListenerUsesManagedCertificates(rule.ListenProtocol, normalizeReverseProxyProtocolAlias(rule.ListenProtocolAlias, rule.ListenProtocol)) {
			ids = append(ids, reverseProxyRuleCertificateIDs(rule)...)
		}
	}
	materials, err := loadReverseProxyParsedCertificateMaterials(ids, false)
	if err != nil {
		return nil, nil, err
	}
	for _, rule := range rules {
		if rule == nil || !reverseProxyListenerUsesManagedCertificates(rule.ListenProtocol, normalizeReverseProxyProtocolAlias(rule.ListenProtocolAlias, rule.ListenProtocol)) {
			continue
		}
		for _, certID := range reverseProxyRuleCertificateIDs(rule) {
			material, exists := materials[certID]
			if !exists {
				return nil, nil, common.NewError("certificate not found")
			}
			if material.Err != nil {
				return nil, nil, material.Err
			}
			binding := &reverseProxyRuleCertificateBinding{
				RuleID:              rule.Id,
				CertificateRecordID: certID,
				Certificate:         material.Certificate,
				Leaf:                material.Leaf,
			}
			certBindingsByRule[rule.Id] = append(certBindingsByRule[rule.Id], binding)
			orderedCertBindings = append(orderedCertBindings, binding)
		}
	}
	return certBindingsByRule, orderedCertBindings, nil
}

func reverseProxyCertificateBindingIPNames(binding *reverseProxyRuleCertificateBinding) []string {
	if binding == nil || binding.Leaf == nil || binding.Leaf.Leaf == nil {
		return nil
	}
	result := make([]string, 0, len(binding.Leaf.Leaf.IPAddresses))
	seen := make(map[string]struct{}, len(binding.Leaf.Leaf.IPAddresses))
	for _, raw := range binding.Leaf.Leaf.IPAddresses {
		addr, ok := netip.AddrFromSlice(raw)
		if !ok {
			continue
		}
		value := addr.Unmap().String()
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func (g *reverseProxyListenerGroup) configureIPCertificateIndexesLocked() {
	if g == nil {
		return
	}
	g.ipCertBindings = make(map[string][]*reverseProxyRuleCertificateBinding)
	g.ipCertificateUniverse = make(map[string]struct{})
	g.natFallbackCertBindings = make(map[string][]*reverseProxyRuleCertificateBinding)
	certIPs := make(map[uint]map[string]struct{})
	universeByFamily := map[string]map[string]struct{}{
		"4": {},
		"6": {},
	}
	for _, binding := range g.orderedCertBindings {
		if binding == nil || binding.CertificateRecordID == 0 {
			continue
		}
		for _, value := range reverseProxyCertificateBindingIPNames(binding) {
			g.ipCertBindings[value] = append(g.ipCertBindings[value], binding)
			g.ipCertificateUniverse[value] = struct{}{}
			if family := reverseProxyIPLiteralFamily(value); family != "" {
				universeByFamily[family][value] = struct{}{}
			}
			items := certIPs[binding.CertificateRecordID]
			if items == nil {
				items = make(map[string]struct{})
				certIPs[binding.CertificateRecordID] = items
			}
			items[value] = struct{}{}
		}
	}
	for family, universe := range universeByFamily {
		if len(universe) == 0 {
			continue
		}
		for _, binding := range g.orderedCertBindings {
			if binding == nil {
				continue
			}
			coversAll := true
			for value := range universe {
				if _, exists := certIPs[binding.CertificateRecordID][value]; !exists {
					coversAll = false
					break
				}
			}
			if coversAll {
				g.natFallbackCertBindings[family] = append(g.natFallbackCertBindings[family], binding)
			}
		}
	}
}

func reverseProxyIPLiteralFamily(value string) string {
	addr, err := netip.ParseAddr(strings.Trim(strings.TrimSpace(value), "[]"))
	if err != nil {
		return ""
	}
	if addr.Unmap().Is4() {
		return "4"
	}
	return "6"
}

func (g *reverseProxyListenerGroup) natFallbackCertificatesForLocalIP(ip net.IP) []*reverseProxyRuleCertificateBinding {
	if g == nil || ip == nil {
		return nil
	}
	return g.natFallbackCertBindings[reverseProxyIPLiteralFamily(ip.String())]
}

func reverseProxyListenerUsesManagedCertificates(listenProtocol string, listenAlias string) bool {
	normalizedProtocol := strings.ToLower(strings.TrimSpace(listenProtocol))
	normalizedAlias := strings.ToLower(strings.TrimSpace(listenAlias))
	if normalizedProtocol == reverseProxyProtocolHTTPS {
		return true
	}
	return normalizedProtocol == reverseProxyProtocolDNS && reverseProxyDNSProtocolUsesTLS(normalizedAlias)
}

func reverseProxyLoadCertificate(record *model.CertificateRecord) (*tls.Certificate, *x509LeafState, error) {
	if record == nil {
		return nil, nil, common.NewError("certificate record is nil")
	}
	if len(record.FullchainPEM) == 0 || len(record.KeyPEM) == 0 {
		return nil, nil, common.NewError("certificate material is incomplete")
	}
	pair, err := tls.X509KeyPair(record.FullchainPEM, record.KeyPEM)
	if err != nil {
		return nil, nil, err
	}
	parsedLeaf, err := network.ParseLeafCertificate(&pair)
	if err != nil {
		return nil, nil, err
	}
	return &pair, &x509LeafState{
		Certificate: &pair,
		Leaf:        parsedLeaf,
		Fingerprint: strings.TrimSpace(record.Fingerprint),
		NotAfter:    parsedLeaf.NotAfter,
		HasIPSAN:    len(parsedLeaf.IPAddresses) > 0,
	}, nil
}

func (g *reverseProxyListenerGroup) getCertificate(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	serverName := ""
	connAddrKey := ""
	localIP := ""
	if hello != nil {
		serverName = reverseProxyNormalizeServerName(hello.ServerName)
		if hello.Conn != nil {
			if _, isLocalHint := hello.Conn.(*reverseProxyClientHelloLocalHintConn); !isLocalHint {
				connAddrKey = reverseProxyConnectionAddrKey(hello.Conn)
			}
			localIP = reverseProxyNormalizeLocalIP(hello.Conn.LocalAddr())
		}
	}
	remoteIP := ""
	if hello != nil && hello.Conn != nil {
		remoteIP = extractRemoteIP(hello.Conn.RemoteAddr().String())
	}
	if serverName == "" {
		requestedIP := ""
		var candidates []*reverseProxyRuleCertificateBinding
		if localAddr := reverseProxyParseIPLiteral(localIP); localAddr != nil {
			canonical := localAddr.String()
			if _, configured := g.ipCertificateUniverse[canonical]; configured {
				requestedIP = canonical
				candidates = g.ipCertBindings[canonical]
			} else if reverseProxyLocalAddressMayHidePublicTarget(localAddr) {
				candidates = g.natFallbackCertificatesForLocalIP(localAddr)
			}
		}
		if certificate, ok := g.selectAndBindCertificate(candidates, requestedIP, connAddrKey, reverseProxyTLSClientNoSNI, requestedIP); ok {
			return certificate, nil
		}
		reverseProxyRuntime.registerMismatch(remoteIP, "missing_sni")
		reverseProxyCloseClientHelloConn(hello)
		return nil, common.NewError("tls listener has no unambiguous ip certificate for an empty sni")
	}
	if sniIP := reverseProxyParseIPLiteral(serverName); sniIP != nil {
		requestedIP := sniIP.String()
		localAddr := reverseProxyParseIPLiteral(localIP)
		if localAddr != nil {
			localValue := localAddr.String()
			_, configured := g.ipCertificateUniverse[localValue]
			if (configured || !reverseProxyLocalAddressMayHidePublicTarget(localAddr)) && !sniIP.Equal(localAddr) {
				reverseProxyRuntime.registerMismatch(remoteIP, "cross_ip_sni")
				reverseProxyCloseClientHelloConn(hello)
				return nil, common.NewError("tls ip sni does not match the visible local target ip")
			}
		}
		candidates := g.ipCertBindings[requestedIP]
		if certificate, ok := g.selectAndBindCertificate(candidates, requestedIP, connAddrKey, reverseProxyTLSClientIP, requestedIP); ok {
			return certificate, nil
		}
		if len(candidates) == 0 {
			reverseProxyRuntime.registerMismatch(remoteIP, "cross_ip_sni")
			reverseProxyCloseClientHelloConn(hello)
			return nil, common.NewError("tls listener has no certificate covering the ip sni")
		}
		reverseProxyRuntime.registerMismatch(remoteIP, "ip_sni")
		reverseProxyCloseClientHelloConn(hello)
		return nil, common.NewError("tls listener ip certificate is unavailable")
	}
	matchedRule := false
	exactCandidates := make([]*reverseProxyRuleCertificateBinding, 0)
	wildcardCandidates := make([]*reverseProxyRuleCertificateBinding, 0)
	for _, rule := range g.rules {
		if !reverseProxyRuleServerNameMatch(rule, serverName) {
			continue
		}
		matchedRule = true
		exactByRule, wildcardByRule := reverseProxySplitSNICertificateCandidates(g.certBindingsByRule[rule.Id], serverName)
		exactCandidates = append(exactCandidates, exactByRule...)
		wildcardCandidates = append(wildcardCandidates, wildcardByRule...)
	}
	if selected, selection, err := g.selectBalancedCertificate(exactCandidates, serverName); err == nil && selected != nil {
		selection.ClientKind = reverseProxyTLSClientDomain
		g.bindCertificateSelectionToConnection(connAddrKey, selection)
		return selected.Certificate, nil
	}
	if selected, selection, err := g.selectBalancedCertificate(wildcardCandidates, serverName); err == nil && selected != nil {
		selection.ClientKind = reverseProxyTLSClientDomain
		g.bindCertificateSelectionToConnection(connAddrKey, selection)
		return selected.Certificate, nil
	}
	reverseProxyRuntime.registerMismatch(remoteIP, "unrecognized_sni")
	reverseProxyCloseClientHelloConn(hello)
	if matchedRule {
		return nil, common.NewError("tls listener certificate is unavailable")
	}
	return nil, common.NewError("unrecognized tls sni")
}

func (g *reverseProxyListenerGroup) selectAndBindCertificate(candidates []*reverseProxyRuleCertificateBinding, sniBucket string, connAddrKey string, clientKind string, requestedIP string) (*tls.Certificate, bool) {
	usable := make([]*reverseProxyRuleCertificateBinding, 0, len(candidates))
	for _, candidate := range candidates {
		if reverseProxyCertificateBindingUsable(candidate, time.Now()) {
			usable = append(usable, candidate)
		}
	}
	selected, selection, err := g.selectBalancedCertificate(usable, sniBucket)
	if err != nil || selected == nil {
		return nil, false
	}
	selection.ClientKind = clientKind
	selection.RequestedIP = strings.TrimSpace(requestedIP)
	g.bindCertificateSelectionToConnection(connAddrKey, selection)
	return selected.Certificate, true
}

func reverseProxyCloseClientHelloConn(hello *tls.ClientHelloInfo) {
	if hello == nil || hello.Conn == nil {
		return
	}
	_ = hello.Conn.Close()
}

func (g *reverseProxyListenerGroup) bindCertificateSelectionToConnection(connAddrKey string, selection reverseProxyCertificateSelection) {
	if g == nil || selection.CertificateRecordID == 0 || strings.TrimSpace(selection.ListenerKey) == "" {
		return
	}
	if strings.TrimSpace(connAddrKey) == "" {
		// A certificate may be selected by a unit test, an unusual listener
		// wrapper, or a connection whose addresses are unavailable.  It cannot
		// be tied to a close event in that case, so release the balance lease
		// immediately instead of retaining it until process shutdown.
		g.releaseCertificateSelection(selection)
		return
	}
	selectionsToRelease := make([]reverseProxyCertificateSelection, 0)
	g.statsMu.Lock()
	if g.localConnStates == nil {
		g.localConnStates = make(map[string]reverseProxyLocalConnectionState)
	}
	if g.localConnAddrToID == nil {
		g.localConnAddrToID = make(map[string]string)
	}
	selectionsToRelease = append(selectionsToRelease, g.prunePendingCertificateSelectionsLocked(connAddrKey, time.Now())...)
	connID := strings.TrimSpace(g.localConnAddrToID[connAddrKey])
	if connID != "" {
		if pending, exists := g.takePendingCertificateSelectionLocked(connAddrKey); exists {
			selectionsToRelease = append(selectionsToRelease, pending)
		}
		state := g.localConnStates[connID]
		if state.HasSelection {
			selectionsToRelease = append(selectionsToRelease, state.Selection)
		}
		state.Selection = selection
		state.HasSelection = true
		g.localConnStates[connID] = state
		g.statsMu.Unlock()
		for _, previous := range selectionsToRelease {
			g.releaseCertificateSelection(previous)
		}
		return
	}
	selectionsToRelease = append(selectionsToRelease, g.putPendingCertificateSelectionLocked(connAddrKey, selection, time.Now())...)
	g.statsMu.Unlock()
	for _, previous := range selectionsToRelease {
		g.releaseCertificateSelection(previous)
	}
}

func (g *reverseProxyListenerGroup) pendingCertificateShard(connAddrKey string) *reverseProxyPendingCertificateShard {
	if g == nil {
		return nil
	}
	index := int(crc32.ChecksumIEEE([]byte(strings.TrimSpace(connAddrKey))) % reverseProxyRuntimeTableShardCount)
	return &g.pendingConnSelectionShards[index]
}

func (g *reverseProxyListenerGroup) prunePendingCertificateSelectionsLocked(connAddrKey string, now time.Time) []reverseProxyCertificateSelection {
	shard := g.pendingCertificateShard(connAddrKey)
	if shard == nil || len(shard.selections) == 0 {
		return nil
	}
	if shard.lru == nil {
		shard.lru = list.New()
		for key, pending := range shard.selections {
			if pending == nil {
				delete(shard.selections, key)
				continue
			}
			pending.element = shard.lru.PushFront(key)
		}
	}
	released := make([]reverseProxyCertificateSelection, 0)
	for shard.lru.Len() > 0 {
		oldestKey, _ := shard.lru.Back().Value.(string)
		pending := shard.selections[oldestKey]
		if pending != nil && !pending.CreatedAt.IsZero() && now.Sub(pending.CreatedAt) < reverseProxyRuntimeTableTTL {
			break
		}
		if pending != nil {
			released = append(released, pending.Selection)
			if pending.element != nil {
				shard.lru.Remove(pending.element)
			}
		}
		delete(shard.selections, oldestKey)
	}
	return released
}

func (g *reverseProxyListenerGroup) putPendingCertificateSelectionLocked(connAddrKey string, selection reverseProxyCertificateSelection, now time.Time) []reverseProxyCertificateSelection {
	if g == nil || strings.TrimSpace(connAddrKey) == "" {
		return nil
	}
	shard := g.pendingCertificateShard(connAddrKey)
	if shard == nil {
		return nil
	}
	if shard.selections == nil {
		shard.selections = make(map[string]*reverseProxyPendingCertificateSelection)
	}
	if shard.lru == nil {
		shard.lru = list.New()
	}
	released := g.prunePendingCertificateSelectionsLocked(connAddrKey, now)
	if previous := shard.selections[connAddrKey]; previous != nil {
		released = append(released, previous.Selection)
		if previous.element != nil {
			shard.lru.Remove(previous.element)
		}
		delete(shard.selections, connAddrKey)
	}
	for len(shard.selections) >= reverseProxyPendingCertificateShardLimit && shard.lru.Len() > 0 {
		oldestKey, _ := shard.lru.Back().Value.(string)
		oldest := shard.selections[oldestKey]
		if oldest != nil {
			released = append(released, oldest.Selection)
			if oldest.element != nil {
				shard.lru.Remove(oldest.element)
			}
		}
		delete(shard.selections, oldestKey)
	}
	pending := &reverseProxyPendingCertificateSelection{Selection: selection, CreatedAt: now}
	pending.element = shard.lru.PushFront(connAddrKey)
	shard.selections[connAddrKey] = pending
	return released
}

func (g *reverseProxyListenerGroup) takePendingCertificateSelectionLocked(connAddrKey string) (reverseProxyCertificateSelection, bool) {
	if g == nil || strings.TrimSpace(connAddrKey) == "" {
		return reverseProxyCertificateSelection{}, false
	}
	shard := g.pendingCertificateShard(connAddrKey)
	if shard == nil {
		return reverseProxyCertificateSelection{}, false
	}
	pending := shard.selections[connAddrKey]
	if pending == nil {
		return reverseProxyCertificateSelection{}, false
	}
	if pending.element != nil && shard.lru != nil {
		shard.lru.Remove(pending.element)
	}
	delete(shard.selections, connAddrKey)
	return pending.Selection, true
}

func (g *reverseProxyListenerGroup) releaseCertificateSelection(selection reverseProxyCertificateSelection) {
	if g == nil {
		return
	}
	if selection.CertificateRecordID == 0 || strings.TrimSpace(selection.ListenerKey) == "" {
		return
	}
	g.releaseCertificateBalanceSelection(selection)
}

func (g *reverseProxyListenerGroup) certificateSelectionFromContext(ctx context.Context) (reverseProxyCertificateSelection, bool) {
	connID := g.connectionIDFromContext(ctx)
	if g == nil || connID == "" {
		return reverseProxyCertificateSelection{}, false
	}
	g.statsMu.Lock()
	state, exists := g.localConnStates[connID]
	g.statsMu.Unlock()
	return state.Selection, exists && state.HasSelection
}

func (g *reverseProxyListenerGroup) newHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r == nil {
			http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
			return
		}
		host := reverseProxyNormalizeRequestHost(r.Host)
		externalPort := reverseProxyHTTP3ExternalPort(r.Host)
		path := normalizeReverseProxyPath(r.URL.Path, true)
		sni := ""
		if r.TLS != nil {
			sni = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(r.TLS.ServerName), "."))
		}
		selection, hasSelection := g.certificateSelectionFromContext(r.Context())
		rule, nameMatched := g.findRuleWithSelection(host, sni, path, selection, hasSelection)
		if rule == nil {
			status := http.StatusNotFound
			if r.TLS != nil && !nameMatched {
				if delay := reverseProxyRuntime.registerMismatch(extractRemoteIP(r.RemoteAddr), "host_sni_mismatch"); delay > 0 {
					w.Header().Set("Retry-After", strconv.Itoa(int(delay.Seconds())))
					http.Error(w, http.StatusText(http.StatusTooManyRequests), http.StatusTooManyRequests)
					return
				}
				status = http.StatusMisdirectedRequest
			} else if nameMatched {
				if altSvc := g.http3AdvertisementHeader(host, sni, r.ProtoMajor, externalPort); altSvc != "" {
					w.Header().Set("Alt-Svc", altSvc)
				}
			}
			http.Error(w, http.StatusText(status), status)
			return
		}
		if reverseProxyIsHTTP3WebSocketRequest(r) {
			http.Error(w, http.StatusText(http.StatusNotImplemented), http.StatusNotImplemented)
			return
		}
		listenAlias := normalizeReverseProxyProtocolAlias(rule.ListenProtocolAlias, rule.ListenProtocol)
		targetAlias := normalizeReverseProxyProtocolAlias(rule.TargetProtocolAlias, rule.TargetProtocol)
		g.mu.RLock()
		strictHTTPS2 := strings.EqualFold(g.protocol, reverseProxyProtocolHTTPS) && g.socketKind == reverseProxySocketKindTCP
		g.mu.RUnlock()
		if strictHTTPS2 && !reverseProxyIsWebSocketAlias(listenAlias) && r.ProtoMajor != 2 {
			http.Error(w, http.StatusText(http.StatusHTTPVersionNotSupported), http.StatusHTTPVersionNotSupported)
			return
		}
		altSvc := g.http3AdvertisementHeader(host, sni, r.ProtoMajor, externalPort)
		if (reverseProxyIsWebSocketAlias(listenAlias) || reverseProxyIsWebSocketAlias(targetAlias)) && !reverseProxyIsWebSocketUpgradeRequest(r) {
			if altSvc != "" {
				w.Header().Set("Alt-Svc", altSvc)
			}
			w.Header().Set("Upgrade", "websocket")
			http.Error(w, http.StatusText(http.StatusUpgradeRequired), http.StatusUpgradeRequired)
			return
		}
		if connID := g.connectionIDFromContext(r.Context()); connID != "" {
			if !g.registerLocalConnectionRule(rule.Id, connID) {
				w.Header().Set("Retry-After", "1")
				http.Error(w, http.StatusText(http.StatusTooManyRequests), http.StatusTooManyRequests)
				return
			}
			if reverseProxyIsWebSocketUpgradeRequest(r) {
				g.setHijackedConnectionRule(connID, rule.Id)
			}
		}
		if reverseProxyIsHTTPDNSAlias(listenAlias) {
			g.mu.RLock()
			dnsHandler := g.dnsHandler
			g.mu.RUnlock()
			if dnsHandler == nil {
				http.Error(w, http.StatusText(http.StatusServiceUnavailable), http.StatusServiceUnavailable)
				return
			}
			reverseProxyRuntime.clearMismatch(extractRemoteIP(r.RemoteAddr))
			dnsHandler.serveDoHRule(w, r, rule.Id)
			return
		}
		if !reverseProxyResources.tryAcquireHTTP() {
			w.Header().Set("Retry-After", "1")
			http.Error(w, http.StatusText(http.StatusTooManyRequests), http.StatusTooManyRequests)
			return
		}
		defer reverseProxyResources.releaseHTTP()
		requestLimiter, acquired := g.acquireRequestSlot(rule.Id)
		if !acquired {
			w.Header().Set("Retry-After", "1")
			http.Error(w, http.StatusText(http.StatusTooManyRequests), http.StatusTooManyRequests)
			return
		}
		defer g.releaseRequestSlot(requestLimiter)
		reverseProxyRuntime.clearMismatch(extractRemoteIP(r.RemoteAddr))
		g.forwardRequest(w, r, rule, altSvc)
	})
}

func (g *reverseProxyListenerGroup) acquireRequestSlot(ruleID uint) (*reverseProxyAdjustableLimiter, bool) {
	if g == nil || ruleID == 0 {
		return nil, true
	}
	g.mu.RLock()
	limiter := g.requestLimiters[ruleID]
	g.mu.RUnlock()
	if limiter == nil {
		return nil, true
	}
	return limiter, limiter.TryAcquire()
}

func (g *reverseProxyListenerGroup) releaseRequestSlot(limiter *reverseProxyAdjustableLimiter) {
	if limiter != nil {
		limiter.Release()
	}
}

func (g *reverseProxyListenerGroup) findRule(host string, sni string, path string) (*model.ReverseProxyRule, bool) {
	return g.findRuleWithSelection(host, sni, path, reverseProxyCertificateSelection{}, false)
}

func (g *reverseProxyListenerGroup) findRuleWithSelection(host string, sni string, path string, selection reverseProxyCertificateSelection, hasSelection bool) (*model.ReverseProxyRule, bool) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	nameMatched := false
	requireSNI := strings.EqualFold(g.protocol, reverseProxyProtocolHTTPS)
	var best *model.ReverseProxyRule
	bestSpecificity := int(^uint(0) >> 1)
	for _, rule := range g.rules {
		matched, _ := g.ruleRequestNameMatchWithSelectionLocked(rule, host, sni, requireSNI, selection, hasSelection)
		if !matched {
			continue
		}
		nameMatched = true
		if !g.ruleRequestPathMatchLocked(rule, path) {
			continue
		}
		hostIP := reverseProxyParseIPLiteral(reverseProxyNormalizeServerName(host))
		if hostIP == nil {
			return rule, true
		}
		specificity := g.ruleIPSpecificityLocked(rule, hostIP.String())
		if specificity < bestSpecificity {
			best = rule
			bestSpecificity = specificity
		}
	}
	return best, nameMatched
}

func (g *reverseProxyListenerGroup) ruleRequestNameMatchLocked(rule *model.ReverseProxyRule, host string, sni string, requireSNI bool) (bool, bool) {
	return g.ruleRequestNameMatchWithSelectionLocked(rule, host, sni, requireSNI, reverseProxyCertificateSelection{}, false)
}

func (g *reverseProxyListenerGroup) ruleRequestNameMatchWithSelectionLocked(rule *model.ReverseProxyRule, host string, sni string, requireSNI bool, selection reverseProxyCertificateSelection, hasSelection bool) (bool, bool) {
	if rule == nil {
		return false, false
	}
	candidates := g.ruleServerNamesLocked(rule)
	host = reverseProxyNormalizeServerName(host)
	sni = reverseProxyNormalizeServerName(sni)
	hostIP := reverseProxyParseIPLiteral(host)
	sniIP := reverseProxyParseIPLiteral(sni)
	if !requireSNI {
		if hostIP != nil {
			return len(candidates) == 0, true
		}
		matched := host != "" && len(candidates) > 0 && reverseProxyRequestNameMatchesAny(candidates, host)
		return matched, matched
	}
	if sni == "" || sniIP != nil {
		if hostIP == nil {
			return false, false
		}
		requestedIP := hostIP.String()
		if sniIP != nil && !hostIP.Equal(sniIP) {
			return false, true
		}
		if hasSelection {
			if sni == "" && selection.ClientKind != reverseProxyTLSClientNoSNI {
				return false, false
			}
			if sniIP != nil && selection.ClientKind != reverseProxyTLSClientIP {
				return false, false
			}
			if !g.certificateSelectionCoversIPLocked(selection, requestedIP) {
				return false, true
			}
			if !g.ruleHasCertificateSelectionLocked(rule, selection, requestedIP) {
				return false, true
			}
		}
		matched := g.ruleHasMatchingIPCertificateLocked(rule, requestedIP)
		return matched, matched
	}
	if len(candidates) == 0 || host == "" || hostIP != nil {
		return false, false
	}
	hostMatch := reverseProxyRequestNameMatchesAny(candidates, host)
	sniMatch := reverseProxyRequestNameMatchesAny(candidates, sni)
	partial := hostMatch || sniMatch
	if !hostMatch || !sniMatch || !strings.EqualFold(host, sni) {
		return false, partial
	}
	if hasSelection && selection.ClientKind != reverseProxyTLSClientDomain {
		return false, true
	}
	if hasSelection && !g.ruleHasCertificateSelectionLocked(rule, selection, sni) {
		return false, true
	}
	return true, true
}

func buildReverseProxyRuleMatchData(rules []*model.ReverseProxyRule) map[uint]reverseProxyRuleMatchData {
	data := make(map[uint]reverseProxyRuleMatchData, len(rules))
	for _, rule := range rules {
		if rule == nil || rule.Id == 0 {
			continue
		}
		listenAlias := normalizeReverseProxyProtocolAlias(rule.ListenProtocolAlias, rule.ListenProtocol)
		matchData := reverseProxyRuleMatchData{
			serverNames: reverseProxyRuleServerNames(rule),
			listenAlias: listenAlias,
			pathPrefix:  reverseProxyNormalizePathPrefix(rule.PathPrefix),
		}
		if reverseProxyIsHTTPDNSAlias(listenAlias) {
			matchData.dnsPath = reverseProxyDNSRulePath(rule)
		}
		data[rule.Id] = matchData
	}
	return data
}

func (g *reverseProxyListenerGroup) ruleServerNamesLocked(rule *model.ReverseProxyRule) []string {
	if g == nil || rule == nil {
		return nil
	}
	if rule.Id != 0 {
		if data, exists := g.ruleMatchData[rule.Id]; exists {
			return data.serverNames
		}
	}
	return reverseProxyRuleServerNames(rule)
}

func (g *reverseProxyListenerGroup) ruleRequestPathMatchLocked(rule *model.ReverseProxyRule, path string) bool {
	if g == nil || rule == nil {
		return false
	}
	data, exists := g.ruleMatchData[rule.Id]
	if !exists || rule.Id == 0 {
		return reverseProxyRuleRequestPathMatch(rule, path)
	}
	if reverseProxyIsHTTPDNSAlias(data.listenAlias) {
		actual := normalizeReverseProxyDNSPath(path)
		if actual == "" {
			actual = "/"
		}
		return actual == data.dnsPath
	}
	if data.pathPrefix == "" {
		return true
	}
	actual := normalizeReverseProxyPath(path, true)
	if actual == "" {
		actual = "/"
	}
	return actual == data.pathPrefix || strings.HasPrefix(actual, data.pathPrefix+"/")
}

func (g *reverseProxyListenerGroup) ruleHasCertificateSelectionLocked(rule *model.ReverseProxyRule, selection reverseProxyCertificateSelection, serverName string) bool {
	if g == nil || rule == nil || selection.CertificateRecordID == 0 {
		return false
	}
	for _, binding := range g.certBindingsByRule[rule.Id] {
		if binding != nil && binding.CertificateRecordID == selection.CertificateRecordID && reverseProxyCertificateBindingMatchesServerName(binding, serverName) {
			return true
		}
	}
	return false
}

func (g *reverseProxyListenerGroup) ruleHasMatchingIPCertificateLocked(rule *model.ReverseProxyRule, ip string) bool {
	if g == nil || rule == nil || reverseProxyParseIPLiteral(ip) == nil {
		return false
	}
	for _, binding := range g.certBindingsByRule[rule.Id] {
		if !reverseProxyCertificateBindingHasIPSAN(binding) {
			continue
		}
		if reverseProxyCertificateBindingMatchesServerName(binding, ip) {
			return true
		}
	}
	return false
}

func (g *reverseProxyListenerGroup) certificateSelectionCoversIPLocked(selection reverseProxyCertificateSelection, ip string) bool {
	if g == nil || selection.CertificateRecordID == 0 || reverseProxyParseIPLiteral(ip) == nil {
		return false
	}
	for _, binding := range g.ipCertBindings[reverseProxyParseIPLiteral(ip).String()] {
		if binding != nil && binding.CertificateRecordID == selection.CertificateRecordID && reverseProxyCertificateBindingMatchesServerName(binding, ip) {
			return true
		}
	}
	return false
}

func (g *reverseProxyListenerGroup) ruleIPSpecificityLocked(rule *model.ReverseProxyRule, ip string) int {
	best := int(^uint(0) >> 1)
	for _, binding := range g.certBindingsByRule[rule.Id] {
		if !reverseProxyCertificateBindingMatchesServerName(binding, ip) {
			continue
		}
		count := len(reverseProxyCertificateBindingIPNames(binding))
		if count > 0 && count < best {
			best = count
		}
	}
	return best
}

func reverseProxyRuleRequestNameMatchDetail(rule *model.ReverseProxyRule, host string, sni string, requireSNI bool) (bool, bool) {
	if rule == nil {
		return false, false
	}
	candidates := reverseProxyRuleServerNames(rule)
	host = reverseProxyNormalizeServerName(host)
	sni = reverseProxyNormalizeServerName(sni)
	hostIsIP := reverseProxyParseIPLiteral(host) != nil
	sniIsIP := reverseProxyParseIPLiteral(sni) != nil
	if requireSNI && (sni == "" || sniIsIP) {
		matched := hostIsIP && (sni == "" || reverseProxyIPLiteralEqual(host, sni))
		return matched, matched
	}
	if len(candidates) == 0 || host == "" || hostIsIP {
		return false, false
	}
	hostMatch := reverseProxyRequestNameMatchesAny(candidates, host)
	sniMatch := sni != "" && !sniIsIP && reverseProxyRequestNameMatchesAny(candidates, sni)
	partial := hostMatch || sniMatch
	if requireSNI {
		return hostMatch && sniMatch && strings.EqualFold(host, sni), partial
	}
	return hostMatch, hostMatch
}

func reverseProxyRequestNameMatchesAny(candidates []string, name string) bool {
	name = reverseProxyNormalizeServerName(name)
	if name == "" {
		return false
	}
	for _, candidate := range candidates {
		if reverseProxyHostPatternMatches(candidate, name) {
			return true
		}
	}
	return false
}

func reverseProxyRuleRequestNameMatch(rule *model.ReverseProxyRule, host string, sni string) bool {
	matched, _ := reverseProxyRuleRequestNameMatchDetail(rule, host, sni, false)
	return matched
}

func reverseProxyRuleServerNameMatch(rule *model.ReverseProxyRule, serverName string) bool {
	serverName = reverseProxyNormalizeServerName(serverName)
	if rule == nil || serverName == "" {
		return false
	}
	candidates := reverseProxyRuleServerNames(rule)
	if len(candidates) == 0 {
		return false
	}
	for _, candidate := range candidates {
		if reverseProxyHostPatternMatches(candidate, serverName) {
			return true
		}
	}
	return false
}

func reverseProxyRulePathMatch(rule *model.ReverseProxyRule, path string) bool {
	if rule == nil {
		return false
	}
	expected := reverseProxyNormalizePathPrefix(rule.PathPrefix)
	if expected == "" {
		return true
	}
	actual := normalizeReverseProxyPath(path, true)
	if actual == "" {
		actual = "/"
	}
	return actual == expected || strings.HasPrefix(actual, expected+"/")
}

func reverseProxyRuleRequestPathMatch(rule *model.ReverseProxyRule, path string) bool {
	if rule == nil {
		return false
	}
	alias := normalizeReverseProxyProtocolAlias(rule.ListenProtocolAlias, rule.ListenProtocol)
	if reverseProxyIsHTTPDNSAlias(alias) {
		actual := normalizeReverseProxyDNSPath(path)
		if actual == "" {
			actual = "/"
		}
		return actual == reverseProxyDNSRulePath(rule)
	}
	return reverseProxyRulePathMatch(rule, path)
}

func reverseProxyNormalizeRequestHost(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return ""
	}
	if strings.Contains(value, ":") {
		host, _, err := net.SplitHostPort(value)
		if err == nil {
			return strings.ToLower(strings.Trim(host, "[]"))
		}
	}
	return strings.ToLower(strings.Trim(value, "[]"))
}

func extractRemoteIP(remoteAddr string) string {
	host, _, err := net.SplitHostPort(strings.TrimSpace(remoteAddr))
	if err == nil {
		return strings.TrimSpace(host)
	}
	return strings.TrimSpace(remoteAddr)
}

func reverseProxyNormalizeLocalIP(addr net.Addr) string {
	if addr == nil {
		return ""
	}
	value := strings.TrimSpace(addr.String())
	if value == "" {
		return ""
	}
	host, _, err := net.SplitHostPort(value)
	if err == nil {
		return strings.ToLower(strings.Trim(host, "[]"))
	}
	return strings.ToLower(strings.Trim(value, "[]"))
}

func reverseProxyConnectionAddrKey(conn net.Conn) string {
	if conn == nil {
		return ""
	}
	return reverseProxyConnectionAddrKeyFromAddrs(conn.LocalAddr(), conn.RemoteAddr())
}

func reverseProxyConnectionAddrKeyFromAddrs(localAddr net.Addr, remoteAddr net.Addr) string {
	local := strings.TrimSpace(reverseProxyNormalizeAddrText(localAddr))
	remote := strings.TrimSpace(reverseProxyNormalizeAddrText(remoteAddr))
	if local == "" || remote == "" {
		return ""
	}
	return local + "|" + remote
}

func reverseProxyNormalizeAddrText(addr net.Addr) string {
	if addr == nil {
		return ""
	}
	value := strings.TrimSpace(addr.String())
	if value == "" {
		return ""
	}
	host, port, err := net.SplitHostPort(value)
	if err != nil {
		return strings.ToLower(strings.Trim(value, "[]"))
	}
	return strings.ToLower(strings.Trim(host, "[]")) + ":" + strings.TrimSpace(port)
}

func reverseProxyNormalizeLocalIPContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	addr, _ := ctx.Value(http.LocalAddrContextKey).(net.Addr)
	return reverseProxyNormalizeLocalIP(addr)
}

func reverseProxyIsTimeoutError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	return errors.As(err, &netErr) && netErr != nil && netErr.Timeout()
}

func reverseProxyWriteGatewayError(w http.ResponseWriter, err error) {
	status := http.StatusBadGateway
	if reverseProxyIsTimeoutError(err) {
		status = http.StatusGatewayTimeout
	}
	http.Error(w, http.StatusText(status), status)
}

func reverseProxyRequestScheme(r *http.Request, fallback string) string {
	if r != nil && r.TLS != nil {
		return reverseProxyProtocolHTTPS
	}
	if strings.EqualFold(strings.TrimSpace(fallback), reverseProxyProtocolHTTPS) {
		return reverseProxyProtocolHTTPS
	}
	return reverseProxyProtocolHTTP
}

func reverseProxyExternalCookieDomain(rawHost string) string {
	host := reverseProxyNormalizeRequestHost(rawHost)
	if host == "" {
		return ""
	}
	if net.ParseIP(host) != nil {
		return ""
	}
	return host
}

func (g *reverseProxyListenerGroup) nextConnectionIDLocked() string {
	g.nextConnID++
	return strconv.FormatUint(g.nextConnID, 10)
}

func (g *reverseProxyListenerGroup) connectionIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	value, _ := ctx.Value(reverseProxyConnContextKey{}).(string)
	return strings.TrimSpace(value)
}

func (g *reverseProxyListenerGroup) registerTCPConnectionContext(ctx context.Context, conn net.Conn) context.Context {
	if g == nil || conn == nil {
		return ctx
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if !g.acquireListenerConnectionSlot() {
		go func() { _ = conn.Close() }()
		return ctx
	}
	selectionsToRelease := make([]reverseProxyCertificateSelection, 0)
	g.statsMu.Lock()
	if g.localConnIDs == nil {
		g.localConnIDs = make(map[net.Conn]string)
	}
	if g.localConnByID == nil {
		g.localConnByID = make(map[string]net.Conn)
	}
	if g.localConnStates == nil {
		g.localConnStates = make(map[string]reverseProxyLocalConnectionState)
	}
	if g.localConnAddrToID == nil {
		g.localConnAddrToID = make(map[string]string)
	}
	if g.localConnAddrByID == nil {
		g.localConnAddrByID = make(map[string]string)
	}
	if g.hijackedConnections == nil {
		g.hijackedConnections = make(map[string]net.Conn)
	}
	if g.connectionSlotIDs == nil {
		g.connectionSlotIDs = make(map[string]struct{})
	}
	connID := g.nextConnectionIDLocked()
	g.connectionSlotIDs[connID] = struct{}{}
	g.localConnIDs[conn] = connID
	g.localConnByID[connID] = conn
	addrKey := reverseProxyConnectionAddrKey(conn)
	if addrKey != "" {
		g.localConnAddrToID[addrKey] = connID
		g.localConnAddrByID[connID] = addrKey
		selectionsToRelease = append(selectionsToRelease, g.prunePendingCertificateSelectionsLocked(addrKey, time.Now())...)
		if pending, exists := g.takePendingCertificateSelectionLocked(addrKey); exists {
			state := g.localConnStates[connID]
			state.Selection = pending
			state.HasSelection = true
			g.localConnStates[connID] = state
		}
	}
	g.statsMu.Unlock()
	for _, selection := range selectionsToRelease {
		g.releaseCertificateSelection(selection)
	}
	return context.WithValue(ctx, reverseProxyConnContextKey{}, connID)
}

func (g *reverseProxyListenerGroup) registerQUICConnectionContext(ctx context.Context, conn *quic.Conn) context.Context {
	if g == nil || conn == nil {
		return ctx
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if !g.acquireListenerConnectionSlot() {
		go func() { _ = conn.CloseWithError(0x10, "reverse proxy connection limit") }()
		return ctx
	}
	selectionsToRelease := make([]reverseProxyCertificateSelection, 0)
	g.statsMu.Lock()
	if g.localConnStates == nil {
		g.localConnStates = make(map[string]reverseProxyLocalConnectionState)
	}
	if g.localConnAddrToID == nil {
		g.localConnAddrToID = make(map[string]string)
	}
	if g.localConnAddrByID == nil {
		g.localConnAddrByID = make(map[string]string)
	}
	if g.hijackedConnections == nil {
		g.hijackedConnections = make(map[string]net.Conn)
	}
	if g.connectionSlotIDs == nil {
		g.connectionSlotIDs = make(map[string]struct{})
	}
	connID := g.nextConnectionIDLocked()
	g.connectionSlotIDs[connID] = struct{}{}
	addrKey := reverseProxyConnectionAddrKeyFromAddrs(conn.LocalAddr(), conn.RemoteAddr())
	if addrKey != "" {
		g.localConnAddrToID[addrKey] = connID
		g.localConnAddrByID[connID] = addrKey
		selectionsToRelease = append(selectionsToRelease, g.prunePendingCertificateSelectionsLocked(addrKey, time.Now())...)
		if pending, exists := g.takePendingCertificateSelectionLocked(addrKey); exists {
			state := g.localConnStates[connID]
			state.Selection = pending
			state.HasSelection = true
			g.localConnStates[connID] = state
		}
	}
	g.statsMu.Unlock()
	for _, selection := range selectionsToRelease {
		g.releaseCertificateSelection(selection)
	}
	go func(id string, c *quic.Conn) {
		if c == nil {
			return
		}
		<-c.Context().Done()
		g.releaseLocalConnectionByID(id)
	}(connID, conn)
	return context.WithValue(ctx, reverseProxyConnContextKey{}, connID)
}

func (g *reverseProxyListenerGroup) registerLocalConnectionRule(ruleID uint, connID string) bool {
	if g == nil || ruleID == 0 || connID == "" {
		return true
	}
	g.mu.RLock()
	ruleLimiter := g.ruleConnectionLimiters[ruleID]
	g.mu.RUnlock()
	g.statsMu.Lock()
	if g.connectionCounts == nil {
		g.connectionCounts = make(map[uint]reverseProxyConnectionCounts)
	}
	if g.localConnStates == nil {
		g.localConnStates = make(map[string]reverseProxyLocalConnectionState)
	}
	state := g.localConnStates[connID]
	if _, alreadyRegistered := state.RuleConnectionLimiters[ruleID]; alreadyRegistered {
		g.statsMu.Unlock()
		return true
	}
	if ruleLimiter != nil && !ruleLimiter.TryAcquire() {
		g.statsMu.Unlock()
		return false
	}
	if state.RuleConnectionLimiters == nil {
		state.RuleConnectionLimiters = make(map[uint]*reverseProxyAdjustableLimiter)
	}
	state.RuleConnectionLimiters[ruleID] = ruleLimiter
	g.localConnStates[connID] = state
	counts := g.connectionCounts[ruleID]
	counts.LocalOpen++
	g.connectionCounts[ruleID] = counts
	g.statsMu.Unlock()
	return true
}

// setHijackedConnectionRule records the rule that initiated a classic
// HTTP/1.1 WebSocket Upgrade.  HTTP/2 and HTTP/3 do not use this path, so a
// later rule refresh can close exactly the old tunnel without disrupting
// unrelated multiplexed requests on the same listener.
func (g *reverseProxyListenerGroup) setHijackedConnectionRule(connID string, ruleID uint) {
	if g == nil || connID == "" || ruleID == 0 {
		return
	}
	g.statsMu.Lock()
	if g.localConnStates == nil {
		g.localConnStates = make(map[string]reverseProxyLocalConnectionState)
	}
	state := g.localConnStates[connID]
	state.HijackedRuleID = ruleID
	g.localConnStates[connID] = state
	g.statsMu.Unlock()
}

func (g *reverseProxyListenerGroup) releaseLocalConnectionByConn(conn net.Conn) {
	if g == nil || conn == nil {
		return
	}
	g.statsMu.Lock()
	connID := ""
	addrKey := ""
	if g.localConnIDs != nil {
		connID = g.localConnIDs[conn]
		delete(g.localConnIDs, conn)
	}
	if connID == "" {
		addrKey = reverseProxyConnectionAddrKey(conn)
		if addrKey != "" && g.localConnAddrToID != nil {
			connID = g.localConnAddrToID[addrKey]
		}
	}
	if connID != "" && g.localConnAddrByID != nil {
		if addrKey == "" {
			addrKey = g.localConnAddrByID[connID]
		}
	}
	g.statsMu.Unlock()
	if connID == "" {
		return
	}
	g.releaseLocalConnectionByID(connID)
}

func (g *reverseProxyListenerGroup) releaseLocalConnectionByAddrKey(addrKey string) {
	if g == nil || strings.TrimSpace(addrKey) == "" {
		return
	}
	g.statsMu.Lock()
	connID := g.localConnAddrToID[addrKey]
	g.statsMu.Unlock()
	if connID != "" {
		g.releaseLocalConnectionByID(connID)
	}
}

func (g *reverseProxyListenerGroup) markHijackedConnection(conn net.Conn) {
	if g == nil || conn == nil {
		return
	}
	g.statsMu.Lock()
	connID := g.localConnIDs[conn]
	if connID == "" {
		connID = g.localConnAddrToID[reverseProxyConnectionAddrKey(conn)]
	}
	if connID != "" {
		if g.hijackedConnections == nil {
			g.hijackedConnections = make(map[string]net.Conn)
		}
		tracked := g.localConnByID[connID]
		if tracked == nil {
			tracked = conn
		}
		g.hijackedConnections[connID] = tracked
	}
	g.statsMu.Unlock()
}

// closeHijackedConnectionsForRules terminates only WebSocket tunnels created
// by rules that disappeared from a still-live listener group.  A full group
// shutdown already closes every tracked tunnel; this narrower path covers a
// disabled or deleted rule sharing its endpoint with other active rules.
func (g *reverseProxyListenerGroup) closeHijackedConnectionsForRules(ruleIDs map[uint]struct{}) {
	if g == nil || len(ruleIDs) == 0 {
		return
	}
	type trackedConnection struct {
		id   string
		conn net.Conn
	}
	connections := make([]trackedConnection, 0)
	g.statsMu.Lock()
	for connID, conn := range g.hijackedConnections {
		state, exists := g.localConnStates[connID]
		if !exists {
			continue
		}
		if _, removed := ruleIDs[state.HijackedRuleID]; !removed {
			continue
		}
		connections = append(connections, trackedConnection{id: connID, conn: conn})
	}
	g.statsMu.Unlock()
	for _, item := range connections {
		if item.conn != nil {
			_ = item.conn.Close()
		}
		// reverseProxyTrackedClientConn invokes this itself.  Calling it here as
		// well keeps raw/listener-wrapped implementations leak-free and is
		// idempotent once the tracked connection callback has removed the state.
		g.releaseLocalConnectionByID(item.id)
	}
}

func (g *reverseProxyListenerGroup) releaseLocalConnectionByID(connID string) {
	if g == nil || connID == "" {
		return
	}
	var selection reverseProxyCertificateSelection
	hasSelection := false
	ruleLimiters := make([]*reverseProxyAdjustableLimiter, 0)
	releaseSlot := false
	g.statsMu.Lock()
	state := g.localConnStates[connID]
	delete(g.localConnStates, connID)
	delete(g.hijackedConnections, connID)
	if conn := g.localConnByID[connID]; conn != nil {
		delete(g.localConnByID, connID)
		delete(g.localConnIDs, conn)
	} else if g.localConnByID != nil {
		delete(g.localConnByID, connID)
	}
	if g.localConnAddrByID != nil {
		if addrKey := g.localConnAddrByID[connID]; addrKey != "" {
			delete(g.localConnAddrByID, connID)
			if g.localConnAddrToID != nil {
				delete(g.localConnAddrToID, addrKey)
			}
		}
	}
	if state.HasSelection {
		selection = state.Selection
		hasSelection = true
	}
	if _, exists := g.connectionSlotIDs[connID]; exists {
		delete(g.connectionSlotIDs, connID)
		releaseSlot = true
	}
	for ruleID, ruleLimiter := range state.RuleConnectionLimiters {
		counts := g.connectionCounts[ruleID]
		if counts.LocalOpen > 0 {
			counts.LocalOpen--
		}
		if counts.LocalOpen == 0 && counts.UpstreamOpen == 0 {
			delete(g.connectionCounts, ruleID)
		} else {
			g.connectionCounts[ruleID] = counts
		}
		if ruleLimiter != nil {
			ruleLimiters = append(ruleLimiters, ruleLimiter)
		}
	}
	g.statsMu.Unlock()
	if releaseSlot {
		g.releaseListenerConnectionSlot()
	}
	if hasSelection {
		g.releaseCertificateSelection(selection)
	}
	for _, ruleLimiter := range ruleLimiters {
		ruleLimiter.Release()
	}
}

func (g *reverseProxyListenerGroup) acquireListenerConnectionSlot() bool {
	if g == nil {
		return true
	}
	g.mu.RLock()
	limiter := g.listenerConnectionLimiter
	g.mu.RUnlock()
	if limiter == nil {
		return true
	}
	return limiter.TryAcquire()
}

func (g *reverseProxyListenerGroup) releaseListenerConnectionSlot() {
	if g == nil {
		return
	}
	g.mu.RLock()
	limiter := g.listenerConnectionLimiter
	g.mu.RUnlock()
	if limiter != nil {
		limiter.Release()
	}
}

func (g *reverseProxyListenerGroup) acquireUpstreamConnection(ruleID uint) (*reverseProxyAdjustableLimiter, bool) {
	if g == nil || ruleID == 0 {
		return nil, true
	}
	g.mu.RLock()
	limiter := g.upstreamLimiters[ruleID]
	g.mu.RUnlock()
	if limiter != nil && !limiter.TryAcquire() {
		return limiter, false
	}
	g.statsMu.Lock()
	if g.connectionCounts == nil {
		g.connectionCounts = make(map[uint]reverseProxyConnectionCounts)
	}
	counts := g.connectionCounts[ruleID]
	counts.UpstreamOpen++
	g.connectionCounts[ruleID] = counts
	g.statsMu.Unlock()
	return limiter, true
}

func (g *reverseProxyListenerGroup) releaseUpstreamConnection(ruleID uint, limiter *reverseProxyAdjustableLimiter) {
	if g == nil || ruleID == 0 {
		if limiter != nil {
			limiter.Release()
		}
		return
	}
	g.statsMu.Lock()
	counts := g.connectionCounts[ruleID]
	if counts.UpstreamOpen > 0 {
		counts.UpstreamOpen--
	}
	if counts.LocalOpen == 0 && counts.UpstreamOpen == 0 {
		delete(g.connectionCounts, ruleID)
	} else {
		g.connectionCounts[ruleID] = counts
	}
	g.statsMu.Unlock()
	if limiter != nil {
		limiter.Release()
	}
}

func (g *reverseProxyListenerGroup) snapshotConnectionCounts() map[uint]reverseProxyConnectionCounts {
	if g == nil {
		return nil
	}
	g.statsMu.Lock()
	defer g.statsMu.Unlock()
	if len(g.connectionCounts) == 0 {
		return nil
	}
	out := make(map[uint]reverseProxyConnectionCounts, len(g.connectionCounts))
	for ruleID, counts := range g.connectionCounts {
		out[ruleID] = counts
	}
	return out
}

func reverseProxyNormalizePathPrefix(raw string) string {
	value := normalizeReverseProxyPath(raw, false)
	if value == "" || value == "/" {
		return ""
	}
	for len(value) > 1 && strings.HasSuffix(value, "/") {
		value = strings.TrimSuffix(value, "/")
	}
	return value
}

func reverseProxyJoinExternalPathPrefix(prefix string, value string) string {
	prefix = reverseProxyNormalizePathPrefix(prefix)
	if prefix == "" {
		return value
	}
	if value == "" {
		return prefix
	}
	if strings.HasPrefix(value, "/") {
		if reverseProxyBodyPathHasPrefix([]byte(value), 0, prefix) {
			return value
		}
		return prefix + value
	}
	return prefix + "/" + strings.TrimPrefix(value, "/")
}

func reverseProxyOriginVariants(scheme string, hostOnly string, hostWithPort string) []string {
	hostOnly = strings.ToLower(strings.TrimSpace(hostOnly))
	hostWithPort = strings.ToLower(strings.TrimSpace(hostWithPort))
	if hostOnly == "" {
		return nil
	}
	items := make([]string, 0, 6)
	if hostWithPort != "" && hostWithPort != hostOnly {
		items = append(items,
			scheme+"://"+hostWithPort,
			"//"+hostWithPort,
			strings.ReplaceAll(scheme+"://"+hostWithPort, "/", `\/`),
			strings.ReplaceAll("//"+hostWithPort, "/", `\/`),
		)
	}
	items = append(items,
		scheme+"://"+hostOnly,
		"//"+hostOnly,
		strings.ReplaceAll(scheme+"://"+hostOnly, "/", `\/`),
		strings.ReplaceAll("//"+hostOnly, "/", `\/`),
	)
	return items
}

func buildReverseProxyResponseRewritePlan(r *http.Request, rule *model.ReverseProxyRule, targetURL *url.URL) reverseProxyResponseRewritePlan {
	if r == nil || rule == nil || targetURL == nil {
		return reverseProxyResponseRewritePlan{}
	}

	externalHostRaw := strings.TrimSpace(r.Host)
	externalHostNormalized := reverseProxyNormalizeRequestHost(externalHostRaw)
	upstreamHostNormalized := strings.ToLower(strings.TrimSpace(targetURL.Hostname()))
	externalPathPrefix := reverseProxyNormalizePathPrefix(rule.PathPrefix)
	if externalHostRaw == "" || externalHostNormalized == "" || upstreamHostNormalized == "" {
		return reverseProxyResponseRewritePlan{}
	}
	if strings.EqualFold(externalHostNormalized, upstreamHostNormalized) && externalPathPrefix == "" {
		return reverseProxyResponseRewritePlan{}
	}

	externalScheme := reverseProxyRequestScheme(r, rule.ListenProtocol)
	externalOrigin := externalScheme + "://" + externalHostRaw + externalPathPrefix
	externalSchemeRelative := "//" + externalHostRaw + externalPathPrefix
	escapedExternalOrigin := strings.ReplaceAll(externalOrigin, "/", `\/`)
	escapedExternalSchemeRelative := strings.ReplaceAll(externalSchemeRelative, "/", `\/`)

	candidates := []string{
		"http",
		"https",
	}
	oldValues := make([]string, 0)
	for _, scheme := range candidates {
		oldValues = append(oldValues, reverseProxyOriginVariants(scheme, upstreamHostNormalized, strings.ToLower(strings.TrimSpace(targetURL.Host)))...)
	}
	seen := make(map[string]struct{}, len(oldValues))
	replacements := make([]reverseProxyStringReplacement, 0, len(oldValues))
	for _, oldValue := range oldValues {
		if oldValue == "" {
			continue
		}
		if _, exists := seen[oldValue]; exists {
			continue
		}
		seen[oldValue] = struct{}{}

		newValue := externalOrigin
		if strings.HasPrefix(oldValue, "//") {
			newValue = externalSchemeRelative
		} else if strings.HasPrefix(oldValue, `\/\/`) {
			newValue = escapedExternalSchemeRelative
		} else if strings.Contains(oldValue, `\/`) {
			newValue = escapedExternalOrigin
		}
		replacements = append(replacements, reverseProxyStringReplacement{
			Old: oldValue,
			New: newValue,
		})
	}
	sort.SliceStable(replacements, func(i, j int) bool {
		return len(replacements[i].Old) > len(replacements[j].Old)
	})

	return reverseProxyResponseRewritePlan{
		Enabled:              len(replacements) > 0 || externalPathPrefix != "",
		Replacements:         replacements,
		UpstreamCookieDomain: upstreamHostNormalized,
		ExternalCookieDomain: reverseProxyExternalCookieDomain(externalHostRaw),
		UpstreamPathPrefix:   reverseProxyNormalizePathPrefix(targetURL.Path),
		ExternalPathPrefix:   externalPathPrefix,
	}
}

func reverseProxyApplyStringReplacements(value string, replacements []reverseProxyStringReplacement) string {
	out := value
	for _, item := range replacements {
		if item.Old == "" || item.Old == item.New {
			continue
		}
		out = strings.ReplaceAll(out, item.Old, item.New)
	}
	return out
}

func reverseProxyRewriteSetCookieHeader(value string, upstreamDomain string, externalDomain string) string {
	return reverseProxyRewriteSetCookieHeaderWithPath(value, upstreamDomain, externalDomain, "", "")
}

func reverseProxyRewriteSetCookieHeaderWithPath(value string, upstreamDomain string, externalDomain string, upstreamPathPrefix string, externalPathPrefix string) string {
	if strings.TrimSpace(value) == "" || strings.TrimSpace(upstreamDomain) == "" {
		return value
	}
	parts := strings.Split(value, ";")
	filtered := make([]string, 0, len(parts))
	isHostCookie := false
	if len(parts) > 0 {
		name, _, found := strings.Cut(strings.TrimSpace(parts[0]), "=")
		isHostCookie = found && strings.HasPrefix(strings.TrimSpace(name), "__Host-")
	}
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		eqIndex := strings.Index(trimmed, "=")
		if eqIndex > 0 && strings.EqualFold(strings.TrimSpace(trimmed[:eqIndex]), "domain") {
			domainValue := strings.TrimSpace(trimmed[eqIndex+1:])
			domainValue = strings.TrimPrefix(domainValue, ".")
			if reverseProxyCookieDomainMatchesUpstream(domainValue, upstreamDomain) {
				if externalDomain == "" || isHostCookie {
					continue
				}
				trimmed = "Domain=" + externalDomain
			}
		}
		if eqIndex > 0 && strings.EqualFold(strings.TrimSpace(trimmed[:eqIndex]), "path") && !isHostCookie {
			pathValue := strings.TrimSpace(trimmed[eqIndex+1:])
			if rewrittenPath, changed := reverseProxyRewriteCookiePath(pathValue, upstreamPathPrefix, externalPathPrefix); changed {
				trimmed = "Path=" + rewrittenPath
			}
		}
		filtered = append(filtered, trimmed)
	}
	return strings.Join(filtered, "; ")
}

func reverseProxyRewriteCookiePath(value string, upstreamPathPrefix string, externalPathPrefix string) (string, bool) {
	externalPathPrefix = reverseProxyNormalizePathPrefix(externalPathPrefix)
	if externalPathPrefix == "" {
		return value, false
	}
	value = strings.TrimSpace(value)
	if value == "" || !strings.HasPrefix(value, "/") {
		return value, false
	}
	upstreamPathPrefix = reverseProxyNormalizePathPrefix(upstreamPathPrefix)
	if upstreamPathPrefix != "" {
		if value == upstreamPathPrefix {
			return externalPathPrefix, true
		}
		if strings.HasPrefix(value, upstreamPathPrefix+"/") {
			return reverseProxyJoinExternalPathPrefix(externalPathPrefix, strings.TrimPrefix(value, upstreamPathPrefix)), true
		}
	}
	return reverseProxyJoinExternalPathPrefix(externalPathPrefix, value), true
}

func reverseProxyCookieDomainMatchesUpstream(domainValue string, upstreamDomain string) bool {
	domainValue = reverseProxyNormalizeServerName(domainValue)
	upstreamDomain = reverseProxyNormalizeServerName(upstreamDomain)
	if domainValue == "" || upstreamDomain == "" {
		return false
	}
	if reverseProxyIPLiteralEqual(domainValue, upstreamDomain) {
		return true
	}
	if strings.EqualFold(domainValue, upstreamDomain) {
		return true
	}
	return strings.HasSuffix(upstreamDomain, "."+domainValue)
}

func reverseProxyRewriteResponseHeaders(header http.Header, plan reverseProxyResponseRewritePlan) {
	if !plan.Enabled || header == nil {
		return
	}
	for key, values := range header {
		if len(values) == 0 {
			continue
		}
		if strings.EqualFold(key, "Set-Cookie") {
			next := make([]string, 0, len(values))
			for _, value := range values {
				next = append(next, reverseProxyRewriteSetCookieHeaderWithPath(value, plan.UpstreamCookieDomain, plan.ExternalCookieDomain, plan.UpstreamPathPrefix, plan.ExternalPathPrefix))
			}
			header[key] = next
			continue
		}
		next := make([]string, 0, len(values))
		for _, value := range values {
			rewritten := reverseProxyApplyStringReplacements(value, plan.Replacements)
			rewritten = reverseProxyRewriteRelativeHeaderValue(key, rewritten, plan.ExternalPathPrefix)
			next = append(next, rewritten)
		}
		header[key] = next
	}
}

func reverseProxyResponseMayContainOriginReferences(contentType string) bool {
	mediaType, _, err := mime.ParseMediaType(strings.TrimSpace(contentType))
	if err != nil {
		mediaType = strings.ToLower(strings.TrimSpace(contentType))
	}
	mediaType = strings.ToLower(strings.TrimSpace(mediaType))
	if mediaType == "text/event-stream" || strings.HasPrefix(mediaType, "multipart/") {
		return false
	}
	if strings.HasPrefix(mediaType, "text/") {
		return true
	}
	switch mediaType {
	case "application/json",
		"application/ld+json",
		"application/javascript",
		"application/x-javascript",
		"application/xml",
		"application/rss+xml",
		"application/atom+xml",
		"application/xhtml+xml",
		"image/svg+xml":
		return true
	default:
		return false
	}
}

type reverseProxyReplayReadCloser struct {
	io.Reader
	io.Closer
}

func reverseProxyRewriteResponseBody(resp *http.Response, plan reverseProxyResponseRewritePlan) error {
	// Keep this small wrapper for callers/tests that do not carry a rule.  It
	// must still use the persisted resource policy rather than resurrecting a
	// legacy hard-coded body ceiling.
	return reverseProxyRewriteResponseBodyForRule(resp, plan, nil)
}

func reverseProxyRewriteResponseBodyForRule(resp *http.Response, plan reverseProxyResponseRewritePlan, rule *model.ReverseProxyRule) error {
	settings := reverseProxyResources.current()
	if !reverseProxyResponseBodyEligible(resp, plan, settings.ResponseRewriteInputBytes) {
		return nil
	}
	inputBytes := settings.ResponseRewriteInputBytes
	outputBytes := settings.ResponseRewriteOutputBytes
	if inputBytes < reverseProxyMinimumRewriteReservationBytes || outputBytes < reverseProxyMinimumRewriteReservationBytes {
		return nil
	}
	// The bounded conversion can hold the input and two output buffers while
	// it applies origin and relative-path rewrites.  Reserve that real peak up
	// front so the shared pool is authoritative instead of only accounting for
	// an optimistic input+output estimate.
	reserve := reverseProxyRewriteReservationBytes(inputBytes, outputBytes)
	if resp.ContentLength >= 0 && resp.ContentLength < inputBytes {
		reserve = reverseProxyRewriteReservationBytes(resp.ContentLength, outputBytes)
	}
	if reserve < reverseProxyMinimumRewriteReservationBytes {
		reserve = reverseProxyMinimumRewriteReservationBytes
	}
	ruleID := uint(0)
	if rule != nil {
		ruleID = rule.Id
	}
	lease, acquired := reverseProxyResources.tryAcquireRewrite(ruleID, reverseProxyEffectiveRuleMemoryLimit(rule), reserve)
	if !acquired {
		// Under pressure we deliberately preserve the upstream stream rather
		// than allocating an untracked buffer or returning a partial rewrite.
		return nil
	}
	var releaseOnce sync.Once
	release := func() {
		releaseOnce.Do(func() { reverseProxyResources.releaseRewrite(lease) })
	}
	return reverseProxyRewriteResponseBodyWithLimits(resp, plan, inputBytes, outputBytes, release)
}

func reverseProxyResponseBodyEligible(resp *http.Response, plan reverseProxyResponseRewritePlan, inputLimit int64) bool {
	if !plan.Enabled || resp == nil || resp.Body == nil {
		return false
	}
	if (resp.StatusCode >= 100 && resp.StatusCode < 200) || resp.StatusCode == http.StatusNoContent || resp.StatusCode == http.StatusResetContent || resp.StatusCode == http.StatusNotModified {
		return false
	}
	if resp.Request != nil {
		if resp.Request.Method == http.MethodHead || resp.Request.Header.Get("Range") != "" {
			return false
		}
	}
	if resp.Header.Get("Content-Range") != "" {
		return false
	}
	if !reverseProxyResponseMayContainOriginReferences(resp.Header.Get("Content-Type")) {
		return false
	}
	contentEncoding := strings.TrimSpace(strings.Join(resp.Header.Values("Content-Encoding"), ","))
	if contentEncoding != "" && !strings.EqualFold(contentEncoding, "identity") {
		return false
	}
	if inputLimit <= 0 || resp.ContentLength < 0 || resp.ContentLength > inputLimit {
		return false
	}
	return true
}

func reverseProxyRewriteResponseBodyWithLimits(resp *http.Response, plan reverseProxyResponseRewritePlan, inputLimit int64, outputLimit int64, release func()) error {
	if !reverseProxyResponseBodyEligible(resp, plan, inputLimit) {
		if release != nil {
			release()
		}
		return nil
	}
	if outputLimit < reverseProxyMinimumRewriteReservationBytes {
		if release != nil {
			release()
		}
		return nil
	}

	originalBody := resp.Body
	expectedLength := resp.ContentLength
	if expectedLength < 0 || expectedLength > inputLimit {
		if release != nil {
			release()
		}
		return nil
	}
	body := make([]byte, int(expectedLength))
	readCount, err := io.ReadFull(originalBody, body)
	if err != nil {
		_ = originalBody.Close()
		if release != nil {
			release()
		}
		return err
	}
	body = body[:readCount]
	var extra [1]byte
	extraCount, extraErr := originalBody.Read(extra[:])
	if extraErr != nil && !errors.Is(extraErr, io.EOF) {
		_ = originalBody.Close()
		if release != nil {
			release()
		}
		return extraErr
	}
	if extraCount > 0 {
		// Preserve a malformed response whose declared length was smaller than its body.
		// Rewriting it would require buffering an unbounded upstream stream.
		resp.Body = &reverseProxyCleanupReadCloser{ReadCloser: &reverseProxyReplayReadCloser{
			Reader: io.MultiReader(bytes.NewReader(body), bytes.NewReader(extra[:extraCount]), originalBody),
			Closer: originalBody,
		}, onClose: release}
		resp.ContentLength = -1
		resp.Header.Del("Content-Length")
		return nil
	}
	_ = originalBody.Close()
	rewritten, ok := reverseProxyApplyBoundedBodyReplacements(body, plan.Replacements, outputLimit)
	if ok {
		rewritten, ok = reverseProxyRewriteRelativeBodyPathsBounded(rewritten, plan.ExternalPathPrefix, outputLimit)
	}
	if !ok {
		resp.Body = &reverseProxyCleanupReadCloser{ReadCloser: io.NopCloser(bytes.NewReader(body)), onClose: release}
		resp.ContentLength = int64(len(body))
		resp.TransferEncoding = nil
		resp.Header.Set("Content-Length", strconv.Itoa(len(body)))
		return nil
	}
	resp.Body = &reverseProxyCleanupReadCloser{ReadCloser: io.NopCloser(bytes.NewReader(rewritten)), onClose: release}
	resp.ContentLength = int64(len(rewritten))
	resp.TransferEncoding = nil
	resp.Header.Set("Content-Length", strconv.Itoa(len(rewritten)))
	resp.Header.Del("ETag")
	resp.Header.Del("Content-MD5")
	return nil
}

func reverseProxyApplyBoundedBodyReplacements(body []byte, replacements []reverseProxyStringReplacement, maximum int64) ([]byte, bool) {
	if maximum < 0 || int64(len(body)) > maximum {
		return body, false
	}
	type replacement struct {
		oldValue []byte
		newValue []byte
	}
	items := make([]replacement, 0, len(replacements))
	for _, item := range replacements {
		oldValue := []byte(item.Old)
		newValue := []byte(item.New)
		if len(oldValue) == 0 || bytes.Equal(oldValue, newValue) {
			continue
		}
		items = append(items, replacement{oldValue: oldValue, newValue: newValue})
	}
	if len(items) == 0 {
		return body, true
	}
	outputLength := int64(0)
	appendLength := func(length int) bool {
		if length < 0 || outputLength > maximum-int64(length) {
			return false
		}
		outputLength += int64(length)
		return outputLength <= int64(^uint(0)>>1)
	}
	for index := 0; index < len(body); {
		matchedIndex := -1
		for itemIndex := range items {
			item := &items[itemIndex]
			if !bytes.HasPrefix(body[index:], item.oldValue) {
				continue
			}
			if !appendLength(len(item.newValue)) {
				return body, false
			}
			index += len(item.oldValue)
			matchedIndex = itemIndex
			break
		}
		if matchedIndex >= 0 {
			continue
		}
		if !appendLength(1) {
			return body, false
		}
		index++
	}
	out := make([]byte, int(outputLength))
	offset := 0
	for index := 0; index < len(body); {
		matchedIndex := -1
		for itemIndex := range items {
			item := &items[itemIndex]
			if !bytes.HasPrefix(body[index:], item.oldValue) {
				continue
			}
			matchedIndex = itemIndex
			break
		}
		if matchedIndex >= 0 {
			item := &items[matchedIndex]
			offset += copy(out[offset:], item.newValue)
			index += len(item.oldValue)
			continue
		}
		out[offset] = body[index]
		offset++
		index++
	}
	return out, true
}

func (g *reverseProxyListenerGroup) forwardRequest(w http.ResponseWriter, r *http.Request, rule *model.ReverseProxyRule, altSvc string) {
	// Keep the incoming HTTP entity opaque.  Compression is a representation
	// negotiation concern; it must not silently consume or rebuild request
	// bodies before the URL-only proxy transformation is applied.
	targetURL, transportBundle, err := g.buildUpstream(rule, r.Context())
	if err != nil {
		reverseProxyRuntime.reportRuleState(rule.Id, "upstream_error", err.Error())
		if altSvc != "" {
			w.Header().Set("Alt-Svc", altSvc)
		}
		reverseProxyWriteGatewayError(w, err)
		return
	}

	proxy := httputil.NewSingleHostReverseProxy(targetURL)
	proxy.Transport = transportBundle.RoundTripper
	rewritePlan := buildReverseProxyResponseRewritePlan(r, rule, targetURL)
	bodyRewriteEnabled := rewritePlan.Enabled && !rule.ApiPassthrough
	cleanup := transportBundle.Cleanup
	var cleanupFn func()
	if cleanup != nil {
		var cleanupOnce sync.Once
		cleanupFn = func() {
			cleanupOnce.Do(cleanup)
		}
		defer cleanupFn()
		proxy.ErrorHandler = func(writer http.ResponseWriter, request *http.Request, proxyErr error) {
			cleanupFn()
			g.invalidateCachedUpstream(rule.Id)
			reverseProxyRuntime.reportRuleState(rule.Id, "proxy_error", proxyErr.Error())
			if altSvc != "" {
				writer.Header().Set("Alt-Svc", altSvc)
			}
			reverseProxyWriteGatewayError(writer, proxyErr)
		}
	} else {
		proxy.ErrorHandler = func(writer http.ResponseWriter, request *http.Request, proxyErr error) {
			g.invalidateCachedUpstream(rule.Id)
			reverseProxyRuntime.reportRuleState(rule.Id, "proxy_error", proxyErr.Error())
			if altSvc != "" {
				writer.Header().Set("Alt-Svc", altSvc)
			}
			reverseProxyWriteGatewayError(writer, proxyErr)
		}
	}
	proxy.ModifyResponse = func(resp *http.Response) error {
		if err := reverseProxyDecodeUpstreamResponse(resp, reverseProxyEffectiveRuleMemoryLimit(rule)); err != nil {
			return err
		}
		resp.Header.Del("Alt-Svc")
		if altSvc != "" {
			resp.Header.Set("Alt-Svc", altSvc)
		}
		reverseProxyRewriteResponseHeaders(resp.Header, rewritePlan)
		if !bodyRewriteEnabled {
			return nil
		}
		if err := reverseProxyRewriteResponseBodyForRule(resp, rewritePlan, rule); err != nil {
			return err
		}
		return nil
	}
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		req.URL.Path, req.URL.RawPath = reverseProxyTrimMatchedPathPrefix(req.URL.Path, req.URL.RawPath, rule.PathPrefix)
		originalDirector(req)
		req.Host = targetURL.Host
		req.URL.RawQuery = r.URL.RawQuery
		// Identity forwarding headers are an ingress trust boundary. Never let a
		// client-provided forwarding chain reach the upstream unchanged.
		for _, header := range []string{
			"Forwarded",
			"X-Forwarded-For",
			"X-Forwarded-Host",
			"X-Forwarded-Proto",
			"X-Forwarded-Port",
			"X-Real-IP",
		} {
			req.Header.Del(header)
		}
		targetAcceptEncoding := reverseProxyTargetAcceptEncoding(rule)
		if rule.ApiPassthrough {
			// API passthrough preserves the caller's representation contract.
			// The request body and its Content-Encoding are forwarded unchanged.
		} else if bodyRewriteEnabled || req.Method == http.MethodHead || req.Header.Get("Range") != "" || reverseProxyIsWebSocketUpgradeRequest(r) {
			if targetAcceptEncoding == "" {
				req.Header.Del("Accept-Encoding")
			} else {
				req.Header.Set("Accept-Encoding", "identity")
			}
		} else {
			setReverseProxyAcceptEncoding(req.Header, targetAcceptEncoding)
		}
		forwardedScheme := reverseProxyRequestScheme(r, rule.ListenProtocol)
		clientIP := extractRemoteIP(r.RemoteAddr)
		req.Header.Set("X-Forwarded-Host", r.Host)
		req.Header.Set("X-Forwarded-Proto", forwardedScheme)
		req.Header.Set("X-Forwarded-Port", strconv.Itoa(g.listenPort))
		req.Header.Set("X-Real-IP", clientIP)
		req.Header.Set("Forwarded", reverseProxyBuildForwardedHeader(clientIP, r.Host, forwardedScheme))
		// httputil.ReverseProxy appends the peer address to X-Forwarded-For
		// after Director returns.  Setting it here would duplicate that address.
	}
	reverseProxyRuntime.reportRuleState(rule.Id, "running", "")
	listenCompressionEnabled, _ := reverseProxyListenCompressionOptions(rule)
	compressedWriter := compressionalgorithm.NewHTTPResponseWriter(w, compressionalgorithm.HTTPResponseOptions{
		Request: r,
		Level:   reverseProxyCompressionLevel,
		Enabled: listenCompressionEnabled,
		MinSize: compressionalgorithm.DefaultMinimumResponseSize,
		AllowedAlgorithms: func() []compressionalgorithm.Algorithm {
			_, values := reverseProxyListenCompressionOptions(rule)
			return values
		}(),
	})
	defer compressedWriter.Close()
	proxy.ServeHTTP(compressedWriter, r)
}

func reverseProxyBuildForwardedHeader(clientIP string, host string, scheme string) string {
	clientIP = strings.TrimSpace(strings.Trim(clientIP, "[]"))
	if strings.Contains(clientIP, ":") {
		clientIP = "\"[" + strings.ReplaceAll(clientIP, "\"", "") + "]\""
	} else {
		clientIP = strings.ReplaceAll(clientIP, "\"", "")
	}
	host = strings.ReplaceAll(strings.TrimSpace(host), "\"", "")
	scheme = strings.ReplaceAll(strings.TrimSpace(scheme), "\"", "")
	parts := make([]string, 0, 3)
	if clientIP != "" {
		parts = append(parts, "for="+clientIP)
	}
	if host != "" {
		parts = append(parts, "host=\""+host+"\"")
	}
	if scheme != "" {
		parts = append(parts, "proto="+scheme)
	}
	return strings.Join(parts, ";")
}

func reverseProxyRewriteRelativeHeaderValue(key string, value string, prefix string) string {
	if prefix == "" {
		return value
	}
	if strings.EqualFold(key, "Location") {
		trimmed := strings.TrimSpace(value)
		if strings.HasPrefix(trimmed, "/") && !strings.HasPrefix(trimmed, "//") {
			return reverseProxyJoinExternalPathPrefix(prefix, trimmed)
		}
	}
	return value
}

func reverseProxyBodyPathHasPrefix(body []byte, start int, prefix string) bool {
	prefix = reverseProxyNormalizePathPrefix(prefix)
	if prefix == "" || start < 0 || start >= len(body) {
		return false
	}
	prefixBytes := []byte(prefix)
	if !bytes.HasPrefix(body[start:], prefixBytes) {
		return false
	}
	end := start + len(prefixBytes)
	if end >= len(body) {
		return true
	}
	switch body[end] {
	case '/', '"', '\'', '?', '#', ')', ' ', '\t', '\n', '\r':
		return true
	case '\\':
		return end+1 < len(body) && body[end+1] == '/'
	default:
		return false
	}
}

func reverseProxyRewriteRelativeBodyPathsBounded(body []byte, prefix string, maximum int64) ([]byte, bool) {
	prefix = reverseProxyNormalizePathPrefix(prefix)
	if prefix == "" || len(body) == 0 {
		return body, int64(len(body)) <= maximum
	}
	if maximum < 0 || int64(len(body)) > maximum {
		return body, false
	}
	// Apply quoted JavaScript/HTML paths and CSS url(/...) paths in one output
	// buffer.  Count first, then allocate the exact final size: this avoids
	// geometric slice growth temporarily retaining a second unaccounted output
	// allocation while the shared memory lease is active.
	prefixBytes := []byte(prefix)
	cssPathPrefix := []byte("url(/")
	needsCSSPrefix := func(index int) bool {
		return bytes.HasPrefix(body[index:], cssPathPrefix) && (index+len(cssPathPrefix) >= len(body) || body[index+len(cssPathPrefix)] != '/')
	}
	needsQuotedPrefix := func(index int) bool {
		ch := body[index]
		if ch != '"' && ch != '\'' {
			return false
		}
		if index+1 < len(body) && body[index+1] == '/' {
			return (index+2 >= len(body) || body[index+2] != '/') && !reverseProxyBodyPathHasPrefix(body, index+1, prefix)
		}
		if index+2 < len(body) && body[index+1] == '\\' && body[index+2] == '/' {
			return (index+4 >= len(body) || body[index+3] != '\\' || body[index+4] != '/') && !reverseProxyBodyPathHasPrefix(body, index+2, prefix)
		}
		return false
	}
	outputLength := int64(0)
	appendLength := func(length int) bool {
		if length < 0 || outputLength > maximum-int64(length) {
			return false
		}
		outputLength += int64(length)
		return outputLength <= int64(^uint(0)>>1)
	}
	for index := 0; index < len(body); {
		if needsCSSPrefix(index) {
			if !appendLength(len("url(") + len(prefixBytes) + len("/")) {
				return body, false
			}
			index += len(cssPathPrefix)
			continue
		}
		if !appendLength(1) {
			return body, false
		}
		if needsQuotedPrefix(index) && !appendLength(len(prefixBytes)) {
			return body, false
		}
		index++
	}
	out := make([]byte, int(outputLength))
	offset := 0
	for index := 0; index < len(body); {
		if needsCSSPrefix(index) {
			offset += copy(out[offset:], "url(")
			offset += copy(out[offset:], prefixBytes)
			offset += copy(out[offset:], "/")
			index += len(cssPathPrefix)
			continue
		}
		out[offset] = body[index]
		offset++
		if needsQuotedPrefix(index) {
			offset += copy(out[offset:], prefixBytes)
		}
		index++
	}
	return out, true
}

func appendForwardedFor(existing string, ip string) string {
	ip = strings.TrimSpace(ip)
	existing = strings.TrimSpace(existing)
	if ip == "" {
		return existing
	}
	if existing == "" {
		return ip
	}
	return existing + ", " + ip
}

func (g *reverseProxyListenerGroup) buildUpstream(rule *model.ReverseProxyRule, baseCtx context.Context) (*url.URL, reverseProxyTransportBundle, error) {
	if rule == nil {
		return nil, reverseProxyTransportBundle{}, common.NewError("rule is nil")
	}
	targets := decodeReverseProxyList(rule.TargetAddresses)
	if len(targets) == 0 {
		return nil, reverseProxyTransportBundle{}, common.NewError("target addresses are empty")
	}
	httpVersionStrategy := strings.TrimSpace(rule.HTTPVersionStrategy)
	if reverseProxyIsWebSocketAlias(rule.TargetProtocolAlias) {
		httpVersionStrategy = ""
	}
	if cached := g.acquireCachedUpstream(rule.Id); cached != nil {
		return buildReverseProxyTargetURL(rule, cached.HostHeader), reverseProxyTransportBundle{
			RoundTripper: cached.RoundTripper,
			Cleanup: func() {
				g.releaseCachedUpstream(cached)
			},
		}, nil
	}

	ctx, cancel := context.WithTimeout(baseCtx, reverseProxyRequestTimeout)
	defer cancel()

	resolved, serverName, hostHeader, transportMode, err := g.service.pickUpstreamTarget(ctx, strings.TrimSpace(rule.TargetProtocol), targets, rule.TargetPort, rule.IPStrategy, httpVersionStrategy, rule.UpstreamTLSVerify, func(candidate reverseProxyTargetCandidate) bool {
		return g.resolvedTargetLoopsToListener(rule, candidate.address)
	})
	if err != nil {
		return nil, reverseProxyTransportBundle{}, err
	}
	transportBundle, err := g.service.buildRoundTripper(g, rule.Id, strings.TrimSpace(rule.TargetProtocol), resolved, rule.TargetPort, serverName, rule.UpstreamTLSVerify, transportMode, reverseProxyRuleLimit(rule.UpstreamMaxConnections), reverseProxyEffectiveUpstreamMaxIdleConnections(rule))
	if err != nil {
		return nil, reverseProxyTransportBundle{}, err
	}
	cached := &reverseProxyCachedUpstream{
		ResolvedAddress: resolved,
		ServerName:      serverName,
		HostHeader:      hostHeader,
		TransportMode:   transportMode,
		ResolvedAt:      time.Now(),
		RoundTripper:    transportBundle.RoundTripper,
		Cleanup:         transportBundle.Cleanup,
	}
	g.storeCachedUpstream(rule.Id, cached)
	return buildReverseProxyTargetURL(rule, hostHeader), reverseProxyTransportBundle{
		RoundTripper: cached.RoundTripper,
		Cleanup: func() {
			g.releaseCachedUpstream(cached)
		},
	}, nil
}

func reverseProxyTrimMatchedPathPrefix(path string, rawPath string, prefix string) (string, string) {
	normalizedPrefix := reverseProxyNormalizePathPrefix(prefix)
	normalizedPath := normalizeReverseProxyPath(path, true)
	if normalizedPath == "" {
		normalizedPath = "/"
	}
	normalizedRawPath := strings.TrimSpace(rawPath)
	if normalizedRawPath == "" {
		normalizedRawPath = (&url.URL{Path: normalizedPath}).EscapedPath()
	}
	if normalizedPrefix == "" {
		return normalizedPath, normalizedRawPath
	}
	if normalizedPath != normalizedPrefix && !strings.HasPrefix(normalizedPath, normalizedPrefix+"/") {
		return normalizedPath, normalizedRawPath
	}
	trimmedPath := strings.TrimPrefix(normalizedPath, normalizedPrefix)
	if trimmedPath == normalizedPath {
		return normalizedPath, normalizedRawPath
	}
	if trimmedPath == "" {
		trimmedPath = "/"
	} else if !strings.HasPrefix(trimmedPath, "/") {
		trimmedPath = "/" + trimmedPath
	}
	return trimmedPath, (&url.URL{Path: trimmedPath}).EscapedPath()
}

func (s *ReverseProxyService) pickUpstreamTarget(ctx context.Context, protocol string, targets []string, port int, ipStrategy string, httpVersionStrategy string, strictVerify bool, loopGuard func(reverseProxyTargetCandidate) bool) (string, string, string, string, error) {
	var firstErr error
	for _, target := range targets {
		candidates, err := s.resolveTargetCandidates(ctx, target, port, ipStrategy)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		preferred := reorderCandidatesByIPStrategy(candidates, ipStrategy)
		for _, candidate := range preferred {
			if loopGuard != nil && loopGuard(candidate) {
				if firstErr == nil {
					firstErr = common.NewError("resolved upstream target points back to the local listener")
				}
				continue
			}
			transportMode, probeErr := s.probeUpstream(ctx, protocol, candidate.address, port, candidate.serverName, strictVerify, httpVersionStrategy)
			if probeErr == nil {
				return candidate.address, candidate.serverName, candidate.hostHeader, transportMode, nil
			} else if firstErr == nil {
				firstErr = probeErr
			}
		}
	}
	if firstErr == nil {
		firstErr = common.NewError("resolve upstream target failed")
	}
	return "", "", "", "", firstErr
}

func (g *reverseProxyListenerGroup) resolvedTargetLoopsToListener(rule *model.ReverseProxyRule, targetAddress string) bool {
	if g == nil || rule == nil {
		return false
	}
	g.mu.RLock()
	listenIPs := append([]string(nil), g.listenIPs...)
	listenPort := g.listenPort
	g.mu.RUnlock()
	if len(listenIPs) == 0 {
		listenIPs = reverseProxyHTTPRuntimeListenIPs(rule)
	}
	return reverseProxyResolvedTargetLoopsToListener(listenIPs, listenPort, rule.TargetPort, targetAddress)
}

func (s *ReverseProxyService) resolveTargetCandidates(ctx context.Context, target string, port int, ipStrategy string) ([]reverseProxyTargetCandidate, error) {
	target = strings.TrimSpace(strings.Trim(target, "[]"))
	if target == "" {
		return nil, common.NewError("empty target")
	}
	if ip := net.ParseIP(target); ip != nil {
		family := "ipv4"
		if ip.To4() == nil {
			family = "ipv6"
		}
		return []reverseProxyTargetCandidate{{
			address:    target,
			serverName: target,
			hostHeader: target,
			family:     family,
		}}, nil
	}

	networkName := "ip"
	switch strings.TrimSpace(ipStrategy) {
	case reverseProxyIPStrategyIPv4Only:
		networkName = "ip4"
	case reverseProxyIPStrategyIPv6Only:
		networkName = "ip6"
	}
	resolver := net.DefaultResolver
	ips, err := resolver.LookupIP(ctx, networkName, target)
	if err != nil {
		return nil, err
	}
	result := make([]reverseProxyTargetCandidate, 0, len(ips))
	for _, ip := range ips {
		if ip == nil {
			continue
		}
		family := "ipv4"
		if ip.To4() == nil {
			family = "ipv6"
		}
		result = append(result, reverseProxyTargetCandidate{
			address:    ip.String(),
			serverName: target,
			hostHeader: target,
			family:     family,
		})
	}
	if len(result) == 0 {
		return nil, common.NewError("dns returned no usable ips")
	}
	return result, nil
}

func reorderCandidatesByIPStrategy(items []reverseProxyTargetCandidate, strategy string) []reverseProxyTargetCandidate {
	if len(items) <= 1 {
		return items
	}
	ipv4 := make([]reverseProxyTargetCandidate, 0)
	ipv6 := make([]reverseProxyTargetCandidate, 0)
	for _, item := range items {
		if item.family == "ipv6" {
			ipv6 = append(ipv6, item)
		} else {
			ipv4 = append(ipv4, item)
		}
	}
	switch strategy {
	case reverseProxyIPStrategyIPv4Only:
		return ipv4
	case reverseProxyIPStrategyIPv6Only:
		return ipv6
	case reverseProxyIPStrategyPreferIPv6:
		return append(ipv6, ipv4...)
	default:
		return append(ipv4, ipv6...)
	}
}

func (s *ReverseProxyService) buildRoundTripper(group *reverseProxyListenerGroup, ruleID uint, protocol string, address string, port int, serverName string, strictVerify bool, transportMode string, maxConnections int, maxIdleConnections int) (reverseProxyTransportBundle, error) {
	if maxConnections < 0 {
		maxConnections = 0
	}
	maxIdleConnections, maxIdleConnectionsPerHost := reverseProxyHTTPTransportIdleConnectionLimits(maxIdleConnections)
	acquire := func() (func(), bool) {
		if group == nil {
			return func() {}, true
		}
		limiter, ok := group.acquireUpstreamConnection(ruleID)
		if !ok {
			return nil, false
		}
		return func() { group.releaseUpstreamConnection(ruleID, limiter) }, true
	}
	if protocol == reverseProxyProtocolHTTP {
		dialer := &net.Dialer{
			Timeout:   12 * time.Second,
			KeepAlive: reverseProxyUpstreamTCPKeepAlive,
		}
		transport := &http.Transport{
			DialContext:           reverseProxyFixedAddressDialContextWithTracking(dialer, address, port, acquire),
			DisableCompression:    true,
			DisableKeepAlives:     false,
			MaxConnsPerHost:       maxConnections,
			MaxIdleConns:          maxIdleConnections,
			MaxIdleConnsPerHost:   maxIdleConnectionsPerHost,
			ResponseHeaderTimeout: reverseProxyUpstreamResponseHeaderTimeout,
			IdleConnTimeout:       reverseProxyUpstreamIdleTimeout,
		}
		return reverseProxyTransportBundle{
			RoundTripper: reverseProxyWithResponseHeaderTimeout(transport),
			Cleanup:      transport.CloseIdleConnections,
		}, nil
	}

	switch transportMode {
	case reverseProxyUpstreamModeHTTPSH3:
		tlsConfig := buildReverseProxyUpstreamTLSConfig(serverName, strictVerify, []string{"h3"})
		transport := buildHTTP3RoundTripper(address, port, tlsConfig, acquire)
		return reverseProxyTransportBundle{
			RoundTripper: reverseProxyWithResponseHeaderTimeout(transport),
			Cleanup: func() {
				_ = transport.Close()
			},
		}, nil
	case reverseProxyUpstreamModeHTTPSH2, reverseProxyUpstreamModeHTTPS:
		nextProtos := []string{"h2", "http/1.1"}
		if transportMode == reverseProxyUpstreamModeHTTPSH2 {
			nextProtos = []string{"h2"}
		}
		tlsConfig := buildReverseProxyUpstreamTLSConfig(serverName, strictVerify, nextProtos)
		dialer := &net.Dialer{
			Timeout:   12 * time.Second,
			KeepAlive: reverseProxyUpstreamTCPKeepAlive,
		}
		transport := &http.Transport{
			DialContext:           reverseProxyFixedAddressDialContextWithTracking(dialer, address, port, acquire),
			DisableCompression:    true,
			DisableKeepAlives:     false,
			ForceAttemptHTTP2:     true,
			MaxConnsPerHost:       maxConnections,
			MaxIdleConns:          maxIdleConnections,
			MaxIdleConnsPerHost:   maxIdleConnectionsPerHost,
			ResponseHeaderTimeout: reverseProxyUpstreamResponseHeaderTimeout,
			IdleConnTimeout:       reverseProxyUpstreamIdleTimeout,
			TLSHandshakeTimeout:   12 * time.Second,
			TLSClientConfig:       tlsConfig,
		}
		if h2Transport, err := http2.ConfigureTransports(transport); err == nil && h2Transport != nil {
			h2Transport.ReadIdleTimeout = reverseProxyUpstreamHTTP2ReadIdleTimeout
			h2Transport.PingTimeout = reverseProxyUpstreamHTTP2PingTimeout
		}
		return reverseProxyTransportBundle{
			RoundTripper: reverseProxyWithResponseHeaderTimeout(transport),
			Cleanup:      transport.CloseIdleConnections,
		}, nil
	default:
		return reverseProxyTransportBundle{}, common.NewError("invalid upstream transport mode")
	}
}

// reverseProxyHTTPTransportIdleConnectionLimits translates the panel value
// into net/http's two related controls.  A panel value of zero means no
// additional idle-pool ceiling.  http.Transport treats a zero
// MaxIdleConnsPerHost as its hidden default of two, so use the largest native
// int explicitly while leaving MaxIdleConns at zero (unlimited) instead.
func reverseProxyHTTPTransportIdleConnectionLimits(value int) (total int, perHost int) {
	if value > 0 {
		return value, value
	}
	return 0, int(^uint(0) >> 1)
}

func buildReverseProxyUpstreamTLSConfig(serverName string, strictVerify bool, nextProtos []string) *tls.Config {
	config := &tls.Config{
		ServerName:         serverName,
		InsecureSkipVerify: !strictVerify,
		MinVersion:         tls.VersionTLS12,
	}
	if len(nextProtos) > 0 {
		config.NextProtos = append([]string(nil), nextProtos...)
	}
	return config
}

func buildHTTP3RoundTripper(address string, port int, tlsConfig *tls.Config, acquire func() (func(), bool)) *http3.Transport {
	if acquire == nil {
		acquire = func() (func(), bool) { return func() {}, true }
	}
	return &http3.Transport{
		TLSClientConfig: cloneTLSConfig(tlsConfig, tlsConfig),
		QUICConfig: &quic.Config{
			KeepAlivePeriod: reverseProxyUpstreamQUICKeepAlivePeriod,
			MaxIdleTimeout:  reverseProxyUpstreamIdleTimeout,
		},
		Dial: func(ctx context.Context, _ string, cfg *tls.Config, quicCfg *quic.Config) (*quic.Conn, error) {
			release, ok := acquire()
			if !ok {
				return nil, errors.New("reverse proxy upstream connection limit reached")
			}
			conn, err := quic.DialAddr(ctx, net.JoinHostPort(address, strconv.Itoa(port)), cloneTLSConfig(cfg, tlsConfig), quicCfg)
			if err != nil {
				release()
				return nil, err
			}
			go func(c *quic.Conn) {
				if c == nil {
					release()
					return
				}
				<-c.Context().Done()
				release()
			}(conn)
			return conn, nil
		},
	}
}

func reverseProxyFixedAddressDialContext(dialer *net.Dialer, address string, port int) func(context.Context, string, string) (net.Conn, error) {
	return func(ctx context.Context, networkName string, _ string) (net.Conn, error) {
		return dialer.DialContext(ctx, networkName, net.JoinHostPort(address, strconv.Itoa(port)))
	}
}

func reverseProxyFixedAddressDialContextWithTracking(dialer *net.Dialer, address string, port int, acquire func() (func(), bool)) func(context.Context, string, string) (net.Conn, error) {
	if acquire == nil {
		acquire = func() (func(), bool) { return func() {}, true }
	}
	return func(ctx context.Context, networkName string, _ string) (net.Conn, error) {
		release, ok := acquire()
		if !ok {
			return nil, errors.New("reverse proxy upstream connection limit reached")
		}
		conn, err := dialer.DialContext(ctx, networkName, net.JoinHostPort(address, strconv.Itoa(port)))
		if err != nil {
			release()
			return nil, err
		}
		return &reverseProxyCountedConn{
			Conn:    conn,
			onClose: release,
		}, nil
	}
}

func cloneTLSConfig(base *tls.Config, fallback *tls.Config) *tls.Config {
	if base != nil {
		return base.Clone()
	}
	if fallback != nil {
		return fallback.Clone()
	}
	return &tls.Config{}
}

func (s *ReverseProxyService) probeUpstream(ctx context.Context, protocol string, address string, port int, serverName string, strictVerify bool, httpVersionStrategy string) (string, error) {
	if protocol == reverseProxyProtocolHTTP {
		dialer := &net.Dialer{Timeout: 10 * time.Second}
		conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(address, strconv.Itoa(port)))
		if err != nil {
			return "", err
		}
		_ = conn.Close()
		return reverseProxyUpstreamModeHTTP, nil
	}

	switch httpVersionStrategy {
	case reverseProxyHTTPVersionH3Only:
		if err := probeReverseProxyHTTP3(ctx, address, port, serverName, strictVerify); err != nil {
			return "", err
		}
		return reverseProxyUpstreamModeHTTPSH3, nil
	case reverseProxyHTTPVersionDualRequiredPreferH3:
		h2ErrCh := make(chan error, 1)
		h3ErrCh := make(chan error, 1)
		go func() {
			h2ErrCh <- probeReverseProxyHTTPS(ctx, address, port, serverName, strictVerify, true)
		}()
		go func() {
			h3ErrCh <- probeReverseProxyHTTP3(ctx, address, port, serverName, strictVerify)
		}()
		h2Err := <-h2ErrCh
		h3Err := <-h3ErrCh
		if h2Err != nil || h3Err != nil {
			return "", common.NewError(fmt.Sprintf("https dual probe failed: h2=%v; h3=%v", h2Err, h3Err))
		}
		return reverseProxyUpstreamModeHTTPSH3, nil
	case reverseProxyHTTPVersionPreferH3:
		h3Err := probeReverseProxyHTTP3(ctx, address, port, serverName, strictVerify)
		if h3Err == nil {
			return reverseProxyUpstreamModeHTTPSH3, nil
		}
		if err := probeReverseProxyHTTPS(ctx, address, port, serverName, strictVerify, false); err != nil {
			return "", common.NewError(fmt.Sprintf("http3 probe failed: %v; tls fallback failed: %v", h3Err, err))
		}
		return reverseProxyUpstreamModeHTTPS, nil
	case reverseProxyHTTPVersionH2Only:
		if err := probeReverseProxyHTTPS(ctx, address, port, serverName, strictVerify, true); err != nil {
			return "", err
		}
		return reverseProxyUpstreamModeHTTPSH2, nil
	default:
		if err := probeReverseProxyHTTPS(ctx, address, port, serverName, strictVerify, false); err != nil {
			return "", err
		}
		return reverseProxyUpstreamModeHTTPS, nil
	}
}

func probeReverseProxyHTTPS(ctx context.Context, address string, port int, serverName string, strictVerify bool, requireH2 bool) error {
	nextProtos := []string{"h2", "http/1.1"}
	if requireH2 {
		nextProtos = []string{"h2"}
	}
	tlsConfig := buildReverseProxyUpstreamTLSConfig(serverName, strictVerify, nextProtos)
	dialer := &tls.Dialer{
		NetDialer: &net.Dialer{Timeout: 10 * time.Second},
		Config:    tlsConfig,
	}
	conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(address, strconv.Itoa(port)))
	if err != nil {
		return err
	}
	defer conn.Close()

	tlsConn, ok := conn.(*tls.Conn)
	if !ok {
		return common.NewError("unexpected tls connection type")
	}
	if requireH2 && tlsConn.ConnectionState().NegotiatedProtocol != "h2" {
		return common.NewError("upstream https did not negotiate h2")
	}
	return nil
}

func probeReverseProxyHTTP3(ctx context.Context, address string, port int, serverName string, strictVerify bool) error {
	tlsConfig := buildReverseProxyUpstreamTLSConfig(serverName, strictVerify, []string{"h3"})
	conn, err := quic.DialAddr(ctx, net.JoinHostPort(address, strconv.Itoa(port)), tlsConfig, nil)
	if err != nil {
		return err
	}
	return conn.CloseWithError(0, "")
}

func (g *reverseProxyListenerGroup) noteHTTP3ServerStopped() {
	if g == nil {
		return
	}
	g.mu.Lock()
	if g.closed {
		g.mu.Unlock()
		return
	}
	if g.h3ServingCount > 0 {
		g.h3ServingCount--
	}
	if g.h3ServingCount == 0 {
		g.h3ListenerAvailable = false
	}
	g.h3AvailabilityKnown = true
	g.mu.Unlock()
	WakeReverseProxyRuntime()
}

func (g *reverseProxyListenerGroup) shutdown() error {
	if g == nil {
		return nil
	}
	var firstErr error
	selections := make([]reverseProxyCertificateSelection, 0)
	ruleLimiters := make([]*reverseProxyAdjustableLimiter, 0)
	hijackedConnections := make([]net.Conn, 0)
	connectionSlotsToRelease := 0
	g.statsMu.Lock()
	for connID, state := range g.localConnStates {
		if state.HasSelection {
			selections = append(selections, state.Selection)
		}
		for _, ruleLimiter := range state.RuleConnectionLimiters {
			if ruleLimiter != nil {
				ruleLimiters = append(ruleLimiters, ruleLimiter)
			}
		}
		delete(g.localConnStates, connID)
	}
	for index := range g.pendingConnSelectionShards {
		shard := &g.pendingConnSelectionShards[index]
		for addrKey, selection := range shard.selections {
			if selection != nil {
				selections = append(selections, selection.Selection)
			}
			delete(shard.selections, addrKey)
		}
		if shard.lru != nil {
			shard.lru.Init()
		}
	}
	g.localConnIDs = make(map[net.Conn]string)
	g.localConnByID = make(map[string]net.Conn)
	g.localConnAddrToID = make(map[string]string)
	g.localConnAddrByID = make(map[string]string)
	for _, conn := range g.hijackedConnections {
		if conn != nil {
			hijackedConnections = append(hijackedConnections, conn)
		}
	}
	g.hijackedConnections = make(map[string]net.Conn)
	g.connectionCounts = make(map[uint]reverseProxyConnectionCounts)
	for connID := range g.connectionSlotIDs {
		delete(g.connectionSlotIDs, connID)
		connectionSlotsToRelease++
	}
	g.statsMu.Unlock()
	for i := 0; i < connectionSlotsToRelease; i++ {
		g.releaseListenerConnectionSlot()
	}
	for _, selection := range selections {
		g.releaseCertificateSelection(selection)
	}
	g.clearCertificateBalanceStates()
	for _, limiter := range ruleLimiters {
		limiter.Release()
	}
	for _, conn := range hijackedConnections {
		_ = conn.Close()
	}
	g.mu.Lock()
	g.closed = true
	oldUpstreams := g.upstreamByRule
	oldDNSHandler := g.dnsHandler
	g.upstreamByRule = make(map[uint]*reverseProxyCachedUpstream)
	g.dnsHandler = nil
	g.mu.Unlock()
	for _, upstream := range oldUpstreams {
		g.disposeCachedUpstream(upstream)
	}
	if err := closeReverseProxyDNSHandler(oldDNSHandler); err != nil && firstErr == nil {
		firstErr = err
	}
	if len(g.h3Servers) > 0 {
		for _, server := range g.h3Servers {
			if server == nil {
				continue
			}
			err := shutdownReverseProxyHTTP3Server(server)
			if err != nil && firstErr == nil {
				firstErr = err
			}
		}
	} else if g.h3Server != nil {
		err := shutdownReverseProxyHTTP3Server(g.h3Server)
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if len(g.servers) > 0 {
		for _, server := range g.servers {
			if server == nil {
				continue
			}
			if err := shutdownReverseProxyHTTPServer(server); err != nil && firstErr == nil {
				firstErr = err
			}
		}
	} else if g.server != nil {
		firstErr = shutdownReverseProxyHTTPServer(g.server)
	}
	if len(g.listeners) > 0 {
		for _, listener := range g.listeners {
			if listener != nil {
				_ = listener.Close()
			}
		}
	} else if g.listener != nil {
		_ = g.listener.Close()
	}
	if len(g.packetConns) > 0 {
		for _, conn := range g.packetConns {
			if conn != nil {
				_ = conn.Close()
			}
		}
	} else if g.packetConn != nil {
		_ = g.packetConn.Close()
	}
	return firstErr
}

func (r *reverseProxyRuntimeManager) registerMismatch(ip string, reason string) time.Duration {
	ip = strings.TrimSpace(ip)
	if ip == "" {
		return 0
	}
	shard := r.mismatchShard(ip)
	if shard == nil {
		return 0
	}
	shard.mu.Lock()
	defer shard.mu.Unlock()
	now := time.Now()
	if shard.entries == nil {
		shard.entries = make(map[string]*reverseProxyMismatchEntry)
	}
	if shard.lru == nil {
		shard.lru = list.New()
	}
	for shard.lru.Len() > 0 {
		oldestKey, _ := shard.lru.Back().Value.(string)
		oldest := shard.entries[oldestKey]
		if oldest != nil && !oldest.LastAttempt.IsZero() && now.Sub(oldest.LastAttempt) < reverseProxyRuntimeTableTTL {
			break
		}
		if oldest != nil && oldest.element != nil {
			shard.lru.Remove(oldest.element)
		}
		delete(shard.entries, oldestKey)
	}
	entry := shard.entries[ip]
	if entry == nil || entry.LastAttempt.IsZero() || now.Sub(entry.LastAttempt) >= reverseProxyRuntimeTableTTL {
		if entry != nil && entry.element != nil {
			shard.lru.Remove(entry.element)
		}
		perShardLimit := reverseProxyMismatchMaxEntries / reverseProxyRuntimeTableShardCount
		for len(shard.entries) >= perShardLimit && shard.lru.Len() > 0 {
			oldestKey, _ := shard.lru.Back().Value.(string)
			oldest := shard.entries[oldestKey]
			if oldest != nil && oldest.element != nil {
				shard.lru.Remove(oldest.element)
			}
			delete(shard.entries, oldestKey)
		}
		entry = &reverseProxyMismatchEntry{}
		entry.element = shard.lru.PushFront(ip)
		shard.entries[ip] = entry
	} else if entry.element != nil {
		shard.lru.MoveToFront(entry.element)
	}
	entry.Count++
	entry.LastAttempt = now
	entry.LastReason = strings.TrimSpace(reason)
	if entry.Count <= reverseProxyMismatchFreeLimit {
		entry.DelayedUntil = time.Time{}
		return 0
	}
	entry.DelayedUntil = now.Add(reverseProxyMismatchDelay)
	return reverseProxyMismatchDelay
}

func (r *reverseProxyRuntimeManager) clearMismatch(ip string) {
	ip = strings.TrimSpace(ip)
	if ip == "" {
		return
	}
	shard := r.mismatchShard(ip)
	if shard == nil {
		return
	}
	shard.mu.Lock()
	if entry := shard.entries[ip]; entry != nil && entry.element != nil && shard.lru != nil {
		shard.lru.Remove(entry.element)
	}
	delete(shard.entries, ip)
	shard.mu.Unlock()
}

func (r *reverseProxyRuntimeManager) mismatchShard(ip string) *reverseProxyMismatchShard {
	if r == nil {
		return nil
	}
	index := int(crc32.ChecksumIEEE([]byte(ip)) % reverseProxyRuntimeTableShardCount)
	return &r.mismatchShards[index]
}

func (r *reverseProxyRuntimeManager) resetMismatchTable() {
	if r == nil {
		return
	}
	for index := range r.mismatchShards {
		shard := &r.mismatchShards[index]
		shard.mu.Lock()
		shard.entries = make(map[string]*reverseProxyMismatchEntry)
		shard.lru = list.New()
		shard.mu.Unlock()
	}
}
