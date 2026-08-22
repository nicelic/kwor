package service

import (
	"sort"
	"sync"
	"time"
)

// RuntimePerformanceSample is a bounded, in-memory diagnostic record for
// periodic runtime work. It is intentionally not persisted: the endpoint is
// meant for short-term diagnosis and must not become another hot-path sink.
type RuntimePerformanceSample struct {
	Task       string `json:"task"`
	StartedAt  int64  `json:"startedAt"`
	DurationMs int64  `json:"durationMs"`
	NftCalls   int    `json:"nftCalls,omitempty"`
	DBWrites   int    `json:"dbWrites,omitempty"`
	Pending    int    `json:"pending,omitempty"`
	JournalB   int    `json:"journalBytes,omitempty"`
	Flushed    bool   `json:"flushed,omitempty"`
}

const runtimePerformanceCapacity = 256

var runtimePerformanceState = struct {
	sync.RWMutex
	items []RuntimePerformanceSample
	next  uint64
}{
	items: make([]RuntimePerformanceSample, 0, runtimePerformanceCapacity),
}

// RecordRuntimePerformance appends one bounded diagnostic sample.
func RecordRuntimePerformance(sample RuntimePerformanceSample) {
	if sample.Task == "" {
		return
	}
	if sample.StartedAt == 0 {
		sample.StartedAt = time.Now().Unix()
	}
	if sample.DurationMs < 0 {
		sample.DurationMs = 0
	}
	runtimePerformanceState.Lock()
	if len(runtimePerformanceState.items) < runtimePerformanceCapacity {
		runtimePerformanceState.items = append(runtimePerformanceState.items, sample)
	} else {
		runtimePerformanceState.items[runtimePerformanceState.next%runtimePerformanceCapacity] = sample
		runtimePerformanceState.next++
	}
	runtimePerformanceState.Unlock()
}

// GetRuntimePerformance returns a stable copy ordered from oldest to newest.
func GetRuntimePerformance(limit int) []RuntimePerformanceSample {
	if limit <= 0 || limit > runtimePerformanceCapacity {
		limit = runtimePerformanceCapacity
	}
	runtimePerformanceState.RLock()
	items := append([]RuntimePerformanceSample(nil), runtimePerformanceState.items...)
	next := runtimePerformanceState.next
	runtimePerformanceState.RUnlock()
	if len(items) == 0 {
		return []RuntimePerformanceSample{}
	}
	if len(items) == runtimePerformanceCapacity && next > 0 {
		start := int(next % uint64(len(items)))
		ordered := make([]RuntimePerformanceSample, 0, len(items))
		ordered = append(ordered, items[start:]...)
		ordered = append(ordered, items[:start]...)
		items = ordered
	}
	if len(items) > limit {
		items = items[len(items)-limit:]
	}
	return items
}

// GetRuntimePerformanceSummary provides a compact aggregate useful for
// operators without requiring the full ring buffer response.
func GetRuntimePerformanceSummary() map[string]interface{} {
	items := GetRuntimePerformance(runtimePerformanceCapacity)
	type aggregate struct {
		Count int
		Total int64
		Max   int64
	}
	aggs := make(map[string]aggregate)
	for _, item := range items {
		value := aggs[item.Task]
		value.Count++
		value.Total += item.DurationMs
		if item.DurationMs > value.Max {
			value.Max = item.DurationMs
		}
		aggs[item.Task] = value
	}
	type summary struct {
		Count int   `json:"count"`
		AvgMs int64 `json:"avgMs"`
		MaxMs int64 `json:"maxMs"`
	}
	keys := make([]string, 0, len(aggs))
	for key := range aggs {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make(map[string]interface{}, len(keys))
	for _, key := range keys {
		value := aggs[key]
		avg := int64(0)
		if value.Count > 0 {
			avg = value.Total / int64(value.Count)
		}
		result[key] = summary{Count: value.Count, AvgMs: avg, MaxMs: value.Max}
	}
	return result
}
