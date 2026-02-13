//go:build e2e

package perf

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/kplane-dev/kplanetest"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

const (
	perfScenarioEnv    = "KPLANETEST_PERF_SCENARIO"
	perfBatchEnv       = "KPLANETEST_PERF_BATCH"
	perfAssertWinEnv   = "KPLANETEST_PERF_ASSERT_WIN"
	perfWorkerTimeout  = "KPLANETEST_PERF_WORKER_TIMEOUT"
	defaultPerfBatch   = 10
	defaultWorkerLimit = 45 * time.Second
	resultPrefix       = "KPLANETEST_PERF_RESULT:"
	scenarioEnvtest    = "envtest"
	scenarioKplane     = "kplane"
)

type comparisonResult struct {
	Scenario           string        `json:"scenario"`
	BatchSize          int           `json:"batch_size"`
	Elapsed            time.Duration `json:"elapsed_ns"`
	GoAllocBytesDelta  uint64        `json:"go_alloc_bytes_delta"`
	SelfUserCPUUsec    int64         `json:"self_user_cpu_usec"`
	SelfSystemCPUUsec  int64         `json:"self_system_cpu_usec"`
	SelfMaxRSSKB       int64         `json:"self_maxrss_kb"`
	ChildUserCPUUsec   int64         `json:"child_user_cpu_usec"`
	ChildSystemCPUUsec int64         `json:"child_system_cpu_usec"`
	ChildMaxRSSKB      int64         `json:"child_maxrss_kb"`
	TotalCPUUsec       int64         `json:"total_cpu_usec"`
	CombinedMaxRSSKB   int64         `json:"combined_maxrss_kb"`
}

func TestPackageLevelEnvComparison(t *testing.T) {
	if os.Getenv("KUBEBUILDER_ASSETS") == "" {
		t.Skip("set KUBEBUILDER_ASSETS to run package-level comparison")
	}
	if os.Getenv("KPLANETEST_PERF") != "1" {
		t.Skip("set KPLANETEST_PERF=1 to run package-level comparison")
	}
	if os.Getenv(perfScenarioEnv) != "" {
		t.Skip("worker mode handled by TestPackageLevelEnvComparisonWorker")
	}

	batch := adjustedBatchForDeadline(t, readPositiveIntEnv(t, perfBatchEnv, defaultPerfBatch))
	if batch == 0 {
		t.Skip("insufficient test timeout budget for package-level comparison; increase go test -timeout or reduce KPLANETEST_PERF_BATCH")
	}
	envtestResult := runWorkerScenario(t, scenarioEnvtest, batch)
	kplaneResult := runWorkerScenario(t, scenarioKplane, batch)

	t.Logf("package-level comparison envtest=%+v", envtestResult)
	t.Logf("package-level comparison kplane=%+v", kplaneResult)

	// Always report whether kplane wins for visibility.
	t.Logf(
		"package-level wins: time=%t totalCPU=%t combinedRSS=%t",
		kplaneResult.Elapsed < envtestResult.Elapsed,
		kplaneResult.TotalCPUUsec < envtestResult.TotalCPUUsec,
		kplaneResult.CombinedMaxRSSKB < envtestResult.CombinedMaxRSSKB,
	)

	if os.Getenv(perfAssertWinEnv) == "1" {
		if kplaneResult.Elapsed >= envtestResult.Elapsed {
			t.Fatalf("expected kplane elapsed to win: kplane=%s envtest=%s", kplaneResult.Elapsed, envtestResult.Elapsed)
		}
		kplaneCPU := kplaneResult.TotalCPUUsec
		envtestCPU := envtestResult.TotalCPUUsec
		if kplaneCPU >= envtestCPU {
			t.Fatalf("expected kplane total cpu to win: kplane=%d envtest=%d", kplaneCPU, envtestCPU)
		}
		if kplaneResult.CombinedMaxRSSKB >= envtestResult.CombinedMaxRSSKB {
			t.Fatalf("expected kplane combined rss to win: kplane=%dKB envtest=%dKB", kplaneResult.CombinedMaxRSSKB, envtestResult.CombinedMaxRSSKB)
		}
	}
}

