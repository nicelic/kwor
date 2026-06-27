package service

import (
	"fmt"
	"math"
	stdnet "net"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/alireza0/s-ui/logger"
	psnet "github.com/shirou/gopsutil/v4/net"
)

const (
	defaultSystemMonitorSampleInterval       = 10 * time.Second
	systemMonitorHighResolutionRetention     = 8 * time.Hour
	systemMonitorRestartCarryDuration        = 5 * time.Minute
	systemMonitorPruneEverySamples           = 90
	systemMonitorPhysicalInterfaceScanPeriod = 30 * time.Second
)

type SystemMonitorSettings struct {
	SampleIntervalSec     int `json:"sampleIntervalSec"`
	PrimaryRetentionHours int `json:"primaryRetentionHours"`
	ArchiveRetentionDays  int `json:"archiveRetentionDays"`
}

type SystemMonitorOverview struct {
	Available bool                  `json:"available"`
	UpdatedAt int64                 `json:"updatedAt"`
	Current   SystemMonitorCurrent  `json:"current"`
	Storage   SystemMonitorStorage  `json:"storage"`
	Settings  SystemMonitorSettings `json:"settings"`
	Error     string                `json:"error,omitempty"`
}

type SystemMonitorCurrent struct {
	CPUPercent         float64  `json:"cpuPercent"`
	MemoryPercent      float64  `json:"memoryPercent"`
	MemoryUsedBytes    uint64   `json:"memoryUsedBytes"`
	MemoryTotalBytes   uint64   `json:"memoryTotalBytes"`
	DiskReadBps        uint64   `json:"diskReadBps"`
	DiskWriteBps       uint64   `json:"diskWriteBps"`
	NetworkUpBps       uint64   `json:"networkUpBps"`
	NetworkDownBps     uint64   `json:"networkDownBps"`
	PhysicalInterfaces []string `json:"physicalInterfaces"`
	SampleWindowSec    int64    `json:"sampleWindowSec"`
}

type SystemMonitorStorage struct {
	DatabaseSizeBytes uint64 `json:"databaseSizeBytes"`
	SampleIntervalSec int64  `json:"sampleIntervalSec"`
	HighResBucketSec  int64  `json:"highResBucketSec"`
	HighResKeepHours  int64  `json:"highResKeepHours"`
	PrimaryBucketMin  int64  `json:"primaryBucketMin"`
	PrimaryKeepHours  int64  `json:"primaryKeepHours"`
	ArchiveBucketMin  int64  `json:"archiveBucketMin"`
	ArchiveKeepDays   int64  `json:"archiveKeepDays"`
}

type SystemMonitorHistoryResponse struct {
	Range                  string                      `json:"range"`
	Granularity            string                      `json:"granularity"`
	BucketMinutes          int64                       `json:"bucketMinutes"`
	BucketSeconds          int64                       `json:"bucketSeconds"`
	SourceBucketSeconds    int64                       `json:"sourceBucketSeconds"`
	UpdatedAt              int64                       `json:"updatedAt"`
	QueryStart             int64                       `json:"queryStart"`
	QueryEnd               int64                       `json:"queryEnd"`
	AvailableGranularities []string                    `json:"availableGranularities"`
	Points                 []SystemMonitorHistoryPoint `json:"points"`
}

type SystemMonitorHistoryPoint struct {
	Timestamp       int64   `json:"timestamp"`
	CPUAvg          float64 `json:"cpuAvg"`
	CPUPeak         float64 `json:"cpuPeak"`
	MemoryAvg       float64 `json:"memoryAvg"`
	MemoryPeak      float64 `json:"memoryPeak"`
	DiskReadAvg     uint64  `json:"diskReadAvg"`
	DiskReadPeak    uint64  `json:"diskReadPeak"`
	DiskWriteAvg    uint64  `json:"diskWriteAvg"`
	DiskWritePeak   uint64  `json:"diskWritePeak"`
	NetworkUpAvg    uint64  `json:"networkUpAvg"`
	NetworkUpPeak   uint64  `json:"networkUpPeak"`
	NetworkDownAvg  uint64  `json:"networkDownAvg"`
	NetworkDownPeak uint64  `json:"networkDownPeak"`
}

type systemMonitorLiveSnapshot struct {
	SampledAt          time.Time
	CPUPercent         float64
	MemoryPercent      float64
	MemoryUsedBytes    uint64
	MemoryTotalBytes   uint64
	DiskReadBps        uint64
	DiskWriteBps       uint64
	NetworkUpBps       uint64
	NetworkDownBps     uint64
	PhysicalInterfaces []string
}

type systemMonitorHistoryRange struct {
	Key      string
	Lookback time.Duration
}

type SystemMonitorHistoryQuery struct {
	RangeKey             string
	CustomValue          int
	CustomUnit           string
	RequestedGranularity string
	StartSec             int64
	EndSec               int64
	BucketSeconds        int
}

type SystemMonitorService struct{}

type systemMonitorHistoryGranularity struct {
	Key            string
	BucketDuration time.Duration
}

type systemMonitorHistoryAggregate struct {
	BucketStart         int64
	SampleCount         int64
	CPUWeightedSum      float64
	CPUPeak             int64
	MemoryWeightedSum   float64
	MemoryPeak          int64
	DiskReadWeighted    float64
	DiskReadPeak        int64
	DiskWriteWeighted   float64
	DiskWritePeak       int64
	NetworkUpWeighted   float64
	NetworkUpPeak       int64
	NetworkDownWeighted float64
	NetworkDownPeak     int64
}

var systemMonitorHistoryGranularities = []systemMonitorHistoryGranularity{
	{
		Key:            "m",
		BucketDuration: time.Minute,
	},
	{
		Key:            "h",
		BucketDuration: time.Hour,
	},
	{
		Key:            "d",
		BucketDuration: 24 * time.Hour,
	},
}

