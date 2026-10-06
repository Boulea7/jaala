#!/bin/sh
# selfcheck runs jaala's checks that need no host: every evaluator against Naive on generated
# programs (#87), work against baselines (#88), fuzzed query text (#89), and answers against Soufflé
# (#23), when souffle is on PATH. See #86. JAALA_GEN_SEEDS overrides the seed count, for the corpus and
# for Soufflé, and JAALA_FUZZ_TIME each target's fuzzing time.
set -u
cd "$(dirname "$0")"
seeds=${JAALA_GEN_SEEDS:-5000}
fuzztime=${JAALA_FUZZ_TIME:-30s}
out=$(mktemp)
trap 'rm -f "$out"' EXIT
status=0

echo "== generated programs: $seeds seeds"
JAALA_GEN_SEEDS=$seeds go test -count=1 ./datalog -run '^(TestGeneratedProgramsAgree|TestBrokenProgramsFailTheSameWay)$' -v >"$out" 2>&1 || status=1
# Relation names a rewrite adds carry a NUL, which would make grep treat the log as binary.
tr -d '\000' <"$out" | grep -E 'seeds$|compared|skipped|known|broken programs|written:|disagrees|refuses|replay|shrunk|facts:|bind:|naive:|planned:|seminaive:|^(--- |ok|FAIL)'

echo "== work against baselines"
go test -count=1 ./datalog -run '^TestWorkStaysWithinBaseline$' -v >"$out" 2>&1 || status=1
grep -E 'work |over its|under its|has no baseline|no workload|^(--- |ok|FAIL)' "$out" | sed 's/^ *bench_test.go:[0-9]*: /  /'

echo "== fuzzed query text: $fuzztime per target"
for target in FuzzParse FuzzEval; do
	# go test fuzzes one target at a time. A failing input is written to datalog/testdata/fuzz/<target>,
	# which plain go test then replays: fix it, and commit the file as a seed.
	if go test -count=1 ./datalog -run '^$' -fuzz "^$target\$" -fuzztime "$fuzztime" >"$out" 2>&1; then
		echo "  $target: no failing input in $fuzztime"
	else
		status=1
		tr -d '\000' <"$out" | grep -E 'Failing input|^\s+fuzz_test.go|panic:|FAIL' | sed "s/^/  $target: /"
	fi
done

echo "== Soufflé differential: $seeds seeds"
souffle_ran=yes
if command -v souffle >/dev/null 2>&1; then
	# The corpus's programs Soufflé can express, translated and run there, against Naive's answers.
	JAALA_SOUFFLE=require JAALA_SOUFFLE_SEEDS=$seeds go test -count=1 -timeout 30m ./datalog -run '^(TestSouffleAgreesWithNaive|TestSouffleCatchesAWrongTranslation)$' -v >"$out" 2>&1 || status=1
	tr -d '\000' <"$out" | grep -E 'seeds$|compared with|skipped:|disagreements|disagrees|refused|replay|shrunk|facts:|bind:|naive:|souffle:|^(--- |ok|FAIL)'
else
	souffle_ran=no
	echo "  could not run: souffle is not on PATH (see #23 for installing it)"
fi

if [ $status -ne 0 ]; then
	echo "selfcheck: FAIL"
elif [ $souffle_ran = no ]; then
	echo "selfcheck: pass, except Soufflé, which could not run"
else
	echo "selfcheck: pass"
fi
exit $status
