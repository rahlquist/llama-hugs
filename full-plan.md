# Llama Hugs — Full Implementation Plan

> **Status:** PLANNING ONLY. Derived from `~/llama-hugs/llama-hugs.md` (the overview,
> read in full). No implementation has been performed or authorized. Every task
> touching the live wimpy host carries an explicit approval gate.
>
> **Source of truth:** the overview. Where this document adds decisions, they are
> marked **[PROVISIONAL]** and must be confirmed in Phase 0 before dependent work starts.

---

## 0. Summary

Build "Llama Hugs," a companion console service beside stock llama-swap v250 on wimpy
(`wimpy.home.lan`, live production box): modern web UI, durable storage, agent/MCP
integration, plugins, benchmark presentation, lifecycle automation, multi-server
federation (stretch). llama-swap stays 100% stock; the console reads its API and
config and owns everything else.

**Non-goals:** forking llama-swap; replacing the router; auto-upgrading llama.cpp;
automatic model deletion; touching `evidence-*` dirs or `/etc/llama-swap/config.yaml`
directly.

---

## 1. Phase 0 — Decisions (must resolve before any code)

Open questions from overview §11, each with a recommended default:

| # | Question | Recommended default [PROVISIONAL] | Rationale |
|---|---|---|---|
| 1 | Name | **DECIDED: "Llama Hugs"** | Matches repo/dir name |
| 2 | Postgres timing | **DECIDED:** hand-rolled portable abstraction; SQLite now, PG in Phase 3 | Portable DDL from day one makes later PG cheap; stdlib-first honored |
| 3 | "NVIDIA data flywheel" referent | **DECIDED:** wait; design 7.15 only in Phase 4 | Blocks nothing before then |
| 4 | Pricing enrichment source | **DECIDED: configurable.** Pricing is a plugin slot with a `pricing_source` console setting (env var or settings row), never hardcoded. Initial candidate: OpenRouter — ToS check remains a real gate before enabling | Source swappable without touching dashboard code |
| 5 | SQLAlchemy vs hand-rolled dual-backend layer | **DECIDED (merged into Q2):** hand-rolled thin abstraction + portable DDL + backend-neutral dump migrator | Stdlib-first rule; surface area is small |
| 6 | MCP transport priority | **DECIDED: HTTP first** (hermesvm01 is the consumer), stdio second | The actual agent lives on the VM |
| 7 | Auth model | **OPEN — user requested clarification** (see §1c). Until decided: mutation surface stays blocked; P2-1 builds a pluggable auth hook on localhost only | Simplest thing that secures a LAN-exposed mutation API |
| 8 | Auto-prune appetite | **DECIDED: NO pruning for now.** Feature deferred until discussed; suggest-don't-execute remains the eventual default shape | Live box |
| 9 | New repo vs in-repo | **OPEN pending user read of §1c explanation**; working default: in-repo now, split at defined marker | Matches overview recommendation |
| 10 | UI stack | **OPEN pending user read of §1c explanation**; working default: single-file HTML/JS with explicit escalation threshold | <100 models, handful of views |
| 11 | YAML parsing *(agent-added)* | **DECIDED: PyYAML from day 1.** Approved dependency exception to stdlib-first (justified: config uses block scalars; hand-rolled parser already missed them once). Install approved when implementation starts; no parallel stdlib parser maintained | Correctness > zero-dep purity here |

Also confirm in Phase 0: port 8181 on 0.0.0.0 (deliberate, for hermesvm01), project layout, this document's sign-off.

**Phase 0 is a hard gate.** Tasks that assume a provisional default are BLOCKED
until that decision is signed off:

| Blocked task(s) | Waiting on |
|---|---|
| P2-4, P2-6 (transport/token identity) | Q6 (MCP transport), Q7 (auth model) |
| P3-6 (PG backend) | Q2/Q5 merged storage decision |
| P3-2 | Q4 (pricing source; if OpenRouter terms fail → fall back to local metadata only or postpone enrichment) |
| P4-1 | Q3 ("data flywheel" referent) |

Not blocked (proceed once repo layout confirmed): P1-1, P1-2 (parser fix +
tests); P2-1's auth/audit/dry-run *skeleton* may be built unauthenticated on
localhost with the token scheme plugged in after Q7 resolves — only the
exposure decision gates mutation-gated work.