var systemMonitorRuntime = &systemMonitorRuntimeState{}

type systemMonitorRuntimeState struct {
	startOnce sync.Once
	loopMu    sync.Mutex
	collectMu sync.Mutex
	stateMu   sync.RWMutex

	loopStopCh chan struct{}
	loopDoneCh chan struct{}

	latest      systemMonitorLiveSnapshot
	latestReady bool
	lastError   string

	collectorState       systemMonitorCollectorState
	collectorStateLoaded bool
	collectorSampleCount int

	physicalInterfaces            []string
	physicalInterfacesRefreshedAt time.Time
}

func (s *SystemMonitorService) EnsureRuntimeReady() error {
	if err := InitSystemMonitorStore(); err != nil {
		return err
	}
	if err := systemMonitorRuntime.ensureLatestSnapshot(); err != nil {
		return err
	}
	systemMonitorRuntime.startBackgroundLoop()
	return nil
}

func (s *SystemMonitorService) SaveSettings(sampleIntervalSec int, primaryRetentionHours int, archiveRetentionDays int) error {
	if err := (&SettingService{}).SaveSystemMonitorSettings(sampleIntervalSec, primaryRetentionHours, archiveRetentionDays); err != nil {
		return err
	}
	if err := pruneSystemMonitorRollups(time.Now()); err != nil {
		logger.Warning("prune system monitor rollups after settings save failed:", err)
	}
	return nil
}

func (s *SystemMonitorService) ClearStats() (*SystemMonitorOverview, error) {
	if err := InitSystemMonitorStore(); err != nil {
		return nil, err
	}

	systemMonitorRuntime.collectMu.Lock()
	defer systemMonitorRuntime.collectMu.Unlock()

	if err := clearSystemMonitorHistoryAndCompact(); err != nil {
		return nil, err
	}

	settings := currentSystemMonitorSettings()
	return buildSystemMonitorOverview(settings), nil
}

func (s *SystemMonitorService) GetOverview() (*SystemMonitorOverview, error) {
	if err := s.EnsureRuntimeReady(); err != nil {
		return nil, err
	}

	settings := currentSystemMonitorSettings()
	return buildSystemMonitorOverview(settings), nil
}

func buildSystemMonitorOverview(settings SystemMonitorSettings) *SystemMonitorOverview {
	latest, ready, lastError := systemMonitorRuntime.getLatestSnapshot()
	storage := systemMonitorStorageInfo(settings)
	overview := &SystemMonitorOverview{
		Available: ready,
		UpdatedAt: latest.SampledAt.Unix(),
		Current: SystemMonitorCurrent{
			CPUPercent:         latest.CPUPercent,
			MemoryPercent:      latest.MemoryPercent,
			MemoryUsedBytes:    latest.MemoryUsedBytes,
			MemoryTotalBytes:   latest.MemoryTotalBytes,
			DiskReadBps:        latest.DiskReadBps,
			DiskWriteBps:       latest.DiskWriteBps,
			NetworkUpBps:       latest.NetworkUpBps,
			NetworkDownBps:     latest.NetworkDownBps,
			PhysicalInterfaces: append([]string(nil), latest.PhysicalInterfaces...),
			SampleWindowSec:    int64(systemMonitorCurrentSampleInterval(settings) / time.Second),
		},
		Storage:  storage,
		Settings: settings,
	}
	if !ready && strings.TrimSpace(lastError) != "" {
		overview.Error = lastError
	}
	return overview
}

func (s *SystemMonitorService) GetHistory(query SystemMonitorHistoryQuery) (*SystemMonitorHistoryResponse, error) {
	if err := s.EnsureRuntimeReady(); err != nil {
		return nil, err
	}

	settings := currentSystemMonitorSettings()
	resolvedQuery, err := systemMonitorResolveHistoryQuery(settings, query)
	if err != nil {
		return nil, err
	}

	sourceRollup, err := systemMonitorSelectSourceRollupForBucket(settings, resolvedQuery.Start, resolvedQuery.BucketDuration, time.Now())
	if err != nil {
		return nil, err
	}

	rows, err := querySystemMonitorHistory(sourceRollup, resolvedQuery.Start, resolvedQuery.End)
	if err != nil {
		return nil, err
	}

	points := aggregateSystemMonitorHistory(rows, resolvedQuery.BucketDuration, systemMonitorHistoryLocation())

	latest, _, _ := systemMonitorRuntime.getLatestSnapshot()
	return &SystemMonitorHistoryResponse{
		Range:                  resolvedQuery.RangeLabel,
		Granularity:            systemMonitorDescribeBucket(resolvedQuery.BucketDuration),
		BucketMinutes:          int64(resolvedQuery.BucketDuration / time.Minute),
		BucketSeconds:          int64(resolvedQuery.BucketDuration / time.Second),
		SourceBucketSeconds:    int64(sourceRollup.BucketDuration / time.Second),
		UpdatedAt:              latest.SampledAt.Unix(),
		QueryStart:             resolvedQuery.Start.Unix(),
		QueryEnd:               resolvedQuery.End.Unix(),
		AvailableGranularities: []string{systemMonitorDescribeBucket(resolvedQuery.BucketDuration)},
		Points:                 points,
	}, nil
}

func (r *systemMonitorRuntimeState) startBackgroundLoop() {
	r.startOnce.Do(func() {
		stopCh := make(chan struct{})
		doneCh := make(chan struct{})
		r.loopMu.Lock()
		r.loopStopCh = stopCh
		r.loopDoneCh = doneCh
		r.loopMu.Unlock()

		go func() {
			defer close(doneCh)
			for {
				interval := systemMonitorCurrentSampleInterval(currentSystemMonitorSettings())
				timer := time.NewTimer(interval)
				select {
				case <-timer.C:
				case <-stopCh:
					if !timer.Stop() {
						select {
						case <-timer.C:
						default:
						}
					}
					return
				}
				if err := r.collectAndPersist(); err != nil {
					logger.Warning("system monitor collect failed:", err)
				}
			}
		}()
	})
}

