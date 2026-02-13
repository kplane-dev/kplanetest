//go:build e2e

package conformance

import (
	"context"
	"os"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

const kplaneKubeconfigEnv = "KPLANETEST_KPLANE_KUBECONFIG"

func loadKplaneExternalConfig(t *testing.T) *rest.Config {
	t.Helper()
	path := os.Getenv(kplaneKubeconfigEnv)
	if path == "" {
		t.Skipf("set %s to run external kplane conformance", kplaneKubeconfigEnv)
	}
	cfg, err := clientcmd.BuildConfigFromFlags("", path)
	if err != nil {
		t.Fatalf("load kubeconfig from %s: %v", path, err)
	}
	return cfg
}

func TestKplaneExternalCRUD(t *testing.T) {
	cfg := loadKplaneExternalConfig(t)
	clientset, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		t.Fatalf("build clientset: %v", err)
	}

	ctx := context.Background()
	nsName := "kplanetest-external-conformance"
	_ = clientset.CoreV1().Namespaces().Delete(ctx, nsName, metav1.DeleteOptions{})

	if _, err := clientset.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: nsName},
	}, metav1.CreateOptions{}); err != nil {
		t.Fatalf("create namespace: %v", err)
	}
	defer func() { _ = clientset.CoreV1().Namespaces().Delete(ctx, nsName, metav1.DeleteOptions{}) }()

	cmName := "kplanetest-cm"
	if _, err := clientset.CoreV1().ConfigMaps(nsName).Create(ctx, &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: cmName, Namespace: nsName},
		Data:       map[string]string{"mode": "external"},
	}, metav1.CreateOptions{}); err != nil {
		t.Fatalf("create configmap: %v", err)
	}

	got, err := clientset.CoreV1().ConfigMaps(nsName).Get(ctx, cmName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get configmap: %v", err)
	}
	if got.Data["mode"] != "external" {
		t.Fatalf("unexpected configmap payload: %#v", got.Data)
	}
}
