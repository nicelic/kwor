package sub

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/alireza0/s-ui/config"
	"github.com/alireza0/s-ui/logger"
	"github.com/alireza0/s-ui/middleware"
	"github.com/alireza0/s-ui/network"
	"github.com/alireza0/s-ui/service"

	"github.com/gin-gonic/gin"
)

type Server struct {
	httpServer *http.Server
	listener   net.Listener
	ctx        context.Context
	cancel     context.CancelFunc

	service.SettingService
	balanceService service.PanelCertificateBalanceService
	tlsMu          sync.RWMutex
	tlsMaterials   []*tlsRuntimeCertificate
	tlsDefaultFP   string
	tlsListenerKey string
	tlsGeneration  atomic.Uint64
	connMu         sync.Mutex
	tlsConns       map[*network.ManagedTLSConn]struct{}
	tlsSelections  map[*network.ManagedTLSConn]service.PanelCertificateBalanceSelection
}

type tlsRuntimeCertificate struct {
	cert         tls.Certificate
	leaf         *x509.Certificate
	certRecordID uint
	fingerprint  string
	notAfter     time.Time
}

func NewServer() *Server {
	ctx, cancel := context.WithCancel(context.Background())
	return &Server{
		ctx:           ctx,
		cancel:        cancel,
		tlsConns:      make(map[*network.ManagedTLSConn]struct{}),
		tlsSelections: make(map[*network.ManagedTLSConn]service.PanelCertificateBalanceSelection),
	}
}

func (s *Server) initRouter() (*gin.Engine, error) {
	if config.IsDebug() {
		gin.SetMode(gin.DebugMode)
	} else {
		gin.DefaultWriter = io.Discard
		gin.DefaultErrorWriter = io.Discard
		gin.SetMode(gin.ReleaseMode)
	}

	engine := gin.Default()

	subPath, err := s.SettingService.GetSubPath()
	if err != nil {
		return nil, err
	}

	subDomain, err := s.SettingService.GetSubDomain()
	if err != nil {
		return nil, err
	}

	if subDomain != "" {
		engine.Use(middleware.DomainValidator(subDomain))
	}

	g := engine.Group(subPath)
	NewSubHandler(g)

	// Anti-probe: wrong path returns delayed fake 504
	engine.NoRoute(func(c *gin.Context) {
		if middleware.IsLocalWhitelistHost(c.Request.Host) {
			c.Status(http.StatusNotFound)
			return
		}
		fake504Handler(c)
	})

	return engine, nil
}

func (s *Server) Start() (err error) {
	//This is an anonymous function, no function name
	defer func() {
		if err != nil {
			s.Stop()
		}
	}()

	engine, err := s.initRouter()
	if err != nil {
		return err
	}

	listen, err := s.SettingService.GetSubListen()
	if err != nil {
		return err
	}
	port, err := s.SettingService.GetSubPort()
	if err != nil {
		return err
	}

	listenAddr := net.JoinHostPort(listen, strconv.Itoa(port))
	listener, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return err
	}
	s.setTLSListenerKey(service.PanelCertificateBalanceListenerKey(service.PanelSelfSignedTargetSub, port))

	materials, _, materialErr := service.EnsurePanelTLSMaterials(&s.SettingService, service.PanelSelfSignedTargetSub, time.Now())
	if len(materials) > 0 {
		certs, loadErr := s.loadTLSCertificateMaterials(materials)
		if loadErr != nil || len(certs) == 0 {
			s.clearTLSState()
			if loadErr != nil {
				logger.Warning("failed to load Sub TLS certificate set, falling back to HTTP:", loadErr)
			}
			listener = network.NewAutoHttpListener(listener)
			logger.Info("Sub server run http on", listener.Addr())
		} else {
			s.setTLSState(certs)
			c := &tls.Config{
				GetCertificate: s.getTLSCertificate,
			}
			listener = network.NewManagedTLSListener(listener)
			listener = network.NewAutoHttpsListener(listener)
			listener = tls.NewListener(listener, c)
			logger.Info("Sub server run https on", listener.Addr())
		}
	} else {
		s.clearTLSState()
		if materialErr != nil {
			logger.Warning("failed to resolve Sub TLS material, falling back to HTTP:", materialErr)
		}
		listener = network.NewAutoHttpListener(listener)
		logger.Info("Sub server run http on", listener.Addr())
	}
	s.listener = listener

	s.httpServer = &http.Server{
		Handler:   engine,
		ConnState: s.trackTLSConn,
	}

	go func() {
		s.httpServer.Serve(listener)
	}()

	return nil
}