func (r *systemMonitorRuntimeState) resetForDatabaseReload() error {
	r.loopMu.Lock()
	stopCh := r.loopStopCh
	doneCh := r.loopDoneCh
	r.loopMu.Unlock()

	if stopCh != nil {
		close(stopCh)
		if doneCh != nil {
			select {
			case <-doneCh:
			case <-time.After(5 * time.Second):
				return fmt.Errorf("system monitor background loop did not stop in time")
			}
		}
	}

	r.loopMu.Lock()
	if r.loopStopCh == stopCh {
		r.loopStopCh = nil
	}
	if r.loopDoneCh == doneCh {
		r.loopDoneCh = nil
	}
	r.startOnce = sync.Once{}
	r.loopMu.Unlock()

	r.collectMu.Lock()
	defer r.collectMu.Unlock()

	r.stateMu.Lock()
	r.latest = systemMonitorLiveSnapshot{}
	r.latestReady = false
	r.lastError = ""
	r.stateMu.Unlock()

	r.collectorState = systemMonitorCollectorState{}
	r.collectorStateLoaded = false
	r.collectorSampleCount = 0
	r.physicalInterfaces = nil
	r.physicalInterfacesRefreshedAt = time.Time{}
	return nil
}

func (r *systemMonitorRuntimeState) ensureLatestSnapshot() error {
	r.stateMu.RLock()
	latestReady := r.latestReady
	latestSampledAt := r.latest.SampledAt
	r.stateMu.RUnlock()

	if latestReady && time.Since(latestSampledAt) <= (systemMonitorCurrentSampleInterval(currentSystemMonitorSettings())*2) {
		return nil
	}
	return r.collectAndPersist()
}

func (r *systemMonitorRuntimeState) getLatestSnapshot() (systemMonitorLiveSnapshot, bool, string) {
	r.stateMu.RLock()
	defer r.stateMu.RUnlock()
	return r.latest, r.latestReady, r.lastError
}

func (r *systemMonitorRuntimeState) collectAndPersist() error {
	r.collectMu.Lock()
	defer r.collectMu.Unlock()

	if err := r.ensureCollectorStateLoaded(); err != nil {
		return err
	}

	physicalInterfaces, err := r.loadPhysicalInterfaces(time.Now())
	if err != nil {
		logger.Warning("detect physical interfaces failed:", err)
	}

	snapshot, nextCollectorState, scaledPoint, err := collectSystemMonitorSnapshot(r.collectorState, physicalInterfaces)
	if err != nil {
		r.stateMu.Lock()
		r.lastError = err.Error()
		r.stateMu.Unlock()
		return err
	}

	for _, def := range systemMonitorRollupDefinitions {
		if err := upsertSystemMonitorRollup(def, snapshot.SampledAt, scaledPoint); err != nil {
			r.stateMu.Lock()
			r.lastError = err.Error()
			r.stateMu.Unlock()
			return err
		}
	}

	if err := saveSystemMonitorCollectorState(nextCollectorState); err != nil {
		r.stateMu.Lock()
		r.lastError = err.Error()
		r.stateMu.Unlock()
		return err
	}

	r.collectorState = nextCollectorState
	r.collectorSampleCount++
	if r.collectorSampleCount%systemMonitorPruneEverySamples == 0 {
		if err := pruneSystemMonitorRollups(snapshot.SampledAt); err != nil {
			logger.Warning("system monitor prune failed:", err)
		}
	}

	r.stateMu.Lock()
	r.latest = snapshot
	r.latestReady = true
	r.lastError = ""
	r.stateMu.Unlock()
	return nil
}

func (r *systemMonitorRuntimeState) ensureCollectorStateLoaded() error {
	if r.collectorStateLoaded {
		return nil
	}
	state, err := loadSystemMonitorCollectorState()
	if err != nil {
		return err
	}
	r.collectorState = state
	r.collectorStateLoaded = true
	return nil
}

func (r *systemMonitorRuntimeState) loadPhysicalInterfaces(now time.Time) ([]string, error) {
	if len(r.physicalInterfaces) > 0 && now.Sub(r.physicalInterfacesRefreshedAt) < systemMonitorPhysicalInterfaceScanPeriod {
		return append([]string(nil), r.physicalInterfaces...), nil
	}
	names, err := scanPhysicalInterfaces()
	if err != nil {
		return nil, err
	}
	r.physicalInterfaces = append([]string(nil), names...)
	r.physicalInterfacesRefreshedAt = now
	return names, nil
}

func collectSystemMonitorSnapshot(previous systemMonitorCollectorState, physicalInterfaces []string) (systemMonitorLiveSnapshot, systemMonitorCollectorState, systemMonitorScaledPoint, error) {
	server := &ServerService{}
	now := time.Now()

	memInfo := server.GetMemInfo()
	diskIO := server.GetDiskIO()

	memoryUsed := readUint64MapValue(memInfo, "current")
	memoryTotal := readUint64MapValue(memInfo, "total")
	memoryPercent := percentFromRatio(memoryUsed, memoryTotal)

	currentReadBytes := readUint64MapValue(diskIO, "read")
	currentWriteBytes := readUint64MapValue(diskIO, "write")
	diskReadBps, diskWriteBps := computeSystemMonitorDiskRates(now, previous, currentReadBytes, currentWriteBytes)

	currentNetStats, networkUpBps, networkDownBps := readPhysicalNetworkRates(now, physicalInterfaces, previous)

	snapshot := systemMonitorLiveSnapshot{
		SampledAt:          now,
		CPUPercent:         clampPercent(server.GetCpuPercent()),
		MemoryPercent:      memoryPercent,
		MemoryUsedBytes:    memoryUsed,
		MemoryTotalBytes:   memoryTotal,
		DiskReadBps:        diskReadBps,
		DiskWriteBps:       diskWriteBps,
		NetworkUpBps:       networkUpBps,
		NetworkDownBps:     networkDownBps,
		PhysicalInterfaces: append([]string(nil), physicalInterfaces...),
	}

	nextState := systemMonitorCollectorState{
		ReadBytes:        currentReadBytes,
		WriteBytes:       currentWriteBytes,
		PhysicalNetStats: currentNetStats,
		SampledAt:        now.Unix(),
	}

	scaled := systemMonitorScaledPoint{
		CPUPercentScaled:    scalePercentToInt(snapshot.CPUPercent),
		MemoryPercentScaled: scalePercentToInt(snapshot.MemoryPercent),
		DiskReadBps:         clampInt64FromUint64(snapshot.DiskReadBps),
		DiskWriteBps:        clampInt64FromUint64(snapshot.DiskWriteBps),
		NetworkUpBps:        clampInt64FromUint64(snapshot.NetworkUpBps),
		NetworkDownBps:      clampInt64FromUint64(snapshot.NetworkDownBps),
	}

	return snapshot, nextState, scaled, nil
}

