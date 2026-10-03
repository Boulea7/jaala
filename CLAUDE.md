# jaala

A Datalog engine for graph-shaped data, extracted from agni. Three packages: `ns/`, the contract
(the namespace tree, value and signature types, members, suggestions, the `Language` hook);
`stdlib/`, the standard vocabulary (`str.*`, `absent`, the glob and regex compilers); and
`datalog/`, the engine (parsing, linking, `Check`'s resolution and inference, evaluation). Read
`ns/doc.go`, then `datalog/doc.go`. jaala is a Datalog engine, not a graph library: a host's fast
path is a generator (its own Go running inside a query), not a jaala function called around the
engine (#41).

## Commands

CI (`.github/workflows/ci.yml`) runs exactly these, and all must pass:

```sh
gofmt -l .                      # must print nothing
go vet ./...
go test -race -count=1 ./...
GOOS=js GOARCH=wasm go build ./...
go list -deps ./... | grep '\.' | grep -v '^github.com/panyam/jaala' | grep -v '^vendor/'   # must print nothing
```

## Constraints

- **Standard library only, and it must build for wasm.** agni runs the engine in the browser, and
  any dependency here becomes every host's dependency.
- **The packages layer one way: `ns` imports nothing in jaala, `stdlib` imports `ns` and never
  `datalog`, production `datalog` imports `ns` only.** A host's fact layer imports `ns` and
  `stdlib` precisely because it may not import an engine (agni's C29), and the engine doesn't
  depend on the standard vocabulary. `TestLayering` (module root) guards it.
- **Error text keeps its `query:` prefix, and existing fragments stay stable** (`unknown relation
  "x"`, `takes N args`, `not stratifiable`). agni prints these messages and its tests match on
  fragments of them. New cases get new wording; existing wording doesn't move.
- **`Base` is shared across concurrent `Eval`s.** Anything mutable reachable from it must be
  per-query (the `idb*` fields on Eval's shallow copy), atomic (`work`), or locked (`edbCache`,
  the vocabulary's `Memo` entries). `TestConcurrentEvalsShareOneCheck` and
  `TestBasesOverDifferentSourcesEvaluateConcurrently` catch a regression under `-race`.
- **Strategy code lives on its strategy, never on shared state.** `Base` is the fact store plus the
  primitives every evaluator shares (`checkRules`, `applyRule`, `solve`); each evaluator owns its
  fixpoint (`Naive.materialize`, `SemiNaive.materialize`), and SemiNaive's rewrites live in their own
  files. Same-package access to `Base`'s fields is not a reason to add a method to it.
- **Every Eval carries its own run state (`Base.run`: context, budget), on its own copy of the
  Base.** Work is counted through `countWork`, which also checks the budget and, every 1024 units, the
  context; a new loop over candidates must call it and return its error. Host code gets the context
  (`Gen`'s first argument, `ns.ContextSource`), and every emitted generator row counts as work. A
  Source read that fails is not cached.
- **A rewrite that rebuilds a `Literal` must keep its `at`, and one that rebuilds a `Rule` its
  `text`.** They carry the written position and form a witness follows (`witness.go`); dropping
  either makes that literal vanish from explanations or shows a rule in its rewritten form, with no
  error. `magic.go` and `readingDelta` both rebuild literals.
- **A constant is read as its argument's type (#65).** `coerceConstants` (`coerce.go`) runs on the
  linked program after `bindGoal`, in `evaluate` and `Validate`, so a bound value is checked as a
  constant: text parses into a number argument or is refused, a number in a text or entity argument
  drops its `Num`. It rewrites `Num` only, never `S`, because the Domain check, the index and answer
  keys read the text. Only a number type pulls a compared constant, which keeps #8's "a number and
  a word have no order". `ValidateBound` binds `ns.Absent()`, which coercion and the Domain check
  (`checkArgValues`, #68) both leave alone, so a variable the host will bind is never refused for
  its type or its value. `checkArgValues` is the only validation that reads a constant's value, so
  a new placeholder or substitution has to pass it.
- **Nothing inside a module resolution may call `Vocabulary.Signature` or `Check`.** They run
  through the memo entry that is mid-computation, and `sync.Once` deadlocks on re-entry. That is
  why the validation base carries `sigs` while it checks module rules.
- **The answer order is a total order, and hosts see it.** `orderValues` ranks absent, then numbers by
  value, then text (#8). Comparing as numbers only when both are numbers cycles on a mixed column
  (2 < 10, "10" < "1a" < "2"). `dedupSort` sorts before it dedups, because dedup keys on text
  (`N(1)` and `S("1")` are one row) and the survivor must not depend on arrival. An absent value
  keys apart from `""` (`keyText`, the index's `absentKey`, #62), in answers and in groups. `order by`, `limit`
  and `offset` apply last, against the columns as written, so a host-bound variable is still one.
  Moving the default order changes agni's goldens, so it ships with an upgrade note.
- **Comments use agni's circuit vocabulary on purpose** (`doc.go` says so). Host-specific test
  data doesn't belong here, though: checks against agni's real catalog, such as its
  `columnkinds.golden`, live in agni. Copying another repo's catalog in creates a fixture that goes
  stale without failing.

## Testing discipline

- Red-check each new test: break the behaviour it guards, keep the symbol, and confirm the test
  fails on its assertion. When scripting mutations, **treat a build failure as "not checked", not
  as red**. An unused variable left by a mutation fails the build, and a naive harness counts that
  as a pass. Likewise a `-run '^Name$'` that matches no test passes; run by prefix and check the
  test actually ran. Restore mutated files in a `finally` and give each run a timeout.
- **Give a test a control that proves its fixture can tell the cases apart.** Most surviving
  mutations here were fixtures that could not: a recursion guard tested only with two-rule
  relations, a column-order test whose sort orders coincided, a "free" call the planner bound, a
  citation-leak fixture whose first derivation happened to follow the leaked path, a `ValidateBound`
  test with no closed-Domain argument (#68, which shipped). A `control:`
  assertion (Naive walks n times; the plan does start with the reordered literal) catches that.
  **Inlining hides rewrites**: a single-rule relation is folded into its caller, so a test of how
  demand, planning or the fixpoint treat a derived relation gives it two rules or runs with
  `Witnesses()` (which turns inlining off). A cost test needs a fixture where the old cost shows:
  a chain stored in walk order closes in one pass (`reversedLine` doesn't), and a reader sees its
  input in round zero only when its name sorts after it.
- **`Naive` is the reference evaluator and stays unoptimized.** `SemiNaive` (semi-naive fixpoint,
  then the rewrites `unfold` → `magic` → `plan`, all off with `WrittenOrder`) must answer as it
  does. The test helpers
  (`eval`, `evalErr`, `evalReg`, `evalRegErr`) route through `both()`, which runs Naive,
  `SemiNaive{WrittenOrder: true}` (same rows and errors; same citations too unless the program is
  recursive, where rounds run in another order and a tuple reachable two ways may cite the other
  path; #22 would make citations canonical) and the planned `SemiNaive{}` (same rows, and Naive's
  error whenever it errors). A new evaluator or option belongs in `both()` too. `seminaive_test.go` adds a
  seeded random-graph corpus, and `plan_test.go` a clause-order shuffle property.
- **Magic tuples carry no citations.** `magic.go` adds relations recording what a query demanded;
  `SemiNaive`'s `derive` clears their citations, or an answer would cite the facts that worked out
  someone else's demand. Two relations the rewrite adds are not demand and keep theirs on purpose:
  a factored reachable set (`factor.go`), so an answer cites one whole path, and a supplementary
  relation (`\x00s:`, #54), a stored body prefix. Under Witnesses a supplementary tuple carries its
  literals' witnesses as `idbTuple.parts`, which `solve` splices back in at their written positions.
  Factoring is off for a witnessed Eval. A call with nothing bound, from a guarded body, calls the
  all-free relation (`reach_ff`) under a zero-argument magic relation, so a clause whose guard never
  holds derives nothing (#60). From the goal or a rule evaluated in full it reads the original, and
  when the original is read anyway `foldFree` points the all-free calls back at it, then drops the
  rewrite's rules that read what it removed (`withoutOrphans`), since a rule reading a relation with
  no rules errs as unknown rather than deriving nothing.
- **A rule head may aggregate (#4), and that rule is its relation's only one.** `applyAggregate`
  reuses the goal's `aggregate`, so grouping, `distinct`, citations and the one-row-over-nothing case
  match an answer's. `stratify` makes its body edges strict, as negation's are. Demand stops at it
  (`magician.aggregates`): a caller binding the count column names no value of the body, and a
  supplementary relation in its body would make the bindings a count reduces a set. Its
  signature reports the aggregate position as declared when the function fixes the type
  (`aggregateFixes`, #78): `count`, `list`, and `sum`/`min`/`max` over a typed number. Over an
  untyped column those stay `Inferred`. A constant in any head leaves its column untyped (#84), so
  a default clause like `r(?n, 0)` needs the type declared on another clause (`?c: number`).
- **Demand goes through negation and into it (#34).** If the rewritten program doesn't stratify
  (a recursive caller negating what it demands), `magic` redoes it with negated calls reading their
  relations in full, which always stratifies: demand and supplementary rules hold no negation, and
  no original relation reads a rewritten one. A rule evaluated in full also has its constant calls
  rewritten (`fromConstants`, #57), adorned by the constants alone, so their demand rules are facts
  and add no dependency; that keeps the fallback's guarantee.
- **Rules are checked as linked before any rewrite renames them** (`checkRules` in `evaluate`), so
  an error names `r`, never `r\x00/bf` or a factored relation. A new rewrite gets this for free;
  a new check that names a relation belongs there too.
- **Inlining must not change multiplicity.** `unfold.go` inlines single-rule, non-recursive derived
  relations, but never into a goal or rule head whose aggregate counts bindings (`count`, `sum`,
  `list` without `distinct`): a derived relation is a set, its inlined body is not. Modes are checked on the linked
  program before any rewrite, so inlining a rule away can't hide an unrunnable body.
- **Generators declare `Modes`; `checkModes` is shared validation, the planner is SemiNaive's.** A
  body that can never satisfy a generator is refused by every evaluator and by `Validate` with one
  message, checked on the linked program before any rewrite. Planning lives in `plan.go`, called
  only by `SemiNaive`. A generator runs as soon as one of its modes is satisfied, after only the
  ready checks (comparisons, filters, and relations with every argument bound), so a host never
  has to write a body generator-first (#36). A body the demand rewrite guarded (a magic,
  supplementary or factored relation, `isGuard`) keeps the guard first when `plan` runs over it (`planRule`); ranked
  from nothing bound, the guard would fall behind any relation bound by constants.
- **SemiNaive derives a stratum component by component** (`components`, Tarjan, in dependency
  order). A stratum is a level, so it mixes recursion with plain dependencies; a relation that
  doesn't read itself, even through others, is derived once, and delta rounds run only inside a
  recursive component (#51). `stratify`'s strata, its errors, and `Naive` are unchanged.
- Fixtures: `graph()` and the `eval`/`evalErr`/`col`/`std`/`baseFor` helpers in `helpers_test.go`;
  `withModules`/`evalReg` in `module_test.go`; the agni-shaped `circuit()` in `signature_test.go`;
  `vocabulary()` (no Source) in `baseover_test.go`; the `stub` language in `ns/vocabulary_test.go`;
  `line`/`walker` (a two-mode generator recording what each call had bound) and `tested()` (line
  with tests as attributes, Declaire's shape) in `plan_test.go`; `hopper` (a generator citing its
  path in walk order) and `workOf` in `magic_test.go`; `reversedLine` and `counter` in
  `seminaive_test.go`; `parts()` (counts and numbers whose text and value orders differ) in
  `order_test.go`; `typedNets()` (number counts, a numeric-looking ref, a pin stored as `ns.N`, an
  untyped relation) and `answersAs` in `coerce_test.go`; `netlist()` (C1's two pins both on GND, so
  counting bindings and distinct values disagree; `ohms` carries a unit) in `aggregate_test.go`. `both()` takes Eval options, so a `Bind`
  case runs through all three evaluators. `both()` compares rows in order, so every test checks the
  answer order too. Don't compare rows by `fmt.Sprint`: `ns.Value.Num` is a pointer, so the text
  carries an address. datalog's tests import `stdlib` for `std()`; production datalog code must not.

## Releasing

Merge the PR, then put an annotated tag on the merge commit and push it
(`git tag -a v0.1.N <merge-sha>`, `git push origin v0.1.N`). The owner picks the version. So far
releases are patch bumps on v0.1.x, breaking changes included, pre-1.0. agni consumes tags only
(`go get github.com/panyam/jaala@vX`), never a `replace`.

## Issues and missions

Issues are ranked by the mission they serve (labels `P0`–`P3`, `waiting`, `mission`,
`mission:active`, `mission_<slug>`), and its tickets are linked as blocked-by. Most of jaala's
missions unblock a host's active mission; such a mission closes when the host's half of its exercise
passes too, so it can stay open after jaala's tickets close. `mission_selfcheck` (#86) is jaala's
own: `./selfcheck.sh` checks the evaluators against generated programs and Soufflé, and cost against
`Work()` baselines. Several missions can be active at once (`queue.sh` prints them). A new issue gets a priority and a mission link when filed, or `waiting` with
the trigger that would unpark it.

## Working with hosts

agni (github.com/panyam/agni) is the first host and Declaire (github.com/panyam/declaire) the
second. Cross-repo work is split by repo: jaala issues are worked from jaala sessions, host issues
from host sessions. A host that needs something jaala lacks files a jaala issue rather than working
around it, and a release that breaks hosts gets an upgrade note on the issue they filed, with the
exact lines each must change (as #7's v0.1.6 comment did for `Modes`).
