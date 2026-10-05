# Next steps

Where each line of work left off. Work items live in GitHub issues; this file points at them.
Maintained by /checkpoint: one thread per branch, pruned when the branch merges.

## At a glance

- Mission #86 `mission_selfcheck`, 11/13: #22 closed by #142 (`CanonicalCites`), so the corpus has
  no known disagreements left. Next ready: #23 (Soufflé), #89 (fuzzing the text), both P2.
  `./selfcheck.sh` passed on 25b65ce (4999/4999 compared, 13 baselines match).
- Release: v0.1.23 is tagged on 25b65ce (#141, #142, #144). Upgrade note posted on #140 (nothing
  breaks; `ns.Versioned` / `Base.Forget` / `Volatile` for hosts whose facts move). agni is asked on
  #139 to re-measure `probed_both` on its real boards and drop the `_test_points` workaround.
- Off-mission, waiting: #145 (evaluate a small demanded relation in full so the Base can keep it;
  unparks on agni's per-click numbers), #143 (P3). #126 (P2) serves no mission.
- Open: draft PR #137 (outside contributor, for #129), waiting on its author.
- No open threads. This run dropped the threads for `probe-projections` (#141) and `derived-cache`
  (#144), both merged. `origin/checkpoint-docsite` (PR #116, merged) still exists on the remote.