func computeSystemMonitorDiskRates(now time.Time, previous systemMonitorCollectorState, currentReadBytes uint64, currentWriteBytes uint64) (uint64, uint64) {
	if previous.SampledAt <= 0 {
		return 0, 0
	}

	lastSampleTime := time.Unix(previous.SampledAt, 0)
	elapsed := now.Sub(lastSampleTime)
	if elapsed <= 0 || elapsed > systemMonitorRestartCarryDuration {
		return 0, 0
	}

	return counterDeltaPerSecond(currentReadBytes, previous.ReadBytes, elapsed), counterDeltaPerSecond(currentWriteBytes, previous.WriteBytes, elapsed)
}

func readPhysicalNetworkRates(now time.Time, physicalInterfaces []string, previous systemMonitorCollectorState) (map[string]systemMonitorInterfaceCounters, uint64, uint64) {
	currentStats := make(map[string]systemMonitorInterfaceCounters)
	if len(physicalInterfaces) == 0 {
		return currentStats, 0, 0
	}

	counters, err := psnet.IOCounters(true)
	if err != nil {
		logger.Warning("read network io counters failed:", err)
		return currentStats, 0, 0
	}

	currentCounterMap := make(map[string]systemMonitorInterfaceCounters, len(counters))
	for _, counter := range counters {
		currentCounterMap[counter.Name] = systemMonitorInterfaceCounters{
			SentBytes: counter.BytesSent,
			RecvBytes: counter.BytesRecv,
		}
	}

	for _, name := range physicalInterfaces {
		if counter, ok := currentCounterMap[name]; ok {
			currentStats[name] = counter
		}
	}

	if previous.SampledAt <= 0 {
		return currentStats, 0, 0
	}

	lastSampleTime := time.Unix(previous.SampledAt, 0)
	elapsed := now.Sub(lastSampleTime)
	if elapsed <= 0 || elapsed > systemMonitorRestartCarryDuration {
		return currentStats, 0, 0
	}

	var uploadBps uint64
	var downloadBps uint64
	for _, name := range physicalInterfaces {
		currentCounter, ok := currentStats[name]
		if !ok {
			continue
		}
		previousCounter, ok := previous.PhysicalNetStats[name]
		if !ok {
			continue
		}
		uploadBps += counterDeltaPerSecond(currentCounter.SentBytes, previousCounter.SentBytes, elapsed)
		downloadBps += counterDeltaPerSecond(currentCounter.RecvBytes, previousCounter.RecvBytes, elapsed)
	}

	return currentStats, uploadBps, downloadBps
}

func counterDeltaPerSecond(current uint64, previous uint64, elapsed time.Duration) uint64 {
	if elapsed <= 0 {
		return 0
	}
	if current < previous {
		return 0
	}
	delta := current - previous
	seconds := elapsed.Seconds()
	if seconds <= 0 {
		return 0
	}
	value := uint64(math.Round(float64(delta) / seconds))
	return value
}

func scanPhysicalInterfaces() ([]string, error) {
	if runtime.GOOS != "linux" {
		return scanNonLinuxPhysicalInterfaces(), nil
	}

	entries, err := os.ReadDir("/sys/class/net")
	if err != nil {
		return nil, err
	}

	names := make([]string, 0)
	for _, entry := range entries {
		name := strings.TrimSpace(entry.Name())
		if name == "" || name == "lo" {
			continue
		}
		linkPath := filepath.Join("/sys/class/net", name)
		resolved, err := filepath.EvalSymlinks(linkPath)
		if err != nil {
			continue
		}
		if strings.Contains(resolved, string(filepath.Separator)+"virtual"+string(filepath.Separator)) {
			continue
		}
		if _, err := os.Stat(filepath.Join(linkPath, "device")); err != nil {
			continue
		}
		names = append(names, name)
	}

	sort.Strings(names)
	return names, nil
}

func scanNonLinuxPhysicalInterfaces() []string {
	interfaces, err := stdnet.Interfaces()
	if err != nil {
		return nil
	}

	names := make([]string, 0)
	for _, iface := range interfaces {
		if iface.Flags&stdnet.FlagLoopback != 0 {
			continue
		}
		if iface.HardwareAddr == nil || len(iface.HardwareAddr) == 0 {
			continue
		}
		names = append(names, iface.Name)
	}
	sort.Strings(names)
	return names
}

func readUint64MapValue(values map[string]interface{}, key string) uint64 {
	raw, exists := values[key]
	if !exists || raw == nil {
		return 0
	}
	switch value := raw.(type) {
	case uint64:
		return value
	case uint32:
		return uint64(value)
	case uint:
		return uint64(value)
	case int64:
		if value < 0 {
			return 0
		}
		return uint64(value)
	case int:
		if value < 0 {
			return 0
		}
		return uint64(value)
	case float64:
		if value <= 0 {
			return 0
		}
		return uint64(math.Round(value))
	case float32:
		if value <= 0 {
			return 0
		}
		return uint64(math.Round(float64(value)))
	default:
		return 0
	}
}

