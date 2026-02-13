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
For perf guardrails, also set `KPLANETEST_PERF=1`.
To validate a live kplane apiserver, set `KPLANETEST_KPLANE_KUBECONFIG` to a kubeconfig path.

## CI helper
Use `scripts/ci-verify.sh` for a CI-friendly default flow:
- always run unit tests,
- run e2e conformance when `KUBEBUILDER_ASSETS` is set,
- run perf guardrails when both `KUBEBUILDER_ASSETS` and `KPLANETEST_PERF=1` are set.

Optional live-kplane checks:
- `go test -tags=e2e ./test/e2e/conformance -run TestKplaneExternalCRUD -v`
- `go test -tags=e2e ./test/perf -run TestKplaneExternalCRUDPerformance -v`

## Attribution policy
When borrowing tests, fixtures, or structure from upstream projects:
- keep the original license header when required,
- add a short source comment at the top of the copied test file,
- mention the source in the PR description.

See also `docs/compatibility-checklist.md` for the required parity gate per commit.
