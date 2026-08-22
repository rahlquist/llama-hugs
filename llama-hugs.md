# Huggnswap Model Console — Platform Plan (for external critique)

> **Status:** PLANNING ONLY. No implementation has been authorized.
> This document is written for an agent or reviewer with ZERO prior insight into
> the wimpy host, its inference stack, or the conversations that produced it.
> Read sections 1–6 before critiquing sections 7–11.

**Goal:** Build a replacement for llama-swap, starting with a fork of its code,
 on the wimpy inference host, providing everything llama-swap has and lacks:
a modern web UI, persistent storage (SQLite + Postgres), agent/MCP integration,
plugins, benchmarking, lifecycle automation, and multi-server federation.

**Architecture:** llama-swap does some basic work as an inference router (it is ok
at exactly one job: swapping one GPU-resident model at a time per group). The
console is a separate companion service that reads llama-swap's API and config,
owns all presentation/management/enrichment features, and exposes agent-facing
interfaces (MCP server, Docker, Hermes skill/plugin). This must be self-contained.
It must not interfere with an existing llama.cpp or llaama-swap setup.
When it comes to model files and projectors, they must coexist.


**Tech Stack (proposed, to be critiqued):** Python 3 stdlib-first for the MVP
backend (already started), SQLite via stdlib `sqlite3` for default storage,
Postgres via `psycopg` when selected, single-file HTML/JS dashboard, Docker for
deployment, MCP stdio/HTTP server for agent integration.

---

## 1. The target machine: wimpy https://github.com/rahlquist/wimpy-setup
(full context for an outside reviewer)

wimpy is a bare-metal home-lab inference + VM host. It is a LIVE production box
serving a KVM guest; it is not a dev sandbox. Any plan must respect that.

### 1.1 Hardware
- CPU: AMD Ryzen 7 7700 (8c/8t), 32 GB DDR5, 2 TB NVMe
- **GPU 0: AMD Radeon AI PRO R9700, 32 GB VRAM** (ROCm/HIP, gfx1201) — primary
  inference GPU, device name `ROCm0`, pinned by UUID `GPU-61fe9ba05af1939a`
  via `HIP_VISIBLE_DEVICES` (UUID pin is mandatory: the Ryzen iGPU also
  enumerates as a ROCm device, so index-based pinning can silently select it)
- **GPU 1: NVIDIA RTX 5060 Ti, 16 GB** (CUDA sm_120) — secondary, pinned via
  `CUDA_VISIBLE_DEVICES=0`, runs a separate CUDA build of llama.cpp
- iGPU: present, never used for inference
- Network: 192.168.8.248, hostname `wimpy.home.lan`; a KVM guest `hermesvm01`
  (192.168.8.249) runs Hermes Agent and reaches the inference stack at
  `http://wimpy.home.lan:8080/v1`. Anything listening on 0.0.0.0 on wimpy is
  reachable by that guest.

### 1.2 OS / environment
- CachyOS (Arch-based), kernel currently 7.1.8-1-cachyos. Recent kernels in the
  7.2.0-rc series had an amdgpu/kfd regression that hung large (>~19 GiB) mmap'd
  GGUF weight uploads; fixed in 7.2.0-rc7. Mitigation is permanently in place:
  `fetch-model.sh` passes `--no-mmap` for GGUFs >= 19 GiB.
- 32 GB system RAM with ~65 GB swap, swappiness 150. OOM is a real historical
  failure mode here (see 6.4).
- `/tmp` is tmpfs (16 GB) — never store durable evidence or large artifacts there.

### 1.3 The inference stack
- **llama-server** (llama.cpp) built from source twice:
  - ROCm build → `/usr/local/bin/llama-server` (canonical, built by
    `05-llama-cpp.sh`, floats to latest master on each rebuild)
  - CUDA build → `/opt/llama-cuda/bin/llama-server` (built by
    `06-llama-cpp-cuda.sh`, for the 5060 Ti aliases)
  - Configs ALWAYS use absolute binary paths. Never a bare `llama-server` on
    PATH — PATH-shadowing bugs have broken this project twice.