func percentFromRatio(current uint64, total uint64) float64 {
	if total == 0 {
		return 0
	}
	return clampPercent((float64(current) * 100) / float64(total))
}

func clampPercent(value float64) float64 {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0
	}
	if value < 0 {
		return 0
	}
	if value > 100 {
		return 100
	}
	return value
}

func clampInt64FromUint64(value uint64) int64 {
	if value > math.MaxInt64 {
		return math.MaxInt64
	}
	return int64(value)
}

func scaledInt64ToUint64(value int64) uint64 {
	if value <= 0 {
		return 0
	}
	return uint64(value)
}

func currentSystemMonitorSettings() SystemMonitorSettings {
	settingSvc := &SettingService{}
	sampleIntervalSec, err := settingSvc.GetSystemMonitorSampleIntervalSec()
	if err != nil {
		logger.Warning("get system monitor sample interval failed:", err)
		sampleIntervalSec = int(defaultSystemMonitorSampleInterval / time.Second)
	}
	primaryRetentionHours, err := settingSvc.GetSystemMonitorPrimaryRetentionHours()
	if err != nil {
		logger.Warning("get system monitor primary retention failed:", err)
		primaryRetentionHours = 48
	}
	archiveRetentionDays, err := settingSvc.GetSystemMonitorArchiveRetentionDays()
	if err != nil {
		logger.Warning("get system monitor archive retention failed:", err)
		archiveRetentionDays = 120
	}
	return SystemMonitorSettings{
		SampleIntervalSec:     sampleIntervalSec,
		PrimaryRetentionHours: primaryRetentionHours,
		ArchiveRetentionDays:  archiveRetentionDays,
	}
}

func systemMonitorCurrentSampleInterval(settings SystemMonitorSettings) time.Duration {
	seconds := settings.SampleIntervalSec
	if seconds <= 0 {
		seconds = int(defaultSystemMonitorSampleInterval / time.Second)
	}
	return time.Duration(seconds) * time.Second
}

func systemMonitorStorageInfo(settings SystemMonitorSettings) SystemMonitorStorage {
	highRes := systemMonitorRollupDefinitions[0]
	primary := systemMonitorRollupDefinitions[1]
	archive := systemMonitorRollupDefinitions[2]
	return SystemMonitorStorage{
		DatabaseSizeBytes: getSystemMonitorDatabaseSizeBytes(),
		SampleIntervalSec: int64(systemMonitorCurrentSampleInterval(settings) / time.Second),
		HighResBucketSec:  int64(highRes.BucketDuration / time.Second),
		HighResKeepHours:  int64(systemMonitorRetentionForRollup(settings, highRes) / time.Hour),
		PrimaryBucketMin:  int64(primary.BucketDuration / time.Minute),
		PrimaryKeepHours:  int64(systemMonitorRetentionForRollup(settings, primary) / time.Hour),
		ArchiveBucketMin:  int64(archive.BucketDuration / time.Minute),
		ArchiveKeepDays:   int64(systemMonitorRetentionForRollup(settings, archive) / (24 * time.Hour)),
	}
}

func systemMonitorRetentionForRollup(settings SystemMonitorSettings, def systemMonitorRollupDefinition) time.Duration {
	if def.TableName == "system_monitor_rollup_8s" {
		return systemMonitorHighResolutionRetention
	}
	if def.TableName == "system_monitor_rollup_1m" {
		return time.Duration(settings.PrimaryRetentionHours) * time.Hour
	}
	return time.Duration(settings.ArchiveRetentionDays) * 24 * time.Hour
}

type systemMonitorResolvedHistoryQuery struct {
	RangeLabel     string
	Start          time.Time
	End            time.Time
	BucketDuration time.Duration
}

func systemMonitorResolveHistoryQuery(settings SystemMonitorSettings, query SystemMonitorHistoryQuery) (systemMonitorResolvedHistoryQuery, error) {
	now := time.Now()
	if query.StartSec > 0 || query.EndSec > 0 || query.BucketSeconds > 0 {
		end := now
		if query.EndSec > 0 {
			end = time.Unix(query.EndSec, 0)
		}
		if end.After(now) {
			end = now
		}

		start := end.Add(-24 * time.Hour)
		if query.StartSec > 0 {
			start = time.Unix(query.StartSec, 0)
		} else if query.CustomValue > 0 || strings.TrimSpace(query.RangeKey) != "" {
			selectedRange, err := systemMonitorResolveRange(settings, query.RangeKey, query.CustomValue, query.CustomUnit)
			if err != nil {
				return systemMonitorResolvedHistoryQuery{}, err
			}
			start = end.Add(-selectedRange.Lookback)
		}

		if !start.Before(end) {
			return systemMonitorResolvedHistoryQuery{}, fmt.Errorf("monitor history start must be earlier than end")
		}
		if start.Before(now.Add(-systemMonitorMaxRetention(settings))) {
			return systemMonitorResolvedHistoryQuery{}, fmt.Errorf("monitor history exceeds retention (%s)", systemMonitorMaxRetention(settings))
		}

		bucketDuration, err := systemMonitorResolveBucketDuration(query.BucketSeconds, end.Sub(start))
		if err != nil {
			return systemMonitorResolvedHistoryQuery{}, err
		}
		return systemMonitorResolvedHistoryQuery{
			RangeLabel:     "window",
			Start:          start,
			End:            end,
			BucketDuration: bucketDuration,
		}, nil
	}

	selectedRange, err := systemMonitorResolveRange(settings, query.RangeKey, query.CustomValue, query.CustomUnit)
	if err != nil {
		return systemMonitorResolvedHistoryQuery{}, err
	}

	end := now
	start := now.Add(-selectedRange.Lookback)
	return systemMonitorResolvedHistoryQuery{
		RangeLabel:     selectedRange.Key,
		Start:          start,
		End:            end,
		BucketDuration: systemMonitorLegacyBucketDuration(query.RequestedGranularity, selectedRange.Lookback),
	}, nil
}

