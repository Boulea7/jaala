# Next steps

Where each line of work left off. Work items live in GitHub issues; this file points at them.
Maintained by /checkpoint: one thread per branch, pruned when the branch merges.

## At a glance

- Mission #172 `mission_aggregate_answers`, 2/6. Next ready: #84 (P1, a numeric constant in a rule
  head leaves its column untyped), the last unmet done-when line; with it merged the mission can
  close, #128 and #80 finished first or moved. Exercise at bb2398a: works, and the Soufflé step skips
  no program for an aggregate's value over nothing (506 at v0.1.29). #122 (v0.1.30) made `min`/`max`
  over no numbers absent and `sum` 0; #71 (v0.1.31) added `?c = count(?n) : { ... }` in a body.
- Release: v0.1.31 is on bb2398a. v0.1.30 (#175) carried an upgrade note on #122, which agni took;
  v0.1.31 (#176) changes nothing that parsed before.
- Retriage after v0.1.31: #84 P2 → P1; #177 closed into #72 (demand on group keys covers a body
  aggregate's braces); #173 and #174 parked with their triggers.
- Parked as `waiting` when #86 closed, each with its trigger in a comment: #156, #167, #169.
- Retriage: P3 orphans parked as `waiting` with triggers (#1, #5, #12, #79, #112, #120, #131, #158);
  stray priorities and #104's `mission:active` removed. `queue.sh` hygiene is clean.
- No open threads or PRs. This run replaced the `str-distance` checkpoint's glance (stale since
  #157, #159 and the releases above).
