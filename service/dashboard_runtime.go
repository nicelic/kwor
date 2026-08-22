package service

import (
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/alireza0/s-ui/logger"
)

// Keep this shorter than the five-second dashboard cadence. It coalesces
// concurrent tabs and duplicate requests without turning the advertised
// five-second refresh into an effective ten-second host probe.
const dashboardRuntimeCacheTTL = 4 * time.Second

type dashboardRuntimeCacheState struct {
	sync.Mutex
	generation uint64
	entries    map[string]dashboardRuntimeCacheEntry
	inflight   map[string]chan struct{}
}

type dashboardRuntimeCacheEntry struct {
	expiresAt time.Time
	value     map[string]interface{}
}

var dashboardRuntimeCache = dashboardRuntimeCacheState{
	entries:  make(map[string]dashboardRuntimeCacheEntry),
	inflight: make(map[string]chan struct{}),
}

// GetDashboardRuntime returns the compact data used by the home page.  It is
// intentionally separate from the full core-status endpoints: opening a core
// dialog may inspect binaries and download preferences, while the dashboard
// only needs a cached runtime view.
func (s *ServerService) GetDashboardRuntime(request string) map[string]interface{} {
	request = normalizeDashboardRuntimeRequest(request)
	for {
		now := time.Now()
		dashboardRuntimeCache.Lock()
		if cached, ok := dashboardRuntimeCache.entries[request]; ok && now.Before(cached.expiresAt) && cached.value != nil {
			value := cloneDashboardRuntimeMap(cached.value)
			dashboardRuntimeCache.Unlock()
			return value
		}
		if done := dashboardRuntimeCache.inflight[request]; done != nil {
			dashboardRuntimeCache.Unlock()
			<-done
			continue
		}
		done := make(chan struct{})
		dashboardRuntimeCache.inflight[request] = done
		generation := dashboardRuntimeCache.generation
		dashboardRuntimeCache.Unlock()

		value := s.collectDashboardRuntime(request)

		dashboardRuntimeCache.Lock()
		delete(dashboardRuntimeCache.inflight, request)
		if dashboardRuntimeCache.generation == generation {
			dashboardRuntimeCache.entries[request] = dashboardRuntimeCacheEntry{
				expiresAt: time.Now().Add(dashboardRuntimeCacheTTL),
				value:     cloneDashboardRuntimeMap(value),
			}
		}
		close(done)
		dashboardRuntimeCache.Unlock()
		return value
	}
}

func InvalidateDashboardRuntimeCache() {
	invalidateSystemdUnitActiveCache()
	invalidateManagedCoreProcessRuntimeCache()
	dashboardRuntimeCache.Lock()
	dashboardRuntimeCache.generation++
	dashboardRuntimeCache.entries = make(map[string]dashboardRuntimeCacheEntry)
	dashboardRuntimeCache.Unlock()
}

func normalizeDashboardRuntimeRequest(request string) string {
	parts := strings.Split(request, ",")
	values := make([]string, 0, len(parts))
	allowed := map[string]struct{}{
		"cpu": {},
		"mem": {},
		"dsk": {},
		"dio": {},
		"swp": {},
		"net": {},
		"sys": {},
		"sbd": {},
	}
	seen := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		part = strings.ToLower(strings.TrimSpace(part))
		if part == "" {
			continue
		}
		if _, ok := allowed[part]; !ok {
			continue
		}
		if _, exists := seen[part]; exists {
			continue
		}
		seen[part] = struct{}{}
		values = append(values, part)
	}
	sort.Strings(values)
	return strings.Join(values, ",")
}

func (s *ServerService) collectDashboardRuntime(request string) (value map[string]interface{}) {
	defer func() {
		if recovered := recover(); recovered != nil {
			logger.Warning("dashboard runtime probe panicked: ", recovered)
			// A status probe must never strand other callers waiting on the
			// single-flight channel. Return an empty but well-shaped snapshot when
			// a platform probe unexpectedly panics.
			value = map[string]interface{}{
				"status":    &map[string]interface{}{},
				"cores":     map[string]interface{}{"singbox": map[string]interface{}{"running": false}, "mihomo": map[string]interface{}{"running": false}},
				"sampledAt": time.Now().Unix(),
			}
		}
	}()

	status := s.GetStatus(request)
	singboxRunning, singboxKnown := dashboardStatusRunning(status, "sbd")
	if !singboxKnown {
		singboxRunning = (&CoreManagerService{}).IsRunning()
	}
	mihomoRunning, mihomoKnown := dashboardMihomoRunning(status)
	if !mihomoKnown {
		mihomoRunning = (&MihomoCoreManagerService{}).IsRunning()
	}
	return map[string]interface{}{
		"status": status,
		"cores": map[string]interface{}{
			"singbox": map[string]interface{}{"running": singboxRunning},
			"mihomo":  map[string]interface{}{"running": mihomoRunning},
		},
		"sampledAt": time.Now().Unix(),
	}
}

func dashboardStatusRunning(status *map[string]interface{}, key string) (bool, bool) {
	if status == nil || *status == nil {
		return false, false
	}
	value, ok := (*status)[key]
	if !ok {
		return false, false
	}
	view, ok := value.(map[string]interface{})
	if !ok {
		return false, false
	}
	running, ok := view["running"].(bool)
	return running, ok
}

func dashboardMihomoRunning(status *map[string]interface{}) (bool, bool) {
	if status == nil || *status == nil {
		return false, false
	}
	value, ok := (*status)["sbd"]
	if !ok {
		return false, false
	}
	view, ok := value.(map[string]interface{})
	if !ok {
		return false, false
	}
	running, ok := view["mihomoRunning"].(bool)
	return running, ok
}

func cloneDashboardRuntimeMap(source map[string]interface{}) map[string]interface{} {
	if source == nil {
		return nil
	}
	cloned := make(map[string]interface{}, len(source))
	for key, value := range source {
		cloned[key] = cloneDashboardRuntimeValue(value)
	}
	return cloned
}

func cloneDashboardRuntimeValue(value interface{}) interface{} {
	switch typed := value.(type) {
	case map[string]interface{}:
		return cloneDashboardRuntimeMap(typed)
	case *map[string]interface{}:
		if typed == nil {
			return (*map[string]interface{})(nil)
		}
		cloned := cloneDashboardRuntimeMap(*typed)
		return &cloned
	case []string:
		return append([]string(nil), typed...)
	case []interface{}:
		cloned := make([]interface{}, len(typed))
		for index, item := range typed {
			cloned[index] = cloneDashboardRuntimeValue(item)
		}
		return cloned
	default:
		return value
	}
}
