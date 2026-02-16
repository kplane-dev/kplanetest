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
- Default backend using `kplane-dev/apiserver` with `kcp` embedded etcd.
- CRD path and webhook options passthrough via envtest-compatible flow.
- In-memory lifecycle metrics for test assertions.
- e2e conformance parity tests against upstream envtest.

## Intended usage
```go
env := &kplanetest.Environment{
  CRDDirectoryPaths: []string{"../config/crd/bases"},
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

## Backend selection
Default backend is kplane (`kplane-dev/apiserver` + embedded etcd).

Optional overrides:
- `KPLANETEST_BACKEND=envtest` for upstream envtest backend.
- `KPLANETEST_BACKEND=envtest-shared` for shared envtest prototype backend.

## Kplane assets setup (envtest-like model)
`kplanetest` can use prebuilt/cached apiserver binaries instead of building at runtime.

Install setup helper:
```bash
go install github.com/kplane-dev/kplanetest/cmd/setup-kplanetest@latest
```

Prepare assets cache and print path:
```bash
export KPLANETEST_ASSETS="$(setup-kplanetest use -p path v0.0.8)"
```

Or set a direct binary path:
```bash
export KPLANETEST_APISERVER_BINARY=/absolute/path/to/kplane-apiserver
```

Resolution order:
1. `KPLANETEST_APISERVER_BINARY`
2. `KPLANETEST_ASSETS` + `/kplane-apiserver`
3. Runtime build fallback (current behavior if nothing is configured)

Run local unit tests:
```bash
go test ./...
```

Run e2e conformance/perf (requires envtest assets):
```bash
KUBEBUILDER_ASSETS=/path/to/envtest/bin go test -tags=e2e ./test/e2e/conformance/...
KUBEBUILDER_ASSETS=/path/to/envtest/bin KPLANETEST_PERF=1 go test -tags=e2e ./test/perf/...
```

Run package-level perf comparison and enforce kplane wins:
```bash
KUBEBUILDER_ASSETS=/path/to/envtest/bin \
KPLANETEST_PERF=1 \
KPLANETEST_PERF_BATCH=10 \
KPLANETEST_PERF_ASSERT_WIN=1 \
go test -tags=e2e ./test/perf -run TestPackageLevelEnvComparison -v -count=1
```

Run internal kplane backend validation:
```bash
KPLANETEST_INTERNAL_E2E=1 go test -tags=e2e ./test/e2e/conformance -run TestInternalKplane -v
```

Run optional live-kplane validation (requires a reachable kplane kubeconfig):
```bash
KPLANETEST_KPLANE_KUBECONFIG=/path/to/kplane.kubeconfig go test -tags=e2e ./test/e2e/conformance -run TestKplaneExternalCRUD -v
KUBEBUILDER_ASSETS=/path/to/envtest/bin KPLANETEST_PERF=1 KPLANETEST_KPLANE_KUBECONFIG=/path/to/kplane.kubeconfig go test -tags=e2e ./test/perf -run TestKplaneExternalCRUDPerformance -v
```
