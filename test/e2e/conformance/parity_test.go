//go:build e2e

package conformance

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kplane-dev/kplanetest"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

type backendRunner struct {
	name  string
	start func(t *testing.T) (*rest.Config, func())
}

func requireEnvtestAssets(t *testing.T) {
	t.Helper()
	assets := os.Getenv("KUBEBUILDER_ASSETS")
	if assets == "" {
		t.Skip("set KUBEBUILDER_ASSETS to run e2e conformance")
	}
	if _, err := os.Stat(assets); err != nil {
		t.Skipf("KUBEBUILDER_ASSETS path is not accessible: %v", err)
	}
}

func allBackends() []backendRunner {
	return []backendRunner{
		{
			name: "envtest",
			start: func(t *testing.T) (*rest.Config, func()) {
				t.Helper()
				e := &envtest.Environment{}
				cfg, err := e.Start()
				if err != nil {
					t.Fatalf("start envtest: %v", err)
				}
				return cfg, func() {
					if err := e.Stop(); err != nil {
						t.Fatalf("stop envtest: %v", err)
					}
				}
			},
		},
		{
			name: "kplanetest",
			start: func(t *testing.T) (*rest.Config, func()) {
				t.Helper()
				e := &kplanetest.Environment{}
				cfg, err := e.Start()
				if err != nil {
					t.Fatalf("start kplanetest: %v", err)
				}
				return cfg, func() {
					if err := e.Stop(); err != nil {
						t.Fatalf("stop kplanetest: %v", err)
					}
				}
			},
		},
	}
}

func TestParityBasicCRUD(t *testing.T) {
	requireEnvtestAssets(t)
	for _, backend := range allBackends() {
		t.Run(backend.name, func(t *testing.T) {
			cfg, stop := backend.start(t)
			defer stop()

			clientset, err := kubernetes.NewForConfig(cfg)
			if err != nil {
				t.Fatalf("build clientset: %v", err)
			}
			ctx := context.Background()

			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "crud-ns"}}
			if _, err := clientset.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{}); err != nil {
				t.Fatalf("create namespace: %v", err)
			}

			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "cm", Namespace: ns.Name},
				Data:       map[string]string{"key": "v1"},
			}
			if _, err := clientset.CoreV1().ConfigMaps(ns.Name).Create(ctx, cm, metav1.CreateOptions{}); err != nil {
				t.Fatalf("create configmap: %v", err)
			}

			got, err := clientset.CoreV1().ConfigMaps(ns.Name).Get(ctx, "cm", metav1.GetOptions{})
			if err != nil {
				t.Fatalf("get configmap: %v", err)
			}
			if got.Data["key"] != "v1" {
				t.Fatalf("unexpected configmap data: %#v", got.Data)
			}

			got.Data["key"] = "v2"
			if _, err := clientset.CoreV1().ConfigMaps(ns.Name).Update(ctx, got, metav1.UpdateOptions{}); err != nil {
				t.Fatalf("update configmap: %v", err)
			}

			if err := clientset.CoreV1().ConfigMaps(ns.Name).Delete(ctx, got.Name, metav1.DeleteOptions{}); err != nil {
				t.Fatalf("delete configmap: %v", err)
			}
		})
	}
}

func TestParityWatch(t *testing.T) {
	requireEnvtestAssets(t)
	for _, backend := range allBackends() {
		t.Run(backend.name, func(t *testing.T) {
			cfg, stop := backend.start(t)
			defer stop()

			clientset, err := kubernetes.NewForConfig(cfg)
			if err != nil {
				t.Fatalf("build clientset: %v", err)
			}
			ctx := context.Background()
			nsName := "watch-ns"
			if _, err := clientset.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{Name: nsName},
			}, metav1.CreateOptions{}); err != nil {
				t.Fatalf("create namespace: %v", err)
			}

			w, err := clientset.CoreV1().ConfigMaps(nsName).Watch(ctx, metav1.ListOptions{})
			if err != nil {
				t.Fatalf("watch configmaps: %v", err)
			}
			defer w.Stop()

			if _, err := clientset.CoreV1().ConfigMaps(nsName).Create(ctx, &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "watch-cm", Namespace: nsName},
			}, metav1.CreateOptions{}); err != nil {
				t.Fatalf("create configmap: %v", err)
			}

			event := waitForEvent(t, w.ResultChan(), 5*time.Second)
			if event.Type != watch.Added {
				t.Fatalf("expected Added event, got %v", event.Type)
			}
		})
	}
}

