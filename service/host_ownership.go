package service

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	hostOwnershipManifestVersion = 1
	hostOwnershipDir             = "/var/lib/kwor"
	hostOwnershipManifestName    = "ownership-v1.json"
)

// HostResourceKind describes a resource that survives outside the panel
// database. The inventory intentionally contains paths and fingerprints only;
// secrets such as private keys and DNS credentials must never be written here.
type HostResourceKind string

const (
	HostResourcePanelRuntime    HostResourceKind = "panel-runtime"
	HostResourceSystemdUnit     HostResourceKind = "systemd-unit"
	HostResourceManagedCore     HostResourceKind = "managed-core"
	HostResourceNftTable        HostResourceKind = "nft-table"
	HostResourceHostFile        HostResourceKind = "host-file"
	HostResourceACME            HostResourceKind = "acme"
	HostResourceVnStat          HostResourceKind = "vnstat"
	HostResourceKernelForward   HostResourceKind = "kernel-forward"
	HostResourceUpdateWorkspace HostResourceKind = "update-workspace"
)

const (
	hostResourceStatePending        = "pending"
	hostResourceStateActive         = "active"
	hostResourceStateCleanupPending = "cleanup-pending"

	HostCleanupDelete       = "delete"
	HostCleanupUnlockOnly   = "unlock-only"
	HostCleanupRestoreValue = "restore-value"
	HostCleanupPreserve     = "preserve"
)

// HostNftTable identifies one table family/name pair. A table can be present
// in inet, ip and ip6 simultaneously on compatibility layouts.
type HostNftTable struct {
	Family string `json:"family"`
	Name   string `json:"name"`
}

// HostPathBeforeState records the observable state before kwor first changes
// a path. It stores no content: a hash is retained only for regular files.
type HostPathBeforeState struct {
	Existed        bool   `json:"existed"`
	Hash           string `json:"hash,omitempty"`
	ImmutableKnown bool   `json:"immutableKnown,omitempty"`
	Immutable      bool   `json:"immutable,omitempty"`
}

// HostResource is deliberately generic so every host-side writer can be
// tracked without coupling the manifest format to database models.
type HostResource struct {
	ID            string                         `json:"id"`
	Kind          HostResourceKind               `json:"kind"`
	State         string                         `json:"state"`
	CleanupPolicy string                         `json:"cleanupPolicy"`
	Paths         []string                       `json:"paths,omitempty"`
	Before        map[string]HostPathBeforeState `json:"before,omitempty"`
	Units         []string                       `json:"units,omitempty"`
	NftTables     []HostNftTable                 `json:"nftTables,omitempty"`
	Hashes        map[string]string              `json:"hashes,omitempty"`
	Metadata      map[string]string              `json:"metadata,omitempty"`
	CreatedAt     int64                          `json:"createdAt"`
	UpdatedAt     int64                          `json:"updatedAt"`
	// VerifiedAt is optional so existing ownership-v1 manifests remain
	// readable. A destructive cleanup may only rely on a record once its
	// post-write state has been verified.
	VerifiedAt int64 `json:"verifiedAt,omitempty"`
}

type HostOwnershipManifest struct {
	Version        int            `json:"version"`
	InstallationID string         `json:"installationId"`
	HostID         string         `json:"hostId"`
	Uninstalling   bool           `json:"uninstalling"`
	Resources      []HostResource `json:"resources"`
	UpdatedAt      int64          `json:"updatedAt"`
}

type hostOwnershipStore struct {
	path string
	mu   *sync.Mutex
}

var (
	hostOwnershipDefaultMu      sync.Mutex
	hostOwnershipManifestPathFn = func() string {
		return filepath.Join(hostOwnershipDir, hostOwnershipManifestName)
	}
	hostOwnershipNowFn     = time.Now
	hostOwnershipReadFile  = os.ReadFile
	hostOwnershipWriteFile = os.WriteFile
	hostOwnershipMkdirAll  = os.MkdirAll
	hostOwnershipRename    = os.Rename
	hostOwnershipRemove    = os.Remove
	hostOwnershipStat      = os.Stat
)

func HostOwnershipManifestPath() string {
	return filepath.Clean(hostOwnershipManifestPathFn())
}

func defaultHostOwnershipStore() *hostOwnershipStore {
	return &hostOwnershipStore{path: HostOwnershipManifestPath(), mu: &hostOwnershipDefaultMu}
}

func newHostOwnershipStore(path string) *hostOwnershipStore {
	return &hostOwnershipStore{path: filepath.Clean(path), mu: &sync.Mutex{}}
}