func (s *Server) Stop() error {
	// Use a timeout context for graceful shutdown instead of the cancelled one
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	var err error
	if s.httpServer != nil {
		err = s.httpServer.Shutdown(shutdownCtx)
		if err != nil {
			logger.Warning("sub server shutdown error:", err)
		}
	}
	if s.listener != nil {
		// listener is already closed by httpServer.Shutdown, ignore error
		s.listener.Close()
	}
	s.releaseAllTLSSelections()
	s.cancel()
	return nil
}

func (s *Server) Restart() error {
	_ = s.Stop()
	s.ctx, s.cancel = context.WithCancel(context.Background())
	return s.Start()
}

func (s *Server) CurrentPort() int {
	if s.listener == nil || s.listener.Addr() == nil {
		return 0
	}
	addr, ok := s.listener.Addr().(*net.TCPAddr)
	if ok && addr != nil {
		return addr.Port
	}
	_, port, err := net.SplitHostPort(s.listener.Addr().String())
	if err != nil {
		return 0
	}
	parsed, err := strconv.Atoi(port)
	if err != nil {
		return 0
	}
	return parsed
}

func (s *Server) GetCtx() context.Context {
	return s.ctx
}

func (s *Server) setTLSState(materials []*tlsRuntimeCertificate) string {
	defaultFP := ""
	if material := firstUsableTLSRuntimeCertificate(materials, time.Now()); material != nil {
		defaultFP = strings.TrimSpace(material.fingerprint)
	}
	s.tlsMu.Lock()
	s.tlsMaterials = append([]*tlsRuntimeCertificate(nil), materials...)
	s.tlsDefaultFP = defaultFP
	s.tlsGeneration.Add(1)
	s.tlsMu.Unlock()
	return defaultFP
}

func (s *Server) setTLSListenerKey(key string) {
	s.tlsMu.Lock()
	s.tlsListenerKey = strings.TrimSpace(key)
	s.tlsMu.Unlock()
}

func (s *Server) clearTLSState() {
	s.tlsMu.Lock()
	s.tlsMaterials = nil
	s.tlsDefaultFP = ""
	s.tlsListenerKey = ""
	s.tlsMu.Unlock()
}

func (s *Server) trackTLSConn(conn net.Conn, state http.ConnState) {
	managedConn, ok := conn.(*network.ManagedTLSConn)
	if !ok || managedConn == nil {
		return
	}
	switch state {
	case http.StateNew:
		managedConn.SetGeneration(s.tlsGeneration.Load())
		s.connMu.Lock()
		s.tlsConns[managedConn] = struct{}{}
		s.connMu.Unlock()
	case http.StateHijacked, http.StateClosed:
		selection, hasSelection := s.takeTLSSelection(managedConn)
		s.connMu.Lock()
		delete(s.tlsConns, managedConn)
		s.connMu.Unlock()
		if hasSelection {
			s.releaseTLSSelection(selection)
		}
	}
}

func (s *Server) scheduleTLSGenerationDrain() {
	generationCutoff := s.tlsGeneration.Load()
	if generationCutoff == 0 {
		return
	}
	s.connMu.Lock()
	snapshot := make(map[*network.ManagedTLSConn]struct{}, len(s.tlsConns))
	for conn := range s.tlsConns {
		snapshot[conn] = struct{}{}
	}
	s.connMu.Unlock()
	network.CloseManagedTLSConnections(snapshot, generationCutoff, service.PanelTLSDrainGracePeriod())
}

