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
- **Nothing inside a module resolution may call `Vocabulary.Signature` or `Check`.** They run
  through the memo entry that is mid-computation, and `sync.Once` deadlocks on re-entry. That is
  why the validation base carries `sigs` while it checks module rules.
- **Comments use agni's circuit vocabulary on purpose** (`doc.go` says so). Host-specific test
  data doesn't belong here, though: checks against agni's real catalog, such as its
  `columnkinds.golden`, live in agni. Copying another repo's catalog in creates a fixture that goes
  stale without failing.

## Testing discipline

- Red-check each new test: break the behaviour it guards, keep the symbol, and confirm the test
  fails on its assertion. When scripting mutations, **treat a build failure as "not checked", not
  as red**. An unused variable left by a mutation fails the build, and a naive harness counts that
  as a pass.
- **Give a test a control that proves its fixture can tell the cases apart.** Most surviving
  mutations here were fixtures that could not: a recursion guard tested only with two-rule
  relations, a column-order test whose sort orders coincided, a "free" call the planner bound, a
  citation-leak fixture whose first derivation happened to follow the leaked path. A `control:`
  assertion (Naive walks n times; the plan does start with the reordered literal) catches that.
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
  someone else's demand. A relation that reads a negation, transitively, is not rewritten (#34).
- **Inlining must not change multiplicity.** `unfold.go` inlines single-rule, non-recursive derived
  relations, but never into a goal whose aggregate counts bindings (`count`, `sum`, `list` without
  `distinct`): a derived relation is a set, its inlined body is not. Modes are checked on the linked
  program before any rewrite, so inlining a rule away can't hide an unrunnable body.
- **Generators declare `Modes`; `checkModes` is shared validation, the planner is SemiNaive's.** A
  body that can never satisfy a generator is refused by every evaluator and by `Validate` with one
  message, checked on the linked program before any rewrite. Planning lives in `plan.go`, called
  only by `SemiNaive`.
- Fixtures: `graph()` and the `eval`/`evalErr`/`col`/`std`/`baseFor` helpers in `helpers_test.go`;
  `withModules`/`evalReg` in `module_test.go`; the agni-shaped `circuit()` in `signature_test.go`;
  `vocabulary()` (no Source) in `baseover_test.go`; the `stub` language in `ns/vocabulary_test.go`.
  datalog's tests import `stdlib` for `std()`; production datalog code must not.

## Releasing

Merge the PR, then put an annotated tag on the merge commit and push it
(`git tag -a v0.1.N <merge-sha>`, `git push origin v0.1.N`). The owner picks the version. So far
releases are patch bumps on v0.1.x, breaking changes included, pre-1.0. agni consumes tags only
(`go get github.com/panyam/jaala@vX`), never a `replace`.

## Working with hosts

agni (github.com/panyam/agni) is the first host and Declaire (github.com/panyam/declaire) the
second. Cross-repo work is split by repo: jaala issues are worked from jaala sessions, host issues
from host sessions. A host that needs something jaala lacks files a jaala issue rather than working
around it, and a release that breaks hosts gets an upgrade note on the issue they filed, with the
exact lines each must change (as #7's v0.1.6 comment did for `Modes`).
