//go:build e2e

package conformance

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/kplane-dev/kplanetest"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

type contractEnvironment interface {
	Start() (*rest.Config, error)
	Stop() error
	AddUser(user envtest.User, baseConfig *rest.Config) (*envtest.AuthenticatedUser, error)
}

type contractFactory struct {
	name string
	new  func() contractEnvironment
}

func allContractFactories() []contractFactory {
	return []contractFactory{
		{
			name: "envtest",
			new: func() contractEnvironment {
				return &envtest.Environment{}
			},
		},
		{
			name: "kplanetest",
			new: func() contractEnvironment {
				e := &kplanetest.Environment{}
				e.SetBackend(kplanetest.EnvtestBackend{})
				return e
			},
		},
	}
}

type lifecycleOutcome struct {
	FirstStartErr       bool
	SecondStartErr      bool
	FirstStopErr        bool
	SecondStopErr       bool
	FirstConfigNil      bool
	SecondConfigNil     bool
	FirstSecondHostSame bool
}

func captureLifecycleOutcome(t *testing.T, env contractEnvironment) lifecycleOutcome {
	t.Helper()
	firstCfg, firstErr := env.Start()
	secondCfg, secondErr := env.Start()
	firstStopErr := env.Stop()
	secondStopErr := env.Stop()

	outcome := lifecycleOutcome{
		FirstStartErr:   firstErr != nil,
		SecondStartErr:  secondErr != nil,
		FirstStopErr:    firstStopErr != nil,
		SecondStopErr:   secondStopErr != nil,
		FirstConfigNil:  firstCfg == nil,
		SecondConfigNil: secondCfg == nil,
	}
	if firstCfg != nil && secondCfg != nil {
		outcome.FirstSecondHostSame = firstCfg.Host == secondCfg.Host
	}
	return outcome
}

func TestContractLifecycleParity(t *testing.T) {
	requireEnvtestAssets(t)
	factories := allContractFactories()
	if len(factories) < 2 {
		t.Fatal("expected at least two contract factories")
	}

	var oracle lifecycleOutcome
	for i, factory := range factories {
		outcome := captureLifecycleOutcome(t, factory.new())
		if i == 0 {
			oracle = outcome
			continue
		}
		if !reflect.DeepEqual(outcome, oracle) {
			t.Fatalf("lifecycle parity mismatch for %s: got=%+v want=%+v", factory.name, outcome, oracle)
		}
	}
}

type addUserOutcome struct {
	StartErr           bool
	AddUserErr         bool
	ReturnedUserNil    bool
	ReturnedConfigNil  bool
	HostMatchesBaseCfg bool
}

func captureAddUserOutcome(t *testing.T, env contractEnvironment) addUserOutcome {
	t.Helper()
	cfg, startErr := env.Start()
	if startErr != nil {
		return addUserOutcome{StartErr: true}
	}
	defer func() { _ = env.Stop() }()

	base := &rest.Config{QPS: 88, Burst: 144}
	authUser, addErr := env.AddUser(envtest.User{
		Name:   "contract-user",
		Groups: []string{"system:masters"},
	}, base)

	outcome := addUserOutcome{
		AddUserErr:      addErr != nil,
		ReturnedUserNil: authUser == nil,
	}
	if authUser != nil {
		userCfg := authUser.Config()
		outcome.ReturnedConfigNil = userCfg == nil
		if userCfg != nil && cfg != nil {
			outcome.HostMatchesBaseCfg = userCfg.Host == cfg.Host
		}
	}
	return outcome
}

func TestContractAddUserParity(t *testing.T) {
	requireEnvtestAssets(t)
	factories := allContractFactories()
	if len(factories) < 2 {
		t.Fatal("expected at least two contract factories")
	}

	var oracle addUserOutcome
	for i, factory := range factories {
		outcome := captureAddUserOutcome(t, factory.new())
		if i == 0 {
			oracle = outcome
			continue
		}
		if !reflect.DeepEqual(outcome, oracle) {
			t.Fatalf("adduser parity mismatch for %s: got=%+v want=%+v", factory.name, outcome, oracle)
		}
	}
}

func TestContractMissingCRDPathStrictModeParity(t *testing.T) {
	requireEnvtestAssets(t)
	missingPath := filepath.Join(t.TempDir(), "definitely-missing")
	factories := []struct {
		name string
		new  func() contractEnvironment
	}{
		{
			name: "envtest",
			new: func() contractEnvironment {
				return &envtest.Environment{
					CRDDirectoryPaths:     []string{missingPath},
					ErrorIfCRDPathMissing: true,
				}
			},
		},
		{
			name: "kplanetest",
			new: func() contractEnvironment {
				e := &kplanetest.Environment{
					CRDDirectoryPaths:     []string{missingPath},
					ErrorIfCRDPathMissing: true,
				}
				e.SetBackend(kplanetest.EnvtestBackend{})
				return e
			},
		},
	}

	var oracle bool
	for i, factory := range factories {
		_, err := factory.new().Start()
		gotErr := err != nil
		if i == 0 {
			oracle = gotErr
			continue
		}
		if gotErr != oracle {
			t.Fatalf("missing CRD path strict mode parity mismatch for %s: gotErr=%t wantErr=%t", factory.name, gotErr, oracle)
		}
	}
}

func TestContractUseExistingClusterParity(t *testing.T) {
	useExisting := true
	factories := []struct {
		name string
		new  func() contractEnvironment
	}{
		{
			name: "envtest",
			new: func() contractEnvironment {
				return &envtest.Environment{UseExistingCluster: &useExisting}
			},
		},
		{
			name: "kplanetest",
			new: func() contractEnvironment {
				e := &kplanetest.Environment{UseExistingCluster: &useExisting}
				e.SetBackend(kplanetest.EnvtestBackend{})
				return e
			},
		},
	}

	var oracle struct {
		hasErr bool
		cfgNil bool
	}
	for i, factory := range factories {
		cfg, err := factory.new().Start()
		outcome := struct {
			hasErr bool
			cfgNil bool
		}{
			hasErr: err != nil,
			cfgNil: cfg == nil,
		}
		if i == 0 {
			oracle = outcome
			continue
		}
		if !reflect.DeepEqual(outcome, oracle) {
			t.Fatalf("use existing cluster parity mismatch for %s: got=%+v want=%+v", factory.name, outcome, oracle)
		}
	}
}
