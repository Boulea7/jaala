# Next steps

Where each line of work left off. Work items live in GitHub issues; this file points at them.
Maintained by /checkpoint: one thread per branch, pruned when the branch merges.

## At a glance

- Mission #86 `mission_selfcheck`, 13/16: #149 closed by #157. Next ready: #23 (Soufflé, P2),
  #148 (a number spelled two ways, P2), #156 (a small derived relation scanned per probe, P3).
  `./selfcheck.sh` passed on 4a8fc0d, merged main with #157 and #159 together (4999/4999, no known
  disagreements, baselines within bounds, fuzz clean). #86 has no log comment for #157 yet.
- Release: v0.1.25 is on f9ca116 (#155). #157 (#149) and #159 (`str.distance`, #77) are merged
  and untagged; the next patch would go on 4a8fc0d. `str.distance` is additive and needs no
  upgrade note; check whether #157 does before tagging.
- Off-mission: #158 (case-insensitive `str.distance`, P3) filed from #77. #143 and #145 are
  `waiting` but still carry priorities (queue.sh hygiene).
- No open threads or PRs. This run dropped `str-distance` (#159, merged); `origin/str-distance`
  still exists on the remote.
