# Llama Hugs — Handoff: Deferred Work

> Written 2026-08-24 and CLOSED as part of the live project. These two items
> are explicitly out of scope going forward; this document exists so a future
> session can pick them up without re-deriving context.

## 1. Federation (multi-server)

**Status:** DEFERRED indefinitely by user decision.

**What it was supposed to be:** handling multiple inference servers (the user
once phrased it as "like nvidia data flywheel" — the exact referent was never
confirmed, which is the first thing to nail down if this ever revives).

**Design groundwork already in place:**
- Upstream llama-swap has a native `peers:` config (proxy other llama-swap
  instances into one gateway, local models take precedence).
- The plan required model identity to be qualified `server_id/model_id` from
  day one so federation wouldn't require a schema migration later. The current
  store (`hugs_model_meta`, keyed on bare `model_id`) does NOT do this — any
  future federation work starts with a migration.
- `/v1/models` already renders peer models when configured.

**If reviving:** confirm the referent first, then choose between llama-swap's
`peers:` (router-level) vs a console-level server registry. Do not design
before the use case is concrete.

## 2. Pricing enrichment

**Status:** SHELVED indefinitely by user decision (locally-run models have no
canonical token price).

**What exists today (working, just empty):**
- `hugs_settings.pricing_source` — configurable file path (settable via
  `POST /api/hugs/settings`)
- `GET /api/hugs/pricing` serves `{model_id: {input_per_mtok,
  output_per_mtok, currency, source}}` from that JSON file
- Graceful degradation: missing/unparsable file yields empty entries + warning;
  nothing in the UI depends on it

**Two candidate interpretations, documented at shelving time:**
- **A. Hosted-equivalent** — OpenRouter rates per model twin. Needs ToS check
  first, plus name-matching (fork-pinned models may not exist there verbatim).
- **B. True local cost** — tokens/s from the 724-record bench data × GPU load
  wattage × electricity $/kWh. No external dependency; needs the user's rate.

**To revive:** pick A or B, generate `/opt/llama-hugs/pricing.json` in the
schema above, set the setting. Zero code changes needed.
