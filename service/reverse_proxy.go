package service

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

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

	reverseProxyMismatchFreeLimit            = 5
	reverseProxyMismatchDelay                = 30 * time.Second
	reverseProxyMismatchCooldown             = 24 * time.Hour
	reverseProxyDialFallbackGap              = 20 * time.Millisecond
	reverseProxyReadHeaderTimeout            = 15 * time.Second
	reverseProxyServerIdleTimeout            = 10 * time.Minute
	reverseProxyRequestTimeout               = 120 * time.Second
	reverseProxyShutdownTimeout              = 5 * time.Second
	reverseProxyAltSvcMaxAgeSeconds          = 2592000
	reverseProxyUpstreamIdleTimeout          = 10 * time.Minute
	reverseProxyUpstreamTCPKeepAlive         = 30 * time.Second
	reverseProxyUpstreamHTTP2ReadIdleTimeout = 30 * time.Second
	reverseProxyUpstreamHTTP2PingTimeout     = 15 * time.Second
	reverseProxyUpstreamQUICKeepAlivePeriod  = 30 * time.Second
	reverseProxyDelayReasonMismatch          = "url_mismatch_penalty"

	reverseProxyUpstreamModeHTTP    = "http"
	reverseProxyUpstreamModeHTTPS   = "https"
	reverseProxyUpstreamModeHTTPSH2 = "https_h2"
	reverseProxyUpstreamModeHTTPSH3 = "https_h3"

	reverseProxyEDNSModeAuto   = "auto"
	reverseProxyEDNSModeCustom = "custom"

	reverseProxyEDNSClientSubnetPolicyClientIP            = "client_ip"
	reverseProxyEDNSClientSubnetPolicyPreferRequestPublic = "prefer_request_public"
)

var reverseProxyHostTokenRe = regexp.MustCompile(`^[A-Za-z0-9\.\-:\[\]]+$`)

type ReverseProxyService struct {
	CertificateInventoryService
}

type ReverseProxyRulePayload struct {
	ID                        uint   `json:"id"`
	Name                      string `json:"name"`
	Enabled                   bool   `json:"enabled"`
	ListenProtocol            string `json:"listenProtocol"`
	ListenProtocolAlias       string `json:"listenProtocolAlias"`
	ListenIP                  string `json:"listenIP"`
	ListenIPs                 string `json:"listenIPs"`
	ListenPort                int    `json:"listenPort"`
	Hosts                     string `json:"hosts"`
	PathPrefix                string `json:"pathPrefix"`
	ListenDNSPath             string `json:"listenDnsPath"`
	TargetProtocol            string `json:"targetProtocol"`
	TargetProtocolAlias       string `json:"targetProtocolAlias"`
	TargetAddresses           string `json:"targetAddresses"`
	TargetPort                int    `json:"targetPort"`
	TargetPath                string `json:"targetPath"`
	TargetDNSPath             string `json:"targetDnsPath"`
	EDNSEnabled               bool   `json:"ednsEnabled"`
	EDNSMode                  string `json:"ednsMode"`
	EDNSCustomIP              string `json:"ednsCustomIp"`
	EDNSClientSubnetPolicy    string `json:"ednsClientSubnetPolicy"`
	DisableIPv4Answer         bool   `json:"disableIpv4Answer"`
	DisableIPv6Answer         bool   `json:"disableIpv6Answer"`
	CertificateRecordIDs      []uint `json:"certificateRecordIds"`
	CertificateRecordID       uint   `json:"certificateRecordId"`
	ListenHTTPVersionStrategy string `json:"listenHttpVersionStrategy"`
	IPStrategy                string `json:"ipStrategy"`
	HTTPVersionStrategy       string `json:"httpVersionStrategy"`
	UpstreamTLSVerify         bool   `json:"upstreamTlsVerify"`
	ApiPassthrough            bool   `json:"apiPassthrough"`
	Remark                    string `json:"remark"`
}

type ReverseProxyRuleReorderPayload struct {
	IDs []uint `json:"ids"`
}

type ReverseProxyRuleDeletePayload struct {
	ID uint `json:"id"`
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
	ID                        uint                                       `json:"id"`
	DisplayID                 uint64                                     `json:"displayId"`
	ListOrder                 int64                                      `json:"listOrder"`
	Name                      string                                     `json:"name"`
	Enabled                   bool                                       `json:"enabled"`
	ListenProtocol            string                                     `json:"listenProtocol"`
	ListenProtocolAlias       string                                     `json:"listenProtocolAlias"`
	ListenIP                  string                                     `json:"listenIP"`
	ListenIPs                 []string                                   `json:"listenIPs"`
	ListenPort                int                                        `json:"listenPort"`
	Hosts                     []string                                   `json:"hosts"`
	PathPrefix                string                                     `json:"pathPrefix"`
	ListenDNSPath             string                                     `json:"listenDnsPath"`
	TargetProtocol            string                                     `json:"targetProtocol"`
	TargetProtocolAlias       string                                     `json:"targetProtocolAlias"`
	TargetAddresses           []string                                   `json:"targetAddresses"`
	TargetPort                int                                        `json:"targetPort"`
	TargetPath                string                                     `json:"targetPath"`
	TargetDNSPath             string                                     `json:"targetDnsPath"`
	EDNSEnabled               bool                                       `json:"ednsEnabled"`
	EDNSMode                  string                                     `json:"ednsMode"`
	EDNSCustomIP              string                                     `json:"ednsCustomIp"`
	EDNSClientSubnetPolicy    string                                     `json:"ednsClientSubnetPolicy"`
	DisableIPv4Answer         bool                                       `json:"disableIpv4Answer"`
	DisableIPv6Answer         bool                                       `json:"disableIpv6Answer"`
	CertificateRecordIDs      []uint                                     `json:"certificateRecordIds"`
	CertificateRecordID       uint                                       `json:"certificateRecordId"`
	CertificateLabel          string                                     `json:"certificateLabel"`
	CertificateLabels         []string                                   `json:"certificateLabels"`
	ListenHTTPVersionStrategy string                                     `json:"listenHttpVersionStrategy"`
	IPStrategy                string                                     `json:"ipStrategy"`
	HTTPVersionStrategy       string                                     `json:"httpVersionStrategy"`
	UpstreamTLSVerify         bool                                       `json:"upstreamTlsVerify"`
	ApiPassthrough            bool                                       `json:"apiPassthrough"`
	Remark                    string                                     `json:"remark"`
	LastError                 string                                     `json:"lastError"`
	RuntimeStatus             string                                     `json:"runtimeStatus"`
	LocalConnectionCount      int                                        `json:"localConnectionCount"`
	UpstreamConnectionCount   int                                        `json:"upstreamConnectionCount"`
	CertificateHints          []string                                   `json:"certificateHints,omitempty"`
	CertificateBalance        []ReverseProxyCertificateBalanceDiagnostic `json:"certificateBalance,omitempty"`
	UpdatedAt                 int64                                      `json:"updatedAt"`
	CreatedAt                 int64                                      `json:"createdAt"`
}

