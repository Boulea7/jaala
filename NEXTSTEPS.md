# Next steps

Where each line of work left off. Work items live in GitHub issues; this file points at them.
Maintained by /checkpoint: one thread per branch, pruned when the branch merges.

## At a glance

- Untagged on `main`: #212 and #230 (PR #232, the Base keeps lookups across queries, a million
  tuples by default, and negated lookups batch). Its release wants an upgrade note for hosts with a
  `LookupSource` (agni in the browser may want `LimitLookupCache` lower, or 0).
- Mission #219 `mission_store_federation`: all six jaala tickets closed; it stays open for
  Declaire's `bench -time` on a federated view (declaire#199). #231 (a walk in one call) moved to
  pushdown, #213.
- v0.1.43 shipped #153 (PR #229) and #226 (PR #228). v0.1.37 (#5) still asks agni to reconcile its
  C24 with query literals folding prefixes (#224).
- Off-mission and ready: #216 and #227 (P2), #179, #205, #206, #209, #215, #222 (P3); #207 `waiting`.
- This run: dropped feat/batched-lookups-153 (PR #229) and feat/lookup-cache-212 (PR #232), both merged.

## Across threads

- FuzzEval in `./selfcheck.sh` can find #227 (`1A` vs `1 V`, Naive and planned disagree), open.