func (s *Server) DrainTLSConnectionsByFingerprint(fingerprint string, gracePeriod time.Duration) {
	fingerprint = strings.TrimSpace(fingerprint)
	if fingerprint == "" {
		return
	}
	s.connMu.Lock()
	snapshot := make(map[*network.ManagedTLSConn]struct{}, len(s.tlsConns))
	for conn := range s.tlsConns {
		snapshot[conn] = struct{}{}
	}
	s.connMu.Unlock()
	network.CloseManagedTLSConnectionsByFingerprint(snapshot, fingerprint, gracePeriod)
}

func (s *Server) tlsSnapshot() ([]*tlsRuntimeCertificate, string, string, bool) {
	s.tlsMu.RLock()
	defer s.tlsMu.RUnlock()
	if len(s.tlsMaterials) == 0 {
		return nil, "", "", false
	}
	materials := append([]*tlsRuntimeCertificate(nil), s.tlsMaterials...)
	return materials, s.tlsDefaultFP, s.tlsListenerKey, true
}

func (s *Server) getTLSCertificate(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	materials, _, listenerKey, ok := s.tlsSnapshot()
	if !ok || len(materials) == 0 {
		return nil, fmt.Errorf("sub tls certificate not loaded")
	}
	serverName := ""
	localIP := ""
	var managedConn *network.ManagedTLSConn
	if hello != nil {
		serverName = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(hello.ServerName), "."))
		localIP = normalizeTLSLocalIP(hello.Conn)
		managedConn = network.ManagedTLSConnFromNetConn(hello.Conn)
	}
	var selected *tlsRuntimeCertificate
	var selection service.PanelCertificateBalanceSelection
	if serverName != "" {
		exactCandidates, wildcardCandidates := splitSNITLSRuntimeCertificateCandidates(materials, serverName)
		selected, selection = s.selectBalancedTLSRuntimeCertificate(exactCandidates, listenerKey, serverName)
		if selected == nil {
			selected, selection = s.selectBalancedTLSRuntimeCertificate(wildcardCandidates, listenerKey, serverName)
		}
		if selected == nil {
			selected, selection = s.selectBalancedTLSRuntimeCertificate(materials, listenerKey, serverName)
		}
	} else {
		ipPreferred, others := splitNoSNITLSRuntimeCertificateCandidates(materials, localIP)
		selected, selection = s.selectBalancedTLSRuntimeCertificate(ipPreferred, listenerKey, service.NormalizePanelCertificateBalanceSNIBucket(""))
		if selected == nil {
			selected, selection = s.selectBalancedTLSRuntimeCertificate(others, listenerKey, service.NormalizePanelCertificateBalanceSNIBucket(""))
		}
	}
	if selected == nil || selected.leaf == nil {
		return nil, fmt.Errorf("no certificate available")
	}
	if managedConn != nil {
		managedConn.SetFingerprint(selected.fingerprint)
		s.bindTLSSelection(managedConn, selection)
	}
	return &selected.cert, nil
}

func (s *Server) loadTLSCertificateMaterial(certPEM []byte, keyPEM []byte, certRecordID uint) (*tlsRuntimeCertificate, error) {
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, err
	}
	leaf, err := network.ParseLeafCertificate(&cert)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(leaf.Raw)
	return &tlsRuntimeCertificate{
		cert:         cert,
		leaf:         leaf,
		certRecordID: certRecordID,
		fingerprint:  hex.EncodeToString(sum[:]),
		notAfter:     leaf.NotAfter,
	}, nil
}

func (s *Server) loadTLSCertificateMaterials(materials []*service.PanelTLSMaterial) ([]*tlsRuntimeCertificate, error) {
	result := make([]*tlsRuntimeCertificate, 0, len(materials))
	for _, material := range materials {
		if material == nil {
			continue
		}
		recordID := uint(0)
		if material.Record != nil {
			recordID = material.Record.Id
		}
		loaded, err := s.loadTLSCertificateMaterial(material.CertPEM, material.KeyPEM, recordID)
		if err != nil {
			return nil, err
		}
		result = append(result, loaded)
	}
	return result, nil
}

