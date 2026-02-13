# kplanetest
envtest-compatible Kubernetes test environment backed by kplane virtual control planes.

## What it is
kplanetest is a drop-in replacement for `sigs.k8s.io/controller-runtime/pkg/envtest`.
It preserves the envtest contract (`Start()`/`Stop()` returning `*rest.Config`),
but swaps per‑test apiserver/etcd for a shared kplane control‑plane core.

## Why it matters
envtest launches a real apiserver and etcd **per test**, which gets expensive at
scale. kplanetest gives each test its own virtual control plane surface while
sharing the core, so memory/CPU per test is dramatically lower.

## How the apiserver works (high level)
- A **shared API server** hosts many virtual control planes (VCPs).
- Each VCP is isolated by **request path**, with native Kubernetes semantics.
- The storage layer uses **cluster‑scoped keying** so data stays isolated.
- Admission/webhooks are **per‑VCP**, so behavior stays compatible.

This keeps the Kubernetes API contract while eliminating the per‑test control‑plane tax.

## Status
Initial v1 is implemented as an envtest-compatible surface with:
- `Environment` `Start()` / `Stop()` contract parity.
- Upstream `envtest.Environment` as the canonical configuration contract.
- CRD path and webhook options passthrough.
- In-memory lifecycle metrics for test assertions.
- e2e conformance parity tests against upstream envtest.

## Intended usage
```go
env := &kplanetest.Environment{
  Environment: envtest.Environment{
    CRDDirectoryPaths: []string{"../config/crd/bases"},
  },
}
cfg, err := env.Start()
if err != nil {
  // handle error
}
defer env.Stop()
```

## Test strategy
- **Unit tests** validate lifecycle behavior and metrics.
- **Conformance (e2e)** runs identical API behavior checks against `envtest` and `kplanetest`.
- **Performance** includes benchmark coverage for metrics overhead and optional startup guardrails.

## Experimental backend mode
Set `KPLANETEST_EXPERIMENTAL_SHARED_BACKEND=1` to opt into an in-process shared
envtest backend prototype. Default behavior remains direct upstream envtest lifecycle.

Run local unit tests:
```bash
go test ./...
```

Run e2e conformance/perf (requires envtest assets):
```bash
KUBEBUILDER_ASSETS=/path/to/envtest/bin go test -tags=e2e ./test/e2e/conformance/...
KUBEBUILDER_ASSETS=/path/to/envtest/bin KPLANETEST_PERF=1 go test -tags=e2e ./test/perf/...
```

## Attribution
If a conformance case or fixture is copied/adapted from upstream projects (for example,
controller-runtime or Kubernetes), include explicit source attribution in comments and in PR notes.
