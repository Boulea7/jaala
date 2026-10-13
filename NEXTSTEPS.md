# Next steps

Where each line of work left off. Work items live in GitHub issues; this file points at them.
Maintained by /checkpoint: one thread per branch, pruned when the branch merges.

## At a glance

- Two active missions (retriage 2026-10-13): #219 `mission_store_federation`, whose last jaala ticket
  is #233 (P1, a whole read stopped at the budget, from declaire#211), and #235
  `mission_store_pushdown`, epic #213's second half, starting at #236 (the exercise: `Stores` reports
  tuples shipped). Pushdown chain: #236 → #237 → #238 → #239/#240/#241.
- v0.1.44 shipped #212 and #230 (PR #232): the Base keeps lookups across queries, a million tuples
  by default. The upgrade note is on #212 (agni in the browser may want `LimitLookupCache` lower, or 0).
- v0.1.43 shipped #153 (PR #229) and #226 (PR #228). v0.1.37 (#5) still asks agni to reconcile its
  C24 with query literals folding prefixes (#224).
- Off-mission and ready: #216 and #227 (P2), #179, #205, #206, #209, #215, #220, #222, #234 (P3); #207 `waiting`.
- This run: tagged v0.1.44; dropped feat/batched-lookups-153 (PR #229) and feat/lookup-cache-212 (PR #232), both merged.

## Across threads

- FuzzEval in `./selfcheck.sh` can find #227 (`1A` vs `1 V`, Naive and planned disagree), open.
