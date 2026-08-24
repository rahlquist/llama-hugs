# `tools:` capability audit — candidate list

Models whose base architecture ships with a native function-calling / tool-use
template in llama.cpp (Jinja chat template advertises tool support). Setting
`capabilities.tools: true` on these makes the Function Calling badge appear and
advertises `supported_parameters: [tools, tool_choice]` via `/v1/models`.

## YES — set tools: true

**Qwen3 family** (all ship native tool-call templates):
- qwen3-8-27b-q4-k-m, qwen3-8-27b-ud-iq2-xxs (+cuda), qwen3-8-27b-q6-k,
  qwen3-8-27b-crack-iq2-m (+cuda), qwen3-8-27b-crack-iq3-m, qwen3-8-27b-crack-q8-0,
  qwen3-8-27b-q3-k-s, qwen3-8-27b-q5-k-m, qwen3-8-27b-ud-iq2-m (+cuda),
  qwen3-8-27b-ud-q3-k-xl, qwen3-8-27b-ud-q5-k-xl
- dagger-qwen3-6-27b-q6-k, dirk-qwen3-8-27b-ud-q6-k-xl, dirk-qwen3-8-27b-ud-q5-k-xl
- nail-qwen3-6-35b-a3b-ud-q5-k-xl, nail-qwen3-6-35b-a3b-ud-q4-k-xl
- qwen3-6-35b-a3b-uncensored-hauhaucs-aggressive-q5-k-p
- qwen3-6-27b-fable-fus-711-unheretic-* (all 4 variants)
- qwen3-30b-a3b-instruct-2507-ud-q5-k-xl, qwen-qwen3-30b-a3b-instruct-2507-q6-k-l
- qwen3-coder-30b-a3b-instruct-q5-k-m
- qwen3-30b-a3b
- qwen3.5-9b-q4/q6/q8 (+cuda aliases)
- ling-mini-2-q4/q5/q6 (+cuda aliases) — Ling-2.0 supports tool calls

**GLM family:**
- glm-4-7-flash-uncen-hrt-neo-code-max-imat-d-au-q6-k — GLM-4 supports tools

**Mistral (v0.3+ templates):**
- mistral-7b (+cuda) — v0.3 template includes [TOOL_CALLS]

**Qwen2.5:**
- qwen2.5-coder-14b (+cuda) — Qwen2.5 supports tool calls

## NO — leave tools unset/false

- llama3.2-3b (+cuda) — llama.cpp template does not emit tool grammar reliably; borderline. Default NO.
- llama-2-7b (+cuda) — no tool template
- granite-4.1-8b/3b (+cuda) — Granite 4.1 has a tool template upstream but llama.cpp support is recent; borderline. Default NO unless you use it.
- phi-4, phi-4-q8 (+cuda) — no reliable tool template in llama.cpp
- deepseek-r1-14b (+cuda) — reasoning model, no tool template
- deepseek-coder-v2-lite (+cuda) — no tool template in llama.cpp
- gemma-3-12b, gemma-4-12b, gemma4-coding-q6-k (+cuda), gemma-4-26b-moe — Gemma has no native tool-call format
- ornith-*, muse-glimmer-*, agents-a1, janus-pro-7b, flux2-klein, ovisocr2 — unknown/fine-tunes without verified tool templates
