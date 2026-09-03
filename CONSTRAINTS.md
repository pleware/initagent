# Constraints

Last reviewed: 2026-09-02

Hard quality bar for **agent-written and human-written** product code.
Agents must read this before changing `initagent/`. Do not weaken this
file to make a change pass — raise tests, not the ceiling.

Sibling repos (ops, infra, workspace kit) keep their own gates; this file
owns the **public product** (`initagent`).

## Floor (always enforced)

- No new suppression comments that silence checks (`//nolint` without a
  one-line reason on the same line, empty `catch`, bare `panic` for
  expected I/O failure).
- No skipped or deleted tests without a reason in the commit message.
- No secrets in source.
- No lowering a number in this file in the same commit as the feature that
  was failing that number.
- Do not duplicate tests for untouched upstream Overseer code (`19`).

## Owned code (coverage scope)

Coverage applies **only** to packages listed in `owned-packages`. That list
is our additive surface (`brand`, `id`, future completion/scheduler/
protocol helpers, **fake coder**, …). Upstream packages we have not carved
out are **out of scope** — do not invent tests there to inflate a number.

When you add a new first-party package, append it to `owned-packages` in
the same commit. **Fake coder is owned code** the moment it lands: same
threshold, same gate. Design inspiration may live under
`../external/inspirations/` (umbrella); never `replace` those checkouts
into this module.

## Enforced with numbers

| Dimension | Rule | Checked by | Runs at |
| --- | --- | --- | --- |
| Format | `gofmt` clean on `owned-packages` | `./scripts/check-owned-format.sh` | every edit, CI |
| Vet | zero issues on owned packages | `go vet $(cat owned-packages)` | every edit |
| Unit | all owned package tests pass | `go test $(cat owned-packages)` | every edit / CI |
| **Coverage** | **≥ 90% statements** on the union of `owned-packages` | `./scripts/check-owned-coverage.sh` | task end, CI |
| Naming lint | banned nouns / prefix registry (`05`, `06`) | existing `internal/id` AST tests + future lint (`19`) | CI |
| Licence | release NOTICE current | `go-licenses` / checker (`19`, `docs/LICENSING.md`) | release CI |

**Why 90%.** Agents and orchestration will write most of this tree. A soft
or deferred coverage bar becomes untested infrastructure. 90% on *our*
packages is high enough that a new helper needs a table test, and scoped
tight enough that Overseer noise cannot dilute it. Global `./...` coverage
is intentionally **not** a gate.

**Fake coder.** It is a test double for harnesses (`19`, `12`), not an
excuse to skip coverage. Controllable exit codes, delays, hang-until-kill,
and sentinel writers are deterministic branches — they must be tested at
the same 90%.

## Measured, not yet enforced

| Metric | Today | Direction |
| --- | --- | --- |
| Owned packages in list | `internal/brand`, `internal/id`, `internal/ai/capability`, `internal/completion`, `internal/scheduler`, `internal/fakecoder`, `internal/gateway` | grow only when we add first-party code |
| Product E2E smoke (Milestone 0) | absent | add with step 3 (`02`); not a coverage substitute |

## Exceptions

| ID | Rule | Path | Reason | Owner | Expires |
| --- | --- | --- | --- | --- | --- |
| _(none)_ | | | | | |

Const-only packages still need a `*_test.go` that locks exported identity
strings (so a rename cannot silently break installers). That is not an
exception — it is the minimum test.

## Agent contract

1. Read this file before writing product Go.
2. After behavior change: update tests **in the same turn**, run
   `./scripts/check-owned-coverage.sh`, then `go test ./...` as in
   `.cursor/rules/tests-sync.mdc`.
3. If coverage fails: add tests. Do not edit `owned-packages` to drop a
   package, do not lower 90%, do not add exclusions.
4. Upstream files you only touch for a brand string still do not enter
   `owned-packages` until that package is ours outright.
