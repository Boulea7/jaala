# Next steps

Where each line of work left off. Work items live in GitHub issues; this file points at them.
Maintained by /checkpoint: one thread per branch, pruned when the branch merges.

## At a glance

- Release v0.1.43: #153 (PR #229, batched lookups) and #226 (PR #228, selfcheck's Soufflé step
  passes every run). v0.1.42 shipped #84, #128 and #224; #184 changed signatures a host reads.
- Mission #219 `mission_store_federation`, 4/5: next ready is #212.
- v0.1.37 (#5) asks agni to reconcile its C24 (scale converted upstream, once) with query literals
  folding prefixes, now in rule heads too (#224).
- Arithmetic follow-ups: #205, #206, #207 (`waiting`), #209, #215. Off-mission and ready: #216 (P2,
  computed float error), #179 (P3, timed facts).
- #227 (P2, filed): FuzzEval found Naive and planned SemiNaive answering `1 V` vs `1 A`.
- This run: v0.1.43 tagged (#153, #226); dropped fix/selfcheck-replay-226 (PR #228 merged).

## Across threads

- FuzzEval in `./selfcheck.sh` can find #227 (`1A` vs `1 V`, Naive and planned disagree), open.