type ReverseProxyOverview struct {
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

type reverseProxyNormalizedRule struct {
	id                        uint
	name                      string
	enabled                   bool
	listenProtocol            string
	listenProtocolAlias       string
	listenIPs                 []string
	listenPort                int
	hosts                     []string
	pathPrefix                string
	listenDNSPath             string
	targetProtocol            string
	targetProtocolAlias       string
	targetAddresses           []string
	targetPort                int
	targetPath                string
	targetDNSPath             string
	ednsEnabled               bool
	ednsMode                  string
	ednsCustomIP              string
	ednsClientSubnetPolicy    string
	disableIPv4Answer         bool
	disableIPv6Answer         bool
	certificateRecordIDs      []uint
	certificateRecordID       uint
	listenHTTPVersionStrategy string
	ipStrategy                string
	httpVersionStrategy       string
	upstreamTLSVerify         bool
	apiPassthrough            bool
	remark                    string
}

type reverseProxyMismatchEntry struct {
	Count        int
	LastAttempt  time.Time
	DelayedUntil time.Time
	LastReason   string
}

type reverseProxyTargetCandidate struct {
	address    string
	serverName string
	hostHeader string
	family     string
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
	lastSyncAt    time.Time
	lastRenderKey string
	warnings      []string
}

type reverseProxyRenderRule struct {
	ID                        uint                                 `json:"id"`
	ListOrder                 int64                                `json:"listOrder"`
	Enabled                   bool                                 `json:"enabled"`
	ListenProtocol            string                               `json:"listenProtocol"`
	ListenHTTPVersionStrategy string                               `json:"listenHttpVersionStrategy"`
	ListenIPs                 []string                             `json:"listenIPs"`
	ListenPort                int                                  `json:"listenPort"`
	Hosts                     []string                             `json:"hosts"`
	PathPrefix                string                               `json:"pathPrefix"`
	ListenDNSPath             string                               `json:"listenDnsPath"`
	TargetProtocol            string                               `json:"targetProtocol"`
	TargetAddresses           []string                             `json:"targetAddresses"`
	TargetPort                int                                  `json:"targetPort"`
	TargetPath                string                               `json:"targetPath"`
	TargetDNSPath             string                               `json:"targetDnsPath"`
	EDNSEnabled               bool                                 `json:"ednsEnabled"`
	EDNSMode                  string                               `json:"ednsMode"`
	EDNSCustomIP              string                               `json:"ednsCustomIp"`
	EDNSClientSubnetPolicy    string                               `json:"ednsClientSubnetPolicy"`
	DisableIPv4Answer         bool                                 `json:"disableIpv4Answer"`
	DisableIPv6Answer         bool                                 `json:"disableIpv6Answer"`
	CertificateRecordIDs      []uint                               `json:"certificateRecordIds,omitempty"`
	CertificateStates         []reverseProxyRenderCertificateState `json:"certificateStates,omitempty"`
	IPStrategy                string                               `json:"ipStrategy"`
	HTTPVersionStrategy       string                               `json:"httpVersionStrategy"`
	UpstreamTLSVerify         bool                                 `json:"upstreamTlsVerify"`
	ApiPassthrough            bool                                 `json:"apiPassthrough"`
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

type reverseProxyCertificateSelection struct {
	ListenerKey         string
	SNIBucket           string
	CertificateRecordID uint
}

type reverseProxyLocalConnectionState struct {
	RuleID       uint
	Selection    reverseProxyCertificateSelection
	HasSelection bool
}

type reverseProxyListenerGroup struct {
	mu                        sync.RWMutex
	statsMu                   sync.Mutex
	key                       string
	listenIP                  string
	listenPort                int
	protocol                  string
	listenHTTPVersionStrategy string
	server                    *http.Server
	listener                  net.Listener
	h3Server                  *http3.Server
	packetConn                net.PacketConn
	servers                   []*http.Server
	listeners                 []net.Listener
	h3Servers                 []*http3.Server
	packetConns               []net.PacketConn
	rules                     []*model.ReverseProxyRule
	service                   *ReverseProxyService
	certBindingsByRule        map[uint][]*reverseProxyRuleCertificateBinding
	orderedCertBindings       []*reverseProxyRuleCertificateBinding
	warnings                  []string
	upstreamByRule            map[uint]*reverseProxyCachedUpstream
	defaultCert               *tls.Certificate
	defaultLeaf               *x509LeafState
	connectionCounts          map[uint]reverseProxyConnectionCounts
	localConnIDs              map[net.Conn]string
	localConnStates           map[string]reverseProxyLocalConnectionState
	localConnAddrToID         map[string]string
	localConnAddrByID         map[string]string
	pendingConnSelections     map[string]reverseProxyCertificateSelection
	nextConnID                uint64
}

type reverseProxyCertificateHint struct {
	ruleID   uint
	messages []string
}

type reverseProxyRuntimeManager struct {
	mu             sync.Mutex
	groups         map[string]*reverseProxyListenerGroup
	mismatchMu     sync.Mutex
	mismatchByIP   map[string]*reverseProxyMismatchEntry
	state          reverseProxyRuntimeState
	reconcileError string
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
	RoundTripper    http.RoundTripper
	Cleanup         func()
	refs            int
	closing         bool
}

type reverseProxyCleanupReadCloser struct {
	io.ReadCloser
	onClose func()
}

func (c *reverseProxyCleanupReadCloser) Close() error {
	err := c.ReadCloser.Close()
	if c.onClose != nil {
		c.onClose()
	}
	return err
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
	ExternalPathPrefix   string
}

var reverseProxyRuntime = &reverseProxyRuntimeManager{
	groups:       make(map[string]*reverseProxyListenerGroup),
	mismatchByIP: make(map[string]*reverseProxyMismatchEntry),
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
	reverseProxyRuntime.mu.Lock()
	defer reverseProxyRuntime.mu.Unlock()

	if reverseProxySupported() {
		if err := reverseProxyRuntime.reconcileLocked(s, 2*time.Second); err != nil {
			reverseProxyRuntime.reconcileError = strings.TrimSpace(err.Error())
		}
	}

	rules, err := s.loadRulesLocked(database.GetDB())
	if err != nil {
		return nil, err
	}
	certOptions, certMap, err := s.listCertificateOptions()
	if err != nil {
		return nil, err
	}
	warnings := append([]string(nil), reverseProxyRuntime.state.warnings...)
	balanceDiagnosticsByRule, err := s.loadRuleCertificateBalanceDiagnostics(rules)
	if err != nil {
		warnings = append(warnings, "reverse proxy certificate balance diagnostics unavailable: "+strings.TrimSpace(err.Error()))
		balanceDiagnosticsByRule = make(map[uint][]ReverseProxyCertificateBalanceDiagnostic)
	}
	connectionCounts := reverseProxySnapshotConnectionCounts(reverseProxyRuntime.groups)
	views := make([]ReverseProxyRuleView, 0, len(rules))
	enabledCount := 0
	for i := range rules {
		if rules[i].Enabled {
			enabledCount++
		}
		views = append(views, buildReverseProxyRuleView(&rules[i], certMap, connectionCounts[rules[i].Id], balanceDiagnosticsByRule[rules[i].Id]))
	}
	lastSyncAt := int64(0)
	if !reverseProxyRuntime.state.lastSyncAt.IsZero() {
		lastSyncAt = reverseProxyRuntime.state.lastSyncAt.Unix()
	}
	listenerCount := reverseProxyListenerCount(reverseProxyRuntime.groups) + reverseProxyDNSRuntime.listenerCount()
	overview := &ReverseProxyOverview{
		Available:        reverseProxySupported(),
		Started:          listenerCount > 0,
		ListenerCount:    listenerCount,
		EnabledCount:     enabledCount,
		RuleCount:        len(rules),
		CertificateCount: len(certOptions),
		LastSyncAt:       lastSyncAt,
		Certificates:     certOptions,
		Rules:            views,
		Warnings:         warnings,
	}
	if reverseProxyRuntime.reconcileError != "" {
		overview.Error = reverseProxyRuntime.reconcileError
	}
	if !overview.Available {
		overview.Error = "reverse proxy runtime is unavailable on this system"
	}
	return overview, nil
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

	savedRow := model.ReverseProxyRule{}
	previousRow := model.ReverseProxyRule{}
	hadPrevious := false
	err = db.Transaction(func(tx *gorm.DB) error {
		existing := &model.ReverseProxyRule{}
		isNew := normalized.id == 0
		if !isNew {
			if err := tx.Where("id = ?", normalized.id).First(existing).Error; err != nil {
				return err
			}
			previousRow = *existing
			hadPrevious = true
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
		row.ListenIP = ""
		row.ListenIPList = ""
		row.ListenPort = normalized.listenPort
		row.HostList = encodeReverseProxyList(normalized.hosts)
		row.PathPrefix = normalized.pathPrefix
		row.ListenDNSPath = normalized.listenDNSPath
		row.TargetProtocol = normalized.targetProtocol
		row.TargetProtocolAlias = normalized.targetProtocolAlias
		row.TargetAddresses = encodeReverseProxyList(normalized.targetAddresses)
		row.TargetPort = normalized.targetPort
		row.TargetPath = normalized.targetPath
		row.TargetDNSPath = normalized.targetDNSPath
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
		row.ApiPassthrough = normalized.apiPassthrough
		row.Remark = normalized.remark
		if row.RuntimeStatus == "" {
			row.RuntimeStatus = "pending"
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
		savedRow = *row
		return nil
	})
	if err != nil {
		return err
	}
	if err := s.syncAllRuntimeNow(); err != nil {
		if hadPrevious {
			restoreErr := db.Model(&model.ReverseProxyRule{}).
				Where("id = ?", previousRow.Id).
				Updates(reverseProxyRulePersistenceMap(&previousRow)).Error
			if restoreErr != nil {
				logger.Warning("reverse proxy rule rollback failed: ", restoreErr)
			}
		} else {
			restoreErr := db.Where("id = ?", savedRow.Id).Delete(&model.ReverseProxyRule{}).Error
			if restoreErr != nil {
				logger.Warning("reverse proxy rule create rollback failed: ", restoreErr)
			}
		}
		_ = s.syncAllRuntimeNow()
		return err
	}
	return nil
}

func (s *ReverseProxyService) DeleteRule(id uint) error {
	if id == 0 {
		return common.NewError("id is required")
	}
	db := database.GetDB()
	if db == nil {
		return common.NewError("database is not ready")
	}
	deletedRow := model.ReverseProxyRule{}
	err := db.Transaction(func(tx *gorm.DB) error {
		row := &model.ReverseProxyRule{}
		if err := tx.Where("id = ?", id).First(row).Error; err != nil {
			return err
		}
		deletedRow = *row
		if err := tx.Delete(row).Error; err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return err
	}
	if err := s.syncAllRuntimeNow(); err != nil {
		restoreErr := db.Create(&deletedRow).Error
		if restoreErr != nil {
			logger.Warning("reverse proxy rule delete rollback failed: ", restoreErr)
		}
		_ = s.syncAllRuntimeNow()
		return err
	}
	return nil
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
		for i := range rules {
			nextOrder, ok := orderMap[rules[i].Id]
			if !ok {
				return common.NewError("reorder ids mismatch current rules")
			}
			if err := tx.Model(&model.ReverseProxyRule{}).Where("id = ?", rules[i].Id).Update("list_order", nextOrder).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	return s.syncAllRuntimeNow()
}

func (s *ReverseProxyService) SyncIfNeeded(minGap time.Duration) error {
	if err := s.MaintainCertificateBalance(false); err != nil {
		return err
	}
	if err := reverseProxyRuntime.SyncIfNeeded(s, minGap); err != nil {
		return err
	}
	rows, err := s.loadRulesLocked(database.GetDB())
	if err != nil {
		return err
	}
	return syncReverseProxyDNSRuntime(s, rows)
}

func (s *ReverseProxyService) StartRuntime() error {
	if err := s.MaintainCertificateBalance(true); err != nil {
		return err
	}
	if err := s.resetRuntimeStateForStartup(); err != nil {
		return err
	}
	if err := reverseProxyRuntime.SyncNow(s); err != nil {
		return err
	}
	rows, err := s.loadRulesLocked(database.GetDB())
	if err != nil {
		return err
	}
	return syncReverseProxyDNSRuntime(s, rows)
}

func (s *ReverseProxyService) StopRuntime() error {
	httpErr := reverseProxyRuntime.Stop()
	dnsErr := stopReverseProxyDNSRuntime()
	if httpErr != nil {
		return httpErr
	}
	return dnsErr
}

func (s *ReverseProxyService) syncAllRuntimeNow() error {
	if err := reverseProxyRuntime.SyncNow(s); err != nil {
		return err
	}
	rows, err := s.loadRulesLocked(database.GetDB())
	if err != nil {
		return err
	}
	return syncReverseProxyDNSRuntime(s, rows)
}

func (s *ReverseProxyService) resetRuntimeStateForStartup() error {
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
	if err := s.repairDisplayIDsTx(db); err != nil {
		return nil, err
	}
	if err := db.Order("list_order asc, id asc").Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (s *ReverseProxyService) listCertificateOptions() ([]ReverseProxyCertificateOption, map[uint]ReverseProxyCertificateOption, error) {
	rows, err := s.CertificateInventoryService.List()
	if err != nil {
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
			MainDomain: rows[i].MainDomain,
			Domains:    append([]string(nil), rows[i].Domains...),
			NotAfter:   rows[i].NotAfter,
			Status:     rows[i].Status,
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
	listenIPs := decodeReverseProxyListenIPs(row)
	hosts := reverseProxyRuleServerNames(row)
	certIDs := reverseProxyRuleCertificateIDs(row)
	view = ReverseProxyRuleView{
		ID:                        row.Id,
		DisplayID:                 row.DisplayID,
		ListOrder:                 row.ListOrder,
		Name:                      strings.TrimSpace(row.Name),
		Enabled:                   row.Enabled,
		ListenProtocol:            strings.TrimSpace(row.ListenProtocol),
		ListenProtocolAlias:       strings.TrimSpace(row.ListenProtocolAlias),
		ListenIP:                  strings.TrimSpace(row.ListenIP),
		ListenIPs:                 listenIPs,
		ListenPort:                row.ListenPort,
		Hosts:                     hosts,
		PathPrefix:                strings.TrimSpace(row.PathPrefix),
		ListenDNSPath:             strings.TrimSpace(row.ListenDNSPath),
		TargetProtocol:            strings.TrimSpace(row.TargetProtocol),
		TargetProtocolAlias:       strings.TrimSpace(row.TargetProtocolAlias),
		TargetAddresses:           decodeReverseProxyList(row.TargetAddresses),
		TargetPort:                row.TargetPort,
		TargetPath:                strings.TrimSpace(row.TargetPath),
		TargetDNSPath:             strings.TrimSpace(row.TargetDNSPath),
		EDNSEnabled:               row.EDNSEnabled,
		EDNSMode:                  strings.TrimSpace(row.EDNSMode),
		EDNSCustomIP:              strings.TrimSpace(row.EDNSCustomIP),
		EDNSClientSubnetPolicy:    strings.TrimSpace(row.EDNSClientSubnetPolicy),
		DisableIPv4Answer:         row.DisableIPv4Answer,
		DisableIPv6Answer:         row.DisableIPv6Answer,
		CertificateRecordIDs:      append([]uint(nil), certIDs...),
		ListenHTTPVersionStrategy: strings.TrimSpace(row.ListenHTTPVersionStrategy),
		IPStrategy:                strings.TrimSpace(row.IPStrategy),
		HTTPVersionStrategy:       strings.TrimSpace(row.HTTPVersionStrategy),
		UpstreamTLSVerify:         row.UpstreamTLSVerify,
		ApiPassthrough:            row.ApiPassthrough,
		Remark:                    strings.TrimSpace(row.Remark),
		LastError:                 strings.TrimSpace(row.LastError),
		RuntimeStatus:             strings.TrimSpace(row.RuntimeStatus),
		LocalConnectionCount:      counts.LocalOpen,
		UpstreamConnectionCount:   counts.UpstreamOpen,
		CertificateBalance:        append([]ReverseProxyCertificateBalanceDiagnostic(nil), balance...),
		UpdatedAt:                 row.UpdatedAt.Unix(),
		CreatedAt:                 row.CreatedAt.Unix(),
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
		view.CertificateHints = buildReverseProxyCertificateHints(view.ListenIPs, view.Hosts, hintCerts)
	}
	return view
}

func reverseProxyRulePersistenceMap(row *model.ReverseProxyRule) map[string]interface{} {
	if row == nil {
		return map[string]interface{}{}
	}
	return map[string]interface{}{
		"display_id":                   row.DisplayID,
		"list_order":                   row.ListOrder,
		"name":                         row.Name,
		"enabled":                      row.Enabled,
		"listen_protocol":              row.ListenProtocol,
		"listen_protocol_alias":        row.ListenProtocolAlias,
		"listen_ip":                    row.ListenIP,
		"listen_ip_list":               row.ListenIPList,
		"listen_port":                  row.ListenPort,
		"host_list":                    row.HostList,
		"path_prefix":                  row.PathPrefix,
		"listen_dns_path":              row.ListenDNSPath,
		"target_protocol":              row.TargetProtocol,
		"target_protocol_alias":        row.TargetProtocolAlias,
		"target_addresses":             row.TargetAddresses,
		"target_port":                  row.TargetPort,
		"target_path":                  row.TargetPath,
		"target_dns_path":              row.TargetDNSPath,
		"edns_enabled":                 row.EDNSEnabled,
		"edns_mode":                    row.EDNSMode,
		"edns_custom_ip":               row.EDNSCustomIP,
		"edns_client_subnet_policy":    row.EDNSClientSubnetPolicy,
		"disable_ipv4_answer":          row.DisableIPv4Answer,
		"disable_ipv6_answer":          row.DisableIPv6Answer,
		"certificate_record_list":      row.CertificateRecordList,
		"certificate_record_id":        row.CertificateRecordID,
		"listen_http_version_strategy": row.ListenHTTPVersionStrategy,
		"ip_strategy":                  row.IPStrategy,
		"http_version_strategy":        row.HTTPVersionStrategy,
		"upstream_tls_verify":          row.UpstreamTLSVerify,
		"api_passthrough":              row.ApiPassthrough,
		"remark":                       row.Remark,
		"last_error":                   row.LastError,
		"runtime_status":               row.RuntimeStatus,
	}
}

func (s *ReverseProxyService) normalizeRulePayload(payload ReverseProxyRulePayload) (reverseProxyNormalizedRule, error) {
	listenIPInput := strings.TrimSpace(payload.ListenIPs)
	if listenIPInput == "" {
		listenIPInput = strings.TrimSpace(payload.ListenIP)
	}
	legacyListenNames, err := normalizeReverseProxyLegacyListenNames(listenIPInput)
	if err != nil {
		return reverseProxyNormalizedRule{}, err
	}
	listenNameInput := strings.TrimSpace(payload.Hosts)
	if listenNameInput == "" && len(legacyListenNames) > 0 {
		listenNameInput = strings.Join(legacyListenNames, ", ")
	}
	listenProtocolAliasInput := strings.ToLower(strings.TrimSpace(payload.ListenProtocolAlias))
	targetProtocolAliasInput := strings.ToLower(strings.TrimSpace(payload.TargetProtocolAlias))
	normalized := reverseProxyNormalizedRule{
		id:                payload.ID,
		name:              strings.TrimSpace(payload.Name),
		enabled:           payload.Enabled,
		listenPort:        payload.ListenPort,
		targetPort:        payload.TargetPort,
		upstreamTLSVerify: payload.UpstreamTLSVerify,
		apiPassthrough:    payload.ApiPassthrough,
		remark:            strings.TrimSpace(payload.Remark),
		listenIPs:         extractReverseProxyLegacyListenIPs(listenIPInput),
		ednsEnabled:       payload.EDNSEnabled,
		disableIPv4Answer: payload.DisableIPv4Answer,
		disableIPv6Answer: payload.DisableIPv6Answer,
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
		normalized.hosts = []string{}
		normalized.pathPrefix = ""
		normalized.targetPath = ""
		normalized.listenHTTPVersionStrategy = ""
		normalized.httpVersionStrategy = ""
		normalized.apiPassthrough = true
		normalized.upstreamTLSVerify = payload.UpstreamTLSVerify
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

	listenHTTPVersionInput := payload.ListenHTTPVersionStrategy
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
	return normalized, nil
}

func (s *ReverseProxyService) validateNormalizedRule(db *gorm.DB, row reverseProxyNormalizedRule) error {
	if db == nil {
		return nil
	}
	if err := validateReverseProxyNoObviousLoop(row); err != nil {
		return err
	}
	if reverseProxyProtocolIsDNS(row.listenProtocolAlias) {
		return s.validateNormalizedDNSRule(db, row)
	}
	if row.listenProtocol == reverseProxyProtocolHTTP && (row.certificateRecordID != 0 || len(row.certificateRecordIDs) > 0) {
		return common.NewError("http listener cannot bind certificate")
	}
	if row.listenProtocol == reverseProxyProtocolHTTPS {
		certIDs := append([]uint(nil), row.certificateRecordIDs...)
		if len(certIDs) == 0 && row.certificateRecordID > 0 {
			certIDs = []uint{row.certificateRecordID}
		}
		if len(certIDs) == 0 {
			return common.NewError("https listener requires certificate")
		}
		for _, certID := range certIDs {
			// Reuse the caller's DB handle; SQLite runs with a single pooled connection.
			cert, err := loadReverseProxyCertificateRecord(db, certID)
			if err != nil {
				if database.IsNotFound(err) {
					return common.NewError("certificate not found")
				}
				return err
			}
			if cert == nil || len(cert.FullchainPEM) == 0 || len(cert.KeyPEM) == 0 {
				return common.NewError("certificate material is incomplete")
			}
		}
	}

	rows := make([]model.ReverseProxyRule, 0)
	if err := db.Where("id <> ?", row.id).Find(&rows).Error; err != nil {
		return err
	}
	for _, existing := range rows {
		if existing.ListenPort != row.listenPort {
			continue
		}
		existingListenAlias := normalizeReverseProxyProtocolAlias(existing.ListenProtocolAlias, existing.ListenProtocol)
		if reverseProxyProtocolIsDNS(existingListenAlias) {
			if reverseProxyProtocolsShareUnderlyingSocket(existing.ListenProtocol, existing.ListenHTTPVersionStrategy, row.listenProtocol, row.listenHTTPVersionStrategy, existingListenAlias, row.listenProtocolAlias) {
				return common.NewError("reverse proxy listener conflicts with existing dns listener on the same port")
			}
			continue
		}
		if existing.ListenProtocol != row.listenProtocol {
			if reverseProxyProtocolsShareUnderlyingSocket(existing.ListenProtocol, existing.ListenHTTPVersionStrategy, row.listenProtocol, row.listenHTTPVersionStrategy, existingListenAlias, row.listenProtocolAlias) {
				return common.NewError("reverse proxy listener conflicts with existing reverse proxy listener on the same port")
			}
			continue
		}
		if row.listenProtocol == reverseProxyProtocolHTTPS {
			existingListenStrategy, strategyErr := normalizeReverseProxyListenHTTPVersionStrategy(existing.ListenHTTPVersionStrategy, existing.ListenProtocol)
			if strategyErr != nil {
				existingListenStrategy = reverseProxyListenHTTPVersionH2H3
			}
			if existingListenStrategy != row.listenHTTPVersionStrategy {
				return common.NewError("reverse proxy rules on the same https listener must use the same local http version strategy")
			}
		}
		existingNames := reverseProxyRuleServerNames(&existing)
		newNames := reverseProxyNormalizedServerNames(row)
		if row.listenProtocol == reverseProxyProtocolHTTPS && reverseProxyRuleNameSetsAreSNIDisjoint(existingNames, newNames) {
			continue
		}
		if !reverseProxyRuleNamesOverlap(existingNames, newNames) {
			continue
		}
		if reverseProxyRulePathsOverlap(existing.PathPrefix, row.pathPrefix) {
			return common.NewError("reverse proxy rule conflicts with existing host/path on the same listener")
		}
	}
	return nil
}

func (s *ReverseProxyService) validateNormalizedDNSRule(db *gorm.DB, row reverseProxyNormalizedRule) error {
	if !reverseProxyProtocolIsDNS(row.targetProtocolAlias) {
		return common.NewError("dns reverse proxy target protocol is invalid")
	}
	if reverseProxyDNSProtocolUsesTLS(row.listenProtocolAlias) {
		certIDs := append([]uint(nil), row.certificateRecordIDs...)
		if len(certIDs) == 0 && row.certificateRecordID > 0 {
			certIDs = []uint{row.certificateRecordID}
		}
		if len(certIDs) == 0 {
			return common.NewError("dns tls listener requires certificate")
		}
		for _, certID := range certIDs {
			cert, err := loadReverseProxyCertificateRecord(db, certID)
			if err != nil {
				if database.IsNotFound(err) {
					return common.NewError("certificate not found")
				}
				return err
			}
			if cert == nil || len(cert.FullchainPEM) == 0 || len(cert.KeyPEM) == 0 {
				return common.NewError("certificate material is incomplete")
			}
		}
	} else if row.certificateRecordID != 0 || len(row.certificateRecordIDs) > 0 {
		return common.NewError("plain dns listener cannot bind certificate")
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
	if err := db.Where("id <> ?", row.id).Find(&rows).Error; err != nil {
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
			if !reverseProxyListenIPSetsOverlap(decodeReverseProxyListenIPs(&existing), row.listenIPs) {
				continue
			}
			if reverseProxyDNSListenersCanSharePathSocket(&existing, row, existingListenAlias) {
				continue
			}
			return common.NewError("dns reverse proxy listener conflicts with existing dns listener on the same port")
		}
		if reverseProxyProtocolsShareUnderlyingSocket(existing.ListenProtocol, existing.ListenHTTPVersionStrategy, row.listenProtocol, row.listenHTTPVersionStrategy, existingListenAlias, row.listenProtocolAlias) {
			return common.NewError("dns reverse proxy listener conflicts with existing reverse proxy listener on the same port")
		}
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
	if !reverseProxyListenIPSetsEqual(decodeReverseProxyListenIPs(existing), row.listenIPs) {
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
	if row.listenPort <= 0 || row.targetPort <= 0 || row.listenPort != row.targetPort {
		return nil
	}
	if row.listenProtocol != row.targetProtocol {
		return nil
	}
	if len(row.listenIPs) == 0 || len(row.targetAddresses) == 0 {
		return nil
	}
	listenSet := make(map[string]struct{}, len(row.listenIPs))
	for _, item := range row.listenIPs {
		value := strings.ToLower(strings.TrimSpace(item))
		if value == "" {
			continue
		}
		listenSet[value] = struct{}{}
	}
	for _, target := range row.targetAddresses {
		value := strings.ToLower(strings.TrimSpace(target))
		if value == "" {
			continue
		}
		if _, exists := listenSet[value]; exists {
			return common.NewError("target address must not point back to the same listener ip and port")
		}
	}
	return nil
}

func (s *ReverseProxyService) repairDisplayIDsTx(db *gorm.DB) error {
	rows := make([]model.ReverseProxyRule, 0)
	if err := db.Order("id asc").Find(&rows).Error; err != nil {
		return err
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
	rows := make([]model.ReverseProxyRule, 0)
	if err := db.Where("display_id > 0").Order("display_id asc").Find(&rows).Error; err != nil {
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
	case reverseProxyDNSProtocolDoH, reverseProxyDNSProtocolDoHH3, reverseProxyDNSProtocolDoQ, reverseProxyDNSProtocolUDP:
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

const (
	reverseProxyTokenModeServerName = iota + 1
	reverseProxyTokenModeListenName
	reverseProxyTokenModeHost
	reverseProxyTokenModeTarget
)

func normalizeReverseProxyTokens(raw string, mode int) ([]string, error) {
	fields := splitReverseProxyTokenFields(raw)
	seen := make(map[string]struct{}, len(fields))
	result := make([]string, 0, len(fields))
	for _, field := range fields {
		token := strings.TrimSpace(field)
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

func collectReverseProxyLegacyListenIPs(values []string) []string {
	result := make([]string, 0)
	seen := make(map[string]struct{})
	for _, raw := range values {
		for _, field := range splitReverseProxyTokenFields(raw) {
			token := strings.TrimSpace(strings.Trim(field, "[]"))
			if token == "" {
				continue
			}
			parsedIP := reverseProxyParseIPLiteral(token)
			if parsedIP == nil {
				continue
			}
			canonical := strings.ToLower(parsedIP.String())
			if _, exists := seen[canonical]; exists {
				continue
			}
			seen[canonical] = struct{}{}
			result = append(result, canonical)
		}
	}
	return result
}

func extractReverseProxyLegacyListenIPs(raw string) []string {
	return collectReverseProxyLegacyListenIPs([]string{raw})
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
	values := make([]string, 0)
	values = append(values, decodeReverseProxyLegacyListenNames(row)...)
	values = append(values, decodeReverseProxyList(row.HostList)...)
	return reverseProxyCleanServerNames(values)
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
		return len(a) == 0 || len(b) == 0
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
	return false
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

func decodeReverseProxyListenIPs(row *model.ReverseProxyRule) []string {
	if row == nil {
		return []string{}
	}
	values := make([]string, 0)
	values = append(values, decodeReverseProxyList(row.ListenIPList)...)
	if strings.TrimSpace(row.ListenIP) != "" {
		values = append(values, row.ListenIP)
	}
	return collectReverseProxyLegacyListenIPs(values)
}

func decodeReverseProxyLegacyListenNames(row *model.ReverseProxyRule) []string {
	if row == nil {
		return []string{}
	}
	values := make([]string, 0)
	values = append(values, decodeReverseProxyList(row.ListenIPList)...)
	if strings.TrimSpace(row.ListenIP) != "" {
		values = append(values, row.ListenIP)
	}
	names, _ := collectReverseProxyLegacyListenNames(values, false)
	return names
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

func buildReverseProxyCertificateHints(listenIPs []string, hosts []string, certs []ReverseProxyCertificateOption) []string {
	hints := make([]string, 0)
	if len(certs) == 0 || len(hosts) == 0 {
		return hints
	}
	hasIPSANCert := false
	certDomains := make([]string, 0, len(certs)*3)
	for _, cert := range certs {
		if reverseProxyCertificateOptionHasIPSAN(cert) {
			hasIPSANCert = true
		}
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
			if hasIPSANCert {
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
		if hasIPSANCert {
			continue
		}
		if !reverseProxyCertificateDomainsCoverHost(certDomains, host) {
			hints = append(hints, "证书未覆盖域名: "+host)
		}
	}
	return hints
}

func reverseProxyCertificateOptionHasIPSAN(cert ReverseProxyCertificateOption) bool {
	values := append([]string{cert.MainDomain}, cert.Domains...)
	for _, value := range values {
		if reverseProxyParseIPLiteral(value) != nil {
			return true
		}
	}
	return false
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

func reverseProxyNoSNICertificateMatchesLocalIP(binding *reverseProxyRuleCertificateBinding, localIP string) bool {
	if !reverseProxyCertificateBindingUsable(binding, time.Now()) || !binding.Leaf.HasIPSAN {
		return false
	}
	localIP = reverseProxyNormalizeServerName(localIP)
	if reverseProxyParseIPLiteral(localIP) == nil {
		return false
	}
	return reverseProxyCertificateBindingMatchesServerName(binding, localIP)
}

func reverseProxySplitNoSNICertificateCandidates(bindings []*reverseProxyRuleCertificateBinding, localIP string) ([]*reverseProxyRuleCertificateBinding, []*reverseProxyRuleCertificateBinding) {
	localIP = reverseProxyNormalizeServerName(localIP)
	ipPreferred := make([]*reverseProxyRuleCertificateBinding, 0, len(bindings))
	others := make([]*reverseProxyRuleCertificateBinding, 0, len(bindings))
	now := time.Now()
	localIPIsLiteral := reverseProxyParseIPLiteral(localIP) != nil
	for _, binding := range bindings {
		if !reverseProxyCertificateBindingUsable(binding, now) {
			continue
		}
		if localIPIsLiteral && reverseProxyCertificateBindingHasIPSAN(binding) && reverseProxyLeafMatchesServerName(binding.Leaf.Leaf, localIP) {
			ipPreferred = append(ipPreferred, binding)
			continue
		}
		others = append(others, binding)
	}
	return ipPreferred, others
}

func reverseProxyPickNoSNIBinding(bindings []*reverseProxyRuleCertificateBinding, localIP string) *reverseProxyRuleCertificateBinding {
	ipPreferred, others := reverseProxySplitNoSNICertificateCandidates(bindings, localIP)
	if selected := reverseProxyFallbackCertificateBinding(ipPreferred); selected != nil {
		return selected
	}
	return reverseProxyFallbackCertificateBinding(others)
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
	return r.reconcileLocked(service, minGap)
}

func (r *reverseProxyRuntimeManager) SyncNow(service *ReverseProxyService) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.reconcileLocked(service, 0)
}

func (r *reverseProxyRuntimeManager) Stop() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	firstErr := shutdownReverseProxyListenerGroups(r.groups)
	r.groups = make(map[string]*reverseProxyListenerGroup)
	r.mismatchMu.Lock()
	r.mismatchByIP = make(map[string]*reverseProxyMismatchEntry)
	r.mismatchMu.Unlock()
	r.state.lastRenderKey = ""
	r.state.lastSyncAt = time.Now()
	r.state.warnings = nil
	r.reconcileError = ""
	return firstErr
}

func (r *reverseProxyRuntimeManager) reconcileLocked(service *ReverseProxyService, minGap time.Duration) error {
	if service == nil {
		return nil
	}
	now := time.Now()
	if minGap > 0 && !r.state.lastSyncAt.IsZero() && now.Sub(r.state.lastSyncAt) < minGap {
		return nil
	}
	db := database.GetDB()
	rows, err := service.loadRulesLocked(db)
	if err != nil {
		return err
	}
	renderKey := computeReverseProxyRenderKey(db, rows)
	if renderKey == r.state.lastRenderKey {
		r.state.lastSyncAt = now
		return nil
	}
	grouped := reverseProxyGroupRules(rows)
	nextGroups := make(map[string]*reverseProxyListenerGroup, len(grouped))
	createdGroups := make(map[string]*reverseProxyListenerGroup)
	cleanupCreatedGroups := func() {
		_ = shutdownReverseProxyListenerGroups(createdGroups)
	}
	for key, groupRows := range grouped {
		if existing, ok := r.groups[key]; ok && existing != nil {
			if err := service.refreshListenerGroup(existing, groupRows); err != nil {
				cleanupCreatedGroups()
				return err
			}
			nextGroups[key] = existing
			continue
		}
		group, err := service.newListenerGroup(key, groupRows)
		if err != nil {
			cleanupCreatedGroups()
			return err
		}
		nextGroups[key] = group
		createdGroups[key] = group
	}
	for key, group := range r.groups {
		if _, exists := nextGroups[key]; exists {
			continue
		}
		if group != nil {
			if err := group.shutdown(); err != nil {
				cleanupCreatedGroups()
				return err
			}
		}
	}
	warnings := make([]string, 0)
	for _, group := range nextGroups {
		if group == nil || len(group.warnings) == 0 {
			continue
		}
		warnings = append(warnings, group.warnings...)
	}
	r.groups = nextGroups
	r.state.lastRenderKey = renderKey
	r.state.lastSyncAt = now
	r.state.warnings = warnings
	r.reconcileError = ""
	return nil
}

func reverseProxyGroupRules(rows []model.ReverseProxyRule) map[string][]*model.ReverseProxyRule {
	grouped := make(map[string][]*model.ReverseProxyRule)
	for i := range rows {
		row := &rows[i]
		if !row.Enabled {
			continue
		}
		if reverseProxyProtocolIsDNS(normalizeReverseProxyProtocolAlias(row.ListenProtocolAlias, row.ListenProtocol)) {
			continue
		}
		key := reverseProxyListenerKey(row.ListenProtocol, row.ListenPort)
		grouped[key] = append(grouped[key], row)
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
	httpRows := make([]model.ReverseProxyRule, 0, len(rows))
	for i := range rows {
		listenAlias := normalizeReverseProxyProtocolAlias(rows[i].ListenProtocolAlias, rows[i].ListenProtocol)
		if reverseProxyProtocolIsDNS(listenAlias) {
			continue
		}
		httpRows = append(httpRows, rows[i])
	}
	certState := loadReverseProxyCertificateRenderState(db, httpRows)
	snapshot := make([]reverseProxyRenderRule, 0, len(httpRows))
	for i := range httpRows {
		row := httpRows[i]
		listenProtocol := strings.ToLower(strings.TrimSpace(row.ListenProtocol))
		listenAlias := normalizeReverseProxyProtocolAlias(row.ListenProtocolAlias, row.ListenProtocol)
		targetProtocol := strings.ToLower(strings.TrimSpace(row.TargetProtocol))
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
			ID:                     row.Id,
			ListOrder:              row.ListOrder,
			Enabled:                row.Enabled,
			ListenProtocol:         listenProtocol,
			ListenIPs:              decodeReverseProxyListenIPs(&row),
			ListenPort:             row.ListenPort,
			Hosts:                  reverseProxyRuleServerNames(&row),
			PathPrefix:             normalizeReverseProxyPath(row.PathPrefix, false),
			ListenDNSPath:          normalizeReverseProxyDNSPath(row.ListenDNSPath),
			TargetProtocol:         targetProtocol,
			TargetAddresses:        decodeReverseProxyList(row.TargetAddresses),
			TargetPort:             row.TargetPort,
			TargetPath:             normalizeReverseProxyPath(row.TargetPath, false),
			TargetDNSPath:          normalizeReverseProxyDNSPath(row.TargetDNSPath),
			EDNSEnabled:            false,
			EDNSMode:               "",
			EDNSCustomIP:           "",
			EDNSClientSubnetPolicy: "",
			DisableIPv4Answer:      false,
			DisableIPv6Answer:      false,
			CertificateRecordIDs:   certificateRecordIDs,
			CertificateStates:      certificateStates,
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
			ApiPassthrough: row.ApiPassthrough,
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

func reverseProxyListenerKey(protocol string, port int) string {
	return strings.TrimSpace(protocol) + "|" + strconv.Itoa(port)
}

func (s *ReverseProxyService) newListenerGroup(key string, rules []*model.ReverseProxyRule) (*reverseProxyListenerGroup, error) {
	if len(rules) == 0 {
		return &reverseProxyListenerGroup{
			key:                   key,
			service:               s,
			upstreamByRule:        make(map[uint]*reverseProxyCachedUpstream),
			connectionCounts:      make(map[uint]reverseProxyConnectionCounts),
			localConnIDs:          make(map[net.Conn]string),
			localConnStates:       make(map[string]reverseProxyLocalConnectionState),
			localConnAddrToID:     make(map[string]string),
			localConnAddrByID:     make(map[string]string),
			pendingConnSelections: make(map[string]reverseProxyCertificateSelection),
		}, nil
	}
	first := rules[0]

	group := &reverseProxyListenerGroup{
		key:                   key,
		listenPort:            first.ListenPort,
		protocol:              strings.TrimSpace(first.ListenProtocol),
		rules:                 rules,
		service:               s,
		certBindingsByRule:    make(map[uint][]*reverseProxyRuleCertificateBinding),
		orderedCertBindings:   make([]*reverseProxyRuleCertificateBinding, 0),
		warnings:              make([]string, 0),
		upstreamByRule:        make(map[uint]*reverseProxyCachedUpstream),
		connectionCounts:      make(map[uint]reverseProxyConnectionCounts),
		localConnIDs:          make(map[net.Conn]string),
		localConnStates:       make(map[string]reverseProxyLocalConnectionState),
		localConnAddrToID:     make(map[string]string),
		localConnAddrByID:     make(map[string]string),
		pendingConnSelections: make(map[string]reverseProxyCertificateSelection),
	}

	certBindingsByRule, orderedCertBindings, err := s.loadRuleCertificates(rules)
	if err != nil {
		return nil, err
	}
	group.certBindingsByRule = certBindingsByRule
	group.orderedCertBindings = orderedCertBindings
	group.defaultCert, group.defaultLeaf = reverseProxyPickDefaultCertificate(orderedCertBindings)

	listenHTTPVersionStrategy, err := normalizeReverseProxyListenHTTPVersionStrategy(first.ListenHTTPVersionStrategy, group.protocol)
	if err != nil {
		return nil, err
	}
	group.listenHTTPVersionStrategy = listenHTTPVersionStrategy
	enableTCP := true
	enableUDP := false
	if group.protocol == reverseProxyProtocolHTTPS {
		switch listenHTTPVersionStrategy {
		case reverseProxyListenHTTPVersionH2Only:
			enableTCP = true
			enableUDP = false
		case reverseProxyListenHTTPVersionH3Only:
			enableTCP = false
			enableUDP = true
		default:
			enableTCP = true
			enableUDP = true
		}
	}

	handler := group.newHandler()
	tcpHandler := handler
	if strings.EqualFold(group.protocol, reverseProxyProtocolHTTPS) && enableUDP {
		tcpHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if w != nil {
				w.Header().Set("Alt-Svc", reverseProxyAltSvcValue(group.listenPort))
			}
			handler.ServeHTTP(w, r)
		})
	}
	var firstErr error
	if enableTCP {
		binds := reverseProxyTCPListenBinds(first.ListenPort)
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
				Handler:           tcpHandler,
				ReadHeaderTimeout: reverseProxyReadHeaderTimeout,
				IdleTimeout:       reverseProxyServerIdleTimeout,
				ConnContext: func(ctx context.Context, conn net.Conn) context.Context {
					return group.registerTCPConnectionContext(ctx, conn)
				},
				ConnState: func(conn net.Conn, state http.ConnState) {
					if state == http.StateClosed || state == http.StateHijacked {
						group.releaseLocalConnectionByConn(conn)
					}
				},
			}
			if group.protocol == reverseProxyProtocolHTTPS {
				tlsConfig := &tls.Config{
					GetCertificate: group.getCertificate,
					MinVersion:     tls.VersionTLS12,
					NextProtos:     []string{"h2", "http/1.1"},
				}
				if err := http2.ConfigureServer(server, nil); err != nil {
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
		udpBinds := reverseProxyUDPListenBinds(first.ListenPort)
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
				GetCertificate: group.getCertificate,
				MinVersion:     tls.VersionTLS13,
			}
			h3Server := &http3.Server{
				Handler:   handler,
				TLSConfig: h3TLS,
				Port:      first.ListenPort,
				QUICConfig: &quic.Config{
					KeepAlivePeriod: reverseProxyUpstreamQUICKeepAlivePeriod,
					MaxIdleTimeout:  reverseProxyServerIdleTimeout,
				},
				ConnContext: func(ctx context.Context, conn *quic.Conn) context.Context {
					return group.registerQUICConnectionContext(ctx, conn)
				},
			}
			group.packetConns = append(group.packetConns, packetConn)
			group.h3Servers = append(group.h3Servers, h3Server)
			go func(srv *http3.Server, conn net.PacketConn) {
				if serveErr := srv.Serve(conn); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
					logger.Warning("reverse proxy http3 server serve failed: ", serveErr)
				}
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

func reverseProxyTCPListenBinds(port int) []reverseProxyListenBind {
	return []reverseProxyListenBind{
		{
			network:  "tcp4",
			listenIP: "0.0.0.0",
			address:  net.JoinHostPort("0.0.0.0", strconv.Itoa(port)),
		},
		{
			network:  "tcp6",
			listenIP: "::",
			address:  net.JoinHostPort("::", strconv.Itoa(port)),
			optional: true,
		},
	}
}

func reverseProxyUDPListenBinds(port int) []reverseProxyListenBind {
	return []reverseProxyListenBind{
		{
			network:  "udp4",
			listenIP: "0.0.0.0",
			address:  net.JoinHostPort("0.0.0.0", strconv.Itoa(port)),
		},
		{
			network:  "udp6",
			listenIP: "::",
			address:  net.JoinHostPort("::", strconv.Itoa(port)),
			optional: true,
		},
	}
}

func reverseProxyAltSvcValue(port int) string {
	return fmt.Sprintf(`h3=":%d"; ma=%d`, port, reverseProxyAltSvcMaxAgeSeconds)
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
	group.mu.Lock()
	group.rules = rules
	if len(rules) > 0 {
		group.listenHTTPVersionStrategy = strings.TrimSpace(rules[0].ListenHTTPVersionStrategy)
	}
	group.certBindingsByRule = certBindingsByRule
	group.orderedCertBindings = orderedCertBindings
	group.defaultCert, group.defaultLeaf = reverseProxyPickDefaultCertificate(orderedCertBindings)
	oldUpstreams := group.upstreamByRule
	group.upstreamByRule = make(map[uint]*reverseProxyCachedUpstream)
	group.mu.Unlock()
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
	defer g.mu.Unlock()
	upstream := g.upstreamByRule[ruleID]
	if upstream == nil || upstream.closing || upstream.RoundTripper == nil {
		return nil
	}
	upstream.refs++
	return upstream
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

func (s *ReverseProxyService) loadRuleCertificates(rules []*model.ReverseProxyRule) (map[uint][]*reverseProxyRuleCertificateBinding, []*reverseProxyRuleCertificateBinding, error) {
	certBindingsByRule := make(map[uint][]*reverseProxyRuleCertificateBinding)
	orderedCertBindings := make([]*reverseProxyRuleCertificateBinding, 0)
	for _, rule := range rules {
		if rule == nil || !reverseProxyListenerUsesManagedCertificates(rule.ListenProtocol, normalizeReverseProxyProtocolAlias(rule.ListenProtocolAlias, rule.ListenProtocol)) {
			continue
		}
		for _, certID := range reverseProxyRuleCertificateIDs(rule) {
			record, err := s.CertificateInventoryService.GetRecordByID(certID)
			if err != nil {
				return nil, nil, err
			}
			cert, leaf, err := reverseProxyLoadCertificate(record)
			if err != nil {
				return nil, nil, err
			}
			binding := &reverseProxyRuleCertificateBinding{
				RuleID:              rule.Id,
				CertificateRecordID: certID,
				Certificate:         cert,
				Leaf:                leaf,
			}
			certBindingsByRule[rule.Id] = append(certBindingsByRule[rule.Id], binding)
			orderedCertBindings = append(orderedCertBindings, binding)
		}
	}
	return certBindingsByRule, orderedCertBindings, nil
}

func reverseProxyListenerUsesManagedCertificates(listenProtocol string, listenAlias string) bool {
	normalizedProtocol := strings.ToLower(strings.TrimSpace(listenProtocol))
	normalizedAlias := strings.ToLower(strings.TrimSpace(listenAlias))
	if normalizedProtocol == reverseProxyProtocolHTTPS {
		return true
	}
	return normalizedProtocol == reverseProxyProtocolDNS && reverseProxyDNSProtocolUsesTLS(normalizedAlias)
}

func reverseProxyPickDefaultCertificate(bindings []*reverseProxyRuleCertificateBinding) (*tls.Certificate, *x509LeafState) {
	binding := reverseProxyPickNoSNIBinding(bindings, "")
	if binding != nil {
		return binding.Certificate, binding.Leaf
	}
	return nil, nil
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
			connAddrKey = reverseProxyConnectionAddrKey(hello.Conn)
			localIP = reverseProxyNormalizeLocalIP(hello.Conn.LocalAddr())
		}
	}
	if serverName == "" {
		ipPreferred, others := reverseProxySplitNoSNICertificateCandidates(g.orderedCertBindings, localIP)
		if selected, selection, err := g.selectBalancedCertificate(ipPreferred, reverseProxyCertBalanceNoSNIBucket); err == nil && selected != nil {
			g.bindCertificateSelectionToConnection(connAddrKey, selection)
			return selected.Certificate, nil
		}
		if selected, selection, err := g.selectBalancedCertificate(others, reverseProxyCertBalanceNoSNIBucket); err == nil && selected != nil {
			g.bindCertificateSelectionToConnection(connAddrKey, selection)
			return selected.Certificate, nil
		}
		reverseProxyCloseClientHelloConn(hello)
		return nil, common.NewError("tls listener certificate is unavailable")
	}
	if reverseProxyParseIPLiteral(serverName) != nil {
		ipPreferred, others := reverseProxySplitNoSNICertificateCandidates(g.orderedCertBindings, serverName)
		if selected, selection, err := g.selectBalancedCertificate(ipPreferred, serverName); err == nil && selected != nil {
			g.bindCertificateSelectionToConnection(connAddrKey, selection)
			return selected.Certificate, nil
		}
		if selected, selection, err := g.selectBalancedCertificate(others, serverName); err == nil && selected != nil {
			g.bindCertificateSelectionToConnection(connAddrKey, selection)
			return selected.Certificate, nil
		}
		reverseProxyCloseClientHelloConn(hello)
		return nil, common.NewError("tls listener certificate is unavailable")
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
		g.bindCertificateSelectionToConnection(connAddrKey, selection)
		return selected.Certificate, nil
	}
	if selected, selection, err := g.selectBalancedCertificate(wildcardCandidates, serverName); err == nil && selected != nil {
		g.bindCertificateSelectionToConnection(connAddrKey, selection)
		return selected.Certificate, nil
	}
	if selected, selection, err := g.selectBalancedCertificate(g.orderedCertBindings, serverName); err == nil && selected != nil {
		g.bindCertificateSelectionToConnection(connAddrKey, selection)
		return selected.Certificate, nil
	}
	reverseProxyCloseClientHelloConn(hello)
	if matchedRule {
		return nil, common.NewError("tls listener certificate is unavailable")
	}
	return nil, common.NewError("unrecognized tls sni")
}

func (g *reverseProxyListenerGroup) noSNICertificateCandidatesLocked(localIP string) []*reverseProxyRuleCertificateBinding {
	if g == nil {
		return nil
	}
	bindings := g.orderedCertBindings
	if len(bindings) == 0 {
		bindings = make([]*reverseProxyRuleCertificateBinding, 0)
		for _, rule := range g.rules {
			bindings = append(bindings, g.certBindingsByRule[rule.Id]...)
		}
	}
	ipPreferred, others := reverseProxySplitNoSNICertificateCandidates(bindings, localIP)
	if len(ipPreferred) > 0 {
		return ipPreferred
	}
	return others
}

func reverseProxyCloseClientHelloConn(hello *tls.ClientHelloInfo) {
	if hello == nil || hello.Conn == nil {
		return
	}
	_ = hello.Conn.Close()
}

func (g *reverseProxyListenerGroup) bindCertificateSelectionToConnection(connAddrKey string, selection reverseProxyCertificateSelection) {
	if g == nil || connAddrKey == "" || selection.CertificateRecordID == 0 || strings.TrimSpace(selection.ListenerKey) == "" {
		return
	}
	g.statsMu.Lock()
	if g.localConnStates == nil {
		g.localConnStates = make(map[string]reverseProxyLocalConnectionState)
	}
	if g.localConnAddrToID == nil {
		g.localConnAddrToID = make(map[string]string)
	}
	if g.pendingConnSelections == nil {
		g.pendingConnSelections = make(map[string]reverseProxyCertificateSelection)
	}
	connID := strings.TrimSpace(g.localConnAddrToID[connAddrKey])
	if connID != "" {
		state := g.localConnStates[connID]
		if state.HasSelection {
			prev := state.Selection
			g.statsMu.Unlock()
			g.releaseCertificateSelection(prev)
			g.statsMu.Lock()
			state = g.localConnStates[connID]
		}
		state.Selection = selection
		state.HasSelection = true
		g.localConnStates[connID] = state
		g.statsMu.Unlock()
		return
	}
	g.pendingConnSelections[connAddrKey] = selection
	g.statsMu.Unlock()
}

func (g *reverseProxyListenerGroup) releaseCertificateSelection(selection reverseProxyCertificateSelection) {
	if g == nil || g.service == nil {
		return
	}
	if selection.CertificateRecordID == 0 || strings.TrimSpace(selection.ListenerKey) == "" {
		return
	}
	if err := g.service.releaseCertificateBalanceSelection(selection); err != nil {
		logger.Warning("reverse proxy certificate selection release failed: ", err)
	}
}

func (g *reverseProxyListenerGroup) newHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r == nil {
			http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
			return
		}
		host := reverseProxyNormalizeRequestHost(r.Host)
		path := normalizeReverseProxyPath(r.URL.Path, true)
		sni := ""
		if r.TLS != nil {
			sni = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(r.TLS.ServerName), "."))
		}
		rule, _ := g.findRule(host, sni, path)
		if rule == nil {
			reverseProxyDropRejectedRequest(w)
			return
		}
		if connID := g.connectionIDFromContext(r.Context()); connID != "" {
			g.registerLocalConnectionRule(rule.Id, connID)
		}
		reverseProxyRuntime.clearMismatch(extractRemoteIP(r.RemoteAddr))
		g.forwardRequest(w, r, rule)
	})
}

func (g *reverseProxyListenerGroup) findRule(host string, sni string, path string) (*model.ReverseProxyRule, bool) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	hostMatched := false
	requireSNI := strings.EqualFold(g.protocol, reverseProxyProtocolHTTPS)
	for _, rule := range g.rules {
		matched, partial := reverseProxyRuleRequestNameMatchDetail(rule, host, sni, requireSNI)
		if partial {
			hostMatched = true
		}
		if !matched {
			continue
		}
		if reverseProxyRulePathMatch(rule, path) {
			return rule, true
		}
	}
	return nil, hostMatched
}

func reverseProxyRuleRequestNameMatchDetail(rule *model.ReverseProxyRule, host string, sni string, requireSNI bool) (bool, bool) {
	if rule == nil {
		return false, false
	}
	candidates := reverseProxyRuleServerNames(rule)
	if len(candidates) == 0 {
		return true, true
	}
	host = reverseProxyNormalizeServerName(host)
	sni = reverseProxyNormalizeServerName(sni)
	hostIsIP := reverseProxyParseIPLiteral(host) != nil
	sniIsIP := reverseProxyParseIPLiteral(sni) != nil
	hostMatch := host == "" || hostIsIP || reverseProxyRequestNameMatchesAny(candidates, host)
	sniMatch := sni == "" || sniIsIP || reverseProxyRequestNameMatchesAny(candidates, sni)
	partial := hostMatch || sniMatch
	if requireSNI {
		if sni != "" && !sniIsIP && !sniMatch {
			return false, partial
		}
		if host != "" && !hostIsIP && !hostMatch {
			return false, true
		}
		return true, true
	}
	if host != "" && !hostIsIP && !hostMatch {
		return false, false
	}
	return true, true
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
		return true
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

func reverseProxyDropRejectedRequest(w http.ResponseWriter) {
	controller := http.NewResponseController(w)
	conn, _, err := controller.Hijack()
	if err == nil {
		_ = conn.Close()
		return
	}
	panic(http.ErrAbortHandler)
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
	g.statsMu.Lock()
	if g.localConnIDs == nil {
		g.localConnIDs = make(map[net.Conn]string)
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
	if g.pendingConnSelections == nil {
		g.pendingConnSelections = make(map[string]reverseProxyCertificateSelection)
	}
	connID := g.nextConnectionIDLocked()
	g.localConnIDs[conn] = connID
	addrKey := reverseProxyConnectionAddrKey(conn)
	if addrKey != "" {
		g.localConnAddrToID[addrKey] = connID
		g.localConnAddrByID[connID] = addrKey
		if pending, exists := g.pendingConnSelections[addrKey]; exists {
			state := g.localConnStates[connID]
			state.Selection = pending
			state.HasSelection = true
			g.localConnStates[connID] = state
			delete(g.pendingConnSelections, addrKey)
		}
	}
	g.statsMu.Unlock()
	return context.WithValue(ctx, reverseProxyConnContextKey{}, connID)
}

func (g *reverseProxyListenerGroup) registerQUICConnectionContext(ctx context.Context, conn *quic.Conn) context.Context {
	if g == nil || conn == nil {
		return ctx
	}
	if ctx == nil {
		ctx = context.Background()
	}
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
	if g.pendingConnSelections == nil {
		g.pendingConnSelections = make(map[string]reverseProxyCertificateSelection)
	}
	connID := g.nextConnectionIDLocked()
	addrKey := reverseProxyConnectionAddrKeyFromAddrs(conn.LocalAddr(), conn.RemoteAddr())
	if addrKey != "" {
		g.localConnAddrToID[addrKey] = connID
		g.localConnAddrByID[connID] = addrKey
		if pending, exists := g.pendingConnSelections[addrKey]; exists {
			state := g.localConnStates[connID]
			state.Selection = pending
			state.HasSelection = true
			g.localConnStates[connID] = state
			delete(g.pendingConnSelections, addrKey)
		}
	}
	g.statsMu.Unlock()
	go func(id string, c *quic.Conn) {
		if c == nil {
			return
		}
		<-c.Context().Done()
		g.releaseLocalConnectionByID(id)
	}(connID, conn)
	return context.WithValue(ctx, reverseProxyConnContextKey{}, connID)
}

func (g *reverseProxyListenerGroup) registerLocalConnectionRule(ruleID uint, connID string) {
	if g == nil || ruleID == 0 || connID == "" {
		return
	}
	g.statsMu.Lock()
	if g.connectionCounts == nil {
		g.connectionCounts = make(map[uint]reverseProxyConnectionCounts)
	}
	if g.localConnStates == nil {
		g.localConnStates = make(map[string]reverseProxyLocalConnectionState)
	}
	state := g.localConnStates[connID]
	if state.RuleID != 0 {
		if state.RuleID != ruleID {
			g.statsMu.Unlock()
			return
		}
		g.statsMu.Unlock()
		return
	}
	state.RuleID = ruleID
	g.localConnStates[connID] = state
	counts := g.connectionCounts[ruleID]
	counts.LocalOpen++
	g.connectionCounts[ruleID] = counts
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
	if connID != "" && g.localConnAddrByID != nil {
		addrKey = g.localConnAddrByID[connID]
		delete(g.localConnAddrByID, connID)
	}
	if addrKey != "" && g.localConnAddrToID != nil {
		delete(g.localConnAddrToID, addrKey)
	}
	g.statsMu.Unlock()
	if connID == "" {
		return
	}
	g.releaseLocalConnectionByID(connID)
}

func (g *reverseProxyListenerGroup) releaseLocalConnectionByID(connID string) {
	if g == nil || connID == "" {
		return
	}
	var selection reverseProxyCertificateSelection
	hasSelection := false
	g.statsMu.Lock()
	state := g.localConnStates[connID]
	delete(g.localConnStates, connID)
	if g.localConnAddrByID != nil {
		if addrKey := g.localConnAddrByID[connID]; addrKey != "" {
			delete(g.localConnAddrByID, connID)
			if g.localConnAddrToID != nil {
				delete(g.localConnAddrToID, addrKey)
			}
		}
	}
	ruleID := state.RuleID
	if state.HasSelection {
		selection = state.Selection
		hasSelection = true
	}
	if ruleID != 0 {
		counts := g.connectionCounts[ruleID]
		if counts.LocalOpen > 0 {
			counts.LocalOpen--
		}
		if counts.LocalOpen == 0 && counts.UpstreamOpen == 0 {
			delete(g.connectionCounts, ruleID)
		} else {
			g.connectionCounts[ruleID] = counts
		}
	}
	g.statsMu.Unlock()
	if hasSelection {
		g.releaseCertificateSelection(selection)
	}
}

func (g *reverseProxyListenerGroup) incrementUpstreamConnection(ruleID uint) {
	if g == nil || ruleID == 0 {
		return
	}
	g.statsMu.Lock()
	if g.connectionCounts == nil {
		g.connectionCounts = make(map[uint]reverseProxyConnectionCounts)
	}
	counts := g.connectionCounts[ruleID]
	counts.UpstreamOpen++
	g.connectionCounts[ruleID] = counts
	g.statsMu.Unlock()
}

func (g *reverseProxyListenerGroup) decrementUpstreamConnection(ruleID uint) {
	if g == nil || ruleID == 0 {
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
	if strings.TrimSpace(value) == "" || strings.TrimSpace(upstreamDomain) == "" {
		return value
	}
	parts := strings.Split(value, ";")
	filtered := make([]string, 0, len(parts))
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
				if externalDomain == "" {
					continue
				}
				trimmed = "Domain=" + externalDomain
			}
		}
		filtered = append(filtered, trimmed)
	}
	return strings.Join(filtered, "; ")
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
				next = append(next, reverseProxyRewriteSetCookieHeader(value, plan.UpstreamCookieDomain, plan.ExternalCookieDomain))
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

func reverseProxyRewriteResponseBody(resp *http.Response, plan reverseProxyResponseRewritePlan) error {
	if !plan.Enabled || resp == nil || resp.Body == nil {
		return nil
	}
	if !reverseProxyResponseMayContainOriginReferences(resp.Header.Get("Content-Type")) {
		return nil
	}

	body, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		return err
	}
	rewritten := body
	for _, item := range plan.Replacements {
		if item.Old == "" || item.Old == item.New {
			continue
		}
		rewritten = bytes.ReplaceAll(rewritten, []byte(item.Old), []byte(item.New))
	}
	rewritten = reverseProxyRewriteRelativeBodyPaths(rewritten, plan.ExternalPathPrefix)
	resp.Body = io.NopCloser(bytes.NewReader(rewritten))
	resp.ContentLength = int64(len(rewritten))
	resp.TransferEncoding = nil
	resp.Header.Set("Content-Length", strconv.Itoa(len(rewritten)))
	resp.Header.Del("ETag")
	resp.Header.Del("Content-MD5")
	return nil
}

func (g *reverseProxyListenerGroup) forwardRequest(w http.ResponseWriter, r *http.Request, rule *model.ReverseProxyRule) {
	targetURL, transportBundle, err := g.buildUpstream(rule, r.Context())
	if err != nil {
		_ = database.GetDB().Model(&model.ReverseProxyRule{}).Where("id = ?", rule.Id).Updates(map[string]interface{}{
			"last_error":     strings.TrimSpace(err.Error()),
			"runtime_status": "upstream_error",
		}).Error
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
			_ = database.GetDB().Model(&model.ReverseProxyRule{}).Where("id = ?", rule.Id).Updates(map[string]interface{}{
				"last_error":     strings.TrimSpace(proxyErr.Error()),
				"runtime_status": "proxy_error",
			}).Error
			reverseProxyWriteGatewayError(writer, proxyErr)
		}
	} else {
		proxy.ErrorHandler = func(writer http.ResponseWriter, request *http.Request, proxyErr error) {
			g.invalidateCachedUpstream(rule.Id)
			_ = database.GetDB().Model(&model.ReverseProxyRule{}).Where("id = ?", rule.Id).Updates(map[string]interface{}{
				"last_error":     strings.TrimSpace(proxyErr.Error()),
				"runtime_status": "proxy_error",
			}).Error
			reverseProxyWriteGatewayError(writer, proxyErr)
		}
	}
	proxy.ModifyResponse = func(resp *http.Response) error {
		reverseProxyRewriteResponseHeaders(resp.Header, rewritePlan)
		if !bodyRewriteEnabled {
			return nil
		}
		if err := reverseProxyRewriteResponseBody(resp, rewritePlan); err != nil {
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
		if bodyRewriteEnabled {
			req.Header.Set("Accept-Encoding", "identity")
		}
		req.Header.Set("X-Forwarded-Host", r.Host)
		req.Header.Set("X-Forwarded-Proto", strings.TrimSpace(rule.ListenProtocol))
		req.Header.Set("X-Forwarded-For", appendForwardedFor(r.Header.Get("X-Forwarded-For"), extractRemoteIP(r.RemoteAddr)))
	}
	_ = database.GetDB().Model(&model.ReverseProxyRule{}).Where("id = ?", rule.Id).Updates(map[string]interface{}{
		"last_error":     "",
		"runtime_status": "running",
	}).Error
	proxy.ServeHTTP(w, r)
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

func reverseProxyRewriteQuotedRelativeBodyPaths(body []byte, prefix string) []byte {
	prefix = reverseProxyNormalizePathPrefix(prefix)
	if prefix == "" || len(body) == 0 {
		return body
	}
	out := make([]byte, 0, len(body)+len(prefix)*4)
	for i := 0; i < len(body); i++ {
		ch := body[i]
		out = append(out, ch)
		if ch != '"' && ch != '\'' {
			continue
		}
		if i+1 < len(body) && body[i+1] == '/' {
			if i+2 < len(body) && body[i+2] == '/' {
				continue
			}
			if reverseProxyBodyPathHasPrefix(body, i+1, prefix) {
				continue
			}
			out = append(out, prefix...)
			continue
		}
		if i+2 < len(body) && body[i+1] == '\\' && body[i+2] == '/' {
			if i+4 < len(body) && body[i+3] == '\\' && body[i+4] == '/' {
				continue
			}
			if reverseProxyBodyPathHasPrefix(body, i+2, prefix) {
				continue
			}
			out = append(out, prefix...)
		}
	}
	return out
}

func reverseProxyRewriteCSSRelativeBodyPaths(body []byte, prefix string) []byte {
	prefix = reverseProxyNormalizePathPrefix(prefix)
	if prefix == "" || len(body) == 0 {
		return body
	}
	protected := bytes.ReplaceAll(body, []byte("url(//"), []byte("@@rp_url_scheme_1@@"))
	protected = bytes.ReplaceAll(protected, []byte("url(/"), []byte("url("+prefix+"/"))
	protected = bytes.ReplaceAll(protected, []byte("@@rp_url_scheme_1@@"), []byte("url(//"))
	return protected
}

func reverseProxyRewriteRelativeBodyPaths(body []byte, prefix string) []byte {
	prefix = reverseProxyNormalizePathPrefix(prefix)
	if prefix == "" || len(body) == 0 {
		return body
	}
	rewritten := reverseProxyRewriteQuotedRelativeBodyPaths(body, prefix)
	rewritten = reverseProxyRewriteCSSRelativeBodyPaths(rewritten, prefix)
	return rewritten
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

	resolved, serverName, hostHeader, transportMode, err := g.service.pickUpstreamTarget(ctx, strings.TrimSpace(rule.TargetProtocol), targets, rule.TargetPort, rule.IPStrategy, rule.HTTPVersionStrategy, rule.UpstreamTLSVerify)
	if err != nil {
		return nil, reverseProxyTransportBundle{}, err
	}
	transportBundle, err := g.service.buildRoundTripper(g, rule.Id, strings.TrimSpace(rule.TargetProtocol), resolved, rule.TargetPort, serverName, rule.UpstreamTLSVerify, transportMode)
	if err != nil {
		return nil, reverseProxyTransportBundle{}, err
	}
	cached := &reverseProxyCachedUpstream{
		ResolvedAddress: resolved,
		ServerName:      serverName,
		HostHeader:      hostHeader,
		TransportMode:   transportMode,
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
	trimmedRawPath := reverseProxyTrimMatchedRawPathPrefix(normalizedRawPath, normalizedPrefix, trimmedPath)
	return trimmedPath, trimmedRawPath
}

func reverseProxyTrimMatchedRawPathPrefix(rawPath string, decodedPrefix string, decodedTrimmedPath string) string {
	fallback := (&url.URL{Path: decodedTrimmedPath}).EscapedPath()
	rawPath = strings.TrimSpace(rawPath)
	if rawPath == "" {
		return fallback
	}
	escapedPrefix := (&url.URL{Path: decodedPrefix}).EscapedPath()
	if escapedPrefix == "" || !strings.HasPrefix(rawPath, escapedPrefix) {
		return fallback
	}
	trimmed := strings.TrimPrefix(rawPath, escapedPrefix)
	if trimmed == "" {
		trimmed = "/"
	} else if !strings.HasPrefix(trimmed, "/") {
		trimmed = "/" + trimmed
	}
	unescaped, err := url.PathUnescape(trimmed)
	if err != nil {
		return fallback
	}
	if normalizeReverseProxyPath(unescaped, true) != decodedTrimmedPath {
		return fallback
	}
	return trimmed
}

func (s *ReverseProxyService) pickUpstreamTarget(ctx context.Context, protocol string, targets []string, port int, ipStrategy string, httpVersionStrategy string, strictVerify bool) (string, string, string, string, error) {
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

func (s *ReverseProxyService) buildRoundTripper(group *reverseProxyListenerGroup, ruleID uint, protocol string, address string, port int, serverName string, strictVerify bool, transportMode string) (reverseProxyTransportBundle, error) {
	if protocol == reverseProxyProtocolHTTP {
		dialer := &net.Dialer{
			Timeout:   12 * time.Second,
			KeepAlive: reverseProxyUpstreamTCPKeepAlive,
		}
		transport := &http.Transport{
			DialContext: reverseProxyFixedAddressDialContextWithTracking(dialer, address, port, func() {
				if group != nil {
					group.incrementUpstreamConnection(ruleID)
				}
			}, func() {
				if group != nil {
					group.decrementUpstreamConnection(ruleID)
				}
			}),
			DisableCompression:    false,
			DisableKeepAlives:     false,
			ResponseHeaderTimeout: 30 * time.Second,
			IdleConnTimeout:       reverseProxyUpstreamIdleTimeout,
		}
		return reverseProxyTransportBundle{
			RoundTripper: transport,
			Cleanup:      transport.CloseIdleConnections,
		}, nil
	}

	switch transportMode {
	case reverseProxyUpstreamModeHTTPSH3:
		tlsConfig := buildReverseProxyUpstreamTLSConfig(serverName, strictVerify, []string{"h3"})
		transport := buildHTTP3RoundTripper(address, port, tlsConfig, func() {
			if group != nil {
				group.incrementUpstreamConnection(ruleID)
			}
		}, func() {
			if group != nil {
				group.decrementUpstreamConnection(ruleID)
			}
		})
		return reverseProxyTransportBundle{
			RoundTripper: transport,
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
			DialContext: reverseProxyFixedAddressDialContextWithTracking(dialer, address, port, func() {
				if group != nil {
					group.incrementUpstreamConnection(ruleID)
				}
			}, func() {
				if group != nil {
					group.decrementUpstreamConnection(ruleID)
				}
			}),
			DisableCompression:    false,
			DisableKeepAlives:     false,
			ForceAttemptHTTP2:     true,
			ResponseHeaderTimeout: 30 * time.Second,
			IdleConnTimeout:       reverseProxyUpstreamIdleTimeout,
			TLSHandshakeTimeout:   12 * time.Second,
			TLSClientConfig:       tlsConfig,
		}
		if h2Transport, err := http2.ConfigureTransports(transport); err == nil && h2Transport != nil {
			h2Transport.ReadIdleTimeout = reverseProxyUpstreamHTTP2ReadIdleTimeout
			h2Transport.PingTimeout = reverseProxyUpstreamHTTP2PingTimeout
		}
		return reverseProxyTransportBundle{
			RoundTripper: transport,
			Cleanup:      transport.CloseIdleConnections,
		}, nil
	default:
		return reverseProxyTransportBundle{}, common.NewError("invalid upstream transport mode")
	}
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

func buildHTTP3RoundTripper(address string, port int, tlsConfig *tls.Config, onOpen func(), onClose func()) *http3.Transport {
	return &http3.Transport{
		TLSClientConfig: cloneTLSConfig(tlsConfig, tlsConfig),
		QUICConfig: &quic.Config{
			KeepAlivePeriod: reverseProxyUpstreamQUICKeepAlivePeriod,
			MaxIdleTimeout:  reverseProxyUpstreamIdleTimeout,
		},
		Dial: func(ctx context.Context, _ string, cfg *tls.Config, quicCfg *quic.Config) (*quic.Conn, error) {
			conn, err := quic.DialAddr(ctx, net.JoinHostPort(address, strconv.Itoa(port)), cloneTLSConfig(cfg, tlsConfig), quicCfg)
			if err != nil {
				return nil, err
			}
			if onOpen != nil {
				onOpen()
			}
			if onClose != nil {
				go func(c *quic.Conn) {
					if c == nil {
						return
					}
					<-c.Context().Done()
					onClose()
				}(conn)
			}
			return conn, nil
		},
	}
}

func reverseProxyFixedAddressDialContext(dialer *net.Dialer, address string, port int) func(context.Context, string, string) (net.Conn, error) {
	return func(ctx context.Context, networkName string, _ string) (net.Conn, error) {
		return dialer.DialContext(ctx, networkName, net.JoinHostPort(address, strconv.Itoa(port)))
	}
}

func reverseProxyFixedAddressDialContextWithTracking(dialer *net.Dialer, address string, port int, onOpen func(), onClose func()) func(context.Context, string, string) (net.Conn, error) {
	return func(ctx context.Context, networkName string, _ string) (net.Conn, error) {
		conn, err := dialer.DialContext(ctx, networkName, net.JoinHostPort(address, strconv.Itoa(port)))
		if err != nil {
			return nil, err
		}
		if onOpen != nil {
			onOpen()
		}
		return &reverseProxyCountedConn{
			Conn: conn,
			onClose: func() {
				if onClose != nil {
					onClose()
				}
			},
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

func (g *reverseProxyListenerGroup) shutdown() error {
	if g == nil {
		return nil
	}
	var firstErr error
	selections := make([]reverseProxyCertificateSelection, 0)
	g.statsMu.Lock()
	for connID, state := range g.localConnStates {
		if state.HasSelection {
			selections = append(selections, state.Selection)
		}
		delete(g.localConnStates, connID)
	}
	for addrKey, selection := range g.pendingConnSelections {
		selections = append(selections, selection)
		delete(g.pendingConnSelections, addrKey)
	}
	g.localConnIDs = make(map[net.Conn]string)
	g.localConnAddrToID = make(map[string]string)
	g.localConnAddrByID = make(map[string]string)
	g.statsMu.Unlock()
	for _, selection := range selections {
		g.releaseCertificateSelection(selection)
	}
	g.mu.Lock()
	oldUpstreams := g.upstreamByRule
	g.upstreamByRule = make(map[uint]*reverseProxyCachedUpstream)
	g.mu.Unlock()
	for _, upstream := range oldUpstreams {
		g.disposeCachedUpstream(upstream)
	}
	if len(g.h3Servers) > 0 {
		for _, server := range g.h3Servers {
			if server == nil {
				continue
			}
			ctx, cancel := context.WithTimeout(context.Background(), reverseProxyShutdownTimeout)
			err := server.Shutdown(ctx)
			cancel()
			if err != nil && !errors.Is(err, http.ErrServerClosed) && firstErr == nil {
				firstErr = err
			}
		}
	} else if g.h3Server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), reverseProxyShutdownTimeout)
		err := g.h3Server.Shutdown(ctx)
		cancel()
		if err != nil && !errors.Is(err, http.ErrServerClosed) && firstErr == nil {
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
	r.mismatchMu.Lock()
	defer r.mismatchMu.Unlock()
	now := time.Now()
	entry, ok := r.mismatchByIP[ip]
	if !ok || now.Sub(entry.LastAttempt) >= reverseProxyMismatchCooldown {
		entry = &reverseProxyMismatchEntry{}
		r.mismatchByIP[ip] = entry
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
	r.mismatchMu.Lock()
	delete(r.mismatchByIP, ip)
	r.mismatchMu.Unlock()
}
