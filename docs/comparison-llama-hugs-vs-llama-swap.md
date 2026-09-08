# Llama Hugs vs. Llama-swap: Feature Comparison

Llama Hugs is a fork of [llama-swap](https://github.com/mostlygeek/llama-swap) that extends the router with canonical model registry, GPU smoke-testing, VRAM tracking, Hugging Face verification, and a richer UI. This document compares the two.

---

## Shared Base Features

Both llama-swap and Llama Hugs inherit these capabilities from the upstream project:

| Feature | Description |
|---|---|
| **Hot-swapping** | Load/unload models on demand with automatic port allocation |
| **OpenAI API** | `/v1/completions`, `/v1/chat/completions`, `/v1/responses`, `/v1/embeddings`, `/v1/models`, `/v1/audio/*`, `/v1/images/*` |
| **Anthropic API** | `/v1/messages`, `/v1/messages/count_tokens` |
| **llama-server endpoints** | `/v1/rerank`, `/infill`, `/completion`, `/props` |
| **SDAPI** | `stable-diffusion.cpp` txt2img/img2img/loras |
| **audio.cpp** | `/audioapi/v1/tasks/run` |
| **ComfyUI** | `/comfyui/` passthrough |
| **Profiles** | Runtime model-ID routing and swap matrices |
| **TTL unloading** | Automatic model unloading after configurable idle timeout |
| **Matrix mode** | Concurrent multi-model serving |
| **Hooks** | Preload models on startup |
| **Filters** | `stripParams`, `setParams`, `setParamsByID` request modification |
| **Profiles** | Named model groupings with runtime switching |
| **API keys** | Restrict access to API endpoints |
| **Docker/Podman** | Container lifecycle via `cmd` / `cmdStop` |
| **Remote logs** | `GET /logs`, `/logs/stream`, `/logs/stream/{model_id}` |
| **Metrics** | Prometheus-compatible `/metrics` endpoint |
| **Health check** | `GET /health` |
| **Web UI** | Playground, model management, activity, logs, performance charts |

---

## Llama Hugs Additions

### Canonical Model Registry

A SQLite-backed persistence layer tracks every registered model, its assets, and smoke-test history.

| Table | Purpose |
|---|---|
| `hugs_models` | Canonical model records with runtime ID, display name, source repo, GGUF path, GPU backend, capabilities, context size |
| `hugs_model_assets` | Per-model file records: GGUF, multimodal projectors, ancillary files. Tracks disk size, VRAM required, system RAM required, load target, offload support |
| `hugs_smoke_tests` | Append-only history of smoke-test runs: VRAM before/peak/after, GPU backend, context size, success/failure, notes |

### REST API Extensions

| Endpoint | Description |
|---|---|
| `GET /api/hugs/models` | List all canonical models |
| `GET /api/hugs/models/{id}` | Get one model |
| `POST /api/hugs/models/{id}` | Upsert model (merge semantics) |
| `GET /api/hugs/models/{id}/assets` | List assets |
| `POST /api/hugs/models/{id}/assets` | Upsert asset |
| `GET /api/hugs/models/{id}/smoke` | List smoke-test history |
| `POST /api/hugs/models/{id}/smoke` | Record smoke-test result |
| `GET /api/hugs/meta` | List model metadata/tags |
| `POST /api/hugs/meta/{model}` | Update metadata |
| `GET /api/hugs/disk` | Scan local HF cache and model files |
| `GET /api/hugs/bench/leaderboard` | Best tokens/s per model×task with memory footprint |
| `POST /api/hugs/bench/ingest` | Ingest a benchmark datapoint |
| `POST /api/hugs/hf/rescan` | Verify models against Hugging Face |
| `GET /api/hugs/files/flags` | List file deletion flags |
| `POST /api/hugs/files/flags` | Set deletion flags |
| `POST /api/hugs/files/delete` | Guarded deletion |
| `GET /api/hugs/pricing` | Pricing enrichment data |
| `GET /api/hugs/settings` | Runtime settings |
| `POST /api/hugs/settings` | Update settings |

### Model Acquisition (`fetch-model.sh`)

A deterministic acquisition pipeline that:

1. Accepts Hugging Face URLs, direct HTTP(S) URLs, or local paths
2. Validates SHA-256 checksums
3. Extracts GGUF metadata (architecture, context length, multimodal support, MTP)
4. Resolves and downloads multimodal projectors
5. Smoke-tests on the configured GPU (AMD ROCm or NVIDIA CUDA)
6. Measures VRAM usage before, during, and after loading
7. Performs CUDA fit estimation
8. Registers CUDA variants automatically when supported
9. Persists the canonical model, assets, and smoke-test result
10. Deploys the live configuration atomically with rollback on failure

### GPU Support

| Feature | Llama Hugs |
|---|---|
| **AMD ROCm** | Primary platform; device pinning via `HIP_VISIBLE_DEVICES` |
| **NVIDIA CUDA** | Automatic detection; smoke testing when available |
| **Multi-GPU** | Per-model device assignment in config |
| **VRAM telemetry** | Before/peak/after measurement per model load |

### Web UI Enhancements

| Page | Llama Hugs Additions |
|---|---|
| **Models** | Capability badges (Vision, Function Calling, context window, etc.), Hugging Face verification badges (hf:checked, hf:vision, hf:tools, hf:image, hf:unmatched), hardware badges (AMD red, Nvidia green), HF rescan panel, disk verification panel |
| **Model Detail** | Memory tab showing all recorded smoke-test runs with VRAM before/peak/after in bytes |
| **Benchmark** | Max memory footprint column showing peak VRAM delta (peak minus before) |
| **Disk & Orphans** | File scanning, orphan detection, deletion flag workflow |
| **Hardware** | GPU monitoring |
| **Performance** | Live charts for GPU utilization, memory, temperature, power |
| **Settings** | Pricing source, HF token configuration |

### File Deletion Workflow

A guarded deletion system:

1. Flag files for deletion (pending state)
2. Review flagged files in the UI
3. Confirm or cancel deletion
4. Protected API endpoints prevent accidental removal

### Deployment

| Tool | Purpose |
|---|---|
| `llama-hugs-deploy` | Atomic config deployment with validation and automatic rollback |
| `install-llama-hugs-autodeploy.sh` | One-time privileged installer with narrow sudoers rule |
| `fetch-model.sh` | Unprivileged model acquisition and persistence |

---

## Summary

| Capability | llama-swap | Llama Hugs |
|---|---|---|
| Model hot-swapping | ✅ | ✅ |
| OpenAI/Anthropic APIs | ✅ | ✅ |
| Profiles / Matrix / TTL | ✅ | ✅ |
| Docker / Podman | ✅ | ✅ |
| Web UI (basic) | ✅ | ✅ |
| Canonical model registry | ❌ | ✅ |
| Persistent asset tracking | ❌ | ✅ |
| Smoke-test history | ❌ | ✅ |
| VRAM measurement | ❌ | ✅ |
| HF verification & rescan | ❌ | ✅ |
| Benchmark leaderboard + memory | ❌ | ✅ |
| Disk & orphan management | ❌ | ✅ |
| Guarded file deletion | ❌ | ✅ |
| CUDA auto-registration | ❌ | ✅ |
| Atomic deploy with rollback | ❌ | ✅ |
| GPU-badge UI (AMD/Nvidia) | ❌ | ✅ |
| Memory tab on model details | ❌ | ✅ |
