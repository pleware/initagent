# Contributing

initagent is source-available under [FSL-1.1-ALv2](LICENSE). Upstream
Overseer remains MIT; see [NOTICE](NOTICE).

## CLA

Every outside contribution needs a signed [Contributor License
Agreement](CLA.md). The pull-request check fails until each author (except
allowlisted bots and `pleware`) comments, exactly:

```
I have read the CLA Document and I hereby sign the CLA
```

That comment is the signature. It is stored on the `cla` branch, not on
`main`. Comment `recheck` if the check is stale.

If you contribute on behalf of an employer, also complete Part B of
`CLA.md` (open an issue titled **Entity CLA**).

## Code

Read [CONSTRAINTS.md](CONSTRAINTS.md) before writing code. Do not weaken it
to make a change pass. Owned packages must stay at or above 90% statement
coverage (`./scripts/check-owned-coverage.sh`).