func (s *Server) ReloadTLSCertificateMaterials(materials []*service.PanelTLSMaterial) (string, error) {
	_, _, _, active := s.tlsSnapshot()
	if !active {
		return "", fmt.Errorf("sub listener is not running in https mode")
	}

	certs, err := s.loadTLSCertificateMaterials(materials)
	if err != nil {
		return "", err
	}
	fingerprint := s.setTLSState(certs)
	s.scheduleTLSGenerationDrain()
	return fingerprint, nil
}

func (s *Server) ReloadTLSCertificatePEM(certPEM []byte, keyPEM []byte) (string, error) {
	_, err := s.loadTLSCertificateMaterial(certPEM, keyPEM, 0)
	if err != nil {
		return "", err
	}
	return s.ReloadTLSCertificateMaterials([]*service.PanelTLSMaterial{{
		CertPEM: certPEM,
		KeyPEM:  keyPEM,
	}})
}

// ReloadTLSCertificate reloads certificate/key files for the running HTTPS sub listener.
// It does not restart the app and never touches sing-box core lifecycle.
func (s *Server) ReloadTLSCertificate(certFile string, keyFile string) (string, error) {
	certPEM, keyPEM, err := service.LoadPanelCertificatePEMFromPaths(certFile, keyFile)
	if err != nil {
		return "", err
	}
	return s.ReloadTLSCertificatePEM(certPEM, keyPEM)
}

// TLSState returns whether sub is currently serving HTTPS and its in-use certificate metadata.
func (s *Server) TLSState() (bool, string, time.Time) {
	materials, fingerprint, _, ok := s.tlsSnapshot()
	if !ok || len(materials) == 0 {
		return false, "", time.Time{}
	}
	material := firstUsableTLSRuntimeCertificate(materials, time.Now())
	if material == nil {
		return true, fingerprint, time.Time{}
	}
	return true, fingerprint, material.notAfter
}

func selectTLSRuntimeCertificate(materials []*tlsRuntimeCertificate, serverName string) *tlsRuntimeCertificate {
	if len(materials) == 0 {
		return nil
	}
	now := time.Now()
	serverName = strings.TrimSpace(strings.TrimSuffix(serverName, "."))
	if serverName == "" {
		return firstUsableTLSRuntimeCertificate(materials, now)
	}
	for _, material := range materials {
		if !tlsRuntimeCertificateUsable(material, now) {
			continue
		}
		if err := material.leaf.VerifyHostname(serverName); err == nil {
			return material
		}
	}
	return nil
}

func (s *Server) selectBalancedTLSRuntimeCertificate(candidates []*tlsRuntimeCertificate, listenerKey string, sniBucket string) (*tlsRuntimeCertificate, service.PanelCertificateBalanceSelection) {
	filtered, ids, byID := uniqueTLSRuntimeCertificates(candidates)
	if len(filtered) == 0 {
		return nil, service.PanelCertificateBalanceSelection{}
	}
	if len(ids) == 0 || strings.TrimSpace(listenerKey) == "" {
		return filtered[0], service.PanelCertificateBalanceSelection{}
	}
	selectedID, selection, err := s.balanceService.Reserve(listenerKey, sniBucket, ids)
	if err == nil {
		if selected := byID[selectedID]; selected != nil {
			return selected, selection
		}
	}
	return filtered[0], service.PanelCertificateBalanceSelection{}
}

func uniqueTLSRuntimeCertificates(candidates []*tlsRuntimeCertificate) ([]*tlsRuntimeCertificate, []uint, map[uint]*tlsRuntimeCertificate) {
	filtered := make([]*tlsRuntimeCertificate, 0, len(candidates))
	ids := make([]uint, 0, len(candidates))
	byID := make(map[uint]*tlsRuntimeCertificate, len(candidates))
	seenIDs := make(map[uint]struct{}, len(candidates))
	now := time.Now()
	for _, candidate := range candidates {
		if !tlsRuntimeCertificateUsable(candidate, now) {
			continue
		}
		if candidate.certRecordID == 0 {
			filtered = append(filtered, candidate)
			continue
		}
		if _, exists := seenIDs[candidate.certRecordID]; exists {
			continue
		}
		seenIDs[candidate.certRecordID] = struct{}{}
		filtered = append(filtered, candidate)
		ids = append(ids, candidate.certRecordID)
		byID[candidate.certRecordID] = candidate
	}
	return filtered, ids, byID
}

