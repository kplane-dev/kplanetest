//go:build e2e

package perf

import (
	"os"
	"strconv"
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

	iterations := readPositiveIntEnv(t, "KPLANETEST_PERF_ITERATIONS", 3)
	budgetPct := readPositiveIntEnv(t, "KPLANETEST_STARTUP_BUDGET_PCT", 20)
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

	envtestAvg := envtestTotal / time.Duration(iterations)
	kplaneAvg := kplaneTotal / time.Duration(iterations)

	// v1 is envtest-backed, so we enforce "no major regression" instead of absolute speedup.
	allowed := envtestAvg + ((envtestAvg * time.Duration(budgetPct)) / 100)
	if kplaneAvg > allowed {
		t.Fatalf(
			"startup regression too high: envtest_avg=%s kplanetest_avg=%s allowed=%s budget_pct=%d",
			envtestAvg,
			kplaneAvg,
			allowed,
			budgetPct,
		)
	}
}

func readPositiveIntEnv(t *testing.T, key string, fallback int) int {
	t.Helper()
	raw := os.Getenv(key)
	if raw == "" {
		return fallback
	}
	v, err := strconv.Atoi(raw)
	if err != nil || v <= 0 {
		t.Fatalf("invalid %s value %q: expected positive integer", key, raw)
	}
	return v
}
