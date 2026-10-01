# jaala

A Datalog engine for graph-shaped data, extracted from agni. Two packages: `ns/`, the vocabulary
(the namespace tree, value and signature types, members, suggestions, the `Language` hook), and
`datalog/`, the engine (parsing, linking, `Check`'s resolution and inference, evaluation). Read
`ns/doc.go`, then `datalog/doc.go`.

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
- **`ns` never imports `datalog`, or any engine.** A host's fact layer imports `ns` precisely
  because it may not import an engine (agni's C29). `TestNsImportsNoEngine` guards it.
- **Error text keeps its `query:` prefix, and existing fragments stay stable** (`unknown relation
  "x"`, `takes N args`, `not stratifiable`). agni prints these messages and its tests match on
  fragments of them. New cases get new wording; existing wording doesn't move.
- **`Base` is shared across concurrent `Eval`s.** Anything mutable reachable from it must be
  per-query (the `idb*` fields on Eval's shallow copy), atomic (`work`), or locked (`edbCache`,
  the vocabulary's `Memo` entries). `TestConcurrentEvalsShareOneCheck` and
  `TestBasesOverDifferentSourcesEvaluateConcurrently` catch a regression under `-race`.
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
- **`Naive` is the reference evaluator and stays unoptimized.** Every faster strategy (`SemiNaive`
  now; join planning and magic sets, #7) must answer exactly as it does. The test helpers
  (`eval`, `evalErr`, `evalReg`, `evalRegErr`) route through `both()`, which runs Naive,
  `SemiNaive{WrittenOrder: true}` (must match exactly: rows, citations, errors) and the planned
  `SemiNaive{}` (same rows, and Naive's error whenever it errors; its citations may come from
  another derivation). A new evaluator or option belongs in `both()` too. `seminaive_test.go` adds a
  seeded random-graph corpus, and `plan_test.go` a clause-order shuffle property.
- **Generators declare `Modes`; `checkModes` is shared validation, the planner is SemiNaive's.** A
  body that can never satisfy a generator is refused by every evaluator and by `Validate` with one
  message. Planning lives in `plan.go`, called only by `SemiNaive` (see the strategy-on-its-type
  rule: nothing planner-specific goes on `Base`).
- Fixtures: `graph()` and the `eval`/`evalErr`/`col`/`std`/`baseFor` helpers in `helpers_test.go`;
  `withModules`/`evalReg` in `module_test.go`; the agni-shaped `circuit()` in `signature_test.go`;
  `vocabulary()` (no Source) in `baseover_test.go`; the `stub` language in `ns/vocabulary_test.go`.

## Releasing

Merge the PR, then put an annotated tag on the merge commit and push it
(`git tag -a v0.1.N <merge-sha>`, `git push origin v0.1.N`). The owner picks the version. So far
releases are patch bumps on v0.1.x, breaking changes included, pre-1.0. agni consumes tags only
(`go get github.com/panyam/jaala@vX`), never a `replace`.

## Working with agni

agni (github.com/panyam/agni) is the first host. Cross-repo work is split by repo: jaala issues are
worked from jaala sessions, agni issues from agni sessions. If agni needs something jaala lacks, it
files a jaala issue rather than working around it.
