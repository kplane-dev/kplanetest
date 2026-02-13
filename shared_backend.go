package kplanetest

import (
	"sync"

	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

var defaultSharedBackend = NewSharedEnvtestBackend()

type sharedBackendState struct {
	mu         sync.Mutex
	refCount   int
	config     *rest.Config
	controlEnv *envtest.Environment
}

// SharedEnvtestBackend is an experimental backend that reuses a single upstream
// envtest control plane across multiple Environment users in-process.
type SharedEnvtestBackend struct {
	delegate Backend
	state    *sharedBackendState
}

func NewSharedEnvtestBackend() *SharedEnvtestBackend {
	return &SharedEnvtestBackend{
		delegate: EnvtestBackend{},
		state:    &sharedBackendState{},
	}
}

func (b *SharedEnvtestBackend) Start(env *envtest.Environment) (*rest.Config, error) {
	b.state.mu.Lock()
	if b.state.refCount > 0 {
		b.state.refCount++
		cfg := b.state.config
		b.state.mu.Unlock()
		return cfg, nil
	}
	b.state.mu.Unlock()

	cfg, err := b.delegate.Start(env)
	if err != nil {
		return nil, err
	}

	b.state.mu.Lock()
	b.state.refCount = 1
	b.state.config = cfg
	b.state.controlEnv = env
	b.state.mu.Unlock()
	return cfg, nil
}

func (b *SharedEnvtestBackend) Stop(_ *envtest.Environment) error {
	b.state.mu.Lock()
	if b.state.refCount == 0 {
		b.state.mu.Unlock()
		return nil
	}
	b.state.refCount--
	if b.state.refCount > 0 {
		b.state.mu.Unlock()
		return nil
	}
	controlEnv := b.state.controlEnv
	b.state.config = nil
	b.state.controlEnv = nil
	b.state.mu.Unlock()

	if controlEnv == nil {
		return nil
	}
	return b.delegate.Stop(controlEnv)
}
