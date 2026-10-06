# Next steps

Where each line of work left off. Work items live in GitHub issues; this file points at them.
Maintained by /checkpoint: one thread per branch, pruned when the branch merges.

## At a glance

- No active mission. #86 (`mission_selfcheck`) closed at v0.1.29 with every done-when met:
  `./selfcheck.sh` ran all four steps on 3f4f5fd (corpus 4999/4999, baselines, fuzz, Soufflé
  3857/5000, no disagreements), and CI's `test` and `souffle` jobs are green on main. `/retriage`
  picks the next mission; the queue has only off-mission P3s.
- Release: v0.1.29 is on 3f4f5fd. This session tagged v0.1.25 (#155), v0.1.26 (#157), v0.1.27
  (#161), v0.1.28 (#168) and v0.1.29 (#170), each with its upgrade note on the issue it closed.
- Parked as `waiting` when #86 closed, each with its trigger in a comment: #156, #167, #169.
- Hygiene for `/retriage`: #104 is closed but still labelled `mission:active`; #143 and #145 are
  `waiting` yet still carry priorities.
- No open threads or PRs. This run replaced the `str-distance` checkpoint's glance (stale since
  #157, #159 and the releases above).
