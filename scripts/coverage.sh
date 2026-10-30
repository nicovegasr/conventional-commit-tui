#!/usr/bin/env bash
# Per-package coverage gate for conventional-commit-tui domain packages.
#
# Fails if ANY domain package falls below THRESHOLD%. Coverage is measured
# per package (not globally) so a well-covered package can never mask a
# poorly-covered one. The UI package is intentionally excluded (0% required).
#
# Source of truth for the coverage gate — called by `make cover` and CI.
set -euo pipefail

THRESHOLD=${COVERAGE_THRESHOLD:-80}

# Domain packages under the coverage gate. Keep in sync with the architecture:
# everything with real logic, never internal/ui (render/IO). internal/history
# and internal/cli are excluded: their logic is unit-tested, but most of their
# code is the git/CLI adapter seam that integration tests cover (ADR-0006).
DOMAIN_PKGS=(
	./internal/commit
	./internal/audit
	./internal/stats
	./internal/git
	./internal/settings
	./internal/index
	./internal/pathignore
	./internal/editor
)

fail=0
printf '%-26s %8s\n' "PACKAGE" "COVERAGE"
printf '%-26s %8s\n' "-------" "--------"

for pkg in "${DOMAIN_PKGS[@]}"; do
	out=$(go test -cover "$pkg" 2>&1) || {
		echo "$out"
		echo "FAIL: tests failed in $pkg"
		fail=1
		continue
	}

	if grep -q "no test files" <<<"$out"; then
		printf '%-26s %8s\n' "$pkg" "NO TESTS"
		echo "FAIL: $pkg has no tests (domain requires >= ${THRESHOLD}%)"
		fail=1
		continue
	fi

	pct=$(grep -oE 'coverage: [0-9.]+%' <<<"$out" | grep -oE '[0-9.]+' | head -1)
	printf '%-26s %7s%%\n' "$pkg" "$pct"

	if awk "BEGIN{exit !($pct < $THRESHOLD)}"; then
		echo "FAIL: $pkg coverage ${pct}% < ${THRESHOLD}%"
		fail=1
	fi
done

if [ "$fail" -ne 0 ]; then
	echo
	echo "Coverage gate FAILED (threshold ${THRESHOLD}% per domain package)."
	exit 1
fi

echo
echo "Coverage gate passed (>= ${THRESHOLD}% per domain package)."
