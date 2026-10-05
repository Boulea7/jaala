# Next steps

Where each line of work left off. Work items live in GitHub issues; this file points at them.
Maintained by /checkpoint: one thread per branch, pruned when the branch merges.

## At a glance

- Mission #86 `mission_selfcheck`, 12/15: #89 closed by #151 (fuzzed query text; `./selfcheck.sh`
  now fuzzes each target for `JAALA_FUZZ_TIME`). Next ready: #23 (Soufflé), #148 (a number spelled
  two ways, which `FuzzEval` counts rather than fails), #149 (demand re-derives round zero), all P2.
  `./selfcheck.sh` passed on 25cd296 (4999/4999, no known disagreements, fuzz clean).
- Release: v0.1.24 is tagged on 25cd296 (#150 Explain, #151, #137). The upgrade note is on #89:
  no API change, but some queries now get a different error, an error, or an answer (comparisons
  are order-free and checked statically; head `_` and empty pieces are refused).
- Off-mission: #147 (Explain, P2) stays open after #150 by choice; #126 (P2) serves no mission;
  #145 and #143 are `waiting`.
- No open threads or PRs. This run dropped `fuzz-text` (#151, merged).
