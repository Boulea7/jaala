# Next steps

Where each line of work left off. Work items live in GitHub issues; this file points at them.
Maintained by /checkpoint: one thread per branch, pruned when the branch merges.

## At a glance

- Two active missions. #219 `mission_store_federation` has all 7 jaala tickets closed (#233 merged
  in PR #245) and waits on Declaire's `bench -time` over a federated view (declaire#199, #211).
  #235 `mission_store_pushdown` is 1/6 (#236 merged in PR #245); next ready is #237 (a store
  evaluates a conjunction, with witnesses and `agreePushed`).
- Next action on `main`: post the mission logs for PR #245 on #219 and #235. #235's exercise has
  never been logged; its baseline (the `shipped` column and the four new questions at 1/10/50 stores)
  is in PR #245's body. `./selfcheck.sh` passed with Soufflé at 5000 seeds on the branch, and the
  merge (556c547) changed nothing else.
- PR #245 is untagged. The next tag (v0.1.45, the owner's pick) needs an upgrade note on #233 and
  declaire#211: `ns.StreamSource`, and a Federation's whole reads now count against `Budget`.
- fix/demand-spelling-227 (another clone): PR #243 open for #227, no thread recorded on it.
- Off-mission and ready: #216 and #227 (P2); #179, #205, #206, #209, #215, #220, #222, #234 (P3);
  #207 and #242 `waiting`.
- This run: retriaged (mission #235 and tickets #236 to #241 filed, #233 to P1 on #219, #242 filed
  `waiting`); merged PR #245; deleted merged branches datalog-functions, federation,
  fix/selfcheck-replay-226, indexed-lookups and stores-benchmark, and the indexed-lookups worktree.

## Across threads

- FuzzEval in `./selfcheck.sh` can find #227 (`1A` vs `1 V`, Naive and planned disagree) until PR
  #243 merges.
