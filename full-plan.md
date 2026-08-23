# Llama Hugs — Implementation Plan (Fork of llama-swap, Minimal MVP)

> **Status:** PLANNING ONLY. Reconstructed from `~/llama-hugs/llama-hugs.md`
> (the overview) plus the user's Phase 0 verdicts. No implementation performed
> or authorized. Everything touching the live wimpy host carries an approval gate.

---

## 0. Summary

**Llama Hugs is a fork of llama-swap.** It eventually replaces llama-swap on
wimpy, but the MVP is deliberately small: take upstream's working code, change
its *identity and paths* (binary name, port, config file, store, install dir),
and have it read the **same model files** llama-swap already uses. Everything
else — UI overhaul, extra store tables, plugins, MCP, benchmarks, federation —
is later iteration on top of the fork. We do not rebuild what upstream already
ships (router, groups, env pins, hot-reload, `apiKeys` auth, SQLite store,
Svelte UI).

**MVP definition (the whole bar):**
1. Fork compiles (Go + embedded Svelte assets).
2. Runs on its own port (8181) with its own config/store/install dirs.
3. Its config references the **same GGUF/mmproj paths** llama-swap uses
   (`~/.cache/llama.cpp/...`) — no model copying.
4. Auth uses upstream's existing `apiKeys` bearer-token model unchanged.
5. Serves `/v1/models` and can load a non-production test model without
   touching llama-swap's files, port, or process.

After that: iterate. The MVP is the floor, not the ceiling.

---

## 0a. Coexistence operating rules (binding through build + dual-run)

**Do-not-touch inventory** (checksummed at F1 start, re-verified after every
Llama Hugs deployment):
- `/usr/local/bin/llama-swap` (binary)
- `/etc/llama-swap/` (config + everything under it)
- `llama-swap.service` (never stopped/restarted/edited by this project)
- llama-swap's store file (`/var/lib/llama-swap/` if present)
- port 8080

**Identity separation:** Llama Hugs = own binary name (`llama-hugs`), own
install dir (`/opt/llama-hugs/`, created only via approval), own config file
name, own store path, own port (8181). No alias/symlink/wrapper touching the
old binary. Cutover = repoint clients; decommission of llama-swap is a
SEPARATE future approval — "first day of replacement use" ≠ "removal day."

**Read-vs-manage boundary:** during the MVP the fork is a *parallel* router
with its own config; it does not proxy or mutate llama-swap. It reads the same
GGUF paths from disk but loads them under its own process. To avoid GPU
contention, the MVP validates loading only against small/scratch models or a
GPU not currently serving production traffic.

**Approval triggers:** creating `/opt/llama-hugs/`; first non-localhost port
bind; any Llama Hugs systemd unit; first non-test model load; cutover repoint;
decommission. Each is a discrete approval event.

---

## 1. Decision ledger (Phase 0 — resolved)

| Item | Decision |
|---|---|
| Path | **Fork & replace** llama-swap (not companion) |
| MVP scope | Minimal: fork works on own paths/port, reads same model files, auth inherited |
| Auth (R6) | **Use upstream `apiKeys` bearer tokens, unchanged** — no custom auth |
| Repo (R2) | **In-repo** under `~/llama-hugs/` beside this plan |
| Build toolchain (R3) | Build on a dev host; deploy binary + embedded assets. **No npm/Go toolchain on wimpy** — upstream embeds the Svelte UI in the binary |
| Pruning (Q8) | No pruning feature |
| Pricing (Q4) | Configurable source slot, later; ToS gate before any source |
| LAN exposure | Widen to 0.0.0.0 after local verification passes |
| Version control | Git repo initialized at `~/llama-hugs` |
| Cutover (R8) | Repoint clients when feature-parity + soak signed off; llama-swap untouched |

**Carried-over now-DEAD assumptions** (from the companion plan, superseded):
hand-rolled Python storage layer, PyYAML, stdlib-first Python, single-file
HTML/JS UI, companion DB separate from llama-swap store. All replaced by
upstream's Go + Svelte + modernc.org/sqlite stack.

---

## 2. Phases

### F0 — Fork setup (local, no live-box touch)
- [ ] Clone upstream `mostlygeek/llama-swap` at a **pinned tag** (v250-era).
      Add `upstream` remote; fork repo lives in-repo as `~/llama-hugs/hugs/`.
- [ ] Rename binary/UI strings to Llama Hugs; change default port to 8181 and
      default config/store paths to Llama Hugs-owned locations.
- [ ] Reproduce upstream build once on the dev host (Go test suite green,
      Svelte UI builds and embeds). Confirm a single self-contained binary.