func (s *hostOwnershipStore) loadLocked() (*HostOwnershipManifest, bool, error) {
	if s == nil || strings.TrimSpace(s.path) == "" {
		return nil, false, errors.New("host ownership manifest path is empty")
	}
	raw, err := hostOwnershipReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	manifest := &HostOwnershipManifest{}
	if err := json.Unmarshal(raw, manifest); err != nil {
		return nil, false, fmt.Errorf("parse host ownership manifest: %w", err)
	}
	if manifest.Version != hostOwnershipManifestVersion {
		return nil, false, fmt.Errorf("unsupported host ownership manifest version: %d", manifest.Version)
	}
	if strings.TrimSpace(manifest.InstallationID) == "" {
		return nil, false, errors.New("host ownership manifest has no installation id")
	}
	normalizeHostOwnershipManifest(manifest)
	return manifest, true, nil
}

func (s *hostOwnershipStore) Load() (*HostOwnershipManifest, bool, error) {
	if s == nil {
		return nil, false, errors.New("host ownership store is nil")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadLocked()
}

func (s *hostOwnershipStore) loadOrCreateLocked() (*HostOwnershipManifest, error) {
	manifest, found, err := s.loadLocked()
	if err != nil {
		return nil, err
	}
	if found {
		return manifest, nil
	}
	installationID, err := newHostOwnershipID()
	if err != nil {
		return nil, err
	}
	return &HostOwnershipManifest{
		Version:        hostOwnershipManifestVersion,
		InstallationID: installationID,
		HostID:         hostOwnershipHostID(),
		Resources:      []HostResource{},
	}, nil
}

func (s *hostOwnershipStore) saveLocked(manifest *HostOwnershipManifest) error {
	if manifest == nil {
		return errors.New("host ownership manifest is nil")
	}
	normalizeHostOwnershipManifest(manifest)
	manifest.Version = hostOwnershipManifestVersion
	manifest.UpdatedAt = hostOwnershipNowFn().Unix()
	if err := hostOwnershipMkdirAll(filepath.Dir(s.path), 0o750); err != nil {
		return fmt.Errorf("create host ownership directory: %w", err)
	}
	if err := os.Chmod(filepath.Dir(s.path), 0o750); err != nil {
		return fmt.Errorf("set host ownership directory permissions: %w", err)
	}
	raw, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	temporaryPath := s.path + ".tmp"
	if err := hostOwnershipWriteFile(temporaryPath, append(raw, '\n'), 0o600); err != nil {
		return fmt.Errorf("write host ownership manifest: %w", err)
	}
	if err := os.Chmod(temporaryPath, 0o600); err != nil {
		return fmt.Errorf("set temporary host ownership manifest permissions: %w", err)
	}
	if err := syncHostOwnershipFile(temporaryPath); err != nil {
		return fmt.Errorf("sync temporary host ownership manifest: %w", err)
	}
	if err := hostOwnershipRename(temporaryPath, s.path); err != nil {
		_ = hostOwnershipRemove(temporaryPath)
		return fmt.Errorf("replace host ownership manifest: %w", err)
	}
	if err := os.Chmod(s.path, 0o600); err != nil {
		return fmt.Errorf("set host ownership manifest permissions: %w", err)
	}
	if err := syncHostOwnershipFile(s.path); err != nil {
		return fmt.Errorf("sync host ownership manifest: %w", err)
	}
	if err := syncHostOwnershipDirectory(filepath.Dir(s.path)); err != nil {
		return fmt.Errorf("sync host ownership manifest directory: %w", err)
	}
	return nil
}

func syncHostOwnershipFile(path string) error {
	if runtime.GOOS != "linux" {
		return nil
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	return file.Sync()
}

func syncHostOwnershipDirectory(path string) error {
	// Windows does not support fsync on a directory. Linux is the only host on
	// which the persistent ownership manifest is used for destructive cleanup.
	if runtime.GOOS != "linux" {
		return nil
	}
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}

func (s *hostOwnershipStore) Upsert(resource HostResource) (HostResource, error) {
	if s == nil {
		return HostResource{}, errors.New("host ownership store is nil")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	manifest, err := s.loadOrCreateLocked()
	if err != nil {
		return HostResource{}, err
	}
	if strings.TrimSpace(resource.ID) == "" {
		resource.ID, err = newHostOwnershipID()
		if err != nil {
			return HostResource{}, err
		}
	}
	if strings.TrimSpace(resource.State) == "" {
		resource.State = hostResourceStatePending
	}
	if strings.TrimSpace(resource.CleanupPolicy) == "" {
		resource.CleanupPolicy = HostCleanupDelete
	}
	now := hostOwnershipNowFn().Unix()
	if resource.CreatedAt == 0 {
		resource.CreatedAt = now
	}
	resource.UpdatedAt = now
	normalizeHostResource(&resource)
	for index := range manifest.Resources {
		if manifest.Resources[index].ID == resource.ID {
			resource.Before = mergeHostPathBeforeStates(manifest.Resources[index].Before, resource.Before)
			manifest.Resources[index] = resource
			if err := s.saveLocked(manifest); err != nil {
				return HostResource{}, err
			}
			return resource, nil
		}
	}
	manifest.Resources = append(manifest.Resources, resource)
	if err := s.saveLocked(manifest); err != nil {
		return HostResource{}, err
	}
	return resource, nil
}

func (s *hostOwnershipStore) MarkState(id string, state string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return errors.New("host resource id is empty")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	manifest, found, err := s.loadLocked()
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("host resource %s is not registered", id)
	}
	for index := range manifest.Resources {
		if manifest.Resources[index].ID != id {
			continue
		}
		manifest.Resources[index].State = state
		manifest.Resources[index].UpdatedAt = hostOwnershipNowFn().Unix()
		return s.saveLocked(manifest)
	}
	return fmt.Errorf("host resource %s is not registered", id)
}

func (s *hostOwnershipStore) Remove(id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	manifest, found, err := s.loadLocked()
	if err != nil || !found {
		return err
	}
	resources := manifest.Resources[:0]
	for _, resource := range manifest.Resources {
		if resource.ID != id {
			resources = append(resources, resource)
		}
	}
	manifest.Resources = resources
	return s.saveLocked(manifest)
}

func (s *hostOwnershipStore) SetUninstalling(uninstalling bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	manifest, found, err := s.loadLocked()
	if err != nil {
		return err
	}
	if !found {
		if !uninstalling {
			return nil
		}
		manifest, err = s.loadOrCreateLocked()
		if err != nil {
			return err
		}
	}
	manifest.Uninstalling = uninstalling
	return s.saveLocked(manifest)
}

func (s *hostOwnershipStore) IsUninstalling() (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	manifest, found, err := s.loadLocked()
	if err != nil || !found {
		return false, err
	}
	return manifest.Uninstalling, nil
}

func (s *hostOwnershipStore) RemoveManifestIfEmpty() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	manifest, found, err := s.loadLocked()
	if err != nil || !found {
		return err
	}
	if len(manifest.Resources) > 0 {
		return fmt.Errorf("host ownership manifest still contains %d resources", len(manifest.Resources))
	}
	if err := hostOwnershipRemove(s.path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	directory := filepath.Dir(s.path)
	if err := syncHostOwnershipDirectory(directory); err != nil {
		return fmt.Errorf("sync host ownership directory after manifest removal: %w", err)
	}
	entries, readErr := os.ReadDir(directory)
	if errors.Is(readErr, os.ErrNotExist) {
		return nil
	}
	if readErr != nil {
		return readErr
	}
	if len(entries) == 0 {
		if err := os.Remove(directory); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if err := syncHostOwnershipDirectory(filepath.Dir(directory)); err != nil {
			return fmt.Errorf("sync host ownership parent directory after cleanup: %w", err)
		}
	}
	return nil
}

func normalizeHostOwnershipManifest(manifest *HostOwnershipManifest) {
	if manifest == nil {
		return
	}
	if manifest.Resources == nil {
		manifest.Resources = []HostResource{}
	}
	for index := range manifest.Resources {
		normalizeHostResource(&manifest.Resources[index])
	}
	sort.Slice(manifest.Resources, func(left, right int) bool {
		return manifest.Resources[left].ID < manifest.Resources[right].ID
	})
}

func normalizeHostResource(resource *HostResource) {
	if resource == nil {
		return
	}
	resource.ID = strings.TrimSpace(resource.ID)
	resource.State = strings.TrimSpace(resource.State)
	resource.CleanupPolicy = strings.TrimSpace(resource.CleanupPolicy)
	resource.Paths = normalizeOwnershipStrings(resource.Paths, true)
	resource.Units = normalizeOwnershipStrings(resource.Units, false)
	if len(resource.NftTables) > 0 {
		tables := make([]HostNftTable, 0, len(resource.NftTables))
		seen := make(map[string]struct{}, len(resource.NftTables))
		for _, table := range resource.NftTables {
			table.Family = strings.TrimSpace(table.Family)
			table.Name = strings.TrimSpace(table.Name)
			if table.Family == "" || table.Name == "" {
				continue
			}
			key := table.Family + "\x00" + table.Name
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
			tables = append(tables, table)
		}
		resource.NftTables = tables
	}
	if len(resource.Hashes) == 0 {
		resource.Hashes = nil
	}
	if len(resource.Before) == 0 {
		resource.Before = nil
	} else {
		before := make(map[string]HostPathBeforeState, len(resource.Before))
		for rawPath, state := range resource.Before {
			path := filepath.Clean(strings.TrimSpace(rawPath))
			if path == "" || path == "." {
				continue
			}
			state.Hash = strings.TrimSpace(state.Hash)
			if current, exists := before[path]; exists {
				current.Existed = current.Existed || state.Existed
				if current.Hash == "" {
					current.Hash = state.Hash
				}
				current.Immutable = current.Immutable || state.Immutable
				current.ImmutableKnown = current.ImmutableKnown || state.ImmutableKnown
				before[path] = current
				continue
			}
			before[path] = state
		}
		resource.Before = before
	}
	if len(resource.Metadata) == 0 {
		resource.Metadata = nil
	}
}

func normalizeOwnershipStrings(values []string, cleanPaths bool) []string {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if cleanPaths {
			value = filepath.Clean(value)
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func newHostOwnershipID() (string, error) {
	buffer := make([]byte, 16)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return hex.EncodeToString(buffer), nil
}

func hostOwnershipHostID() string {
	parts := []string{runtime.GOOS}
	if raw, err := os.ReadFile("/etc/machine-id"); err == nil {
		parts = append(parts, strings.TrimSpace(string(raw)))
	} else if name, nameErr := os.Hostname(); nameErr == nil {
		parts = append(parts, name)
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "\n")))
	return hex.EncodeToString(sum[:])
}

// RegisterHostResource persists a project-owned resource before it can be
// lost with Promanager_data. Callers should create a pending record, perform
// their external action, then call ActivateHostResource.
func RegisterHostResource(resource HostResource) (HostResource, error) {
	return defaultHostOwnershipStore().Upsert(resource)
}

// BeginHostResource writes a durable pending record before a caller changes a
// host resource. A crash between the external change and ActivateHostResource
// intentionally leaves the record available for uninstall/recovery.
func BeginHostResource(resource HostResource) (HostResource, error) {
	resource.State = hostResourceStatePending
	// A pending entry is evidence only that an operation began. In particular,
	// it must never retain a pre-write hash and later be mistaken for ownership
	// of a file that existed before kwor touched it.
	resource.Hashes = nil
	resource.VerifiedAt = 0
	if len(resource.Before) == 0 {
		resource.Before = ownershipPathBeforeStates(resource.Paths)
	}
	return RegisterHostResource(resource)
}

func ownershipPathBeforeStates(paths []string) map[string]HostPathBeforeState {
	paths = normalizeOwnershipStrings(paths, true)
	if len(paths) == 0 {
		return nil
	}
	hashes := ownershipPathHashes(paths)
	states := make(map[string]HostPathBeforeState, len(paths))
	for _, path := range paths {
		state := HostPathBeforeState{}
		if _, err := os.Lstat(path); err == nil {
			state.Existed = true
			state.Hash = hashes[path]
		}
		states[path] = state
	}
	return states
}

func ownershipHostFileBeforeStates(paths []string) map[string]HostPathBeforeState {
	states := ownershipPathBeforeStates(paths)
	for _, rawPath := range normalizeOwnershipStrings(paths, true) {
		path := filepath.Clean(rawPath)
		state := states[path]
		if !state.Existed {
			// An absent path cannot have carried an immutable flag before kwor.
			state.ImmutableKnown = true
			states[path] = state
			continue
		}
		immutable, err := detectFileImmutable(path)
		if err == nil {
			state.ImmutableKnown = true
			state.Immutable = immutable
			states[path] = state
		}
	}
	return states
}

func mergeHostPathBeforeStates(existing map[string]HostPathBeforeState, incoming map[string]HostPathBeforeState) map[string]HostPathBeforeState {
	if len(existing) == 0 && len(incoming) == 0 {
		return nil
	}
	merged := make(map[string]HostPathBeforeState, len(existing)+len(incoming))
	for path, state := range existing {
		merged[path] = state
	}
	for path, state := range incoming {
		if _, exists := merged[path]; !exists {
			merged[path] = state
		}
	}
	return merged
}

func ActivateHostResource(id string) error {
	return VerifyAndActivateHostResource(id)
}

// VerifyAndActivateHostResource records fingerprints after a host-side write
// has succeeded. Pending records deliberately have no post-write fingerprint:
// they represent work which may have stopped half way through.
func VerifyAndActivateHostResource(id string) error {
	id = strings.TrimSpace(id)
	if id == "" || runtime.GOOS != "linux" {
		return nil
	}
	return VerifyAndActivateHostResourceForStore(defaultHostOwnershipStore(), id)
}

// VerifyAndActivateHostResourceForStore is used by legacy bootstrap before a
// default store has necessarily been selected. It is intentionally narrow so
// callers cannot activate a pending record without recording its post-write
// fingerprints first.
func VerifyAndActivateHostResourceForStore(store *hostOwnershipStore, id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil
	}
	if store == nil {
		return errors.New("host ownership store is nil")
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	manifest, found, err := store.loadLocked()
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("host resource %s is not registered", id)
	}
	for index := range manifest.Resources {
		resource := &manifest.Resources[index]
		if resource.ID != id {
			continue
		}
		resource.Hashes = ownershipPathHashes(resource.Paths)
		resource.State = hostResourceStateActive
		resource.UpdatedAt = hostOwnershipNowFn().Unix()
		resource.VerifiedAt = resource.UpdatedAt
		return store.saveLocked(manifest)
	}
	return fmt.Errorf("host resource %s is not registered", id)
}

func MarkHostResourceCleanupPending(id string) error {
	return defaultHostOwnershipStore().MarkState(id, hostResourceStateCleanupPending)
}

func RemoveHostResource(id string) error {
	return defaultHostOwnershipStore().Remove(id)
}

func LoadHostOwnershipManifest() (*HostOwnershipManifest, bool, error) {
	return defaultHostOwnershipStore().Load()
}

func MarkKworUninstalling() error {
	return defaultHostOwnershipStore().SetUninstalling(true)
}

func IsKworUninstalling() (bool, error) {
	return defaultHostOwnershipStore().IsUninstalling()
}

func RegisterPanelHostOwnership(binaryPath string, dataDir string, serviceName string) error {
	return RegisterPanelHostOwnershipWithPaths(binaryPath, dataDir, serviceName, nil)
}

func RegisterPanelHostOwnershipWithPaths(binaryPath string, dataDir string, serviceName string, runtimePaths []string) error {
	if runtime.GOOS != "linux" {
		return nil
	}
	paths := panelRuntimeOwnershipPaths(binaryPath, dataDir, runtimePaths, nil)
	manifest, found, err := LoadHostOwnershipManifest()
	if err != nil {
		return err
	}
	if found && manifest != nil {
		for _, existing := range manifest.Resources {
			if existing.ID != "panel-runtime" || existing.Kind != HostResourcePanelRuntime {
				continue
			}
			// 升级可能替换主二进制，而旧 kwor 别名仍在运行。保留之前已验证的
			// 所有运行路径，使卸载仍能匹配其精确（包括 " (deleted)"）进程路径。
			paths = panelRuntimeOwnershipPaths(binaryPath, dataDir, runtimePaths, existing.Paths)
			break
		}
	}
	resource, err := RegisterHostResource(HostResource{
		ID:            "panel-runtime",
		Kind:          HostResourcePanelRuntime,
		State:         hostResourceStatePending,
		CleanupPolicy: HostCleanupDelete,
		Paths:         paths,
		Before:        ownershipPathsAssumedNew(paths),
		Units:         []string{serviceName},
	})
	if err != nil {
		return err
	}
	return VerifyAndActivateHostResource(resource.ID)
}

func panelRuntimeOwnershipPaths(binaryPath string, dataDir string, runtimePaths []string, existingPaths []string) []string {
	paths := append([]string{}, existingPaths...)
	paths = append(paths, binaryPath, dataDir)
	paths = append(paths, runtimePaths...)
	paths = append(paths, panelBinaryAliasPaths(binaryPath)...)
	return normalizeOwnershipStrings(paths, true)
}

// RefreshPanelHostOwnership is used after a panel update replaces the binary
// or its runtime support files. It preserves the original Before map while
// refreshing only post-write fingerprints.
func RefreshPanelHostOwnership(binaryPath string, dataDir string, serviceName string, runtimePaths []string) error {
	return RegisterPanelHostOwnershipWithPaths(binaryPath, dataDir, serviceName, runtimePaths)
}

func panelBinaryAliasPaths(binaryPath string) []string {
	binaryPath = filepath.Clean(strings.TrimSpace(binaryPath))
	if binaryPath == "" || binaryPath == "." {
		return nil
	}
	expectedResolved := binaryPath
	if resolved, err := filepath.EvalSymlinks(binaryPath); err == nil {
		expectedResolved = filepath.Clean(resolved)
	}
	expectedHash := ownershipPathHashes([]string{binaryPath})[binaryPath]
	aliases := make([]string, 0, 3)
	for _, name := range []string{"kwor", "kwor_amd64", "kwor_arm64"} {
		candidate := filepath.Join(filepath.Dir(binaryPath), name)
		if candidate == binaryPath || !pathExists(candidate) {
			continue
		}
		if resolved, err := filepath.EvalSymlinks(candidate); err == nil && filepath.Clean(resolved) == expectedResolved {
			aliases = append(aliases, candidate)
			continue
		}
		if expectedHash != "" && ownershipPathHashes([]string{candidate})[candidate] == expectedHash {
			aliases = append(aliases, candidate)
		}
	}
	return normalizeOwnershipStrings(aliases, true)
}

func RegisterSystemdHostOwnership(id string, unit string, paths []string, metadata map[string]string) error {
	if runtime.GOOS != "linux" {
		return nil
	}
	resource, err := RegisterHostResource(HostResource{
		ID:            id,
		Kind:          HostResourceSystemdUnit,
		State:         hostResourceStatePending,
		CleanupPolicy: HostCleanupDelete,
		Paths:         paths,
		Units:         []string{unit},
		Hashes:        ownershipPathHashes(paths),
		Metadata:      metadata,
	})
	if err != nil {
		return err
	}
	return ActivateHostResource(resource.ID)
}

// RefreshPanelSystemdHostOwnershipIfVerified prevents startup from adopting an
// unrelated pre-existing kwor.service merely because its filename matches.
func RefreshPanelSystemdHostOwnershipIfVerified(unit string, unitPath string, binaryPath string) (bool, error) {
	if runtime.GOOS != "linux" {
		return false, nil
	}
	unitPath = filepath.Clean(strings.TrimSpace(unitPath))
	binaryPath = filepath.Clean(strings.TrimSpace(binaryPath))
	_, err := os.ReadFile(unitPath)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !panelSystemdUnitMatchesBinary(unitPath, binaryPath) {
		return false, nil
	}
	if err := RegisterSystemdHostOwnership("panel-systemd", unit, []string{unitPath}, map[string]string{"binary": binaryPath}); err != nil {
		return false, err
	}
	return true, nil
}

func panelSystemdUnitMatchesBinary(unitPath string, binaryPath string) bool {
	raw, err := os.ReadFile(filepath.Clean(strings.TrimSpace(unitPath)))
	if err != nil || !strings.Contains(strings.ToLower(string(raw)), kworOwnershipMarker) {
		return false
	}
	return PanelSystemdUnitExecutesBinary(unitPath, binaryPath)
}

func PanelSystemdUnitExecutesBinary(unitPath string, binaryPath string) bool {
	execPath := extractPanelExecStartPath(unitPath)
	return managedCoreProcessPathEquals(binaryPath, execPath)
}

func PanelSystemdUnitIsManaged(unitPath string, binaryPath string) bool {
	raw, err := os.ReadFile(filepath.Clean(strings.TrimSpace(unitPath)))
	if err != nil {
		return false
	}
	if strings.Contains(strings.ToLower(string(raw)), "kwor-owner:v1 resource=panel-systemd") {
		return true
	}
	return PanelSystemdUnitExecutesBinary(unitPath, binaryPath)
}

func BeginSystemdHostOwnership(id string, unit string, paths []string, metadata map[string]string) (HostResource, error) {
	if runtime.GOOS != "linux" {
		return HostResource{}, nil
	}
	return BeginHostResource(HostResource{
		ID:            id,
		Kind:          HostResourceSystemdUnit,
		CleanupPolicy: HostCleanupDelete,
		Paths:         paths,
		Units:         []string{unit},
		Hashes:        ownershipPathHashes(paths),
		Metadata:      metadata,
	})
}

func RegisterManagedCoreHostOwnership(coreName string, binaryPath string, unit string) error {
	if runtime.GOOS != "linux" {
		return nil
	}
	id := "core-" + strings.ToLower(strings.TrimSpace(coreName))
	return refreshHostResourceAfterWrite(HostResource{
		ID:            id,
		Kind:          HostResourceManagedCore,
		CleanupPolicy: HostCleanupDelete,
		Paths:         []string{binaryPath},
		Units:         []string{unit},
		Metadata:      map[string]string{"core": strings.ToLower(strings.TrimSpace(coreName))},
	})
}

func RegisterNftHostOwnership(id string, tables []HostNftTable) error {
	if runtime.GOOS != "linux" {
		return nil
	}
	resource, err := RegisterHostResource(HostResource{
		ID:            id,
		Kind:          HostResourceNftTable,
		State:         hostResourceStatePending,
		CleanupPolicy: HostCleanupDelete,
		NftTables:     tables,
	})
	if err != nil {
		return err
	}
	return ActivateHostResource(resource.ID)
}

func RegisterHostFileOwnership(id string, paths []string, cleanupPolicy string) error {
	if runtime.GOOS != "linux" {
		return nil
	}
	resource, err := RegisterHostResource(HostResource{
		ID:            id,
		Kind:          HostResourceHostFile,
		State:         hostResourceStatePending,
		CleanupPolicy: cleanupPolicy,
		Paths:         paths,
		Hashes:        ownershipPathHashes(paths),
	})
	if err != nil {
		return err
	}
	return ActivateHostResource(resource.ID)
}

func BeginHostFileOwnership(id string, paths []string, cleanupPolicy string) (HostResource, error) {
	if runtime.GOOS != "linux" {
		return HostResource{}, nil
	}
	return BeginHostResource(HostResource{
		ID:            id,
		Kind:          HostResourceHostFile,
		CleanupPolicy: cleanupPolicy,
		Paths:         paths,
		Hashes:        ownershipPathHashes(paths),
		Before:        ownershipHostFileBeforeStates(paths),
	})
}

func BeginNftHostOwnership(id string, tables []HostNftTable) (HostResource, error) {
	if runtime.GOOS != "linux" {
		return HostResource{}, nil
	}
	return BeginHostResource(HostResource{
		ID:            id,
		Kind:          HostResourceNftTable,
		CleanupPolicy: HostCleanupDelete,
		NftTables:     tables,
	})
}

func BeginUpdateWorkspaceOwnership(id string, workDir string, targetBinary string) (HostResource, error) {
	if runtime.GOOS != "linux" {
		return HostResource{}, nil
	}
	workDir = filepath.Clean(strings.TrimSpace(workDir))
	targetBinary = filepath.Clean(strings.TrimSpace(targetBinary))
	paths := []string{workDir}
	before := ownershipPathsAssumedNew([]string{workDir})
	metadata := map[string]string{}
	if targetBinary != "" && targetBinary != "." {
		backupPath := targetBinary + ".bak"
		paths = append(paths, backupPath)
		before = mergeHostPathBeforeStates(before, ownershipPathBeforeStates([]string{backupPath}))
		metadata["targetBinary"] = targetBinary
	}
	return BeginHostResource(HostResource{
		ID:            id,
		Kind:          HostResourceUpdateWorkspace,
		CleanupPolicy: HostCleanupDelete,
		Paths:         paths,
		Before:        before,
		Metadata:      metadata,
	})
}

// BeginAcmeHostOwnership preserves any already-confirmed ACME paths while a
// new managed runtime or certificate push is being written. The caller must
// later refresh the exact active inventory with RegisterAcmeHostOwnership.
func BeginAcmeHostOwnership(paths []string, units []string) (HostResource, error) {
	if runtime.GOOS != "linux" {
		return HostResource{}, nil
	}
	mergedPaths := append([]string{}, paths...)
	mergedUnits := append([]string{}, units...)
	manifest, found, err := LoadHostOwnershipManifest()
	if err != nil {
		return HostResource{}, err
	}
	if found && manifest != nil {
		for _, resource := range manifest.Resources {
			if resource.ID != "acme-managed-runtime" || resource.Kind != HostResourceACME {
				continue
			}
			mergedPaths = append(mergedPaths, resource.Paths...)
			mergedUnits = append(mergedUnits, resource.Units...)
			break
		}
	}
	return BeginHostResource(HostResource{
		ID:            "acme-managed-runtime",
		Kind:          HostResourceACME,
		CleanupPolicy: HostCleanupDelete,
		Paths:         mergedPaths,
		Units:         mergedUnits,
		Hashes:        ownershipPathHashes(mergedPaths),
	})
}

func RegisterAcmeHostOwnership(paths []string, units []string) error {
	if runtime.GOOS != "linux" {
		return nil
	}
	return refreshHostResourceAfterWrite(HostResource{
		ID:            "acme-managed-runtime",
		Kind:          HostResourceACME,
		CleanupPolicy: HostCleanupDelete,
		Paths:         paths,
		Units:         units,
	})
}

func RegisterVnstatHostOwnership(paths []string, units []string, metadata map[string]string) error {
	if runtime.GOOS != "linux" {
		return nil
	}
	return refreshHostResourceAfterWrite(HostResource{
		ID:            "vnstat-managed-runtime",
		Kind:          HostResourceVnStat,
		CleanupPolicy: HostCleanupDelete,
		Paths:         paths,
		Units:         units,
		Metadata:      metadata,
	})
}

func BeginVnstatHostOwnership(paths []string, units []string, metadata map[string]string) (HostResource, error) {
	if runtime.GOOS != "linux" {
		return HostResource{}, nil
	}
	return BeginHostResource(HostResource{
		ID:            "vnstat-managed-runtime",
		Kind:          HostResourceVnStat,
		CleanupPolicy: HostCleanupDelete,
		Paths:         paths,
		Units:         units,
		Metadata:      metadata,
	})
}

// refreshHostResourceAfterWrite completes a pending write without recapturing
// the current files as their pre-write state. If a caller has no pending
// record, the existing files are conservatively treated as pre-existing and
// therefore cannot later be removed by ownership hash alone.
func refreshHostResourceAfterWrite(resource HostResource) error {
	if runtime.GOOS != "linux" {
		return nil
	}
	resource.ID = strings.TrimSpace(resource.ID)
	if resource.ID == "" {
		return errors.New("host resource id is empty")
	}
	store := defaultHostOwnershipStore()
	manifest, found, err := store.Load()
	if err != nil {
		return err
	}
	if found && manifest != nil {
		for _, existing := range manifest.Resources {
			if existing.ID != resource.ID || existing.Kind != resource.Kind {
				continue
			}
			existing.Paths = normalizeOwnershipStrings(append(existing.Paths, resource.Paths...), true)
			existing.Units = normalizeOwnershipStrings(append(existing.Units, resource.Units...), false)
			existing.Before = mergeHostPathBeforeStates(existing.Before, ownershipPathBeforeStates(resource.Paths))
			existing.Hashes = ownershipPathHashes(existing.Paths)
			existing.State = hostResourceStateActive
			existing.CleanupPolicy = resource.CleanupPolicy
			existing.Metadata = mergeHostResourceMetadata(existing.Metadata, resource.Metadata)
			existing.VerifiedAt = hostOwnershipNowFn().Unix()
			_, err := store.Upsert(existing)
			return err
		}
	}
	resource, err = BeginHostResource(resource)
	if err != nil {
		return err
	}
	return VerifyAndActivateHostResource(resource.ID)
}

func mergeHostResourceMetadata(existing map[string]string, incoming map[string]string) map[string]string {
	if len(existing) == 0 && len(incoming) == 0 {
		return nil
	}
	result := make(map[string]string, len(existing)+len(incoming))
	for key, value := range existing {
		result[key] = value
	}
	for key, value := range incoming {
		result[key] = value
	}
	return result
}

func ownershipPathsAssumedNew(paths []string) map[string]HostPathBeforeState {
	paths = normalizeOwnershipStrings(paths, true)
	if len(paths) == 0 {
		return nil
	}
	states := make(map[string]HostPathBeforeState, len(paths))
	for _, path := range paths {
		states[path] = HostPathBeforeState{}
	}
	return states
}

func hostResourceHasVerifiedPostWriteState(resource HostResource) bool {
	if resource.State != hostResourceStateActive && resource.State != hostResourceStateCleanupPending {
		return false
	}
	return resource.VerifiedAt > 0
}

func ownershipPathHashes(paths []string) map[string]string {
	result := make(map[string]string)
	for _, rawPath := range paths {
		path := filepath.Clean(strings.TrimSpace(rawPath))
		if path == "" {
			continue
		}
		fingerprint, err := ownershipPathFingerprint(path)
		if err != nil || fingerprint == "" {
			continue
		}
		result[path] = fingerprint
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func ownershipPathFingerprint(path string) (string, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	sum := sha256.New()
	if info.Mode()&os.ModeSymlink != 0 {
		target, readErr := os.Readlink(path)
		if readErr != nil {
			return "", readErr
		}
		_, _ = io.WriteString(sum, "symlink\x00"+target)
		return hex.EncodeToString(sum.Sum(nil)), nil
	}
	if !info.IsDir() {
		file, openErr := os.Open(path)
		if openErr != nil {
			return "", openErr
		}
		_, copyErr := io.Copy(sum, file)
		closeErr := file.Close()
		if copyErr != nil {
			return "", copyErr
		}
		if closeErr != nil {
			return "", closeErr
		}
		return hex.EncodeToString(sum.Sum(nil)), nil
	}

	root := filepath.Clean(path)
	err = filepath.WalkDir(root, func(current string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, relErr := filepath.Rel(root, current)
		if relErr != nil {
			return relErr
		}
		if relative == "." {
			relative = ""
		}
		_, _ = io.WriteString(sum, relative+"\x00"+entry.Type().String()+"\x00")
		if entry.Type()&os.ModeSymlink != 0 {
			target, readErr := os.Readlink(current)
			if readErr != nil {
				return readErr
			}
			_, _ = io.WriteString(sum, target+"\x00")
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		file, openErr := os.Open(current)
		if openErr != nil {
			return openErr
		}
		_, copyErr := io.Copy(sum, file)
		closeErr := file.Close()
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	})
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(sum.Sum(nil)), nil
}
