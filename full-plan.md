# Llama Hugs — Implementation Plan (Fork/Replace Architecture)

> **Status:** PLANNING ONLY. Supersedes the previous companion-service plan
> (git history preserves it). Derived from `~/llama-hugs/llama-hugs.md` plus the
> user's explicit Phase 0 verdicts, including the decisive one:
> **Llama Hugs forks llama-swap and eventually REPLACES it. It is not a
> companion service.** During the build it must stand up alongside the live
> llama-swap on wimpy WITHOUT modifying its directory structures, config,
> binaries, or anything about it.
>
> Items marked **PROVISIONAL** are open Round-2 decisions; they gate dependent
> tasks but not this document's existence.

---

## 0. Summary

Llama Hugs is a fork of llama-swap (Go, Svelte web UI) evolved into the full
platform: modern UI, durable storage beyond the stock activity store, agent/MCP
integration, plugins/enrichment, benchmark presentation, lifecycle automation,
multi-server federation. End state: Llama Hugs replaces llama-swap on wimpy.
Build state: both run side by side, zero interference with the live router.

**Hard coexistence constraint (binding at all times):**
- Llama Hugs never writes inside llama-swap's directories (`/etc/llama-swap/`,
  `/usr/local/bin/llama-swap`, its store file, systemd unit).
- Llama Hugs binds its OWN port (never 8080 while llama-swap is live).
- During coexistence Llama Hugs READS llama-swap's API/config/model files;
  it does not proxy-manage them until cutover.
- Any interaction with the live service (even reads) follows live-box rules:
  approval gates, no sudo/service changes unapproved.

**Non-goals:** modifying the running llama-swap in any way during the build;
auto-upgrading llama.cpp; automatic model deletion (decided Q8: no pruning);
touching `evidence-*` dirs or storing artifacts in tmpfs.

---

## 0a. Coexistence operating rules (binding, F1–F3)

**Do-not-touch inventory** (checksummed at F1 start, verified after every
Llama Hugs deployment):
- `/usr/local/bin/llama-swap` (binary)
- `/etc/llama-swap/config.yaml` and everything under `/etc/llama-swap/`
- llama-swap's systemd unit (`llama-swap.service`) — never stopped/restarted/edited by this project
- llama-swap's store file (`/var/lib/llama-swap/` if enabled)
- port 8080

**Identity separation:** Llama Hugs ships as its own binary name
(`llama-hugs`), own install dir (`/opt/llama-hugs/`, created only via
approval), own config file name, own store path, own port (8181). No alias,
symlink, or wrapper touching the old binary. Cutover = repointing clients;
decommission of llama-swap is a SEPARATE future approval with its own
checklist — "first day of replacement use" ≠ "removal day."

**Read-vs-manage boundary:** during F1 all access to the live router is
observational reads of documented GET endpoints (`/v1/models`, `/api/*`,
`/metrics`). Any endpoint that could mutate state or trigger a model load is
off-limits until Phase F4, and even read traffic stays low-rate polling.

**Approval triggers during build:** creating `/opt/llama-hugs/`, any new port
binding beyond localhost, any systemd unit for Llama Hugs, first non-test
model load, cutover repoint, decommission — each is a discrete approval event.


---

## 1. Phase 0 — Decision ledger

### 1a. Decided (round 1 + fork verdict)

| Q | Decision |
|---|---|
| Name | **Llama Hugs** |
| Path | **Fork & replace** (not companion) |
| Coexistence | Alongside live llama-swap; never modify its files/ports/processes |
| Pruning | NO pruning feature for now (Q8) |
| Pricing enrichment | Configurable source slot; ToS check gates any source (Q4) |
| LAN exposure | Widen to 0.0.0.0 after Phase-local verification passes (Q-LAN: yes) |
| Version control | Git repo initialized at ~/llama-hugs |

### 1b. Round-2 decisions (PROVISIONAL — each blocks its dependent tasks)

| # | Question | Options / considerations | Recommended default |
|---|---|---|---|
| R1 | Fork base & cadence | Fork upstream master (v250-era commit) pinned; sync policy vs upstream's 1–2-day release train (cherry-pick critical fixes only?) | Pin a tagged commit; manual selective merges; document every divergence |
| R2 | Repo layout | Separate repo for the fork (`~/llama-hugs/hugs/` or fresh clone dir); plan repo stays separate | Fresh clone of upstream renamed `llama-hugs`, remotes: origin=fork repo, upstream=mostlygeek |
| R3 | Build toolchain location | Go builds are cheap anywhere; Node/npm needed only to build the Svelte UI. Build on wimpy vs build elsewhere + deploy binary+assets | Toolchain on a dev host; wimpy receives built binary + embedded assets. No npm on the live box |
| R4 | Runtime model during build | Llama Hugs runs read-only observer first (own port, reads llama-swap API/config), gains routing only against TEST configs/models before any cutover | Staged: observer → parallel router on scratch models → cutover |
| R5 | Storage scope in-fork | Extend upstream's SQLite store (new tables/migrations via goose) vs console-style separate DB; Postgres support later? | Stay on upstream's modernc.org/sqlite store, add migrations in-fork; PG deferred |
| R6 | Auth model | Upstream has static `apiKeys` bearer tokens. Match exactly for drop-in client compat, then add labels/scopes in-fork | Ship upstream-compatible apiKeys first; scoped/labeled tokens as fast-follow (keeps hermesvm01/clients working through cutover) |
| R7 | Agent/MCP surface timing | MCP server in-process (Go) vs sidecar; MVP scope | Read tools early, mutation gated; transport HTTP-first per earlier verdict |
| R8 | Replacement trigger & cutover | What defines "done enough to replace": feature bar (list), dual-run soak period, traffic-switch mechanism (change Hermes clients' endpoint to Llama Hugs' port; llama-swap untouched until decommission day) | Explicit checklist: parity features + N days soak + user sign-off; cutover = repoint clients, not touching llama-swap files |
| R9 | Feature roadmap ordering | Which of the old §7 features land pre-cutover vs post-cutover | Pre-cutover: persistent store, UI overhaul, search/tags/disk view. Post-cutover: agent mutation API, plugins, benchmarks ingestion, federation |
| R10 | Divergence budget | Policy limiting how far the fork drifts (every PR either tracks an upstream-file change or is isolated in new packages/dirs) | Isolate new code in separate packages; minimize edits to upstream files to keep merge cost low |

