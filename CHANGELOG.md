# Changelog

## Unreleased — product

- 2026-09-13 — names: the enrolled thing is a connector, not a device
- 2026-09-13 — hosted: Plans in the cockpit, Stripe Checkout, Fakturownia+KSeF after payment
- 2026-09-12 — wdrozenie: command cheat sheet, a tighter guide, and Ubuntu in the diagram
- 2026-09-12 — wdrozenie: name the worker before joining, and how to remove the distro
- 2026-09-12 — wdrozenie: say which machine the restart means
- 2026-09-12 — cockpit: open a project without waiting on devices
- 2026-09-12 — cockpit: open a project without waiting on /api/devices
- 2026-09-12 — docs: several workers on a box means containers, not a second account
- 2026-09-12 — docs: the partner deployment guide, and the ignore rule that hid it
- 2026-09-12 — names: the last five draft numbers become the claim they stood for
- 2026-09-12 — gitattributes: pin the data files the tests read
- 2026-09-12 — Ignore the local graphify cache
- 2026-09-12 — names: drop the pointers a public reader cannot follow
- 2026-09-12 — console: the services table carries a port of its own
- 2026-09-12 — Generate the name registry from names/names.yaml
- 2026-09-12 — console: offer the hatch's own address with the key on it
- 2026-09-12 — gdesk: the console lists what this box runs
- 2026-09-12 — gdesk: the console links to the desktop it can see
- 2026-09-12 — gdesk: serve the operator console from the connector
- 2026-09-12 — Ignore agentize-generated rules and skills (agentize.auto.generated*).
- 2026-09-12 — gdeskseam: bound the operator ring by age as well as by count
- 2026-09-12 — gdeskseam: name the envelope version in the log instead of shrugging
- 2026-09-12 — Point the comments at the names that exist
- 2026-09-11 — Move the glass desk onto the gdesk vocabulary
- 2026-09-12 — gdesk: the glass desk moves onto the gdesk vocabulary. `initagent gdesk`,
  `~/.initagent/gdesk.yaml`, `INITAGENT_GDESK_*`, seam at `/gdesk` with `SEAM_VERSION` 2.
  The old command, filename and variables are still read.
- 2026-09-11 — deskseam: keep an operator ring and serve it at GET /desk/logs
- 2026-09-11 — desk: open a local seam from YAML, and accept OPENAI_API_KEY
- 2026-09-11 — deskseam: fan out to connections, and answer a command the desk refuses
- 2026-09-11 — Add internal/deskseam: the desk seam vocabulary the connector speaks
- 2026-09-11 — gateway: stop the timeout test racing its own setup
- 2026-09-11 — desk: turn an utterance into a streamed answer
- 2026-09-11 — desk: resolve who was addressed, and claim a turn once
- 2026-09-11 — Answer a desk turn over the openai dialect
- 2026-09-11 — Check Go and shell files out with LF
- 2026-09-11 — authz: gofmt the invite table
- 2026-09-11 — desk: seams, env configuration and retry policy for the desk loop
- 2026-09-10 — Record the hosted funnel so Administration can count acquisition before Stripe.
- 2026-09-10 — Rename generated rules to agentize.auto.generated.*; stop committing them (per-instance)
- 2026-09-10 — Unify scratch-scripts into one generic cascade rule
- 2026-09-10 — Unify workspace-manifest into one generic cascade rule
- 2026-09-10 — Unify where-to-commit into one generic cascade rule (mani.yaml is the source)
- 2026-09-10 — Purge spent secrets and delete idle free projects on a clock.
- 2026-09-10 — Plant the shared cascade/ rules
- 2026-09-10 — Keep finished task streams on the hub so hosted plans can purge them.
- 2026-09-10 — Drop the leftover composition-patterns AGENTS.md dump.
- 2026-09-10 — Let an owner invite a second person into an existing organization.
- 2026-09-10 — Pin Vite preview ports for site and cockpit.
- 2026-09-10 — Plant the generated leave-alone Cursor rule.
- 2026-09-08 — rules: add constraints gate pointer and track .cursor/rules
- 2026-09-08 — Pin CI/release Node to 22 (max, not 24)
- 2026-09-08 — Fix UI build and gateway WebSocket welcome write
- 2026-09-08 — Proxy device operations and fan out fleet views across the gateway
- 2026-09-07 — Vendor Vercel web-design and composition skills for site and ui.
- 2026-09-07 — Add a WebGL tile lift on the guest login art.
- 2026-09-07 — Make login art tiles 20px with a 100px lift and fill the pane.
- 2026-09-07 — Extrude login art tiles as cubes up to 1000px.
- 2026-09-07 — Show the first-task result in the same code box as the command, a shade darker
- 2026-09-07 — Read the Windows ConPTY exit code instead of inventing 1
- 2026-09-07 — Proxy the fleet terminal through the gateway so + Terminal works
- 2026-09-07 — Keep boarding open until the funnel finishes, and add I'll decide later
- 2026-09-07 — Enroll the self-host box as a worker when the first project is created
- 2026-09-07 — Show the initAgent wordmark in headers
- 2026-09-06 — Label the site GitHub control GitHub, not Source
- 2026-09-06 — Require a CLA on outside pull requests
- 2026-09-06 — Relicense the product under FSL-1.1-ALv2; Overseer stays MIT in NOTICE
- 2026-09-06 — Publish owned-package coverage to Codecov and add public README badges.
- 2026-09-06 — Cut v0.3.11 so a queued run can finish through process or send_keys and a reconnect does not repeat it

