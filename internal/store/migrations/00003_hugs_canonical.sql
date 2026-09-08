-- +goose Up
-- Canonical model registry, ancillary asset inventory, and GPU smoke-test
-- history. See llama-hugs-erd-updated.html (PROPOSED entities) for the design
-- intent; these tables make that schema real. hugs_model_meta, hugs_bench and
-- hugs_file_flags are intentionally left untouched: meta is a lazily-created
-- 1:0..1 extension of hugs_models, bench/flags predate this migration and are
-- managed additively by their own ensure-* helpers.

CREATE TABLE IF NOT EXISTS hugs_models (
    model_id          TEXT PRIMARY KEY,              -- runtime id as configured (e.g. "hugs-llama3.2-3b")
    base_model_id     TEXT NOT NULL DEFAULT '',      -- canonical HF id when the runtime id is an alias/variant
    display_name      TEXT NOT NULL DEFAULT '',
    source_repo       TEXT NOT NULL DEFAULT '',      -- HF repo or local source
    gguf_path         TEXT NOT NULL DEFAULT '',      -- primary model file path
    gpu_backend       TEXT NOT NULL DEFAULT '',      -- ROCm | CUDA | CPU ...
    is_cuda_variant   INTEGER NOT NULL DEFAULT 0,    -- alias registered for the CUDA GPU group
    context_size      INTEGER NOT NULL DEFAULT 0,    -- effective context used at runtime
    capabilities_json TEXT NOT NULL DEFAULT '',      -- freeform capability flags, JSON-encoded
    status            TEXT NOT NULL DEFAULT '',      -- e.g. active | pinned | excluded
    notes             TEXT NOT NULL DEFAULT '',
    created_at        INTEGER NOT NULL DEFAULT 0,    -- unix ts
    updated_at        INTEGER NOT NULL DEFAULT 0     -- unix ts
);

CREATE INDEX IF NOT EXISTS idx_hugs_models_base_model_id
    ON hugs_models (base_model_id);

CREATE TABLE IF NOT EXISTS hugs_model_assets (
    id                       INTEGER PRIMARY KEY AUTOINCREMENT,
    model_id                 TEXT NOT NULL REFERENCES hugs_models(model_id) ON DELETE CASCADE,
    asset_type               TEXT NOT NULL DEFAULT '',   -- mmproj | vision_encoder | text_encoder | VAE | LoRA | ControlNet | draft_model | chat_template | tokenizer | ...
    asset_name               TEXT NOT NULL DEFAULT '',   -- e.g. "clip.vision", "qwen3-vl-mmproj.gguf"
    local_path               TEXT NOT NULL DEFAULT '',
    local_filename           TEXT NOT NULL DEFAULT '',
    disk_size_bytes          INTEGER NOT NULL DEFAULT 0,
    vram_required_bytes      INTEGER NOT NULL DEFAULT 0,
    system_ram_required_bytes INTEGER NOT NULL DEFAULT 0,
    load_target              TEXT NOT NULL DEFAULT '',   -- vram | system_ram | disk_only | unknown
    offload_supported        INTEGER NOT NULL DEFAULT 0,
    purpose                  TEXT NOT NULL DEFAULT '',   -- freeform role note
    measurement_source       TEXT NOT NULL DEFAULT '',   -- estimated | observed | metadata | manual
    sha256                   TEXT NOT NULL DEFAULT '',
    detected_at              INTEGER NOT NULL DEFAULT 0, -- unix ts when the asset was found/measured
    created_at               INTEGER NOT NULL DEFAULT 0,
    updated_at               INTEGER NOT NULL DEFAULT 0
);

-- One asset per (model, type, name); the upsert conflict target.
CREATE UNIQUE INDEX IF NOT EXISTS idx_hugs_model_assets_model_type_name
    ON hugs_model_assets (model_id, asset_type, asset_name);

CREATE INDEX IF NOT EXISTS idx_hugs_model_assets_model_type
    ON hugs_model_assets (model_id, asset_type);

CREATE TABLE IF NOT EXISTS hugs_smoke_tests (
    id                     INTEGER PRIMARY KEY AUTOINCREMENT,
    model_id               TEXT NOT NULL REFERENCES hugs_models(model_id) ON DELETE CASCADE,
    run_at                 INTEGER NOT NULL DEFAULT 0,   -- unix ts of the run
    gpu_backend            TEXT NOT NULL DEFAULT '',     -- device: ROCm0 | CUDA0 | ...
    context_size           INTEGER NOT NULL DEFAULT 0,
    gpu_vram_before_bytes  INTEGER NOT NULL DEFAULT 0,
    gpu_vram_peak_bytes    INTEGER NOT NULL DEFAULT 0,
    gpu_vram_after_bytes   INTEGER NOT NULL DEFAULT 0,
    success                INTEGER NOT NULL DEFAULT 0,   -- 0 = failed, 1 = passed
    error                  TEXT NOT NULL DEFAULT '',     -- failure reason when success = 0
    notes                  TEXT NOT NULL DEFAULT '',
    created_at             INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_hugs_smoke_tests_model_run
    ON hugs_smoke_tests (model_id, run_at DESC);

CREATE INDEX IF NOT EXISTS idx_hugs_smoke_tests_model_success
    ON hugs_smoke_tests (model_id, success);

-- +goose Down
DROP TABLE hugs_smoke_tests;
DROP TABLE hugs_model_assets;
DROP TABLE hugs_models;
