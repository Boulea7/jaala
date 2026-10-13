# Next steps

Where each line of work left off. Work items live in GitHub issues; this file points at them.
Maintained by /checkpoint: one thread per branch, pruned when the branch merges.

## At a glance

- Two active missions. #219 `mission_store_federation` has all 7 jaala tickets closed and waits on
  Declaire's `bench -time` over a federated view (declaire#199, #211). #235
  `mission_store_pushdown` is 1/6; next ready is #237 (a store evaluates a conjunction, with
  witnesses and `agreePushed`).
- Next action on `main`: post the mission logs for PR #245 on #219 and #235 (neither has one yet).
  #235's exercise has never been logged; its baseline (the `shipped` column and the four new
  questions at 1/10/50 stores) is in PR #245's body.
- Untagged on `main`: PR #245 (#233, #236) and PR #243 (#227), merged separately, so run the CI set
  and `./selfcheck.sh` on `main` (1f199fd) before tagging v0.1.45 (the owner's pick). Its upgrade
  note goes on #233 and declaire#211: `ns.StreamSource`, and a Federation's whole reads now count
  against `Budget`. #227 needs none: a query's number with a unit can now answer in the data's unit,
  as Naive always did.
- Off-mission and ready: #216 (P2); #179, #205, #206, #209, #215, #220, #222, #234 (P3); #207, #242
  and #244 `waiting`.
- This run: merged PR #243 (#227, closed) and dropped fix/demand-spelling-227; filed #244
  (`waiting`, a host generator reading a number before every binder has run).
