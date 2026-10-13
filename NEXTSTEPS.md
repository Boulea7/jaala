# Next steps

Where each line of work left off. Work items live in GitHub issues; this file points at them.
Maintained by /checkpoint: one thread per branch, pruned when the branch merges.

## At a glance

- Release v0.1.42: #84 (PR #184, numeric head constants type their column), #128 (PR #185, one
  `query:` prefix in `having` and `order by` errors), #224 (PR #225, a head's `1kV` folds to 1000 V).
  #184 changes signatures a host reads; agni's failure list is unchanged by it.
- Mission #219 `mission_store_federation`, 3/5: next ready is #153 (batched lookups), then #212.
- v0.1.37 (#5) asks agni to reconcile its C24 (scale converted upstream, once) with query literals
  folding prefixes, now in rule heads too (#224).
- Arithmetic follow-ups: #205, #206, #207 (`waiting`), #209, #215. Off-mission and ready: #216 (P2,
  computed float error), #179 (P3, timed facts).
- fix/selfcheck-replay-226 — #226 (P1) — PR open; next: merge, rerun selfcheck on main.
- #227 (P2, filed): FuzzEval found Naive and planned SemiNaive answering `1 V` vs `1 A`.
- This run: #226 filed, #172's exercise is flaky (noted on #172, left closed); no threads pruned.

## Across threads

- Until #226 merges, `./selfcheck.sh` on `main` fails about half its runs with Soufflé's
  "Ungrounded variable"; that's #226. FuzzEval can also find #227 (`1A` vs `1 V`), which is open.

## fix/selfcheck-replay-226

- **Last touched**: 2026-10-13 (jaala/main)
- **Ticket**: #226
- **Why**: selfcheck is the check before every tag; a flaky Soufflé step makes it useless.
- **Where it stopped**: PR open. Generator walks `genTypes` in order; translator writes a goal-bound
  counted variable as its constant; `TestASeedGeneratesOneProgram` and a translator case added.
  Selfcheck's Soufflé step passed three runs in a row on the branch with identical counts.
- **Next action**: after merge, run selfcheck on `main` and say on #172 that its exercise holds.
