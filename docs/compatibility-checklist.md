# Envtest compatibility checklist

Use this checklist to keep `kplanetest` behavior aligned with upstream
`sigs.k8s.io/controller-runtime/pkg/envtest` while backend internals evolve.

## Lifecycle contract
- `Start()` behavior on first and repeated calls matches upstream.
- `Stop()` behavior on first and repeated calls matches upstream.
- Returned `*rest.Config` shape (host/non-nil) matches upstream behavior.
- No extra error wrapping that changes upstream error identity.

## Auth/user contract
- `AddUser(...)` behavior and returned auth config shape match upstream.
- Returned user config is valid against the started control plane.

## Installation contract
- CRD path install behavior parity.
- Missing CRD path behavior parity when strict flags are enabled.
- Webhook install/cleanup behavior parity.

## Test execution contract
- `go test ./...` passes.
- `go test -tags=e2e ./test/e2e/conformance/...` runs with `KUBEBUILDER_ASSETS`.
- `go test -tags=e2e ./test/perf/...` runs with perf env flags for guardrails.

## Per-commit policy
For each backend implementation commit:
- run unit and e2e suites,
- document any known parity gaps explicitly,
- avoid changing public behavior without a matching parity test update.
