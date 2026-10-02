# Next steps

Where each line of work left off. Work items live in GitHub issues; this file points at them.
Maintained by /checkpoint: one thread per branch, pruned when the branch merges.

## At a glance

- main — #4 (PR #73 merged) — next: tag v0.1.18 on `3e736bb` and post the upgrade note on #4
- This run: created the file; no other branch carried a copy

## Across threads

- `demand-all-free` (#60, another clone, no PR yet) rewrites `magic.go`'s all-free calls. It already
  keeps an aggregating relation (#4) read in full, and merges cleanly onto `3e736bb`.

## main

- **Last touched**: 2026-10-02 07:09 (dev:/workspace/repos/newstack/jaala/main)
- **Ticket**: #4 (PR #73 merged); follow-ups #71 (body aggregate, zero counts), #72 (demand on group keys)
- **Why**: rules can aggregate, for agni's "exactly two" and Declaire #69's min and count workarounds.
- **Where it stopped**: #73 merged as `3e736bb`, untagged. v0.1.17 has #61, #62, #65 and #68.
- **Next action**: tag `v0.1.18` on `3e736bb` (the owner picks the number), then comment on #4 with
  the upgrade note: new syntax only, one rule per aggregating relation, no zero groups (#71).
- **Open questions**: whether #72 is worth it before a host shows an aggregating relation in a
  profile; agni's `net.role`-style callers would be the first to.
