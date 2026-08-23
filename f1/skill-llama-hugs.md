---
name: llama-hugs
description: Drive the Llama Hugs model router (llama-swap fork) on wimpy over HTTP - list models, disk report, tags/notes, benchmarks, load/unload.
---

# Llama Hugs — agent HTTP API

Llama Hugs is the llama-swap-fork router on wimpy.home.lan:8181. All requests
need the bearer key (ask the user; never hardcode it). Base URL:
`http://wimpy.home.lan:8181`

Every example: `-H "Authorization: Bearer $HUGS_KEY"`.

## Read tools

| Purpose | Endpoint |
|---|---|
| Fleet listing (OpenAI format, incl. status + metadata) | `GET /v1/models` |
| Currently loaded models | `GET /running` |
| Per-model tags/notes/lifecycle | `GET /api/hugs/meta/{model}` |
| All model metadata | `GET /api/hugs/meta` |
| GGUF disk usage + orphans | `GET /api/hugs/disk` (add `&files=1` for per-file) |
| Pricing enrichment (if configured) | `GET /api/hugs/pricing` |
| Benchmark leaderboard | `GET /api/hugs/bench/leaderboard` |
| Activity stats | `GET /api/metrics/stats`, `GET /api/metrics/activity` |

## Write tools (use sparingly)

| Purpose | Endpoint |
|---|---|
| Set tags/notes for a model | `POST /api/hugs/meta/{model}` body `{"tags":"a,b","notes":"..."}` |
| Configure pricing source file | `POST /api/hugs/settings` body `{"pricing_source":"/opt/llama-hugs/pricing.json"}` |
| Ingest benchmark JSONL | `POST /api/hugs/bench/ingest` body `{"source_file":"/path.jsonl"}` |
| Unload one model | `POST /api/models/unload/{model}` |
| Unload all | `POST /api/models/unload` |

## Load a model (OpenAI-compatible)

Any chat completion against a model ID loads it:

```
curl -X POST http://wimpy.home.lan:8181/v1/chat/completions \
  -H "Authorization: Bearer $HUGS_KEY" -H "Content-Type: application/json" \
  -d '{"model":"hugs-llama3.2-3b","messages":[{"role":"user","content":"hi"}]}'
```

Model IDs are `hugs-`-prefixed. The full fleet config mirrors production
llama-swap entries; see /v1/models for what this instance knows.

## Rules

- Never touch llama-swap on :8080 — that is the production router.
- Mutating calls are logged server-side; prefer reads.
- Large-model sequential loads must wait for unload completion (OOM history).
