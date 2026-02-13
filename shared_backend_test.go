package kplanetest

import (
	"testing"

	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

type countingBackend struct {
	startCalls int
	stopCalls  int
	cfg        *rest.Config
}

func (b *countingBackend) Start(_ *envtest.Environment) (*rest.Config, error) {
	b.startCalls++
	return b.cfg, nil
}

func (b *countingBackend) Stop(_ *envtest.Environment) error {
	b.stopCalls++
	return nil
}

func TestSharedEnvtestBackendRefCounting(t *testing.T) {
	delegate := &countingBackend{cfg: &rest.Config{Host: "https://shared"}}
	shared := &SharedEnvtestBackend{
		delegate: delegate,
		state:    &sharedBackendState{},
	}

	envA := &envtest.Environment{}
	envB := &envtest.Environment{}

	cfgA, err := shared.Start(envA)
	if err != nil {
		t.Fatalf("unexpected start error for envA: %v", err)
	}
	cfgB, err := shared.Start(envB)
	if err != nil {
		t.Fatalf("unexpected start error for envB: %v", err)
	}
	if cfgA == nil || cfgB == nil || cfgA.Host != cfgB.Host {
		t.Fatalf("expected shared config for both starts, got cfgA=%#v cfgB=%#v", cfgA, cfgB)
	}
	if delegate.startCalls != 1 {
		t.Fatalf("expected one delegate start call, got %d", delegate.startCalls)
	}

	if err := shared.Stop(envA); err != nil {
		t.Fatalf("unexpected first stop error: %v", err)
	}
	if delegate.stopCalls != 0 {
		t.Fatalf("expected no delegate stop until last reference, got %d", delegate.stopCalls)
	}

	if err := shared.Stop(envB); err != nil {
		t.Fatalf("unexpected second stop error: %v", err)
	}
	if delegate.stopCalls != 1 {
		t.Fatalf("expected one delegate stop on last reference, got %d", delegate.stopCalls)
	}
}

func TestSharedEnvtestBackendStopWithoutStartIsSafe(t *testing.T) {
	shared := &SharedEnvtestBackend{
		delegate: &countingBackend{},
		state:    &sharedBackendState{},
	}
	if err := shared.Stop(&envtest.Environment{}); err != nil {
		t.Fatalf("stop without start should be safe: %v", err)
	}
}
