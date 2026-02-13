#!/usr/bin/env bash
set -euo pipefail

echo "==> unit tests"
go test ./... -count=1

if [[ -n "${KUBEBUILDER_ASSETS:-}" ]]; then
  echo "==> e2e conformance tests"
  go test -tags=e2e ./test/e2e/conformance/... -count=1
else
  echo "==> skipping e2e conformance (KUBEBUILDER_ASSETS not set)"
fi

if [[ -n "${KUBEBUILDER_ASSETS:-}" && "${KPLANETEST_PERF:-}" == "1" ]]; then
  echo "==> perf guardrail tests"
  go test -tags=e2e ./test/perf/... -count=1
else
  echo "==> skipping perf guardrails (requires KUBEBUILDER_ASSETS and KPLANETEST_PERF=1)"
fi