func TestPackageLevelEnvComparisonWorker(t *testing.T) {
	scenario := os.Getenv(perfScenarioEnv)
	if scenario == "" {
		t.Skip("worker mode not requested")
	}

	batch := readPositiveIntEnv(t, perfBatchEnv, defaultPerfBatch)
	result, err := runScenarioOnce(t, scenario, batch)
	if err != nil {
		t.Fatalf("run scenario %s: %v", scenario, err)
	}
	raw, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal result: %v", err)
	}
	fmt.Printf("%s%s\n", resultPrefix, string(raw))
}

func runWorkerScenario(t *testing.T, scenario string, batch int) comparisonResult {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("resolve test binary path: %v", err)
	}
	timeout := workerTimeoutForTest(t)
	if timeout <= 2*time.Second {
		t.Skipf("insufficient worker timeout budget (%s); increase go test -timeout or reduce KPLANETEST_PERF_BATCH", timeout)
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, exe, "-test.run", "^TestPackageLevelEnvComparisonWorker$", "-test.v")
	cmd.Env = append(os.Environ(),
		"KPLANETEST_PERF=1",
		perfScenarioEnv+"="+scenario,
		perfBatchEnv+"="+strconv.Itoa(batch),
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			t.Skipf(
				"worker scenario %s timed out after %s (batch=%d). increase go test -timeout, lower KPLANETEST_PERF_BATCH, or set %s",
				scenario, timeout, batch, perfWorkerTimeout,
			)
		}
		t.Fatalf("worker scenario %s failed: %v\n%s", scenario, err, string(out))
	}

	var line string
	for _, candidate := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(candidate, resultPrefix) {
			line = strings.TrimPrefix(candidate, resultPrefix)
		}
	}
	if line == "" {
		t.Fatalf("worker scenario %s produced no result line\n%s", scenario, string(out))
	}
	var result comparisonResult
	if err := json.Unmarshal([]byte(line), &result); err != nil {
		t.Fatalf("parse worker scenario %s result: %v", scenario, err)
	}
	return result
}

func runScenarioOnce(t *testing.T, scenario string, batch int) (comparisonResult, error) {
	t.Helper()
	var beforeMem runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&beforeMem)

	beforeChildren, err := getRusageChildren()
	if err != nil {
		return comparisonResult{}, fmt.Errorf("read child rusage before: %w", err)
	}
	beforeSelf, err := getRusageSelf()
	if err != nil {
		return comparisonResult{}, fmt.Errorf("read self rusage before: %w", err)
	}

	start := time.Now()
	switch scenario {
	case scenarioEnvtest:
		if err := runBatchEnvtest(t, batch); err != nil {
			return comparisonResult{}, err
		}
	case scenarioKplane:
		if err := runBatchKplane(t, batch); err != nil {
			return comparisonResult{}, err
		}
	default:
		return comparisonResult{}, fmt.Errorf("unknown scenario %q", scenario)
	}
	elapsed := time.Since(start)

	var afterMem runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&afterMem)

	afterChildren, err := getRusageChildren()
	if err != nil {
		return comparisonResult{}, fmt.Errorf("read child rusage after: %w", err)
	}
	afterSelf, err := getRusageSelf()
	if err != nil {
		return comparisonResult{}, fmt.Errorf("read self rusage after: %w", err)
	}

	selfUser := timevalToUsec(afterSelf.Utime) - timevalToUsec(beforeSelf.Utime)
	selfSystem := timevalToUsec(afterSelf.Stime) - timevalToUsec(beforeSelf.Stime)
	childUser := timevalToUsec(afterChildren.Utime) - timevalToUsec(beforeChildren.Utime)
	childSystem := timevalToUsec(afterChildren.Stime) - timevalToUsec(beforeChildren.Stime)

	selfMax := maxRSSKB(afterSelf.Maxrss)
	childMax := maxRSSKB(afterChildren.Maxrss)
	combinedMax := selfMax
	if childMax > combinedMax {
		combinedMax = childMax
	}

	return comparisonResult{
		Scenario:           scenario,
		BatchSize:          batch,
		Elapsed:            elapsed,
		GoAllocBytesDelta:  afterMem.TotalAlloc - beforeMem.TotalAlloc,
		SelfUserCPUUsec:    selfUser,
		SelfSystemCPUUsec:  selfSystem,
		SelfMaxRSSKB:       selfMax,
		ChildUserCPUUsec:   childUser,
		ChildSystemCPUUsec: childSystem,
		ChildMaxRSSKB:      childMax,
		TotalCPUUsec:       selfUser + selfSystem + childUser + childSystem,
		CombinedMaxRSSKB:   combinedMax,
	}, nil
}