**LAN exposure rule:** the console binds `127.0.0.1` during all development.
Widening to `0.0.0.0:8181` (for hermesvm01 agent access) is a separate,
explicit user-approval event — not part of "port 8181 is chosen." Reads expose
internal topology too, so the widening covers the read surface as well.

**Parser truth-validation (early, before dashboard code):** beyond unit tests
(P1-2), add an automated cross-check that the console's config-derived view
matches llama-swap's live `/v1/models` for every fleet model (paths resolved,
entries present). The console must never silently disagree with the router.

**Portable schema contract (define before P1-4 implementation):** enumerate
tables/columns/types up front; JSON stored as TEXT, parsed in-app; DDL free of
PG-only features; migration via backend-neutral dump format (not raw SQL dumps).
This makes Phase 3 PG a driver swap, not a rework. *Verification: DDL script
must run clean against SQLite and pass a PG syntax dry-check (e.g. pg_query or
a documented manual review) before P1-4 merges.*

**Gate semantics:** two classes, kept distinct throughout this plan.
- **BLOCKED** — irreversible or exposure-changing; cannot start without sign-off.
  Only these: widening bind beyond `127.0.0.1`, any live-config mutation
  (P1-3 deploy step), real model fetch/register/unload on wimpy (P2-2/P2-3),
  PG driver adoption if it changes the abstraction contract, pricing plugin if
  ToS unconfirmed.
- **PROCEEDS ON DEFAULT** — reversible work that starts immediately using the
  recommended default and re-evaluates if the default flips: parser fix + tests,
  storage abstraction shape, auth skeleton on localhost, MCP HTTP-first layout.

**Verification notes:** parser truth-validation ships as an executable assertion
(fails loudly when console view ≠ live `/v1/models`), not documentation; the
portable schema contract ships as the actual DDL file with the SQLite+PG checks
above. Both land before dependent code (P1-2 / P1-4 respectively).

---

## 1b. Phase 0 sign-off sheet (updated after round 1)

| Q | Decision | Remaining action |
|---|---|---|
| 1 Name | **DECIDED: "Llama Hugs"** | none |
| 2 Storage | **DECIDED:** hand-rolled portable abstraction; SQLite now, PG in Phase 3 | none |
| 3 Flywheel referent | **DECIDED:** defer to Phase 4 | none |
| 4 Pricing source | **DECIDED:** configurable plugin slot; OpenRouter candidate behind a ToS gate (configurability does NOT waive the gate) | ToS check before enabling |
| 5 Storage layer | **DECIDED:** merged into Q2 | none |
| 6 MCP transport | **DECIDED:** HTTP first | none |
| 7 Auth | **OPEN — see §1c.1** | user picks an option |
| 8 Auto-prune | **DECIDED: NO pruning for now** — feature undefined, not merely off; returns as its own planned item if wanted later | discussion when desired |
| 9 Repo placement | **OPEN — see §1c.2** | user picks an option |
| 10 UI stack | **OPEN — see §1c.3** | user picks an option |
| 11 YAML parsing | **DECIDED:** PyYAML from day 1 (justified dependency exception) | install at implementation start |

Plus one yes/no still pending: **approve widening to `0.0.0.0:8181` after Phase 1
MVP passes local verification?** (Required for hermesvm01 agent access.)

---

## 1c. Clarifications for the three open decisions

### 1c.1 Q7 — Auth model

The real axes: how many credentials, what each can do, where they live, and who
can revoke what. Options:

- **A. Single shared token** — one bearer token in config/env; every consumer
  uses it. Simplest possible. Audit log can't tell agents apart; rotation
  affects everyone at once.
- **B. Per-agent tokens** — each consumer (hermesvm01, local CLI, future
  agents) gets its own labeled token. Audit trail shows who did what;
  individual revocation without touching other consumers. Cost: slightly more
  config surface.
- **C. No auth + localhost-only bind** — safest exposure posture, but hermesvm01
  can't reach it without an SSH tunnel per use; awkward for the primary consumer.

**Recommended: B-lite** — implement a token *store* (list of `{token, label,
scope}` entries in console settings) from day one. Start with one entry
(equivalent to A in daily use), but adding a second labeled token for a new
agent later is a settings change, not a code change. Scopes (`read` vs
`mutate`) ride along for free: read tools for dashboards/agents, mutation tools
require a mutate-scoped token regardless of which one it is.

