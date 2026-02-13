//go:build e2e

package conformance

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kplane-dev/kplanetest"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/discovery"
	"sigs.k8s.io/yaml"
)

const debugCRDEnv = "KPLANETEST_DEBUG_CRD"

func TestDebugKplaneCRDRegistration(t *testing.T) {
	requireEnvtestAssets(t)
	if os.Getenv(debugCRDEnv) != "1" {
		t.Skipf("set %s=1 to run CRD registration debug test", debugCRDEnv)
	}

	crdPath := filepath.Join("..", "..", "..", "testdata", "crds", "testing.kplanetest.io_testwidgets.yaml")
	crd := readCRDFromFile(t, crdPath)

	env := &kplanetest.Environment{AttachControlPlaneOutput: true}
	cfg, err := env.Start()
	if err != nil {
		t.Fatalf("start kplanetest env: %v", err)
	}
	defer func() { _ = env.Stop() }()

	apix, err := apiextensionsclient.NewForConfig(cfg)
	if err != nil {
		t.Fatalf("build apiextensions client: %v", err)
	}
	disc, err := discovery.NewDiscoveryClientForConfig(cfg)
	if err != nil {
		t.Fatalf("build discovery client: %v", err)
	}

	ctx := context.Background()
	_ = apix.ApiextensionsV1().CustomResourceDefinitions().Delete(ctx, crd.Name, metav1.DeleteOptions{})
	created, err := apix.ApiextensionsV1().CustomResourceDefinitions().Create(ctx, crd, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("create CRD %s: %v", crd.Name, err)
	}
	t.Logf("created CRD %s generation=%d", created.Name, created.Generation)

	pollCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	err = wait.PollUntilContextCancel(pollCtx, 250*time.Millisecond, true, func(ctx context.Context) (bool, error) {
		got, getErr := apix.ApiextensionsV1().CustomResourceDefinitions().Get(ctx, crd.Name, metav1.GetOptions{})
		if getErr != nil {
			if apierrors.IsNotFound(getErr) {
				return false, nil
			}
			return false, getErr
		}

		established := false
		namesAccepted := false
		for _, c := range got.Status.Conditions {
			if c.Type == apiextensionsv1.Established && c.Status == apiextensionsv1.ConditionTrue {
				established = true
			}
			if c.Type == apiextensionsv1.NamesAccepted && c.Status == apiextensionsv1.ConditionTrue {
				namesAccepted = true
			}
		}
		if established && namesAccepted {
			t.Logf("CRD status ready established=%t namesAccepted=%t", established, namesAccepted)
			return true, nil
		}
		t.Logf("CRD conditions not ready yet: %#v", got.Status.Conditions)
		return false, nil
	})
	if err != nil {
		t.Fatalf("wait for CRD conditions: %v", err)
	}

	if _, err := disc.ServerResourcesForGroupVersion("testing.kplanetest.io/v1alpha1"); err != nil {
		groups, groupsErr := disc.ServerGroups()
		if groupsErr != nil {
			t.Fatalf("discover CRD group version failed: %v; servergroups query also failed: %v", err, groupsErr)
		}
		t.Fatalf("discover CRD group version failed: %v; known groups=%d", err, len(groups.Groups))
	}
}

func readCRDFromFile(t *testing.T, path string) *apiextensionsv1.CustomResourceDefinition {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read CRD file %s: %v", path, err)
	}
	var crd apiextensionsv1.CustomResourceDefinition
	if err := yaml.Unmarshal(raw, &crd); err != nil {
		t.Fatalf("parse CRD yaml %s: %v", path, err)
	}
	return &crd
}
