package kplanetest

import (
	"errors"
	"testing"
	"time"

	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

type fakeBackend struct {
	startFn func(env *envtest.Environment) (*rest.Config, error)
	stopFn  func(env *envtest.Environment) error
}

func (f *fakeBackend) Start(env *envtest.Environment) (*rest.Config, error) {
	return f.startFn(env)
}

func (f *fakeBackend) Stop(env *envtest.Environment) error {
	return f.stopFn(env)
}

func TestEnvironmentStartStopAndMetrics(t *testing.T) {
	now := time.Now()
	cfg := &rest.Config{Host: "https://example.local"}
	e := &Environment{}
	e.setClockForTesting(func() time.Time {
		now = now.Add(10 * time.Millisecond)
		return now
	})
	e.setBackendForTesting(&fakeBackend{
		startFn: func(_ *envtest.Environment) (*rest.Config, error) { return cfg, nil },
		stopFn:  func(_ *envtest.Environment) error { return nil },
	})

	_, err := e.Start()
	if err != nil {
		t.Fatalf("unexpected start error: %v", err)
	}

	if err := e.Stop(); err != nil {
		t.Fatalf("unexpected stop error: %v", err)
	}

	snap := e.MetricsSnapshot()
	if snap.StartAttempts != 1 || snap.StartSuccesses != 1 {
		t.Fatalf("unexpected start metrics: %#v", snap)
	}
	if snap.StopAttempts != 1 || snap.StopSuccesses != 1 {
		t.Fatalf("unexpected stop metrics: %#v", snap)
	}
}

func TestEnvironmentStartErrorIsRecorded(t *testing.T) {
	boom := errors.New("boom")
	e := &Environment{}
	e.setBackendForTesting(&fakeBackend{
		startFn: func(_ *envtest.Environment) (*rest.Config, error) { return nil, boom },
		stopFn:  func(_ *envtest.Environment) error { return nil },
	})

	_, err := e.Start()
	if err == nil {
		t.Fatal("expected start error")
	}
	if !errors.Is(err, boom) {
		t.Fatalf("expected original start error, got %v", err)
	}

	snap := e.MetricsSnapshot()
	if snap.StartAttempts != 1 || snap.StartSuccesses != 0 {
		t.Fatalf("unexpected start metrics after error: %#v", snap)
	}
}

func TestEnvironmentStopErrorIsRecorded(t *testing.T) {
	boom := errors.New("boom")
	e := &Environment{}
	e.setBackendForTesting(&fakeBackend{
		startFn: func(_ *envtest.Environment) (*rest.Config, error) { return &rest.Config{Host: "https://ok"}, nil },
		stopFn:  func(_ *envtest.Environment) error { return boom },
	})

	if _, err := e.Start(); err != nil {
		t.Fatalf("unexpected start error: %v", err)
	}
	err := e.Stop()
	if err == nil {
		t.Fatal("expected stop error")
	}
	if !errors.Is(err, boom) {
		t.Fatalf("expected original stop error, got %v", err)
	}

	snap := e.MetricsSnapshot()
	if snap.StopAttempts != 1 || snap.StopSuccesses != 0 {
		t.Fatalf("unexpected stop metrics after error: %#v", snap)
	}
}

func TestEmbeddedUpstreamDefinitionIsUsedDirectly(t *testing.T) {
	e := &Environment{
		CRDDirectoryPaths:        []string{"a", "b"},
		ErrorIfCRDPathMissing:    true,
		AttachControlPlaneOutput: true,
	}

	var seenCRDPaths []string
	var seenErrorIfMissing bool
	var seenAttachOutput bool

	e.setBackendForTesting(&fakeBackend{
		startFn: func(env *envtest.Environment) (*rest.Config, error) {
			seenCRDPaths = append(seenCRDPaths, env.CRDDirectoryPaths...)
			seenErrorIfMissing = env.ErrorIfCRDPathMissing
			seenAttachOutput = env.AttachControlPlaneOutput
			return &rest.Config{Host: "https://ok"}, nil
		},
		stopFn: func(_ *envtest.Environment) error { return nil },
	})

	if _, err := e.Start(); err != nil {
		t.Fatalf("unexpected start error: %v", err)
	}
	if len(seenCRDPaths) != 2 || seenCRDPaths[0] != "a" || seenCRDPaths[1] != "b" {
		t.Fatalf("CRDDirectoryPaths not propagated, got %#v", seenCRDPaths)
	}
	if !seenErrorIfMissing {
		t.Fatal("ErrorIfCRDPathMissing not propagated")
	}
	if !seenAttachOutput {
		t.Fatal("AttachControlPlaneOutput not propagated")
	}
}

func TestSetBackendIsUsed(t *testing.T) {
	called := false
	e := &Environment{}
	e.SetBackend(&fakeBackend{
		startFn: func(_ *envtest.Environment) (*rest.Config, error) {
			called = true
			return &rest.Config{Host: "https://field-backend"}, nil
		},
		stopFn: func(_ *envtest.Environment) error { return nil },
	})

	cfg, err := e.Start()
	if err != nil {
		t.Fatalf("unexpected start error: %v", err)
	}
	if !called {
		t.Fatal("expected explicit Backend field to be used")
	}
	if cfg == nil || cfg.Host != "https://field-backend" {
		t.Fatalf("unexpected config from explicit backend: %#v", cfg)
	}
}

func TestResolveBackendUsesSharedBackendWhenFlagEnabled(t *testing.T) {
	t.Setenv(experimentalSharedBackendEnv, "1")
	e := &Environment{}
	backend := stateFor(e).resolveBackend()
	if _, ok := backend.(*SharedEnvtestBackend); !ok {
		t.Fatalf("expected shared backend when %s=1, got %T", experimentalSharedBackendEnv, backend)
	}
}
