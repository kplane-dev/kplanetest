package perf

import (
	"time"

	"testing"

	"github.com/kplane-dev/kplanetest"
)

func BenchmarkMetricsRecording(b *testing.B) {
	b.ReportAllocs()
	m := kplanetest.NewMetrics()

	for i := 0; i < b.N; i++ {
		m.ObserveStart(5*time.Millisecond, true)
		m.ObserveStop(3*time.Millisecond, true)
		_ = m.Snapshot()
	}
}
