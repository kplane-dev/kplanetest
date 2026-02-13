# Performance guardrails

`kplanetest` tracks startup/teardown behavior with a small regression budget model.

## Startup latency guardrail test
File: `test/perf/envtest_latency_test.go`

This test compares average startup+stop latency between:
- upstream `envtest`, and
- `kplanetest` on the current backend.

It is intentionally budget-based (not absolute) to keep it stable in CI.

## Package-level comparison test
File: `test/perf/package_level_compare_test.go`

This compares package-level env testing behavior between:
- upstream `envtest` (one control plane per environment), and
- default `kplanetest` backend (shared kplane process with per-environment cluster path).

It reports wall-time plus worker total CPU (self + children) and a combined RSS signal.
The combined RSS signal is `max(self_max_rss, child_max_rss)` per scenario to reduce
architecture bias between in-process and child-process work.

## Environment variables
- `KUBEBUILDER_ASSETS` (required): envtest binary path.
- `KPLANETEST_PERF=1` (required): enables perf guardrail tests.
- `KPLANETEST_PERF_ITERATIONS` (optional, default `3`): number of samples.
- `KPLANETEST_STARTUP_BUDGET_PCT` (optional, default `20`): allowed percent over envtest average.
- `KPLANETEST_KPLANE_KUBECONFIG` (optional): enables live kplane CRUD performance comparison.
- `KPLANETEST_KPLANE_CRUD_BUDGET_PCT` (optional, default `200`): budget when enforcing live kplane CRUD comparisons.
- `KPLANETEST_PERF_ENFORCE=1` (optional): fail tests on live kplane CRUD budget breach; otherwise logs only.
- `KPLANETEST_BACKEND` (optional): backend override (`kplane` default, `envtest`, `envtest-shared`).
- `KPLANETEST_PERF_BATCH` (optional, default `10`): number of envs in package-level comparison.
- `KPLANETEST_PERF_ASSERT_WIN=1` (optional): enforce that package-level comparison shows kplane win in time/cpu/rss.
- `KPLANETEST_PERF_WORKER_TIMEOUT` (optional, default `45s`): per-scenario worker timeout for package-level comparison.

Note: package-level comparison is heavier than micro-benchmarks. With `go test -timeout=15s`,
the test may auto-reduce batch size and skip if there is insufficient deadline budget.

## CI command
```bash
KUBEBUILDER_ASSETS=/path/to/envtest/bin \
KPLANETEST_PERF=1 \
KPLANETEST_PERF_ITERATIONS=5 \
KPLANETEST_STARTUP_BUDGET_PCT=20 \
go test -tags=e2e ./test/perf/... -count=1
```

Or use:
```bash
./scripts/ci-verify.sh
```

Run package-level comparison directly:
```bash
KUBEBUILDER_ASSETS=/path/to/envtest/bin \
KPLANETEST_PERF=1 \
KPLANETEST_PERF_BATCH=10 \
KPLANETEST_PERF_ASSERT_WIN=1 \
go test -tags=e2e ./test/perf -run TestPackageLevelEnvComparison -v -count=1
```
