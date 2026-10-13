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
- fix/selfcheck-replay-226 — #226 (P1) — next: open the PR (seed replay, translator, two tests).
- This run: #226 filed, #172's exercise is flaky (noted on #172, left closed); no threads pruned.

## Across threads

- `./selfcheck.sh` fails on about half its runs on `main` until #226 merges (Soufflé refuses seed
  1209's translation). A failure there with "Ungrounded variable" is #226, not the change at hand.

## fix/selfcheck-replay-226

- **Last touched**: 2026-10-13 (jaala/main)
- **Ticket**: #226
- **Why**: selfcheck is the check before every tag; a flaky Soufflé step makes it useless.
- **Where it stopped**: diagnosed. `bodyAggregate` (`datalog/generate_test.go`) ranges over the
  `b.bound` map while drawing `g.chance`, so a seed's program varies; the Soufflé translator
  (`souffle_test.go`) counts a goal-bound variable it dropped from the bindings relation.
- **Next action**: sort the map walks, keep the bound variable as its constant in the helper
  relation, add a replay test and seed 1209's program as a translator case.