func waitForEvent(t *testing.T, ch <-chan watch.Event, timeout time.Duration) watch.Event {
	t.Helper()
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case ev := <-ch:
		return ev
	case <-timer.C:
		t.Fatalf("timed out waiting for watch event after %s", timeout)
		return watch.Event{}
	}
}

func TestKplanetestCRDInstallParity(t *testing.T) {
	requireEnvtestAssets(t)
	crdPath := filepath.Join("..", "..", "..", "testdata", "crds")

	for _, backendName := range []string{"envtest", "kplanetest"} {
		t.Run(backendName, func(t *testing.T) {
			var cfg *rest.Config
			var stop func()
			switch backendName {
			case "envtest":
				e := &envtest.Environment{CRDDirectoryPaths: []string{crdPath}}
				var err error
				cfg, err = e.Start()
				if err != nil {
					t.Fatalf("start envtest: %v", err)
				}
				stop = func() { _ = e.Stop() }
			case "kplanetest":
				e := &kplanetest.Environment{
					Environment: envtest.Environment{
						CRDDirectoryPaths: []string{crdPath},
					},
				}
				var err error
				cfg, err = e.Start()
				if err != nil {
					t.Fatalf("start kplanetest: %v", err)
				}
				stop = func() { _ = e.Stop() }
			default:
				t.Fatalf("unexpected backend: %s", backendName)
			}
			defer stop()

			verifyCRDApisReachable(t, cfg)
		})
	}
}

func verifyCRDApisReachable(t *testing.T, cfg *rest.Config) {
	t.Helper()
	discovery, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		t.Fatalf("build discovery client: %v", err)
	}
	_, err = discovery.Discovery().ServerResourcesForGroupVersion("testing.kplanetest.io/v1alpha1")
	if err != nil {
		t.Fatalf("discover installed CRD group version: %v", err)
	}
}

func TestParityNamespaceIsolation(t *testing.T) {
	requireEnvtestAssets(t)
	for _, backend := range allBackends() {
		t.Run(backend.name, func(t *testing.T) {
			cfg, stop := backend.start(t)
			defer stop()

			clientset, err := kubernetes.NewForConfig(cfg)
			if err != nil {
				t.Fatalf("build clientset: %v", err)
			}
			ctx := context.Background()

			for _, nsName := range []string{"ns-a", "ns-b"} {
				if _, err := clientset.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{Name: nsName},
				}, metav1.CreateOptions{}); err != nil {
					t.Fatalf("create namespace %s: %v", nsName, err)
				}
				if _, err := clientset.CoreV1().ConfigMaps(nsName).Create(ctx, &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{Name: "same-name", Namespace: nsName},
					Data:       map[string]string{"ns": nsName},
				}, metav1.CreateOptions{}); err != nil {
					t.Fatalf("create configmap in %s: %v", nsName, err)
				}
			}

			for _, nsName := range []string{"ns-a", "ns-b"} {
				cm, err := clientset.CoreV1().ConfigMaps(nsName).Get(ctx, "same-name", metav1.GetOptions{})
				if err != nil {
					t.Fatalf("get configmap in %s: %v", nsName, err)
				}
				if got := cm.Data["ns"]; got != nsName {
					t.Fatalf("namespace isolation failed: expected %s, got %s", nsName, got)
				}
			}
		})
	}
}

func TestParitySchemeRoundTrip(t *testing.T) {
	requireEnvtestAssets(t)
	for _, backend := range allBackends() {
		t.Run(backend.name, func(t *testing.T) {
			cfg, stop := backend.start(t)
			defer stop()

			scheme := runtime.NewScheme()
			if err := corev1.AddToScheme(scheme); err != nil {
				t.Fatalf("add corev1 to scheme: %v", err)
			}

			clientset, err := kubernetes.NewForConfig(cfg)
			if err != nil {
				t.Fatalf("build clientset: %v", err)
			}
			ctx := context.Background()
			nsName := fmt.Sprintf("scheme-%s", backend.name)
			if _, err := clientset.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{Name: nsName},
			}, metav1.CreateOptions{}); err != nil {
				t.Fatalf("create namespace: %v", err)
			}
		})
	}
}
