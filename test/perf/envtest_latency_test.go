//go:build e2e

package perf

import (
	"os"
	"testing"
	"time"

	"github.com/kplane-dev/kplanetest"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

func TestStartupLatencyGuardrail(t *testing.T) {
	if os.Getenv("KUBEBUILDER_ASSETS") == "" {
		t.Skip("set KUBEBUILDER_ASSETS to run perf suite")
	}
	if os.Getenv("KPLANETEST_PERF") != "1" {
		t.Skip("set KPLANETEST_PERF=1 to run perf guardrails")
	}

	const iterations = 3
	var envtestTotal time.Duration
	var kplaneTotal time.Duration

	for i := 0; i < iterations; i++ {
		start := time.Now()
		e := &envtest.Environment{}
		_, err := e.Start()
		if err != nil {
			t.Fatalf("envtest start: %v", err)
		}
		if err := e.Stop(); err != nil {
			t.Fatalf("envtest stop: %v", err)
		}
		envtestTotal += time.Since(start)
	}

	for i := 0; i < iterations; i++ {
		start := time.Now()
		e := &kplanetest.Environment{}
		_, err := e.Start()
		if err != nil {
			t.Fatalf("kplanetest start: %v", err)
		}
		if err := e.Stop(); err != nil {
			t.Fatalf("kplanetest stop: %v", err)
		}
		kplaneTotal += time.Since(start)
	}

	envtestAvg := envtestTotal / iterations
	kplaneAvg := kplaneTotal / iterations

	// v1 is envtest-backed, so we enforce "no major regression" instead of absolute speedup.
	if kplaneAvg > envtestAvg+(envtestAvg/5) {
		t.Fatalf("startup regression too high: envtest_avg=%s kplanetest_avg=%s", envtestAvg, kplaneAvg)
	}
}