- **llama-swap** (model router, https://github.com/mostlygeek/llama-swap)
  - Version: **v250**, installed at `/usr/local/bin/llama-swap`
  - systemd service `llama-swap`, listens `0.0.0.0:8080`, runs with
    `-watch-config` (hot-reloads `/etc/llama-swap/config.yaml` on change)
  - Repo copy of config: `/home/rahlquist/wimpy-setup/llama-swap-config.yaml`
    (deploy = manual `cp` to `/etc/llama-swap/config.yaml`; no restart needed)
- **Models:** ~73 registered entries (post-removals of 4 broken models, which
  are still pending live deploy). GGUF files live in
  `~/.cache/llama.cpp/` (~/.cache/llama.cpp, owned by the service user).
- **Groups (llama-swap feature):** two GPU pools:
  - `amd-r9700` — `swap: true`, one model at a time on the R9700
  - `nvidia-5060ti` — the `-cuda` suffixed aliases on the 5060 Ti
  - Every entry carries an `env:` pin + `--device` flag; `--device` doubles as
    a hard-fail guard against silent CPU fallback. Removing either is forbidden.
- **Context:** every model runs `--ctx-size 65536` (64K minimum, hard project
  rule — Hermes Agent needs it).

### 1.4 Repo conventions and hard rules (from /home/rahlquist/wimpy-setup/CLAUDE.md)
1. **Back up before modifying any working config** — timestamped copy first.
2. **Never remove GPU env pins or `--device` flags.**
3. **Use local `--model` paths, never `-hf`** (avoids double downloads).
4. **Context stays 65536** unless explicitly told otherwise for an OOM fix.
5. **Confirm before sudo / service / network changes** — show the command and
   wait. The box is live.
6. **Tune one model at a time.**
- Additional operational rule learned from a hard crash (2026-08-15): when
  unloading/reloading large models sequentially, issue `GET /unload` AND wait
  for the llama-server process to actually exit before the next load; two
  `--no-mmap` 24–27 GB models resident simultaneously in 32 GB RAM caused an
  OOM-kill cascade that hard-crashed the machine.
- Git: repo `rahlquist/wimpy-setup`, branch `main`. Always `git fetch` and
  check for divergence before pushing; never force-push (the remote receives
  commits from other sources too).
- Evidence discipline: incident artifacts (run logs, dossiers, gdb traces) are
  kept in the repo, never discarded.

---

## 2. Why a companion platform instead of forking llama-swap (decision record)

The user's dissatisfaction list (section 7) was evaluated against three paths:

- **Fork llama-swap and build inside its UI.** Rejected as the primary path.
  llama-swap ships a release every 1–2 days (189 tags, 559 commits, ~84% from
  one author, 17–35 commits/month over the past year). Building a whole
  platform inside someone else's Svelte UI means permanent merge-conflict rent
  concentrated in the fastest-churning part of the codebase. Upstream is not
  receptive to UI-philosophy changes (its minimalism is intentional).
- **Switch router (Ollama, llama.cpp router mode, LiteLLM, vLLM).** Rejected.
  Nothing else offers groups + per-model env/cmd pinning + hot-reload + unload
  API + peers, which is exactly what wimpy's dual-GPU scheme depends on.
  llama.cpp's built-in router mode (2026) is worth watching but cannot express
  per-model env pins. LiteLLM is a gateway, not a model manager.
- **Chosen: companion platform beside stock llama-swap.** llama-swap stays
  pristine (zero fork maintenance, free upstream upgrades forever); the console
  owns everything missing. Source of truth for model identity remains
  llama-swap's config + `/v1/models`; the console never forks that truth.

This decision is load-bearing for everything below. Critique of the plan should
engage with this tradeoff first.

---

## 3. Facts about llama-swap v250 that shape the design (verified in source)

From a local clone of upstream master (== v250, commit 60226b6):

- **Persistent store already exists, but only SQLite.**
  `internal/store/store.go`: `modernc.org/sqlite` (pure-Go driver), WAL mode,
  goose migrations (`00001_create_activity.sql`), a single `activity` table
  (timestamp, model, request path, status, input/output/cached/draft tokens,
  prompt/s + tokens/s, duration_ms, error). Config knob:
  `store: { path: /file.sqlite }`. **Without it, the store is `:memory:` capped
  at 1000 rows — this is why wimpy loses all stats on every restart.** Enabling
  the stock `store:` block is a prerequisite task (8.1) independent of the
  console. Postgres is NOT supported upstream; the driver is hardcoded.
- **Capabilities are derived, not freeform.** Config: `capabilities: {in:
  [text|audio|image], out: [...], tools: bool, reranker: bool, context: int}`.
  The API renderer (`internal/server/api.go renderCapabilities`) maps these to
  at most 7 boolean tags (vision, audio_transcriptions, audio_speech,
  image_generation, image_to_image, function_calling, reranker) + a
  context_length field. Arbitrary user tags must come from somewhere else.
- **Freeform `metadata:` passes through `/v1/models` verbatim.** Wimpy already
  populates it (source_repo, repo_url, file_size_bytes, file_sha256,
  has_checksum, mtp, mtp_flag, pipeline_tag, vision, mmproj, mmproj_filename)
  on all entries via `backfill_metadata.py`/`apply_backfill.py`. This is the
  sanctioned extension point — no fork needed to carry extra facts.
- **`peers:` config exists** for federating other llama-swap instances (proxy
  URL + model list, local models take precedence). Relevant to stretch goal 7.15.
- **API surface the console will use:** `/v1/models`,
  `/api/model/details/{id}`, `/api/metrics/activity`, `/api/metrics/stats`,
  `/api/performance`, `/metrics` (Prometheus format), `GET /unload`,
  `/api/events` (SSE stream of status/logs/metrics).
- **No search, no static/custom page serving, no plugin API, no theming, no
  mmproj management, no disk visibility, no lifecycle dates** — confirmed by
  source inspection. These are all genuinely missing, not undiscovered.

---

## 4. Current wimpy state the plan inherits (as of 2026-08-15)

### 4.1 Uncommitted repo changes (from earlier work today)
- `llama-swap-config.yaml`: 4 broken models removed (tifa-deepsex-14b,
  qwen3-6-35b-a3b-uncensored-hauhaucs-aggressive-q4-k-m,
  muse-glimmer-30b-ud-q3-k-xl-cuda, model-f16). **Live deploy of these
  removals is still pending** (`sudo /usr/local/sbin/llama-swap-deploy` —
  needs user approval).
- `model-inventory.html` regenerated; `cutoff-dates-20260815.csv` untracked.

### 4.2 The console MVP prototype (exists, uncommitted, partially verified)
Location: `/home/rahlquist/wimpy-setup/console/`
- `FEATURE-SPEC.md` — the original 15-item feature list with status column.
- `server.py` — Python 3 stdlib HTTP server (port 8181). Merges:
  llama-swap `/v1/models` + config group membership + config `cmd:`
  `--model`/`--mmproj` paths + cutoff CSV + real on-disk `stat()` of GGUF and
  mmproj files. Endpoints: `/` (dashboard), `/api/models`, `/api/health`.
  **Known defect, fix applied but UNVERIFIED:** the config parser originally
  missed YAML block-scalar `cmd: |` bodies (all 77 models had unresolved GGUF
  paths). A pushback-buffer fix is in the file; the server was killed before
  retesting. First task of Phase 1 is restart + verify.
  **Known parser fragility:** it is a hand-rolled YAML subset parser. The
  config format is stable and repo-controlled, but a critic should weigh
  stdlib-only parsing vs. adding a PyYAML dependency.
- `index.html` — dark-themed single-file dashboard: client-side search,
  sortable columns (id/status/group/ctx/size/cutoff), capability badges
  (vision/MTP/ctx/pipeline/group/mmproj), per-model HF links from
  `repo_url`, disk sizes, fleet-wide disk total, group/status filters, 15s
  auto-refresh. Unverified against corrected data.

### 4.3 Data assets the console consumes
- `~/.cache/llama.cpp/*.gguf` (+ `.mmproj.gguf` files) — real file sizes/mtimes.
- `cutoff-dates-20260815.csv` — 77 models' self-reported knowledge cutoffs
  (columns: model,status,cutoff_answer,cutoff_date,notes,seconds,
  completion_tokens). Produced by a live sweep; day-of-month defaults to 1st.
- `benching/` — nightly benchmark sweep infrastructure (see wimpy-benchmark-ops
  skill); results land on disk and are the input for feature 7.13.
- `evidence-*` dirs — incident artifacts; the console must never touch these.

### 4.4 Tooling the console will eventually absorb
- `fetch-model.sh` — download + GPU smoke test + config registration
  (`hf download`, smoke at full context with production pins, refuses to
  register without a GPU pin). `-y`, `--no-smoke`, `--no-register` flags.
  Agent-integration feature 7.10 aims to replace this with API calls.
- `remove-model.sh` — removal with group cleanup, deploy, verification.
- `backfill_metadata.py`, `apply_backfill.py` — metadata backfill patterns.

### 4.5 UI

---

## 5. Glossary and naming (so a critique uses consistent terms)

- **console** — the new companion platform this plan builds (working name;
  rename suggestions welcome in critique).
- **llama-swap** — stock router, untouched.
- **entry** — one model definition in llama-swap's config.
- **group** — llama-swap GPU pool (`amd-r9700`, `nvidia-5060ti`).
- **store (llama-swap)** — upstream activity-log SQLite.
- **console DB** — the console's own database (section 7.2). Distinct from the
  llama-swap store; the plan keeps them separate and explains why in 8.2.

---

## 6. Constraints that apply to ALL phases

1. Live-box rule: no sudo/service/network changes without explicit user
   approval, shown as exact commands first.
2. Console must be read-only against llama-swap until Phase 2 explicitly adds
   mutation endpoints, and even then every mutation must go through reviewable
   steps (backup → edit repo config → deploy), mirroring fetch-model.sh's
   manual-deploy philosophy.
3. No new system services without approval; the console must run fine as a
   plain process first, Docker second, systemd last.
4. Stdlib-first Python: the MVP must not require pip installs on the host.
   Later phases may add deps but must declare them and justify each.
5. Evidence/data is never discarded: any schema migration or file move must be
   additive or backed up first.
6. The console listens on a NEW port (8181 proposed); it must not conflict
   with 8080 (llama-swap), 9119 (VM), or benchmark processes. Binding
   0.0.0.0 exposes it to hermesvm01 — deliberate (agent access) but noted.
7. Everything the console claims about models must be verifiable against
   llama-swap's live API and the config file; no cached truth that can drift.

---

## 7. Full feature inventory (everything discussed, consolidated)

Each item: original user wording (abridged) → disposition → design sketch.
Items marked MVP exist partially in the prototype; items marked TODO are
planned; DEFER means explicitly postponed; STRETCH means post-completion.

### 7.1 Modern web UI (MVP → refine)
Original: "incredibly basic and boring, barely serves modern models."
Disposition: console owns the whole UI. Prototype proves the look (dark,
badges, sortable). Refinements: model detail drawer, GPU utilization sparkline
(from `/api/performance` polling), activity feed (from llama-swap's
`/api/metrics/activity` once the store is enabled), compare-view (select N
models → side-by-side table), saved filter presets, configurable columns
(links to 7.8).

### 7.2 Persistent backend (TODO — two parts!)
Original: "back end entirely RAM based; loses all statistics on restart;
needs a database."
**Part A (stock, trivial):** enable llama-swap's own SQLite store via
`store: { path: /var/lib/llama-swap/llama-swap.sqlite }` in the config
(requires sudo mkdir/chown for the service user + config deploy with backup).
This alone fixes the restart-loss complaint for request/token statistics.
**Part B (console DB):** the console needs its own durable store for
lifecycle dates, custom tags, enrichment, benchmark history, user settings.
Design: storage abstraction layer with two interchangeable backends —
SQLite (default, stdlib `sqlite3`, file at e.g.
`/var/lib/model-console/console.sqlite` or repo-local for dev) and Postgres
(user's choice, `psycopg`). Identical DDL for both (keep SQL portable: no
JSONB-only features in core tables; store JSON as TEXT and parse in the app).
**Migration tooling:** a `console migrate` command that copies rows between
backends in either direction (SQLite→PG and PG→SQLite) using a
backend-neutral dump format; versioned schema migrations applied to whichever
backend is active. User explicitly wants both directions supported.
Critique point: is a hand-rolled portable-DDL + row-copy migrator the right
size, or should this use SQLAlchemy/Alembic? Stdlib-first bias argues for
hand-rolled; maintainability argues for SQLAlchemy. Decide in Phase 0 review.

### 7.3 User-extensible web pages (inherent in companion design)
Original: "no room for users to append or add their own web pages."
Disposition: solved structurally — the console is our app. Design: a
`console/pages/` directory of user-supplied HTML fragments or markdown pages
rendered under a "Custom" nav section; later, a simple plugin manifest
(overlaps 7.12). MVP scope: static file serving + nav injection only.

### 7.4 Custom capability tags (MVP partial → TODO)
Original: "no way to customize capabilities."
Disposition: two layers. (a) llama-swap layer: the 7 derived tags are fixed;
`capabilities.tools: true` also advertises `supported_parameters`
(function-calling) — an audit should set this flag on models that genuinely
support tools (most Qwen3/GLM entries qualify; currently unset). (b) Console
layer: freeform user tags stored in the console DB, rendered as badges,
filterable, editable in the UI. These are presentation facts about models,
not router facts — keeping them out of llama-swap config avoids config bloat
and fork-like coupling.

### 7.5 Lifecycle dates + unused-model automation (TODO)
Original: "keep information such as dates and times; automate getting rid of
old models that aren't used anymore."
Design: console DB records per model: first_seen, last_loaded (from llama-swap
activity log once 7.2A lands), last_request_at, registered_at, notes.
A "stale models" view ranks by days-since-last-use + disk cost. Pruning is
suggested, NEVER automatic in Phase 1–2: the console emits the exact
`remove-model.sh` command (or, post-7.10, a prepared API call) for the user
to approve. Automation level is a per-user setting with the default off.
Safety: prune candidates exclude ready/loading models and anything in a group
with no other members.

### 7.6 Disk space visibility (MVP)
Original: "no visibility into disk space consumption."
MVP: per-model GGUF + mmproj sizes from `stat()`, fleet total, sort by size
(prototype does this). TODO: cache-dir total vs. registered-models total
(orphans detection — GGUFs on disk with no config entry, and entries pointing
at missing files, which is how qwen3-6-35b-...-q4-k-m was discovered broken).
Orphan scanning must be read-only; deletion only via approved flow.

### 7.7 Additional files management (mmproj/projectors) (MVP partial)
Original: "no management over additional files like projectors."
MVP: mmproj path, existence, size shown per model (prototype). TODO: an
"auxiliary files" registry in the console DB (mmproj, draft/MTP tensors,
vocab files) with orphan detection and fetch-assist (given a model's HF repo,
suggest its companion files).

### 7.8 Configurable display (TODO)
Original: "no configurability of the on-screen display."
Design: user settings stored in console DB — visible columns, column order,
default sort, badge filters, theme accent. MVP ships one dark theme;
theme-config is a later refinement, not a priority.

### 7.9 Search (MVP)
Original: "search is not really there."
Prototype has client-side substring search across id/name/description/repo/
pipeline/group/cutoff. TODO: tokenized/fuzzy matching and saved searches once
the fleet grows; server-side search if the API ever paginates.

### 7.10 Agent integration replacing fetch-model.sh (TODO — Phase 2 core)
Original: "direct agent integration ... downloading new models and eliminating
my current script."
Design: console exposes an authenticated HTTP API (and MCP tools, see 7.16)
implementing the fetch-model.sh pipeline as discrete steps:
  1. `POST /api/models/fetch` {hf_url_or_repo, expected_file?} → server-side
     `hf download` with progress streaming (SSE),
  2. smoke test on the correct GPU with production pins (reuse the exact
     fetch-model.sh smoke invocation; refuse registration without a pin),
  3. propose config entry (group auto-suggested by size/GPU headroom, never
     auto-applied),
  4. write to REPO config, print/execute deploy only with approval.
Unloading/swapping endpoints (`GET llama-swap /unload` passthrough,
load-a-model-by-requesting-it, status poll) complete the agent surface.
This is the riskiest feature (mutating a live box via API): auth token
required, every mutation logged with timestamp + caller, dry-run mode default.

### 7.11 HF source links (MVP)
Prototype renders `metadata.repo_url` as a clickable link per model; entries
without repo_url show "no source" (3 models currently). TODO: a backfill pass
for the missing ones (gemma4-coding-q6-k, its -cuda alias, glm-4-7-flash).

### 7.12 Plugins / enrichment (TODO — Phase 3)
Original: "accept plugins such as ones from sites that have listings of model
pricing for augmentation."
Design: an enrichment plugin = a named Python module with a declared contract
`enrich(model_id, metadata) -> dict` plus a refresh schedule; results stored
in the console DB and rendered as extra badges/columns/detail fields.
Reference plugin: pricing lookup (source TBD — OpenRouter API is the obvious
candidate; confirm terms before implementing). Plugin loading is confined to a
directory, no arbitrary code from network. Critique point: plugin security
model needs a real answer before Phase 3.

### 7.13 Benchmark integration (TODO — Phase 3)
Original: "integration of better benchmarking like we run every night ... flat
comparison across models."
wimpy already runs a nightly llama.cpp benchmark sweep (benching/, see
wimpy-benchmark-ops skill). Design: the console ingests sweep result files
into its DB (benchmark_runs, benchmark_results tables) and renders a
leaderboard: tokens/s by model × task, trend over time, regression flags.
Operational rules from the 2026-08-15 OOM incident MUST be preserved:
GET /unload between models, wait for the llama-server pid to actually exit
before the next, stream results to disk-backed paths (never tmpfs).
The console does not replace the sweep's scheduler in Phase 3; it presents
results. Triggering sweeps from the UI is Phase 4+.

### 7.14 Self-upgrade management (DEFER)
Original: "managing its own upgrades as well as that of llama.cpp."
Disposition: explicitly deferred. Auto-upgrading llama.cpp on a live box with
known kernel/driver sensitivities (see 1.2) is a reliability risk, and
llama-swap upgrades are a single-binary swap already handled by
05-llama-cpp.sh. The console may later show "available upstream versions"
(read-only check) — upgrade execution stays manual.

### 7.15 Multiple inference servers / federation (STRETCH — discuss last)
Original: "able to handle multiple inference servers like nvidia data flywheel
... stretch goal to discuss after everything else is complete."
(NOTE for critic: the user's phrasing names "nvidia data flywheel"; intent is
read as 'federate additional inference servers/backends' — possibly including
NVIDIA-hosted or multi-node deployments. Confirm exact referent with the user
before designing.) Available building blocks: llama-swap's native `peers:`
config (proxy other llama-swap instances into one gateway), the console's own
server registry (declare remote llama-swap/llama.cpp/vLLM endpoints, health
poll, merged inventory view), and later cross-server operations (where should
this model live?). Design is intentionally deferred; Phase 1–3 must not paint
into a corner (keep the console's model identity qualified by server from the
start: `server_id/model_id`).

### 7.16 Delivery forms: MCP / Docker / skill / Hermes plugin (TODO — Phase 2)
Original: "run as an MCP or standalone as a Docker server, accessible through
a skill through whatever agent you possess, or perhaps as a hermes plugin."
Design, all four, layered:
  - **Standalone process** (MVP): `python3 console/server.py` — already works.
  - **Docker**: image bundling the console; mounts llama-swap's config
    read-only + model cache read-only (for stats) + console DB volume;
    `network_mode: host` or explicit port mapping to reach
    `host:8080`. Docker is already installed on wimpy (02-docker.sh).
    Critique point: host-network vs. bridge + host.docker.internal on Linux.
  - **MCP server**: expose console operations as MCP tools (list models,
    search, disk report, fetch/register [Phase 2 mutation tools], benchmark
    leaderboard). Transport: stdio for local agents, HTTP for remote. The
    agent running Hermes on hermesvm01 would consume it over HTTP.
  - **Hermes skill**: a `wimpy-model-console` skill documenting the HTTP API
    + curl patterns so ANY Hermes profile can drive the console without MCP.
  - **Hermes plugin** (optional, evaluate in Phase 2): a plugin wrapping the
    API as first-class Hermes tools. Decide skill-vs-plugin after the MCP
    surface stabilizes — do not build both speculatively.

### 7.17 Dual database support + migrations (TODO — see 7.2B)
Original: "continue support for SQLite and also add support for Postgres ...
user's choice ... built-in functionality to do migrations from one to the
other and back."
Covered by 7.2 Part B. Reiterated because it is a hard user requirement, not
a preference: both backends first-class, bidirectional migration tool,
user-selectable via console config.

---

## 8. Architecture

### 8.1 Component diagram

```
                        ┌──────────────────────────────────────────┐
                        │ wimpy host (live inference box)          │
                        │                                          │
 agents (hermesvm01)    │  llama-swap :8080  ◄── config hot-reload │
   │  MCP / HTTP        │  (stock router, groups, env pins)        │
   │                    │      │ spawns llama-server per model     │
   ▼                    │      ▼                                   │
┌────────────────────┐  │  ROCm build /usr/local/bin  (R9700)      │
│ model console      │  │  CUDA build /opt/llama-cuda (5060 Ti)    │
│  :8181             │  │                                          │
│  ┌──────────────┐  │  │  reads: /v1/models, /api/*, /metrics     │
│  │ web UI (SPA) │  │  │  reads: llama-swap-config.yaml           │
│  ├──────────────┤  │  │  reads: ~/.cache/llama.cpp (stat)        │
│  │ REST API     │  │  │  reads: benching/ results, cutoff CSV    │
│  ├──────────────┤  │  │                                          │
│  │ MCP server   │  │  │  writes: console DB ONLY (Phase 1)       │
│  ├──────────────┤  │  │  writes: repo config via approved flow   │
│  │ storage      │──┼──┼─►        (Phase 2+, never /etc directly) │
│  │ SQLite|PG    │  │  │                                          │
│  └──────────────┘  │  │  llama-swap store: /var/lib/llama-swap/  │
└────────────────────┘  │    llama-swap.sqlite  (stock, 7.2A)      │
   also ships as Docker │                                          │
   image + skill doc    └──────────────────────────────────────────┘
```

### 8.2 Two databases, deliberately separate
- **llama-swap's store** (7.2A) belongs to llama-swap: request/token activity.
  The console READS it (via `/api/metrics/activity`) but never writes it.
- **Console DB** (7.2B) belongs to the console: tags, lifecycle dates,
  enrichment, benchmark history, settings, plugin state.
Rationale: keeps llama-swap 100% stock; a console DB reset never loses router
history and vice versa. Critique point: does this create confusing duplication
(e.g., "last used" exists in both)? Answer: activity log = raw events,
console = derived rollups; acceptable, but the critic should push back if
they see a cleaner split.

### 8.3 Truth hierarchy
1. llama-swap live `/v1/models` — current fleet truth (status, capabilities).
2. `llama-swap-config.yaml` (repo) — declared truth (cmd, groups, env, metadata).
3. Console DB — presentation/management facts ONLY (tags, dates, notes,
   enrichment). Nothing in 3 may contradict 1 or 2; on conflict, 1/2 win and
   the console flags the discrepancy.

---

## 9. Phases

### Phase 0 — decisions & review (before any more code)
- [ ] This document reviewed by second agent; open questions (section 11) resolved.
- [ ] Backend language/framework confirmed (Python stdlib-first assumed).
- [ ] SQLAlchemy-vs-handrolled decision for 7.2B.
- [ ] Port/bind surface confirmed (8181, 0.0.0.0, VM reachability intended).
- [ ] Console project layout confirmed (`console/` in this repo vs. new repo).
      Recommendation: keep in-repo for now; split when Docker image matures.

### Phase 1 — MVP dashboard, durable, deployed
- [ ] Verify the prototype parser fix (restart server.py, confirm GGUF paths
      resolve for all entries; fix until clean).
- [ ] Enable llama-swap stock store (7.2A): requires user-approved sudo
      (mkdir/chown /var/lib/llama-swap), config backup + `store:` block +
      deploy; verify activity persists across a service restart.
- [ ] Add console DB (SQLite default): schema v1 (models_extra: tags, notes,
      first_seen, registered_at; settings; schema_migrations).
- [ ] Dashboard v2: detail drawer, orphan-file detection view, activity feed
      (reads llama-swap store via API), HF-link backfill for the 3 missing.
- [ ] `tools:` capability audit → flag entries that should set
      `capabilities.tools: true` (config change follows normal approval flow).
- [ ] Run console under a user systemd unit (needs approval) OR document the
      manual start; Dockerfile draft.
- [ ] Commit everything; evidence kept.

### Phase 2 — agent surface (MCP + API + skill/plugin)
- [ ] Mutation API behind token auth + audit log + dry-run default (7.10).
- [ ] fetch/register/unload/swap tools; absorb fetch-model.sh semantics
      exactly (pin enforcement, smoke gate, repo-config-then-deploy).
- [ ] MCP server (stdio + HTTP) exposing read tools first, mutation tools
      gated.
- [ ] Hermes skill `wimpy-model-console`; evaluate Hermes plugin wrapper.
- [ ] Docker image finalized; compose file with volume mounts.

### Phase 3 — enrichment & benchmarks
- [ ] Plugin contract + directory loading; reference pricing plugin.
- [ ] Benchmark ingestion + leaderboard UI; preserve sweep safety rules.
- [ ] Custom pages (7.3), display settings (7.8).
- [ ] Postgres backend + bidirectional migration tool (7.2B/7.17) —
      deliberately here rather than Phase 1: build the abstraction in Phase 1
      against SQLite only, but keep DDL portable from day one. Critique point:
      is deferring PG to Phase 3 compatible with "user's choice" requirement?
      Position: abstraction ships in 1, second backend in 3; flag if the user
      needs PG earlier.

### Phase 4 — stretch
- [ ] Multi-server federation (7.15): confirm "nvidia data flywheel" referent,
      design server registry, evaluate llama-swap `peers:` vs. console-level
      federation. Everything before this must keep `server_id` qualification
      ready.
- [ ] Upgrade advisories (read-only, 7.14).

---

## 10. Risks and mitigations

| Risk | Mitigation |
|---|---|
| Console becomes a second writer to live config → drift/accidents | Repo-config-then-deploy flow only; backups; approval gates; console never touches /etc directly |
| Mutation API reachable from LAN (0.0.0.0) | Token auth from day one of Phase 2; audit log; dry-run default; consider bind 127.0.0.1 + SSH tunnel alternative |
| Hand-rolled YAML parser breaks on config edits | Unit tests against the real config; consider PyYAML if fragility recurs |
| Console pollutes RAM on 32 GB box | Python process is tiny; benchmark ingestion reads files lazily; never hold model weights — console touches metadata only |
| OOM repeat during any console-triggered loads (Phase 2) | Enforce unload + pid-exit wait + non-tmpfs result paths (lesson of 2026-08-15) baked into the fetch tool |
| Feature creep into fork territory | Decision record (section 2) revisited at each phase gate; anything requiring llama-swap source changes is rejected by default |
| Plugin security (Phase 3) | Directory-confined loading, no network code fetch, review before enabling |
| Scope sprawl — 17 features | Phases are the control; stretch items explicitly deferred |

---

## 11. Open questions for the critic / user

1. **Naming:** "model console" is a placeholder. Better name?
2. **Postgres timing:** acceptable that PG lands in Phase 3 while the
   abstraction exists from Phase 1, or does the user need it sooner?
3. **"NVIDIA data flywheel":** what exactly is meant? A specific product,
   or a general "federate more servers" wish? Blocks 7.15 design.
4. **Pricing enrichment source:** OpenRouter API assumed; confirm.
5. **SQLAlchemy vs. hand-rolled** for the dual-backend layer.
6. **MCP transport priority:** stdio (local agents) vs. HTTP (hermesvm01)?
7. **Auth model:** single shared token vs. per-agent tokens.
8. **Auto-prune:** user said "automate getting rid of old models" — plan
   defaults to suggest-don't-execute. Confirm the appetite for true automation
   with a safety list.
9. **New repo vs. in-repo** for the console once it Dockerizes.
10. **UI stack:** single-file HTML/JS is fine at this scale; flag if the
    critic thinks it will hit a wall (current estimate: no, well under 100
    models and a handful of views).

---

## 12. Acceptance criteria (whole project)

- [ ] Stats survive service restart and reboot (7.2A verifiable by test).
- [ ] Dashboard shows every fleet model with status, GPU, tags, ctx, size,
      cutoff, HF link; search + sort + filters work on real data.
- [ ] Orphan files and missing-file entries are surfaced.
- [ ] Agent can list/search/inspect via MCP and HTTP without touching configs.
- [ ] Agent can fetch+register a model through the console with the same
      safety gates as fetch-model.sh (pin enforced, smoke gated, approved deploy).
- [ ] Nightly benchmark results browsable as a flat comparison.
- [ ] Console DB switchable SQLite↔Postgres with data preserved both ways.
- [ ] Zero modifications to llama-swap binaries/source; config changes only
      through the approved backup→edit→deploy flow.
- [ ] Runs as: bare process, Docker container, MCP endpoint.

---

*End of plan. Everything above the phase checklists is context a naive reviewer
needs; everything in sections 7–11 is open to critique. No implementation
beyond the existing uncommitted prototype has been performed or authorized.*