func systemMonitorResolveBucketDuration(bucketSeconds int, window time.Duration) (time.Duration, error) {
	if window <= 0 {
		return 0, fmt.Errorf("monitor history window must be positive")
	}
	if bucketSeconds <= 0 {
		return systemMonitorLegacyBucketDuration("", window), nil
	}

	bucketDuration := time.Duration(bucketSeconds) * time.Second
	if bucketDuration < 8*time.Second {
		bucketDuration = 8 * time.Second
	}
	if bucketDuration > 24*time.Hour {
		bucketDuration = 24 * time.Hour
	}
	if bucketDuration > window {
		bucketDuration = window
	}
	return bucketDuration, nil
}

func systemMonitorLegacyBucketDuration(requested string, lookback time.Duration) time.Duration {
	switch normalizeSystemMonitorGranularityKey(requested) {
	case "m":
		return time.Minute
	case "h":
		return time.Hour
	case "d":
		return 24 * time.Hour
	default:
		switch {
		case lookback <= time.Hour:
			return time.Minute
		case lookback <= 48*time.Hour:
			return time.Hour
		default:
			return 24 * time.Hour
		}
	}
}

func systemMonitorDescribeBucket(bucketDuration time.Duration) string {
	switch {
	case bucketDuration%(24*time.Hour) == 0:
		return fmt.Sprintf("%dd", int(bucketDuration/(24*time.Hour)))
	case bucketDuration%time.Hour == 0:
		return fmt.Sprintf("%dh", int(bucketDuration/time.Hour))
	case bucketDuration%time.Minute == 0:
		return fmt.Sprintf("%dm", int(bucketDuration/time.Minute))
	default:
		return fmt.Sprintf("%ds", int(bucketDuration/time.Second))
	}
}

func systemMonitorMaxRetention(settings SystemMonitorSettings) time.Duration {
	maxRetention := time.Duration(0)
	for _, def := range systemMonitorRollupDefinitions {
		retention := systemMonitorRetentionForRollup(settings, def)
		if retention > maxRetention {
			maxRetention = retention
		}
	}
	if maxRetention <= 0 {
		return 24 * time.Hour
	}
	return maxRetention
}

func systemMonitorSelectSourceRollupForBucket(settings SystemMonitorSettings, start time.Time, bucketDuration time.Duration, now time.Time) (systemMonitorRollupDefinition, error) {
	if bucketDuration <= 0 {
		return systemMonitorRollupDefinition{}, fmt.Errorf("monitor bucket must be positive")
	}
	if now.IsZero() {
		now = time.Now()
	}

	for index := len(systemMonitorRollupDefinitions) - 1; index >= 0; index-- {
		def := systemMonitorRollupDefinitions[index]
		if def.BucketDuration > bucketDuration {
			continue
		}
		if start.Before(now.Add(-systemMonitorRetentionForRollup(settings, def))) {
			continue
		}
		return def, nil
	}

	return systemMonitorRollupDefinition{}, fmt.Errorf("requested precision %s is not available for that time window", systemMonitorDescribeBucket(bucketDuration))
}

func systemMonitorResolveRange(settings SystemMonitorSettings, rangeKey string, customValue int, customUnit string) (systemMonitorHistoryRange, error) {
	if customValue > 0 {
		lookback, key, err := systemMonitorCustomRange(customValue, customUnit)
		if err != nil {
			return systemMonitorHistoryRange{}, err
		}
		if err := systemMonitorEnsureLookbackSupported(settings, lookback); err != nil {
			return systemMonitorHistoryRange{}, err
		}
		return systemMonitorHistoryRange{
			Key:      key,
			Lookback: lookback,
		}, nil
	}

	key := strings.TrimSpace(strings.ToLower(rangeKey))
	if key == "" {
		key = "24h"
	}
	for _, item := range systemMonitorSupportedRanges(settings) {
		if item.Key == key {
			return item, nil
		}
	}
	return systemMonitorHistoryRange{}, fmt.Errorf("unsupported monitor range: %s", rangeKey)
}

func systemMonitorSupportedRanges(settings SystemMonitorSettings) []systemMonitorHistoryRange {
	candidates := []systemMonitorHistoryRange{
		{Key: "15m", Lookback: 15 * time.Minute},
		{Key: "30m", Lookback: 30 * time.Minute},
		{Key: "1h", Lookback: time.Hour},
		{Key: "6h", Lookback: 6 * time.Hour},
		{Key: "24h", Lookback: 24 * time.Hour},
		{Key: "7d", Lookback: 7 * 24 * time.Hour},
		{Key: "30d", Lookback: 30 * 24 * time.Hour},
		{Key: "90d", Lookback: 90 * 24 * time.Hour},
	}

	filtered := make([]systemMonitorHistoryRange, 0, len(candidates))
	for _, item := range candidates {
		if err := systemMonitorEnsureLookbackSupported(settings, item.Lookback); err != nil {
			continue
		}
		filtered = append(filtered, item)
	}
	return filtered
}

func systemMonitorCustomRange(value int, unit string) (time.Duration, string, error) {
	if value <= 0 {
		return 0, "", fmt.Errorf("custom monitor range must be positive")
	}
	switch strings.TrimSpace(strings.ToLower(unit)) {
	case "minute", "minutes", "min", "mins", "m", "分", "分钟":
		return time.Duration(value) * time.Minute, fmt.Sprintf("%dm", value), nil
	case "day", "days", "d", "天":
		return time.Duration(value) * 24 * time.Hour, fmt.Sprintf("%dd", value), nil
	default:
		return time.Duration(value) * time.Hour, fmt.Sprintf("%dh", value), nil
	}
}