- [ ] Document the divergence budget (R10): new code isolated in new
      packages/dirs; minimize edits to upstream files to keep future merges cheap.

### F1 — MVP parallel run (first live-box touch, gated)
- [ ] **Pre-flight (read-only):** record checksums of the §0a inventory;
      confirm 8181 is free; confirm Llama Hugs can reach llama-swap's read
      endpoints (low-rate poll); pick a small scratch GGUF already on disk for
      the load test.
- [ ] Create `/opt/llama-hugs/` (approval gate).
- [ ] Author `llama-hugs` config: same GGUF/mmproj paths as llama-swap entries,
      own `apiKeys`, own store path, own port 8181. No production model-ID clash.
- [ ] Deploy built binary; bind localhost first; verify `/v1/models` matches
      the fleet and a scratch model loads under Llama Hugs.
- [ ] Verify llama-swap is byte-identical (checksums) before/after.
- [ ] (Approval) Widen to 0.0.0.0:8181 for hermesvm01 agent access.

### F2 — Iterate on the fork (post-MVP, ordered)
Each item is independent and stacks on the working MVP. Order is a suggestion,
not a gate.
1. **UI overhaul** — modern dark Svelte dashboard: search, sortable columns,
   capability badges, disk/orphan views, model detail drawer. (Upstream UI is
   the starting point, not a rewrite-from-scratch.)
2. **Persistent store extensions** — add tables/migrations (goose, as upstream
   uses) for custom tags, lifecycle dates, benchmark history, settings. Keeps
   upstream's SQLite; no separate DB.
3. **Lifecycle / disk visibility** — orphan-file detection, missing-file
   detection, stale-model ranking (suggest-only; no auto-delete per Q8).
4. **Agent surface** — MCP server (HTTP-first) + Hermes skill; read tools
   first, mutation tools gated behind `apiKeys`. Reuses upstream's mutation
   endpoints where possible.
5. **Plugins / enrichment** — pricing + arbitrary enrichment behind a
   configurable source slot; directory-confined loading.
6. **Benchmark ingestion** — nightly sweep results → store → leaderboard UI.
   Preserve OOM-safety rules (unload + pid-exit wait, non-tmpfs paths).
7. **Federation (stretch)** — multi-server; confirm "nvidia data flywheel"
   referent before design.

### F3 — Cutover (per R8 checklist)
- [ ] Feature parity + soak period passed; user sign-off.
- [ ] Repoint consumers (hermesvm01, etc.) from :8080 to Llama Hugs :8181.
- [ ] llama-swap remains installed and untouched. Decommission = separate
      future approval.

---

## 3. Constraints (binding)

1. Live-box rules unchanged: approvals for sudo/service/network/dir creation;
   one change at a time; backups; documented backout.
2. Coexistence constraint (§0a) is absolute during F1–F2.
3. Llama Hugs never binds 8080 while llama-swap is alive.
4. Every divergence from upstream recorded (file + reason) — merge-rent control.
5. GPU safety rules inherited: env pins, `--device` guards, 64K context,
   unload + pid-exit wait between large loads, no tmpfs artifacts.
6. No toolchain installed on wimpy (R3): build off-host, ship binary + assets.

## 4. Risks

| Risk | Mitigation |
|------|-----------|
| Merge burden vs upstream's 1–2-day cadence | Pinned tag + isolated new code (R10); cherry-pick critical fixes only |
| Accidental modification of live llama-swap | Checksum baseline; verified after every deployment |
| Port/process collision | Own port/identity forever until decommission |
| GPU contention between parallel routers | MVP load-test only on scratch/small models or idle GPU |
| Router regression at cutover | Parity check + soak + repoint-not-remove |
| Scope creep | MVP bar is explicit and small; iteration items are opt-in |

## 5. Acceptance criteria

**MVP (F1):**
- Fork builds to a single binary; runs on 8181 with own config/store/install.
- Serves the fleet via `/v1/models` reading the same GGUF paths as llama-swap.
- Auth via upstream `apiKeys` works.
- A scratch model loads under Llama Hugs; llama-swap confirmed byte-identical.

**Eventual (F2–F3):**
- UI overhaul, persistent store extensions, disk/orphan views shipped.
- Agent read + gated mutation via MCP/HTTP.
- Benchmark leaderboard browsable.
- Cutover = client repoint only; llama-swap files untouched throughout.

---

*No implementation performed. Next step: user approves F0 start (local fork
setup, no live-box touch). F1 is the first gated live-box action.*