func collectSNIMatchingTLSRuntimeCertificates(materials []*tlsRuntimeCertificate, serverName string) []*tlsRuntimeCertificate {
	serverName = strings.TrimSpace(strings.TrimSuffix(serverName, "."))
	if serverName == "" {
		return nil
	}
	matched := make([]*tlsRuntimeCertificate, 0, len(materials))
	now := time.Now()
	for _, material := range materials {
		if !tlsRuntimeCertificateUsable(material, now) {
			continue
		}
		if material.leaf.VerifyHostname(serverName) == nil {
			matched = append(matched, material)
		}
	}
	return matched
}

func splitSNITLSRuntimeCertificateCandidates(materials []*tlsRuntimeCertificate, serverName string) ([]*tlsRuntimeCertificate, []*tlsRuntimeCertificate) {
	serverName = strings.TrimSpace(strings.TrimSuffix(serverName, "."))
	if serverName == "" {
		return nil, nil
	}
	exact := make([]*tlsRuntimeCertificate, 0, len(materials))
	wildcard := make([]*tlsRuntimeCertificate, 0, len(materials))
	now := time.Now()
	for _, material := range materials {
		if !tlsRuntimeCertificateUsable(material, now) || material == nil || material.leaf == nil {
			continue
		}
		matchType := tlsRuntimeCertificateSNIMatchType(material, serverName)
		switch matchType {
		case tlsRuntimeCertificateSNIMatchExact:
			exact = append(exact, material)
		case tlsRuntimeCertificateSNIMatchWildcard:
			wildcard = append(wildcard, material)
		}
	}
	return exact, wildcard
}

func splitNoSNITLSRuntimeCertificateCandidates(materials []*tlsRuntimeCertificate, localIP string) ([]*tlsRuntimeCertificate, []*tlsRuntimeCertificate) {
	localIP = strings.TrimSpace(localIP)
	ipPreferred := make([]*tlsRuntimeCertificate, 0, len(materials))
	others := make([]*tlsRuntimeCertificate, 0, len(materials))
	now := time.Now()
	for _, material := range materials {
		if !tlsRuntimeCertificateUsable(material, now) {
			continue
		}
		if tlsRuntimeCertificateHasIPSAN(material) && tlsRuntimeCertificateMatchesNoSNILocalIP(material, localIP) {
			ipPreferred = append(ipPreferred, material)
			continue
		}
		others = append(others, material)
	}
	return ipPreferred, others
}

func firstUsableTLSRuntimeCertificate(materials []*tlsRuntimeCertificate, now time.Time) *tlsRuntimeCertificate {
	for _, material := range materials {
		if tlsRuntimeCertificateUsable(material, now) {
			return material
		}
	}
	return nil
}

func tlsRuntimeCertificateUsable(material *tlsRuntimeCertificate, now time.Time) bool {
	if material == nil || material.leaf == nil {
		return false
	}
	if now.IsZero() {
		now = time.Now()
	}
	notAfter := material.notAfter
	if notAfter.IsZero() {
		notAfter = material.leaf.NotAfter
	}
	return notAfter.IsZero() || now.Before(notAfter)
}

func tlsRuntimeCertificateHasIPSAN(material *tlsRuntimeCertificate) bool {
	return material != nil && material.leaf != nil && len(material.leaf.IPAddresses) > 0
}

func tlsRuntimeCertificateMatchesNoSNILocalIP(material *tlsRuntimeCertificate, localIP string) bool {
	if material == nil || material.leaf == nil || !tlsRuntimeCertificateHasIPSAN(material) {
		return false
	}
	localIP = strings.TrimSpace(strings.Trim(localIP, "[]"))
	if net.ParseIP(localIP) == nil {
		return false
	}
	return material.leaf.VerifyHostname(localIP) == nil
}

