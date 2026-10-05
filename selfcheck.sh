#!/bin/sh
# selfcheck runs jaala's checks that need no host: every evaluator against Naive on generated
# programs (#87), work against baselines (#88), and answers against Soufflé (#23). See #86.
# JAALA_GEN_SEEDS overrides the seed count.
set -u
cd "$(dirname "$0")"
seeds=${JAALA_GEN_SEEDS:-5000}
out=$(mktemp)
trap 'rm -f "$out"' EXIT
status=0

echo "== generated programs: $seeds seeds"
JAALA_GEN_SEEDS=$seeds go test -count=1 ./datalog -run '^(TestGeneratedProgramsAgree|TestBrokenProgramsFailTheSameWay)$' -v >"$out" 2>&1 || status=1
# Relation names a rewrite adds carry a NUL, which would make grep treat the log as binary.
tr -d '\000' <"$out" | grep -E 'seeds$|compared|skipped|known|disagrees|refuses|replay|shrunk|facts:|bind:|naive:|planned:|seminaive:|^(--- |ok|FAIL)'

echo "== work against baselines"
go test -count=1 ./datalog -run '^TestWorkStaysWithinBaseline$' -v >"$out" 2>&1 || status=1
grep -E 'work |over its|under its|has no baseline|no workload|^(--- |ok|FAIL)' "$out" | sed 's/^ *bench_test.go:[0-9]*: /  /'

echo "== Soufflé differential: not yet (#23)"

[ $status -eq 0 ] && echo "selfcheck: pass" || echo "selfcheck: FAIL"
exit $status