func runBatchEnvtest(t *testing.T, batch int) error {
	t.Helper()
	return runComparableBatch(t, batch, "perf-envtest", func() startStopEnvironment {
		return &envtest.Environment{}
	})
}

func runBatchKplane(t *testing.T, batch int) error {
	t.Helper()
	return runComparableBatch(t, batch, "perf-kplane", func() startStopEnvironment {
		return &kplanetest.Environment{}
	})
}

type startStopEnvironment interface {
	Start() (*rest.Config, error)
	Stop() error
}

func runComparableBatch(t *testing.T, batch int, nsPrefix string, newEnvironment func() startStopEnvironment) error {
	t.Helper()
	envs := make([]startStopEnvironment, 0, batch)
	clients := make([]*kubernetes.Clientset, 0, batch)
	for i := 0; i < batch; i++ {
		e := newEnvironment()
		cfg, err := e.Start()
		if err != nil {
			return fmt.Errorf("start index %d: %w", i, err)
		}
		cs, err := kubernetes.NewForConfig(cfg)
		if err != nil {
			_ = e.Stop()
			return fmt.Errorf("client index %d: %w", i, err)
		}
		envs = append(envs, e)
		clients = append(clients, cs)
	}
	ctx := context.Background()
	for i := range clients {
		ns := fmt.Sprintf("%s-%d", nsPrefix, i)
		_ = clients[i].CoreV1().Namespaces().Delete(ctx, ns, metav1.DeleteOptions{})
		if _, err := clients[i].CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: ns},
		}, metav1.CreateOptions{}); err != nil {
			return fmt.Errorf("create namespace index %d: %w", i, err)
		}
	}
	for i := range envs {
		if err := envs[i].Stop(); err != nil {
			return fmt.Errorf("stop index %d: %w", i, err)
		}
	}
	return nil
}

func getRusageChildren() (*syscall.Rusage, error) {
	var usage syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_CHILDREN, &usage); err != nil {
		return nil, err
	}
	return &usage, nil
}

func getRusageSelf() (*syscall.Rusage, error) {
	var usage syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &usage); err != nil {
		return nil, err
	}
	return &usage, nil
}

func timevalToUsec(tv syscall.Timeval) int64 {
	return tv.Sec*1_000_000 + int64(tv.Usec)
}

func adjustedBatchForDeadline(t *testing.T, batch int) int {
	t.Helper()
	deadline, ok := t.Deadline()
	if !ok {
		return batch
	}
	remaining := time.Until(deadline)
	switch {
	case remaining < 12*time.Second:
		return 0
	case remaining < 20*time.Second && batch > 1:
		t.Logf("short test timeout budget (%s); reducing KPLANETEST_PERF_BATCH from %d to 1", remaining.Round(time.Second), batch)
		return 1
	case remaining < 40*time.Second && batch > 2:
		t.Logf("limited test timeout budget (%s); reducing KPLANETEST_PERF_BATCH from %d to 2", remaining.Round(time.Second), batch)
		return 2
	default:
		return batch
	}
}

func workerTimeoutForTest(t *testing.T) time.Duration {
	t.Helper()
	workerTimeout := defaultWorkerLimit
	if raw := strings.TrimSpace(os.Getenv(perfWorkerTimeout)); raw != "" {
		if parsed, err := time.ParseDuration(raw); err == nil && parsed > 0 {
			workerTimeout = parsed
		}
	}
	if deadline, ok := t.Deadline(); ok {
		remaining := time.Until(deadline) - 2*time.Second
		if remaining < workerTimeout {
			return remaining / 2
		}
	}
	return workerTimeout
}

func maxRSSKB(maxrss int64) int64 {
	// On Darwin ru_maxrss is bytes; on Linux it is kilobytes.
	if runtime.GOOS == "darwin" {
		return maxrss / 1024
	}
	return maxrss
}