What's reversible: everything here — it's config shape, not architecture. What
isn't cheap to reverse is shipping mutations LAN-exposed without any of this.

### 1c.2 Q9 — Repo placement

- **In-repo (`~/llama-hugs/console/`)** — code lives beside this plan. One git
  history, one clone. Cost: planning docs mix with code; Docker build context
  needs `.dockerignore` trimming.
- **Separate repo** — clean boundaries, independent versioning, clean Docker
  builds. Cost: second repo to maintain; plan↔code coordination across repos.

**Recommended: in-repo now**, with a defined split marker rather than a vague
"later": split when the Docker image is built AND published AND you want clean
release tags. Splitting later is a `git subtree split` — mechanical. Merging a
prematurely-split repo back is not.

### 1c.3 Q10 — UI stack

- **Single-file HTML/JS** — one `index.html`, inline script, no build step, no
  npm/node on wimpy at all. Right size for <100 models and a handful of views.
  Wall: interactive state that must survive across views (e.g., benchmark
  compare with persistent selections).
- **Multi-page + shared JS** — several HTML files plus a shared `app.js`. Still
  no build step. The middle rung.
- **Frontend framework (Vue/Svelte/etc.)** — component model, but drags npm, a
  bundler, node_modules onto a 32 GB live inference box. Overkill today.

**Recommended: single-file for the MVP**, with an explicit escalation trigger:
the moment a view needs persistent cross-view interactive state or the file
passes ~2000 lines, refactor to multi-page + shared JS (no framework). Framework
evaluation only if multi-page then shows its limits. The Python API is
stack-agnostic — this choice never touches the backend.

---

## 2. Task breakdown

Format: ID · task · dependencies · approval needed · verification.

### Phase 1 — MVP dashboard, durable, deployed

| ID | Task | Deps | Approval | Verification |
|----|------|------|----------|--------------|
| P1-1 | Integrate PyYAML into prototype `server.py`; verify GGUF paths resolve for all entries | — | none | `/api/models` shows real paths × all entries; zero unresolved |
| P1-2 | Unit tests for config loading against the real repo config (block scalars, comments, aliases) via PyYAML; keep the truth-validation cross-check vs live `/v1/models` as executable assertion | P1-1 | none | Tests pass; console view matches live router for every entry |
| P1-3 | Enable llama-swap stock store (7.2A): timestamped backup of config; sudo mkdir/chown `/var/lib/llama-swap` for service user; add `store:` block; deploy via approved flow | P0 sign-off | **sudo + config change — show exact commands, wait** | Activity survives a llama-swap service restart (query `/api/metrics/activity` before/after) |
| P1-4 | Console DB schema v1 (SQLite, stdlib): `models_extra` (tags, notes, first_seen, registered_at), `settings`, `schema_migrations`; portable DDL (JSON-as-TEXT) behind a storage-abstraction interface | P1-1 | none | CRUD round-trip test; DDL runs clean on SQLite (and dry-checked against PG syntax) |
| P1-5 | Dashboard v2: model detail drawer, orphan-file detection view, activity feed from llama-swap store API, HF-link backfill for the 3 missing entries | P1-1, P1-3, P1-4 | none | All fleet models render with status/GPU/tags/ctx/size/cutoff/link; search/sort/filter on live data |
| P1-6 | Orphan scan: GGUFs on disk with no entry; entries pointing at missing files. Read-only | P1-5 | none | Detects known-broken entries correctly |
| P1-7 | `tools:` capability audit → list entries that should set `capabilities.tools: true` | — | none (config change itself goes through approval flow separately) | Audit list reviewed by user |
| P1-8 | Run console as user systemd unit OR document manual start; Dockerfile draft | P1-5 | systemd unit needs approval | Service survives logout / container builds and serves dashboard |
| P1-9 | Commit everything; keep evidence | all | git push per wimpy-setup rules (fetch/divergence check first) | Repo state committed |

### Phase 2 — Agent surface