func systemMonitorEnsureLookbackSupported(settings SystemMonitorSettings, lookback time.Duration) error {
	maxRetention := time.Duration(0)
	for _, def := range systemMonitorRollupDefinitions {
		retention := systemMonitorRetentionForRollup(settings, def)
		if lookback <= retention {
			return nil
		}
		if retention > maxRetention {
			maxRetention = retention
		}
	}
	if maxRetention <= 0 {
		maxRetention = 24 * time.Hour
	}
	return fmt.Errorf("monitor history exceeds retention (%s)", maxRetention)
}

func systemMonitorResolveGranularity(settings SystemMonitorSettings, lookback time.Duration, requested string) (systemMonitorHistoryGranularity, []string, error) {
	available := systemMonitorAvailableGranularities(settings, lookback)
	if len(available) == 0 {
		return systemMonitorHistoryGranularity{}, nil, fmt.Errorf("no monitor history granularity available for %s", lookback)
	}

	availableKeys := systemMonitorGranularityKeys(available)
	normalized := normalizeSystemMonitorGranularityKey(requested)
	if normalized == "" || normalized == "auto" {
		return systemMonitorDefaultGranularity(lookback, available), availableKeys, nil
	}

	if item, ok := systemMonitorGranularityByKey(normalized); ok && systemMonitorGranularityAvailable(item, available) {
		return item, availableKeys, nil
	}

	return systemMonitorNearestGranularity(normalized, lookback, available), availableKeys, nil
}

func systemMonitorAvailableGranularities(settings SystemMonitorSettings, lookback time.Duration) []systemMonitorHistoryGranularity {
	available := make([]systemMonitorHistoryGranularity, 0, len(systemMonitorHistoryGranularities))
	for _, item := range systemMonitorHistoryGranularities {
		if item.Key == "d" && lookback < 24*time.Hour {
			continue
		}
		if _, err := systemMonitorSelectSourceRollup(settings, lookback, item); err != nil {
			continue
		}
		available = append(available, item)
	}
	return available
}

func systemMonitorDefaultGranularity(lookback time.Duration, available []systemMonitorHistoryGranularity) systemMonitorHistoryGranularity {
	switch {
	case lookback <= time.Hour:
		if item, ok := systemMonitorPickAvailableGranularity("m", available); ok {
			return item
		}
	case lookback <= 48*time.Hour:
		if item, ok := systemMonitorPickAvailableGranularity("h", available); ok {
			return item
		}
	default:
		if item, ok := systemMonitorPickAvailableGranularity("d", available); ok {
			return item
		}
	}

	if lookback <= time.Hour {
		if item, ok := systemMonitorPickAvailableGranularity("h", available); ok {
			return item
		}
	}
	if item, ok := systemMonitorPickAvailableGranularity("m", available); ok {
		return item
	}
	if item, ok := systemMonitorPickAvailableGranularity("h", available); ok {
		return item
	}
	return available[len(available)-1]
}

func systemMonitorNearestGranularity(requested string, lookback time.Duration, available []systemMonitorHistoryGranularity) systemMonitorHistoryGranularity {
	order := []string{"m", "h", "d"}
	index := -1
	for i, key := range order {
		if key == requested {
			index = i
			break
		}
	}
	if index < 0 {
		return systemMonitorDefaultGranularity(lookback, available)
	}

	for step := 1; step < len(order); step++ {
		nextIndex := index + step
		if nextIndex < len(order) {
			if item, ok := systemMonitorPickAvailableGranularity(order[nextIndex], available); ok {
				return item
			}
		}
		prevIndex := index - step
		if prevIndex >= 0 {
			if item, ok := systemMonitorPickAvailableGranularity(order[prevIndex], available); ok {
				return item
			}
		}
	}
	return available[len(available)-1]
}

func systemMonitorGranularityKeys(items []systemMonitorHistoryGranularity) []string {
	keys := make([]string, 0, len(items))
	for _, item := range items {
		keys = append(keys, item.Key)
	}
	return keys
}

func systemMonitorPickAvailableGranularity(key string, available []systemMonitorHistoryGranularity) (systemMonitorHistoryGranularity, bool) {
	for _, item := range available {
		if item.Key == key {
			return item, true
		}
	}
	return systemMonitorHistoryGranularity{}, false
}

func systemMonitorGranularityAvailable(target systemMonitorHistoryGranularity, available []systemMonitorHistoryGranularity) bool {
	for _, item := range available {
		if item.Key == target.Key {
			return true
		}
	}
	return false
}

func systemMonitorGranularityByKey(key string) (systemMonitorHistoryGranularity, bool) {
	for _, item := range systemMonitorHistoryGranularities {
		if item.Key == key {
			return item, true
		}
	}
	return systemMonitorHistoryGranularity{}, false
}

func normalizeSystemMonitorGranularityKey(raw string) string {
	switch strings.TrimSpace(strings.ToLower(raw)) {
	case "", "auto", "a", "自动":
		return "auto"
	case "minute", "minutes", "min", "mins", "m", "分", "分钟":
		return "m"
	case "hour", "hours", "h", "时", "小时":
		return "h"
	case "day", "days", "d", "天":
		return "d"
	default:
		return ""
	}
}

func systemMonitorSelectSourceRollup(settings SystemMonitorSettings, lookback time.Duration, granularity systemMonitorHistoryGranularity) (systemMonitorRollupDefinition, error) {
	maxRetention := time.Duration(0)
	for index := len(systemMonitorRollupDefinitions) - 1; index >= 0; index-- {
		def := systemMonitorRollupDefinitions[index]
		retention := systemMonitorRetentionForRollup(settings, def)
		if retention > maxRetention {
			maxRetention = retention
		}
		if def.BucketDuration > granularity.BucketDuration {
			continue
		}
		if lookback > retention {
			continue
		}
		return def, nil
	}
	if maxRetention <= 0 {
		maxRetention = 24 * time.Hour
	}
	return systemMonitorRollupDefinition{}, fmt.Errorf("monitor history exceeds retention (%s)", maxRetention)
}

