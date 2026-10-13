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
- This run: closed missions #172 and #193; dropped datalog-functions (PR #214 merged). No open
  threads of ours.