| ID | Task | Deps | Approval | Verification |
|----|------|------|----------|--------------|
| P2-1 | Mutation API skeleton: token auth (**built as a pluggable auth hook with placeholder until Q7 resolves**), audit log (timestamp+caller+action; file/stdout sink acceptable until P1-4 lands), dry-run default | P1-1 | none (localhost dev) | Unauthenticated requests rejected; every mutation logged |
| P2-2 | Fetch/register pipeline absorbing fetch-model.sh exactly: `POST /api/models/fetch` (hf download, SSE progress), smoke gate with production GPU pins (refuse registration without pin), propose config entry (group auto-suggested, never auto-applied), write REPO config, deploy only with approval | P2-1 | Each live mutation; deploy step always user-approved | Full dry-run pass; one real fetch+register end-to-end with gates |
| P2-3 | Unload/swap/status endpoints: `/unload` passthrough, load-by-request, status poll. Enforce unload + pid-exit wait between large loads (2026-08-15 OOM rule) | P2-1 | Live-box mutations gated | Sequential 24–27 GB model swaps complete without OOM |
| P2-4 | MCP server: HTTP transport first, stdio second. Read tools ungated; mutation tools gated | P2-1, P2-2 | none | Agent on hermesvm01 lists/searches models over HTTP MCP |
| P2-5 | Hermes skill `wimpy-model-console` documenting HTTP API + curl patterns; evaluate plugin wrapper (don't build both speculatively) | P2-4 | skill install | Any Hermes profile drives console read APIs from the doc alone |
| P2-6 | Docker image finalized; compose file (config + cache read-only mounts, console DB volume; host-network vs bridge decided here) | P1-8 | none (Docker already installed) | Container serves dashboard and reaches llama-swap :8080 |

### Phase 3 — Enrichment & benchmarks

| ID | Task | Deps | Verification |
|----|------|------|--------------|
| P3-1 | Plugin contract `enrich(model_id, metadata) -> dict` + refresh schedule; directory-confined loading, no network code fetch | P1-4 | Reference plugin loads and populates badges |
| P3-2 | Pricing plugin behind a configurable `pricing_source` setting (OpenRouter is one candidate; ToS check gates enabling ANY source — configurability does not waive it). Dashboard renders pricing generically ("present if a plugin supplied it"), never an OpenRouter-specific column | P3-1 | Prices render as columns/badges when a source is enabled; dashboard degrades cleanly with no source configured |
| P3-3 | Benchmark ingestion into console DB (`benchmark_runs`, `benchmark_results`); leaderboard UI (tokens/s by model×task, trends, regression flags). Preserve sweep safety rules; console presents results, doesn't schedule | P1-4 | Nightly results browsable as flat comparison |
| P3-4 | Custom pages (7.3): `console/pages/` static fragments/markdown under "Custom" nav | P1-5 | User-added page renders |
| P3-5 | Display settings (7.8): visible columns, order, default sort, filters persisted per user | P1-4 | Settings survive restart |
| P3-6 | Postgres backend (`psycopg`) + bidirectional migration tool using backend-neutral dump format | P1-4 | Round-trip SQLite↔PG preserves rows; acceptance criterion met. **Migration discipline:** each migration is a discrete, approval-gated task — pre-migration dump of the source backend, one direction at a time (never auto-sync), post-migration row-count + spot-check verification against the dump, documented revert = restore from dump. No background/scheduled migrations ever. |

### Phase 4 — Stretch

| ID | Task | Deps | Notes |
|----|------|------|-------|
| P4-1 | Multi-server federation (7.15): confirm "nvidia data flywheel" referent with user FIRST; design server registry; evaluate llama-swap `peers:` vs console-level federation | P0 Q3 resolved | Requires `server_id/model_id` qualification kept ready since Phase 1 |
| P4-2 | Read-only upstream upgrade advisories (llama.cpp, llama-swap versions) | P1-5 | Display only; execution stays manual |

Dependency spine: **P1-1 → P1-4 → (everything DB-backed)**; **P1-3 unblocks activity feed**; **P2-1 gates all Phase 2**; **P0 sign-off gates P1-3**.

---

## 3. Constraints applying to every phase (from overview §6, binding)

1. No sudo/service/network changes without explicit approval — exact commands shown first.
2. Console read-only against llama-swap until Phase 2; mutations only via backup → edit repo config → deploy.
3. Plain process first, Docker second, systemd last.
4. Stdlib-first Python; new deps declared and justified individually.
   **Approved exception: PyYAML** — decided at Phase 0 (Q11) for config parsing
   from day 1. Any further dependency still needs its own justification.
5. Data never discarded; migrations additive or backed up.
6. Port 8181; no conflict with 8080/9119/benchmarks.
7. Everything claimed about models verifiable against live API/config — no cached truth that drifts. Truth hierarchy: live `/v1/models` > repo config > console DB; conflicts flag, never override.
8. Never touch `evidence-*` dirs; never store artifacts in tmpfs `/tmp`.
9. **No assumed context:** nothing about llama-swap internals, config quirks,
   or prior conversations is trusted beyond what the overview and this document
   state. Any step needing an unwritten fact is verified on the machine or asked.
10. Live-box changes are approved one at a time — never batched — each with its
    own backup and documented backout.

## 3.1 Truth reconciliation workflow (live API vs repo config)

When the console's config-derived view disagrees with llama-swap's live
`/v1/models` (entry present in one but not both, or status mismatch):
1. Surface it as a first-class "discrepancy" badge/view entry — never silently
   pick a winner or hide either side.
2. Show both values side by side (config says X, router reports Y) with
   timestamps of when each was observed.
3. Likely causes are suggested but not asserted: pending deploy (config edited,
   not yet deployed), stale repo copy, hot-reload lag, broken model removed from
   one side only.
4. Resolution is always via existing flows — deploy pending config, or fix the
   repo — never by editing console state to match. The console DB never absorbs
   the discrepancy; it only records that it existed.

## 3.2 Phase 2 mutation acceptance criteria (definition of done)

P2-2/P2-3 are not "done" on one happy-path run. Required observable tests:
- **Pin enforcement refusal:** fetch/register attempt without a GPU pin is rejected with an explicit error; no files registered.
- **Smoke gate:** registration blocked when smoke fails; failure surfaced to caller.
- **Proposed ≠ applied:** proposed config entry exists only in repo working tree until explicit deploy approval; audit log shows proposal and deploy as separate events.
- **Unload + pid-exit wait:** sequential large-model swaps show the llama-server pid exit between loads (observable in logs); no OOM.
- **Audit completeness:** every mutation logged with timestamp, caller, action, dry-run flag; dry-run default verified by attempting a mutation without enabling execution.
- **Revert path:** documented + tested rollback for each step (restore backup config → redeploy; delete fetched GGUF; remove proposed-but-not-deployed entries).

## 4. Risk register

| Risk | Mitigation |
|------|-----------|
| Second writer to live config → drift | Repo-config-then-deploy only; backups; approval gates; never touch `/etc` directly |
| Mutation API exposed on LAN | Token auth from day one; audit log; dry-run default; consider 127.0.0.1 bind + SSH tunnel alternative |
| YAML parser breaks | Unit tests vs real config (P1-2); PyYAML if fragility recurs |
| RAM pressure on 32 GB box | Console touches metadata only; lazy file reads |
| OOM during console-triggered loads | Unload + pid-exit wait + non-tmpfs result paths baked into fetch tool |
| Fork-creep | Anything needing llama-swap source changes rejected by default; decision record revisited each phase gate |
| Plugin security | Directory-confined, no network code fetch, review before enable |
| Scope sprawl | Phases are the control; stretch deferred |

## 5. Rollback plan

- Config changes: timestamped backup restored via approved deploy flow (rule 1.4.1).
- Store enablement (P1-3): remove `store:` block + redeploy; data dir retained.
- Console DB: file delete/recreate is safe by design (presentation facts only).
- systemd unit / Docker: stop + disable removes all footprint; no system packages installed beyond declared deps.

## 6. Acceptance criteria (= overview §12)

Stats survive restart ✓(P1-3) · full-fleet dashboard with search/sort/filters ✓(P1-5)
· orphans surfaced ✓(P1-6) · agent read via MCP/HTTP ✓(P2-4) · agent fetch+register
with fetch-model.sh safety gates ✓(P2-2) · benchmarks browsable ✓(P3-3) ·
SQLite↔PG switchable both ways ✓(P3-6) · zero llama-swap source modifications ✓(all)
· runs as bare process / Docker / MCP ✓(P1-8, P2-6, P2-4).

---

*No step of this plan has been executed. Execution begins only after Phase 0 sign-off and per-task approvals.*
