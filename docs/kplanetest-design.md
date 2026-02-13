# kplanetest design doc (minimal)

## Summary
kplanetest is an envtest‑compatible library backed by kplane VCPs. It keeps the
envtest contract while replacing per‑test apiserver/etcd with a shared core.

## Goals
- **Import swap**: callers only change imports.
- **Exact contract**: match `envtest.Environment` semantics.
- **Shared core**: one shared control‑plane core, per‑test VCPs.
- **Proof**: conformance + performance tests.

## Non‑goals (v1)
- Full e2e cluster behavior (nodes/workers).
- Complete coverage of every envtest flag.

## MVP scope
- Start/Stop parity with envtest.
- CRD install into the VCP.
- Optional webhooks (same options as envtest).
- Single‑process pool manager.

## API shape (identical semantics)
```
type Environment struct {
  CRDDirectoryPaths []string
  ErrorIfCRDPathMissing bool
  WebhookInstallOptions *WebhookInstallOptions
}

func (e *Environment) Start() (*rest.Config, error)
func (e *Environment) Stop() error
```

## Lifecycle (shared core)
- `Start()` ensures shared core is running, creates a VCP, returns kubeconfig.
- `Stop()` deletes the VCP and releases a refcount; last user tears down core.

## Tests
- **Conformance**: parity suite vs envtest (CRUD, watch, webhook, namespace).
- **Performance**: CPU/memory per VCP vs envtest baseline.

Current v1 implementation includes:
- unit tests for lifecycle and metrics behavior,
- `e2e`-tagged conformance parity tests against envtest,
- `e2e`-tagged startup latency guardrail and perf benchmarks.

## Future‑flex hooks
- Versioned pools (one shared core per K8s version).
- Cross‑process coordination (file lock / daemon).
- Optional embedded etcd backend.