func aggregateSystemMonitorHistory(rows []systemMonitorRollupRow, bucketDuration time.Duration, location *time.Location) []SystemMonitorHistoryPoint {
	if len(rows) == 0 {
		return []SystemMonitorHistoryPoint{}
	}

	buckets := make(map[int64]*systemMonitorHistoryAggregate)
	orderedStarts := make([]int64, 0)
	for _, row := range rows {
		bucketStart := systemMonitorAggregateBucketStart(time.Unix(row.BucketStart, 0), bucketDuration, location).Unix()
		bucket, exists := buckets[bucketStart]
		if !exists {
			bucket = &systemMonitorHistoryAggregate{
				BucketStart: bucketStart,
			}
			buckets[bucketStart] = bucket
			orderedStarts = append(orderedStarts, bucketStart)
		}

		weight := row.SampleCount
		if weight <= 0 {
			weight = 1
		}
		bucket.SampleCount += weight
		bucket.CPUWeightedSum += float64(row.CPUAvg) * float64(weight)
		bucket.MemoryWeightedSum += float64(row.MemoryAvg) * float64(weight)
		bucket.DiskReadWeighted += float64(row.DiskReadAvg) * float64(weight)
		bucket.DiskWriteWeighted += float64(row.DiskWriteAvg) * float64(weight)
		bucket.NetworkUpWeighted += float64(row.NetworkUpAvg) * float64(weight)
		bucket.NetworkDownWeighted += float64(row.NetworkDownAvg) * float64(weight)

		if row.CPUMax > bucket.CPUPeak {
			bucket.CPUPeak = row.CPUMax
		}
		if row.MemoryMax > bucket.MemoryPeak {
			bucket.MemoryPeak = row.MemoryMax
		}
		if row.DiskReadMax > bucket.DiskReadPeak {
			bucket.DiskReadPeak = row.DiskReadMax
		}
		if row.DiskWriteMax > bucket.DiskWritePeak {
			bucket.DiskWritePeak = row.DiskWriteMax
		}
		if row.NetworkUpMax > bucket.NetworkUpPeak {
			bucket.NetworkUpPeak = row.NetworkUpMax
		}
		if row.NetworkDownMax > bucket.NetworkDownPeak {
			bucket.NetworkDownPeak = row.NetworkDownMax
		}
	}

	sort.Slice(orderedStarts, func(i, j int) bool {
		return orderedStarts[i] < orderedStarts[j]
	})

	points := make([]SystemMonitorHistoryPoint, 0, len(orderedStarts))
	for _, start := range orderedStarts {
		bucket := buckets[start]
		if bucket == nil || bucket.SampleCount <= 0 {
			continue
		}
		points = append(points, SystemMonitorHistoryPoint{
			Timestamp:       bucket.BucketStart,
			CPUAvg:          unscalePercentToFloat(weightedAverageToInt64(bucket.CPUWeightedSum, bucket.SampleCount)),
			CPUPeak:         unscalePercentToFloat(bucket.CPUPeak),
			MemoryAvg:       unscalePercentToFloat(weightedAverageToInt64(bucket.MemoryWeightedSum, bucket.SampleCount)),
			MemoryPeak:      unscalePercentToFloat(bucket.MemoryPeak),
			DiskReadAvg:     scaledInt64ToUint64(weightedAverageToInt64(bucket.DiskReadWeighted, bucket.SampleCount)),
			DiskReadPeak:    scaledInt64ToUint64(bucket.DiskReadPeak),
			DiskWriteAvg:    scaledInt64ToUint64(weightedAverageToInt64(bucket.DiskWriteWeighted, bucket.SampleCount)),
			DiskWritePeak:   scaledInt64ToUint64(bucket.DiskWritePeak),
			NetworkUpAvg:    scaledInt64ToUint64(weightedAverageToInt64(bucket.NetworkUpWeighted, bucket.SampleCount)),
			NetworkUpPeak:   scaledInt64ToUint64(bucket.NetworkUpPeak),
			NetworkDownAvg:  scaledInt64ToUint64(weightedAverageToInt64(bucket.NetworkDownWeighted, bucket.SampleCount)),
			NetworkDownPeak: scaledInt64ToUint64(bucket.NetworkDownPeak),
		})
	}
	return points
}

func weightedAverageToInt64(sum float64, sampleCount int64) int64 {
	if sampleCount <= 0 {
		return 0
	}
	return int64(math.Round(sum / float64(sampleCount)))
}

func systemMonitorAggregateBucketStart(sampledAt time.Time, bucketDuration time.Duration, location *time.Location) time.Time {
	if location == nil {
		location = time.Local
	}
	if bucketDuration <= 0 {
		bucketDuration = time.Minute
	}
	localTime := sampledAt.In(location)
	switch {
	case bucketDuration >= 24*time.Hour:
		return time.Date(localTime.Year(), localTime.Month(), localTime.Day(), 0, 0, 0, 0, location)
	case bucketDuration%time.Hour == 0:
		return localTime.Truncate(bucketDuration)
	case bucketDuration%time.Minute == 0:
		return localTime.Truncate(bucketDuration)
	default:
		bucketSeconds := int64(bucketDuration / time.Second)
		if bucketSeconds <= 0 {
			bucketSeconds = 1
		}
		return time.Unix((sampledAt.Unix()/bucketSeconds)*bucketSeconds, 0).In(location)
	}
}

func systemMonitorHistoryLocation() *time.Location {
	location, err := (&SettingService{}).GetTimeLocation()
	if err != nil || location == nil {
		return time.Local
	}
	return location
}
