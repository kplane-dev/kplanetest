//go:build e2e

package perf

import (
	"context"
	"os"
	"strconv"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

func loadExternalKplaneConfig(t *testing.T) *rest.Config {
	t.Helper()
	path := os.Getenv("KPLANETEST_KPLANE_KUBECONFIG")
	if path == "" {
		t.Skip("set KPLANETEST_KPLANE_KUBECONFIG to run kplane external perf comparisons")
	}
	cfg, err := clientcmd.BuildConfigFromFlags("", path)
	if err != nil {
		t.Fatalf("load kplane kubeconfig: %v", err)
	}
	return cfg
}

func measureCRUDIteration(t *testing.T, cfg *rest.Config, prefix string) time.Duration {
	t.Helper()
	clientset, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		t.Fatalf("build clientset: %v", err)
	}
	ctx := context.Background()
	start := time.Now()
	nsName := prefix + "-perf-ns"
	_ = clientset.CoreV1().Namespaces().Delete(ctx, nsName, metav1.DeleteOptions{})

	if _, err := clientset.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: nsName},
	}, metav1.CreateOptions{}); err != nil {
		t.Fatalf("create namespace: %v", err)
	}
	defer func() { _ = clientset.CoreV1().Namespaces().Delete(ctx, nsName, metav1.DeleteOptions{}) }()

	if _, err := clientset.CoreV1().ConfigMaps(nsName).Create(ctx, &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "cm", Namespace: nsName},
	}, metav1.CreateOptions{}); err != nil {
		t.Fatalf("create configmap: %v", err)
	}
	if _, err := clientset.CoreV1().ConfigMaps(nsName).Get(ctx, "cm", metav1.GetOptions{}); err != nil {
		t.Fatalf("get configmap: %v", err)
	}
	if err := clientset.CoreV1().ConfigMaps(nsName).Delete(ctx, "cm", metav1.DeleteOptions{}); err != nil {
		t.Fatalf("delete configmap: %v", err)
	}
	return time.Since(start)
}

func TestKplaneExternalCRUDPerformance(t *testing.T) {
	if os.Getenv("KUBEBUILDER_ASSETS") == "" {
		t.Skip("set KUBEBUILDER_ASSETS to run envtest baseline")
	}
	if os.Getenv("KPLANETEST_PERF") != "1" {
		t.Skip("set KPLANETEST_PERF=1 to run perf comparisons")
	}

	kplaneCfg := loadExternalKplaneConfig(t)
	iterations := readPositiveIntEnvExternal(t, "KPLANETEST_PERF_ITERATIONS", 3)
	budgetPct := readPositiveIntEnvExternal(t, "KPLANETEST_KPLANE_CRUD_BUDGET_PCT", 200)
	enforce := os.Getenv("KPLANETEST_PERF_ENFORCE") == "1"

	var envtestTotal time.Duration
	for i := 0; i < iterations; i++ {
		e := &envtest.Environment{}
		cfg, err := e.Start()
		if err != nil {
			t.Fatalf("envtest start: %v", err)
		}
		envtestTotal += measureCRUDIteration(t, cfg, "envtest")
		if err := e.Stop(); err != nil {
			t.Fatalf("envtest stop: %v", err)
		}
	}

	var kplaneTotal time.Duration
	for i := 0; i < iterations; i++ {
		kplaneTotal += measureCRUDIteration(t, kplaneCfg, "kplane")
	}

	envtestAvg := envtestTotal / time.Duration(iterations)
	kplaneAvg := kplaneTotal / time.Duration(iterations)
	t.Logf("external kplane CRUD avg=%s envtest CRUD avg=%s", kplaneAvg, envtestAvg)

	if enforce {
		allowed := envtestAvg + ((envtestAvg * time.Duration(budgetPct)) / 100)
		if kplaneAvg > allowed {
			t.Fatalf(
				"kplane external CRUD regression too high: kplane_avg=%s envtest_avg=%s allowed=%s budget_pct=%d",
				kplaneAvg,
				envtestAvg,
				allowed,
				budgetPct,
			)
		}
	}
}

func readPositiveIntEnvExternal(t *testing.T, key string, fallback int) int {
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
