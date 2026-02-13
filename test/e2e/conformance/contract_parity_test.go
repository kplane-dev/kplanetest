//go:build e2e

package conformance

import (
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
				return &kplanetest.Environment{}
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
