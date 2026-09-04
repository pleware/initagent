# Changelog

## Unreleased — product

- 2026-09-04 — Prove the organization backfill on Postgres before shipping it
## v0.3.0 — 2026-09-04

- 2026-09-04 — Stop advertising self-host as single-user in the README
- 2026-09-04 — Give a hub organizations and two separate people surfaces

## v0.2.0 — 2026-09-04

- 2026-09-04 — Claim a fresh hub with an account and a one-time token
- 2026-09-04 — Lead the site with hosted CTAs and read hub offering at serve start.
- 2026-09-04 — Read hub offering from the data-dir file at serve start
- 2026-09-04 — Point the marketing Software CTAs at hosted first, self-host second
- 2026-09-03 — Hold the entry point path in brand.CommandDir
- 2026-09-03 — Deduplicate the device joiner into internal/join
- 2026-09-03 — Rename the entry point directory to cmd/initagent
- 2026-09-03 — Give the hub UI Docker stage the theme tokens it imports
- 2026-09-03 — Split cockpit colours into theme families behind a resolver
- 2026-09-03 — Bump CI/release actions to Node 24 runtimes
- 2026-09-03 — Bump docker actions to Node 24 runtimes
- 2026-09-03 — Add the hub and site Dockerfiles and publish images to GHCR
- 2026-09-03 — Let the hub run on Postgres through an internal/store dialect seam
- 2026-09-03 — Move the task registry to internal/registry/ai/capability and add internal/registry/db/kinds
- 2026-09-03 — Fix Windows test failures (hub SQLite handle, agent PATH IsAbs)
- 2026-09-03 — Proxy task create/get from the hub to the gateway and add a cockpit Tasks page
- 2026-09-03 — Route task completion through the resolver registry.
- 2026-09-03 — Dispatch tasks over the agent WebSocket with TypeExec.
- 2026-09-03 — Persist claim, lease, and heartbeat on the gateway SQLite queue.
- 2026-09-03 — Document the hosted hub at app.initagent.dev on PostgreSQL.
- 2026-09-03 — Note that the hosted hub is PostgreSQL, not SQLite.
- 2026-09-03 — Point README at initagent.dev (site) and app.initagent.dev (hosted hub).
- 2026-09-03 — Point public site and hub installers at initagent.dev.
- 2026-09-03 — Point README and the marketing install commands at initagent.dev
- 2026-09-03 — Fix the Unix installer test after the binary rename.
- 2026-09-03 — Fix a missing `]` in the Unix installer harness
- 2026-09-03 — Rename product identity from Overseer to initagent (module, services, env, installers)
- 2026-09-03 — Enroll workers on the gateway (tokens, install scripts, agent WS)
- 2026-09-03 — Watch a per-run done file at high trust
- 2026-09-03 — Clarify fpr- as a SaaS project reached through MCP
- 2026-09-03 — Register fpr- for a tool's own project on a worker
- 2026-09-03 — Gate gofmt on owned packages in CI
- 2026-09-03 — Move fake coder behavior under the owned coverage gate
- 2026-09-02 — Add Milestone 0 foundation: fake coder, completion signal, scheduler
- 2026-09-02 — Record the product changelog entry for coverage finalization.
- 2026-09-02 — Add coverage gate, constraints, and brand tests (Milestone 0 Step 2 finalization).
- 2026-09-02 — Add i18n setup with react-i18next (English only for now).
- 2026-09-02 — Add internal/ai/capability package with HuggingFace-aligned task registry.
- 2026-09-02 — docs: start the product changelog
- 2026-09-02 — feat: add brand and id packages with prefixed identifiers