type tlsRuntimeCertificateSNIMatchCategory int

const (
	tlsRuntimeCertificateSNIMatchNone tlsRuntimeCertificateSNIMatchCategory = iota
	tlsRuntimeCertificateSNIMatchExact
	tlsRuntimeCertificateSNIMatchWildcard
)

func tlsRuntimeCertificateSNIMatchType(material *tlsRuntimeCertificate, serverName string) tlsRuntimeCertificateSNIMatchCategory {
	if material == nil || material.leaf == nil {
		return tlsRuntimeCertificateSNIMatchNone
	}
	serverName = strings.TrimSpace(strings.TrimSuffix(serverName, "."))
	if serverName == "" {
		return tlsRuntimeCertificateSNIMatchNone
	}
	if material.leaf.VerifyHostname(serverName) != nil {
		return tlsRuntimeCertificateSNIMatchNone
	}
	if ip := net.ParseIP(serverName); ip != nil {
		return tlsRuntimeCertificateSNIMatchExact
	}
	for _, name := range material.leaf.DNSNames {
		candidate := strings.ToLower(strings.TrimSpace(strings.TrimSuffix(name, ".")))
		if candidate == "" {
			continue
		}
		if candidate == serverName {
			return tlsRuntimeCertificateSNIMatchExact
		}
	}
	return tlsRuntimeCertificateSNIMatchWildcard
}

func normalizeTLSLocalIP(conn net.Conn) string {
	if conn == nil {
		return ""
	}
	addr := conn.LocalAddr()
	if addr == nil {
		return ""
	}
	host := strings.TrimSpace(addr.String())
	parsedHost, _, err := net.SplitHostPort(host)
	if err == nil {
		host = parsedHost
	}
	host = strings.TrimSpace(strings.Trim(host, "[]"))
	if ip := net.ParseIP(host); ip != nil {
		return ip.String()
	}
	return ""
}

func (s *Server) bindTLSSelection(conn *network.ManagedTLSConn, selection service.PanelCertificateBalanceSelection) {
	if conn == nil {
		return
	}
	shouldBind := selection.CertificateRecordID != 0 && strings.TrimSpace(selection.ListenerKey) != ""
	var previous service.PanelCertificateBalanceSelection
	hasPrevious := false
	s.connMu.Lock()
	if s.tlsSelections == nil {
		s.tlsSelections = make(map[*network.ManagedTLSConn]service.PanelCertificateBalanceSelection)
	}
	previous, hasPrevious = s.tlsSelections[conn]
	if shouldBind {
		s.tlsSelections[conn] = selection
	} else {
		delete(s.tlsSelections, conn)
	}
	s.connMu.Unlock()
	if hasPrevious {
		s.releaseTLSSelection(previous)
	}
}

func (s *Server) takeTLSSelection(conn *network.ManagedTLSConn) (service.PanelCertificateBalanceSelection, bool) {
	if conn == nil {
		return service.PanelCertificateBalanceSelection{}, false
	}
	s.connMu.Lock()
	defer s.connMu.Unlock()
	if s.tlsSelections == nil {
		return service.PanelCertificateBalanceSelection{}, false
	}
	selection, ok := s.tlsSelections[conn]
	if ok {
		delete(s.tlsSelections, conn)
	}
	return selection, ok
}

func (s *Server) releaseTLSSelection(selection service.PanelCertificateBalanceSelection) {
	if selection.CertificateRecordID == 0 || strings.TrimSpace(selection.ListenerKey) == "" {
		return
	}
	if err := s.balanceService.Release(selection); err != nil {
		logger.Warning("sub tls certificate selection release failed:", err)
	}
}

func (s *Server) releaseAllTLSSelections() {
	s.connMu.Lock()
	selections := make([]service.PanelCertificateBalanceSelection, 0, len(s.tlsSelections))
	for conn, selection := range s.tlsSelections {
		delete(s.tlsSelections, conn)
		selections = append(selections, selection)
	}
	s.connMu.Unlock()
	for _, selection := range selections {
		s.releaseTLSSelection(selection)
	}
}
