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
  - optional startup latency guardrail and package-level env comparison (`envtest` vs `kplane`).
- `go test -bench=. ./test/perf/...`
  - benchmark metrics overhead.

## Prerequisites for e2e
Set `KUBEBUILDER_ASSETS` to an envtest assets directory for upstream parity tests.
For perf guardrails, also set `KPLANETEST_PERF=1`.
For package-level comparison tuning, set `KPLANETEST_PERF_BATCH` (default `10`).
To enforce a strict perf win gate (time/cpu/rss), set `KPLANETEST_PERF_ASSERT_WIN=1`.
To validate the internal default kplane backend path, set `KPLANETEST_INTERNAL_E2E=1`.
To validate a live kplane apiserver, set `KPLANETEST_KPLANE_KUBECONFIG` to a kubeconfig path.

## CI helper
Use `scripts/ci-verify.sh` for a CI-friendly default flow:
- always run unit tests,
- run e2e conformance when `KUBEBUILDER_ASSETS` is set,
- run perf guardrails when both `KUBEBUILDER_ASSETS` and `KPLANETEST_PERF=1` are set.

Optional live-kplane checks:
- `KPLANETEST_INTERNAL_E2E=1 go test -tags=e2e ./test/e2e/conformance -run TestInternalKplane -v`
- `go test -tags=e2e ./test/e2e/conformance -run TestKplaneExternalCRUD -v`
- `go test -tags=e2e ./test/perf -run TestKplaneExternalCRUDPerformance -v`
- `KUBEBUILDER_ASSETS=/path/to/envtest/bin KPLANETEST_PERF=1 KPLANETEST_PERF_ASSERT_WIN=1 go test -tags=e2e ./test/perf -run TestPackageLevelEnvComparison -v -count=1`

## Attribution policy
When borrowing tests, fixtures, or structure from upstream projects:
- keep the original license header when required,
- add a short source comment at the top of the copied test file,
- mention the source in the PR description.

See also `docs/compatibility-checklist.md` for the required parity gate per commit.
