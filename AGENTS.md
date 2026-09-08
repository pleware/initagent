# Agents

Read [`CONSTRAINTS.md`](CONSTRAINTS.md) before writing code in this
repository. Do not weaken it to make a change pass.

## Drafts

Design status lives in the parent workspace, not in this git:

[`../drafts/99.NEEDS-ATTENTION.md`](../drafts/99.NEEDS-ATTENTION.md)

Do not create `drafts/` here. If a file appears, it is local scratch
(gitignored) — move the decision up in the same turn.

Owned coverage (≥ 90% statements on packages listed in `owned-packages`) is a
blocking gate. The fake coder, when added, is owned code and must meet the
same bar. Run `./scripts/check-owned-coverage.sh` before handing work back.
