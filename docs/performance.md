# Performance guardrails

`kplanetest` tracks startup/teardown behavior with a small regression budget model.

## Startup latency guardrail test
File: `test/perf/envtest_latency_test.go`

This test compares average startup+stop latency between:
- upstream `envtest`, and
- `kplanetest` on the current backend.

It is intentionally budget-based (not absolute) to keep it stable in CI.

## Environment variables
- `KUBEBUILDER_ASSETS` (required): envtest binary path.
- `KPLANETEST_PERF=1` (required): enables perf guardrail tests.
- `KPLANETEST_PERF_ITERATIONS` (optional, default `3`): number of samples.
- `KPLANETEST_STARTUP_BUDGET_PCT` (optional, default `20`): allowed percent over envtest average.

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
