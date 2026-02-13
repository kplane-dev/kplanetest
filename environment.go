package kplanetest

import (
	"sync"
	"time"

	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

// Backend defines the control-plane operations required by Environment.
// v1 defaults to EnvtestBackend; future commits can swap this implementation.
type Backend interface {
	Start(env *envtest.Environment) (*rest.Config, error)
	Stop(env *envtest.Environment) error
}

// EnvtestBackend delegates lifecycle operations to upstream envtest directly.
type EnvtestBackend struct{}

func (b EnvtestBackend) Start(env *envtest.Environment) (*rest.Config, error) {
	return env.Start()
}

func (b EnvtestBackend) Stop(env *envtest.Environment) error {
	return env.Stop()
}

type instrumentedBackend struct {
	inner   Backend
	metrics *Metrics
	now     func() time.Time
}

func (b instrumentedBackend) Start(env *envtest.Environment) (*rest.Config, error) {
	started := b.now()
	cfg, err := b.inner.Start(env)
	b.metrics.ObserveStart(b.now().Sub(started), err == nil)
	return cfg, err
}

func (b instrumentedBackend) Stop(env *envtest.Environment) error {
	started := b.now()
	err := b.inner.Stop(env)
	b.metrics.ObserveStop(b.now().Sub(started), err == nil)
	return err
}

// Environment is an envtest-compatible test environment.
// v1 embeds upstream envtest.Environment as the canonical definition and adds
// optional lifecycle metrics plus backend indirection for future swap work.
type Environment struct {
	envtest.Environment

	// Backend optionally overrides the lifecycle implementation.
	// If nil, EnvtestBackend is used.
	Backend Backend

	// Metrics is optional. If nil, a default in-memory collector is used.
	Metrics *Metrics

	mu         sync.Mutex
	newBackend func() Backend
	now        func() time.Time
}

func (e *Environment) clock() func() time.Time {
	if e.now != nil {
		return e.now
	}
	return time.Now
}

func (e *Environment) collector() *Metrics {
	if e.Metrics == nil {
		e.Metrics = NewMetrics()
	}
	return e.Metrics
}

func (e *Environment) resolveBackend() Backend {
	if e.newBackend != nil {
		return e.newBackend()
	}
	if e.Backend != nil {
		return e.Backend
	}
	return EnvtestBackend{}
}

func (e *Environment) instrumentedBackend() Backend {
	return instrumentedBackend{
		inner:   e.resolveBackend(),
		metrics: e.collector(),
		now:     e.clock(),
	}
}

// Start boots the environment and returns a rest.Config, following upstream
// envtest behavior.
func (e *Environment) Start() (*rest.Config, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	return e.instrumentedBackend().Start(&e.Environment)
}

// Stop tears down the environment, following upstream envtest behavior.
func (e *Environment) Stop() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	return e.instrumentedBackend().Stop(&e.Environment)
}
