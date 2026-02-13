//go:build e2e

package conformance

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/kplane-dev/kplanetest"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const internalKplaneE2EEnv = "KPLANETEST_INTERNAL_E2E"

func requireInternalKplaneE2E(t *testing.T) {
	t.Helper()
	if os.Getenv(internalKplaneE2EEnv) != "1" {
		t.Skipf("set %s=1 to run internal kplane backend e2e tests", internalKplaneE2EEnv)
	}
}

func TestInternalKplaneDefaultBackendCRUD(t *testing.T) {
	requireInternalKplaneE2E(t)
	env := &kplanetest.Environment{}
	cfg, err := env.Start()
	if err != nil {
		t.Fatalf("start kplanetest default backend: %v", err)
	}
	defer func() { _ = env.Stop() }()

	clientset, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		t.Fatalf("build clientset: %v", err)
	}
	ctx := context.Background()
	nsName := "kpt-internal-crud"
	_ = clientset.CoreV1().Namespaces().Delete(ctx, nsName, metav1.DeleteOptions{})
	if _, err := clientset.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: nsName},
	}, metav1.CreateOptions{}); err != nil {
		t.Fatalf("create namespace: %v", err)
	}
	defer func() { _ = clientset.CoreV1().Namespaces().Delete(ctx, nsName, metav1.DeleteOptions{}) }()

	if _, err := clientset.CoreV1().ConfigMaps(nsName).Create(ctx, &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "cm", Namespace: nsName},
		Data:       map[string]string{"backend": "kplane"},
	}, metav1.CreateOptions{}); err != nil {
		t.Fatalf("create configmap: %v", err)
	}
}

func TestInternalKplaneSharedControlPlane(t *testing.T) {
	requireInternalKplaneE2E(t)
	envA := &kplanetest.Environment{}
	envB := &kplanetest.Environment{}

	cfgA, err := envA.Start()
	if err != nil {
		t.Fatalf("start envA: %v", err)
	}
	cfgB, err := envB.Start()
	if err != nil {
		_ = envA.Stop()
		t.Fatalf("start envB: %v", err)
	}
	defer func() {
		_ = envB.Stop()
		_ = envA.Stop()
	}()

	if cfgA.Host != cfgB.Host {
		// Each environment gets its own virtual cluster path on the same shared server.
		if !strings.Contains(cfgA.Host, "/clusters/") || !strings.Contains(cfgB.Host, "/clusters/") {
			t.Fatalf("expected cluster-routed hosts, got envA=%s envB=%s", cfgA.Host, cfgB.Host)
		}
		baseA := strings.Split(cfgA.Host, "/clusters/")[0]
		baseB := strings.Split(cfgB.Host, "/clusters/")[0]
		if baseA != baseB {
			t.Fatalf("expected shared apiserver base URL, got envA=%s envB=%s", cfgA.Host, cfgB.Host)
		}
	} else {
		t.Fatalf("expected unique cluster host paths per environment, got envA=%s envB=%s", cfgA.Host, cfgB.Host)
	}

	clientB, err := kubernetes.NewForConfig(cfgB)
	if err != nil {
		t.Fatalf("build client B: %v", err)
	}
	if err := envA.Stop(); err != nil {
		t.Fatalf("stop envA: %v", err)
	}

	// envB should keep working until it is explicitly stopped.
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	nsName := "kpt-shared-after-stop-a"
	_ = clientB.CoreV1().Namespaces().Delete(ctx, nsName, metav1.DeleteOptions{})
	if _, err := clientB.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: nsName},
	}, metav1.CreateOptions{}); err != nil {
		t.Fatalf("envB should still function after envA stop: %v", err)
	}
}
