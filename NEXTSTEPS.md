# Next steps

Where each line of work left off. Work items live in GitHub issues; this file points at them.
Maintained by /checkpoint: one thread per branch, pruned when the branch merges.

## At a glance

- datalog-functions — #208, PR #214 (CI green, selfcheck and hosts clean) — next: owner review and
  merge, then tag (no host upgrade note needed: agni's and Declaire's failure lists are unchanged).
- Mission #193 `mission_browser_memory`: jaala's tickets shipped in v0.1.33. agni PR #976 builds
  #896 on it; when #896 closes, post #193's closing log and close it.
- Mission #172 `mission_aggregate_answers`, 4/6. Both open tickets wait on the outside contributor
  Boulea7: #84 through PR #184, #128 through PR #185, changes requested, no reply since
  2026-10-12. If they stay quiet, finish them here and close the PRs with credit.
- v0.1.37 (#5) asks agni to reconcile its C24 (scale converted upstream, once) with query literals
  now folding prefixes.
- Arithmetic follow-ups: #205, #206, #207 (`waiting`), #209; #215 (P3, filed from #214: a head
  variable bound only by a filter passes validation, and evaluators then disagree on a broken goal).
- Off-mission and ready: #179 (P2, timed facts), #211 (P2, a Source over several sources).
- This run replaced the post-v0.1.37 glance.

## datalog-functions

- **Last touched**: 2026-10-12 (sri's machine:/home/sri/newstack/jaala/main)
- **Ticket**: #208 (PR #214)
- **Why**: let a one-rule datalog function (`power(?v, ?i, ?p) :- ?p = ?v * ?i`) be called as
  `power(?v, ?i)`, by making #200's input detection read generator modes (`bindsFrom` in
  `datalog/inputs.go`).
- **Where it stopped**: done and pushed; all checks green. A self-comment on the PR explains that
  the head's left-to-right input order only names inputs in the signature.
- **Next action**: after merge, run the CI set and `./selfcheck.sh` on main, tag, release page, and
  an upgrade note on #208 saying hosts need no change.
