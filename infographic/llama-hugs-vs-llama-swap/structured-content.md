# Llama Hugs vs Llama-swap

## Section 1: Shared Base (llama-swap)
- Hot-swapping: Load/unload models on demand
- OpenAI API: completions, chat, responses, embeddings, models
- Anthropic API: messages, count_tokens
- llama-server endpoints: rerank, infill, completion
- SDAPI: stable-diffusion.cpp support
- ComfyUI passthrough
- Profiles / Matrix / TTL unloading
- Docker / Podman support
- Web UI with playground

## Section 2: Llama Hugs Additions
- Canonical Model Registry (SQLite)
  - hugs_models, hugs_model_assets, hugs_smoke_tests
- REST API Extensions (15+ endpoints)
- fetch-model.sh acquisition pipeline
- AMD ROCm + NVIDIA CUDA telemetry
- VRAM before/peak/after measurement
- Hugging Face verification & rescan
- Benchmark leaderboard + memory tracking
- Disk & orphan management
- Guarded file deletion workflow
- Atomic deploy with rollback

## Section 3: UI Enhancements
- Models page: capability + HF + hardware badges
- Model Detail: Memory tab (VRAM history)
- Benchmark: Max memory footprint column
- Disk & Orphans: file scanning + deletion flags
- Hardware: GPU monitoring
- Performance: live charts

## Visual Elements
- Vertical divider: Llama-swap (left) vs Llama Hugs (right)
- Neon glow on Llama Hugs side
- Circuit patterns in background
- Chrome accents for new features
