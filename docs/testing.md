# Testing strategy (v1)

## Goals
- Prove envtest contract compatibility for `Start()` and `Stop()`.
- Prove API behavior parity vs upstream `envtest`.
- Track startup/teardown metrics and guard against regressions.

## Suites
- `go test ./...`
  - unit tests for lifecycle behavior and metrics correctness.
- `go test -tags=e2e ./test/e2e/conformance/...`
  - conformance parity (CRUD, watch, namespace isolation, CRD install, lifecycle/AddUser contract).
- `go test -tags=e2e ./test/perf/...`
  - optional startup latency guardrail.
- `go test -bench=. ./test/perf/...`
  - benchmark metrics overhead.

## Prerequisites for e2e
Set `KUBEBUILDER_ASSETS` to an envtest assets directory.

## Attribution policy
When borrowing tests, fixtures, or structure from upstream projects:
- keep the original license header when required,
- add a short source comment at the top of the copied test file,
- mention the source in the PR description.

See also `docs/compatibility-checklist.md` for the required parity gate per commit.
