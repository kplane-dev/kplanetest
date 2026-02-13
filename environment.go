package kplanetest

import (
	"sync"
	"time"

	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

type environmentBackend interface {
	Start(env *envtest.Environment) (*rest.Config, error)
	Stop(env *envtest.Environment) error
}

type envtestBackend struct{}

func (b *envtestBackend) Start(env *envtest.Environment) (*rest.Config, error) {
	return env.Start()
}

func (b *envtestBackend) Stop(env *envtest.Environment) error {
	return env.Stop()
}

// Environment is an envtest-compatible test environment.
// v1 embeds upstream envtest.Environment as the canonical definition and adds
// optional lifecycle metrics plus backend indirection for future swap work.
type Environment struct {
	envtest.Environment

	// Metrics is optional. If nil, a default in-memory collector is used.
	Metrics *Metrics

	mu         sync.Mutex
	newBackend func() environmentBackend
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

func (e *Environment) backendForStart() environmentBackend {
	if e.newBackend != nil {
		return e.newBackend()
	}
	return &envtestBackend{}
}

// Start boots the environment and returns a rest.Config, following upstream
// envtest behavior.
func (e *Environment) Start() (*rest.Config, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	backend := e.backendForStart()
	start := e.clock()()
	cfg, err := backend.Start(&e.Environment)
	duration := e.clock()().Sub(start)

	collector := e.collector()
	collector.ObserveStart(duration, err == nil)
	return cfg, err
}

// Stop tears down the environment, following upstream envtest behavior.
func (e *Environment) Stop() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	backend := e.backendForStart()
	start := e.clock()()
	err := backend.Stop(&e.Environment)
	duration := e.clock()().Sub(start)

	collector := e.collector()
	collector.ObserveStop(duration, err == nil)
	return err
}
