# Next steps

Where each line of work left off. Work items live in GitHub issues; this file points at them.
Maintained by /checkpoint: one thread per branch, pruned when the branch merges.

## At a glance

- Mission #172 `mission_aggregate_answers`, 3/7. #84 is the last unmet done-when line, and its PR
  (#184) waits on the contributor; #128's (#185) too. #80 (P3, mixed-unit aggregates) is the one
  mission ticket with no PR, so it's next for us (`/start-pr 80`). Exercise last ran at bb2398a
  (works); one code PR (#187) merged since, its CI Soufflé job green.
- #186 (a recursive clause binding its head variable outside the cycle was ignored when typing) is
  fixed on `main` by #187, untagged. Tag it with #84 so agni takes one signature change, not two;
  #187's PR body carries the upgrade note.
- External PRs, both from Boulea7, both changes requested on 2026-10-11:
  - #184 (#84): rebase onto #187 and drop `headProbe`; a bare number takes the column's one unit;
    `Inferred` and `ns/types.go` unchanged; reword the `bodyagg.go` comment (still true for min/max).
  - #185 (#128): `order by` doubles the `query:` prefix too (`parseOrderBy`); one shared wrapper.
  - Before #184 merges: agni's suite through a scratch `go.work`, and the 5000-seed Soufflé step.
- This run: replaced the v0.1.31 glance (no open PRs then). `origin/recursive-clause-types` is
  merged (#187) and can be deleted.
