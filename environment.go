package kplanetest

import (
	"os"
	"strings"
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

type environmentState struct {
	mu             sync.Mutex
	override       Backend
	runningBackend Backend
	metrics        *Metrics
	now            func() time.Time
}

var environmentStates sync.Map // map[*Environment]*environmentState

var defaultKplaneBackend = NewKplaneBackend()

// Environment is a drop-in compatible envtest surface.
// It keeps the same field contract as upstream envtest while allowing backend
// selection and instrumentation through package internals.
type Environment envtest.Environment

func (e *Environment) upstream() *envtest.Environment {
	return (*envtest.Environment)(e)
}

func stateFor(e *Environment) *environmentState {
	if e == nil {
		return &environmentState{}
	}
	if s, ok := environmentStates.Load(e); ok {
		return s.(*environmentState)
	}
	s := &environmentState{}
	actual, _ := environmentStates.LoadOrStore(e, s)
	return actual.(*environmentState)
}

func defaultBackend() Backend {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("KPLANETEST_BACKEND"))) {
	case "envtest":
		return EnvtestBackend{}
	case "envtest-shared":
		return defaultSharedBackend
	}
	return defaultKplaneBackend
}

func (s *environmentState) clock() func() time.Time {
	if s.now != nil {
		return s.now
	}
	return time.Now
}

func (s *environmentState) collector() *Metrics {
	if s.metrics == nil {
		s.metrics = NewMetrics()
	}
	return s.metrics
}

func (s *environmentState) resolveBackend() Backend {
	if s.override != nil {
		return s.override
	}
	return defaultBackend()
}

func (s *environmentState) instrumentedBackend(inner Backend) Backend {
	return instrumentedBackend{
		inner:   inner,
		metrics: s.collector(),
		now:     s.clock(),
	}
}

// SetBackend overrides backend selection for this Environment instance.
func (e *Environment) SetBackend(b Backend) {
	s := stateFor(e)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.override = b
}

// MetricsSnapshot returns lifecycle metrics for this Environment.
func (e *Environment) MetricsSnapshot() MetricsSnapshot {
	s := stateFor(e)
	s.mu.Lock()
	m := s.collector()
	s.mu.Unlock()
	return m.Snapshot()
}

func (e *Environment) setClockForTesting(now func() time.Time) {
	s := stateFor(e)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.now = now
}

func (e *Environment) setBackendForTesting(b Backend) {
	s := stateFor(e)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.override = b
}

// AddUser matches upstream envtest behavior.
func (e *Environment) AddUser(user envtest.User, baseConfig *rest.Config) (*envtest.AuthenticatedUser, error) {
	return e.upstream().AddUser(user, baseConfig)
}

// Start boots the environment and returns a rest.Config, following upstream
// envtest behavior.
func (e *Environment) Start() (*rest.Config, error) {
	s := stateFor(e)
	s.mu.Lock()
	defer s.mu.Unlock()

	backend := s.resolveBackend()
	cfg, err := s.instrumentedBackend(backend).Start(e.upstream())
	if err == nil {
		s.runningBackend = backend
	}
	return cfg, err
}

// Stop tears down the environment, following upstream envtest behavior.
func (e *Environment) Stop() error {
	s := stateFor(e)
	s.mu.Lock()
	defer s.mu.Unlock()

	backend := s.runningBackend
	if backend == nil {
		backend = s.resolveBackend()
	}
	err := s.instrumentedBackend(backend).Stop(e.upstream())
	if err == nil {
		s.runningBackend = nil
	}
	return err
}