### 1c. Carried-over verdicts that need re-justification in fork context

| Old verdict | Fork status |
|---|---|
| Hand-rolled portable storage abstraction (Python) | DEAD — replaced by R5 |
| PyYAML from day 1 | DEAD — Go yaml libs already in upstream |
| Stdlib-first Python constraint | DEAD — stack is Go+Svelte per upstream |
| MCP HTTP-first | SURVIVES as R7 input |
| Single-file HTML/JS UI question | DEAD — Svelte comes with the fork |
| Configurable pricing source | SURVIVES (design detail, post-cutover) |

---

## 2. Phases (fork framing — PROVISIONAL until R-decisions lock)

### Phase F0 — Round-2 decisions + fork setup
- [ ] R1–R10 answered by user.
- [ ] Fork created: clone upstream at pinned tag, add fork remote, initial
      commit untouched, rename binary/UI strings to Llama Hugs.
- [ ] Reproduce upstream build once (Go test suite green) on the dev host.

### Phase F1 — Coexisting observer (zero-risk footprint on wimpy)
- [ ] **Pre-flight (before any deploy):** baseline checksums recorded for the
      §0a do-not-touch inventory; verify port 8181 is free on wimpy; verify
      Llama Hugs can reach llama-swap's read endpoints at planned poll rate;
      pick self-contained test models for F2 (small GGUFs already on disk,
      sized to fit the smaller GPU without disturbing production entries).
      All checks read-only.
- [ ] Deploy built binary to wimpy under `/opt/llama-hugs/` (new dir; approval
      gate: creating a new top-level dir on the live box).
- [ ] Runs on its own port (default proposal 8181, unchanged from prior plan),
      bind localhost initially; reads llama-swap `/v1/models`, config.yaml,
      activity store; presents overhauled Svelte UI.
- [ ] Verification: llama-swap untouched (checksum its binary/config/unit
      before and after); Llama Hugs shows correct fleet data.

### Phase F2 — Router parity on scratch
- [ ] **Test-isolation rule:** F2 configs are a separate config file using a
      non-production model-ID namespace (e.g. `test-*` prefixes) and dummy or
      small scratch GGUFs. No production entry, env pin, or group name ever
      appears in an F2 config, so a passing test cannot become a production load.
- [ ] Llama Hugs routes TEST models on its own port using its own config copy —
      groups, env pins, hot-reload semantics replicated from upstream behavior.
- [ ] Parity tests named per observable behavior: swap-on-request, unload API,
      ttl auto-unload, profile switching, refusal to load without required env
      pin, hot-reload pickup of config edits.
- [ ] No production model ever loaded by Llama Hugs yet.

### Phase F3 — Platform features (post-parity)
- Persistent store extensions, custom tags, disk/orphan views, benchmark
  ingestion, MCP surface (R7), auth evolution (R6). Ordering per R9.

### Phase F4 — Cutover (per R8 checklist)
- [ ] Soak period passed; user sign-off.
- [ ] Repoint consumers (hermesvm01 etc.) from :8080 to Llama Hugs port.
- [ ] llama-swap remains installed and untouched; decommission is a separate
      future approval, never part of cutover.

---

## 3. Constraints (binding)

1. Live-box rules unchanged: approvals for sudo/service/network/dir creation,
   one change at a time, backups, documented backout.
2. Coexistence constraint (§0) is absolute during Phases F1–F3.
3. Llama Hugs never binds 8080 while llama-swap is alive.
4. Every divergence from upstream is recorded (file + reason) — merge-rent
   control per R10.
5. GPU safety rules inherited: env pins, --device guards, 64K context, unload +
   pid-exit wait between large loads, no tmpfs artifacts.
6. New dependencies (Node build chain etc.) declared per R3 decision; nothing
   installed on wimpy without approval.

## 4. Risks

| Risk | Mitigation |
|------|-----------|
| Merge burden vs 1–2-day upstream cadence | R1 pinning + R10 isolation policy; cherry-pick only critical fixes |
| Accidental modification of live llama-swap | Checksum baseline taken at F1 start; verified after every deployment |
| Port/process collision | Own port forever until decommission day |
| Node/npm creep onto the live box | R3 default builds off-host |
| Router regression at cutover | Parity suite (F2) + soak + repoint-not-remove cutover |
| Scope sprawl (17 features × fork maintenance) | F-phases gate platform work behind parity |
| GPU/OOM incidents from Llama Hugs loads | Same rules that govern fetch-model.sh; production loads not until F4 |

## 5. Acceptance criteria (revised)

- Llama Hugs and llama-swap run simultaneously on wimpy with zero mutual
  interference (checksummed).
- Fleet dashboard parity: every model visible with status/GPU/tags/ctx/size/
  cutoff/link; search/sort/filter.
- Stats persist across restarts (in-fork store).
- Routing parity proven on scratch models before any production load.
- Cutover = client repoint only; llama-swap files untouched throughout.
- Post-cutover: agent surfaces (MCP/HTTP), benchmarks browsable, pricing slot
  configurable.

---

*No implementation performed. Next step: user answers R1–R10; nothing in §2
starts until they're locked.*
