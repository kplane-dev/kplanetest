package kplanetest

import (
	"sort"
	"sync"
	"time"
)

// MetricsSnapshot is a point-in-time view of observed lifecycle metrics.
type MetricsSnapshot struct {
	StartAttempts  int
	StartSuccesses int
	StopAttempts   int
	StopSuccesses  int
	StartP50       time.Duration
	StartP95       time.Duration
	StopP50        time.Duration
	StopP95        time.Duration
}

// Metrics stores lifecycle timings in-memory for tests and assertions.
type Metrics struct {
	mu             sync.Mutex
	startAttempts  int
	startSuccesses int
	stopAttempts   int
	stopSuccesses  int
	startDurations []time.Duration
	stopDurations  []time.Duration
}

func NewMetrics() *Metrics {
	return &Metrics{}
}

func (m *Metrics) ObserveStart(d time.Duration, success bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.startAttempts++
	if success {
		m.startSuccesses++
	}
	m.startDurations = append(m.startDurations, d)
}

func (m *Metrics) ObserveStop(d time.Duration, success bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stopAttempts++
	if success {
		m.stopSuccesses++
	}
	m.stopDurations = append(m.stopDurations, d)
}

func percentile(values []time.Duration, p float64) time.Duration {
	if len(values) == 0 {
		return 0
	}
	cp := append([]time.Duration(nil), values...)
	sort.Slice(cp, func(i, j int) bool { return cp[i] < cp[j] })
	idx := int(float64(len(cp)-1) * p)
	return cp[idx]
}

// Snapshot returns counters and percentile timings for lifecycle operations.
func (m *Metrics) Snapshot() MetricsSnapshot {
	m.mu.Lock()
	defer m.mu.Unlock()

	return MetricsSnapshot{
		StartAttempts:  m.startAttempts,
		StartSuccesses: m.startSuccesses,
		StopAttempts:   m.stopAttempts,
		StopSuccesses:  m.stopSuccesses,
		StartP50:       percentile(m.startDurations, 0.50),
		StartP95:       percentile(m.startDurations, 0.95),
		StopP50:        percentile(m.stopDurations, 0.50),
		StopP95:        percentile(m.stopDurations, 0.95),
	}
}