## v0.3.11 — 2026-09-06

- 2026-09-06 — Write a per-run done file so a reconnect does not repeat finished work
- 2026-09-06 — Let the cockpit Tasks page pick how a run finishes
- 2026-09-06 — Finish a queued run through process or send_keys, not only exec
- 2026-09-06 — Print the first-run claim token at the end of hub install
- 2026-09-06 — Cut v0.3.10 so hosted login is per client and an outdated connector takes no new work

## v0.3.10 — 2026-09-06

- 2026-09-06 — Key pre-auth rate limits on X-Forwarded-For only from trusted hops
- 2026-09-06 — Stop claiming onto a connector behind the advertised version
- 2026-09-06 — Name the product copyright PWare, not Pleware
- 2026-09-06 — Give every API token a subject, a boundary and a verb list
- 2026-09-05 — Serve every project from one gateway and route by the project's own gateway URL
- 2026-09-05 — Stop a fleet update from killing a run the connector is still serving
- 2026-09-05 — Stop the hosted platform admin from founding a company
- 2026-09-05 — Let a hosted customer reset a forgotten password in the language they signed up with
- 2026-09-05 — Queue outbound mail on the hub so hosted send can retry without a second process
- 2026-09-05 — Keep hosted plan numbers in one YAML catalogue for the hub and the site

- 2026-09-05 — Cut v0.3.9 so a project can enroll two machines and the free walls go live

## v0.3.9 — 2026-09-05

- 2026-09-05 — Let a project enroll more than one machine and refuse a third on free
- 2026-09-05 — Enforce hosted plan caps on create-project and a second person
- 2026-09-05 — Cut v0.3.8 so hosted boarding and the Plans pages go live

## v0.3.8 — 2026-09-05

- 2026-09-05 — Finish hosted boarding and put plans on their own site pages.
- 2026-09-05 — Site pages for Plans, For Developers, and Hardware
- 2026-09-05 — Hosted plans: $5 per person; site Plans section
- 2026-09-05 — Finish hosted boarding: template, first project without a worker, repo bind, enroll, first task
- 2026-09-05 — Call the hosted door Open app so a visitor does not have to already have an account
- 2026-09-05 — Put shared shadcn controls in web so both apps share one catalogue
- 2026-09-05 — Ask a new customer to name the organization and add a first project
- 2026-09-05 — Cut v0.3.7 so the hosted login card can switch themes

## v0.3.7 — 2026-09-05

- 2026-09-05 — Put the marketing nav on the unsigned-in hub
- 2026-09-05 — Cut v0.3.6 so the hosted site image includes the theme switcher

## v0.3.6 — 2026-09-05

- 2026-09-05 — Copy the shared theme into the site image so the page can build
- 2026-09-05 — Cut v0.3.5 so the hosted site and hub can switch themes

## v0.3.5 — 2026-09-05

- 2026-09-04 — Put a theme switcher on the cockpit and the marketing site
- 2026-09-04 — Cut v0.3.4 so a claimed hosted hub can mint a customer account

## v0.3.4 — 2026-09-04

- 2026-09-04 — Let a stranger open a customer account on a claimed hosted hub
- 2026-09-04 — Cut v0.3.3 so a live projects table can gain org_id

## v0.3.3 — 2026-09-04

- 2026-09-04 — Add project org columns before indexing them
- 2026-09-04 — Cut v0.3.2 with projects belonging to an organization

## v0.3.2 — 2026-09-04

- 2026-09-04 — Hang every project on an organization and the hub's existing gateway
- 2026-09-04 — Cut v0.3.1 with the default organization name

## v0.3.1 — 2026-09-04

- 2026-09-04 — Call an unnamed first organization default
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
