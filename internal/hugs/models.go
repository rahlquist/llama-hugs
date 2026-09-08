package hugs

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ErrModelNotRegistered is returned when an asset or smoke-test write names a
// model with no hugs_models row. The canonical FK is declarative only
// (SQLite enforcement is off in this store), so the parent row is checked
// explicitly; the server maps this to 404.
var ErrModelNotRegistered = errors.New("hugs: model not registered")

// Canonical model registry, ancillary asset inventory, and GPU smoke-test
// history. Backed by goose migration 00003_hugs_canonical.sql; the ensure DDL
// below mirrors that migration so environments that open a raw db (tests) or
// stores that predate the migration still get the tables additively, exactly
// like hugs_bench and hugs_file_flags.

// ensureCanonicalDDL mirrors internal/store/migrations/00003_hugs_canonical.sql
// (idempotent form) for raw-db environments.
const ensureCanonicalDDL = `
CREATE TABLE IF NOT EXISTS hugs_models (
    model_id          TEXT PRIMARY KEY,
    base_model_id     TEXT NOT NULL DEFAULT '',
    display_name      TEXT NOT NULL DEFAULT '',
    source_repo       TEXT NOT NULL DEFAULT '',
    gguf_path         TEXT NOT NULL DEFAULT '',
    gpu_backend       TEXT NOT NULL DEFAULT '',
    is_cuda_variant   INTEGER NOT NULL DEFAULT 0,
    context_size      INTEGER NOT NULL DEFAULT 0,
    capabilities_json TEXT NOT NULL DEFAULT '',
    status            TEXT NOT NULL DEFAULT '',
    notes             TEXT NOT NULL DEFAULT '',
    created_at        INTEGER NOT NULL DEFAULT 0,
    updated_at        INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_hugs_models_base_model_id ON hugs_models (base_model_id);

CREATE TABLE IF NOT EXISTS hugs_model_assets (
    id                       INTEGER PRIMARY KEY AUTOINCREMENT,
    model_id                 TEXT NOT NULL REFERENCES hugs_models(model_id) ON DELETE CASCADE,
    asset_type               TEXT NOT NULL DEFAULT '',
    asset_name               TEXT NOT NULL DEFAULT '',
    local_path               TEXT NOT NULL DEFAULT '',
    local_filename           TEXT NOT NULL DEFAULT '',
    disk_size_bytes          INTEGER NOT NULL DEFAULT 0,
    vram_required_bytes      INTEGER NOT NULL DEFAULT 0,
    system_ram_required_bytes INTEGER NOT NULL DEFAULT 0,
    load_target              TEXT NOT NULL DEFAULT '',
    offload_supported        INTEGER NOT NULL DEFAULT 0,
    purpose                  TEXT NOT NULL DEFAULT '',
    measurement_source       TEXT NOT NULL DEFAULT '',
    sha256                   TEXT NOT NULL DEFAULT '',
    detected_at              INTEGER NOT NULL DEFAULT 0,
    created_at               INTEGER NOT NULL DEFAULT 0,
    updated_at               INTEGER NOT NULL DEFAULT 0
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_hugs_model_assets_model_type_name
    ON hugs_model_assets (model_id, asset_type, asset_name);
CREATE INDEX IF NOT EXISTS idx_hugs_model_assets_model_type
    ON hugs_model_assets (model_id, asset_type);

CREATE TABLE IF NOT EXISTS hugs_smoke_tests (
    id                     INTEGER PRIMARY KEY AUTOINCREMENT,
    model_id               TEXT NOT NULL REFERENCES hugs_models(model_id) ON DELETE CASCADE,
    run_at                 INTEGER NOT NULL DEFAULT 0,
    gpu_backend            TEXT NOT NULL DEFAULT '',
    context_size           INTEGER NOT NULL DEFAULT 0,
    gpu_vram_before_bytes  INTEGER NOT NULL DEFAULT 0,
    gpu_vram_peak_bytes    INTEGER NOT NULL DEFAULT 0,
    gpu_vram_after_bytes   INTEGER NOT NULL DEFAULT 0,
    success                INTEGER NOT NULL DEFAULT 0,
    error                  TEXT NOT NULL DEFAULT '',
    notes                  TEXT NOT NULL DEFAULT '',
    created_at             INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_hugs_smoke_tests_model_run
    ON hugs_smoke_tests (model_id, run_at DESC);
CREATE INDEX IF NOT EXISTS idx_hugs_smoke_tests_model_success
    ON hugs_smoke_tests (model_id, success);
`

// ensureCanonical creates the canonical tables if missing (idempotent).
func ensureCanonical(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, ensureCanonicalDDL); err != nil {
		return fmt.Errorf("hugs: ensure canonical schema: %w", err)
	}
	return nil
}

// Model is one row of the canonical model registry: a single identity per
// runtime model variant.
type Model struct {
	ModelID          string `json:"model_id"`
	BaseModelID      string `json:"base_model_id,omitempty"`
	DisplayName      string `json:"display_name,omitempty"`
	SourceRepo       string `json:"source_repo,omitempty"`
	GGUFPath         string `json:"gguf_path,omitempty"`
	GPUBackend       string `json:"gpu_backend,omitempty"`
	IsCudaVariant    int64  `json:"is_cuda_variant"`
	ContextSize      int64  `json:"context_size"`
	CapabilitiesJSON string `json:"capabilities_json,omitempty"`
	Status           string `json:"status,omitempty"`
	Notes            string `json:"notes,omitempty"`
	CreatedAt        int64  `json:"created_at"`
	UpdatedAt        int64  `json:"updated_at"`
}

const modelColumns = `model_id, base_model_id, display_name, source_repo, gguf_path,
	gpu_backend, is_cuda_variant, context_size, capabilities_json, status, notes, created_at, updated_at`

// UpsertModel registers (or updates) one canonical model. Merge semantics:
// empty strings and zero values leave the stored field untouched, matching
// UpdateModelMeta; created_at is preserved across updates. The canonical
// record is authoritative, so callers that need to clear a field should send
// the full desired state for all non-empty fields.
func UpsertModel(ctx context.Context, db *sql.DB, in Model) (Model, error) {
	if err := ensureCanonical(ctx, db); err != nil {
		return Model{}, err
	}
	in.ModelID = strings.TrimSpace(in.ModelID)
	if in.ModelID == "" {
		return Model{}, fmt.Errorf("hugs: model_id required")
	}
	now := time.Now().Unix()
	if in.CreatedAt == 0 {
		in.CreatedAt = now
	}
	in.UpdatedAt = now
	_, err := db.ExecContext(ctx, `
		INSERT INTO hugs_models (model_id, base_model_id, display_name, source_repo, gguf_path,
			gpu_backend, is_cuda_variant, context_size, capabilities_json, status, notes, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(model_id) DO UPDATE SET
			base_model_id     = CASE WHEN excluded.base_model_id <> '' THEN excluded.base_model_id ELSE hugs_models.base_model_id END,
			display_name      = CASE WHEN excluded.display_name  <> '' THEN excluded.display_name  ELSE hugs_models.display_name END,
			source_repo       = CASE WHEN excluded.source_repo   <> '' THEN excluded.source_repo   ELSE hugs_models.source_repo END,
			gguf_path         = CASE WHEN excluded.gguf_path     <> '' THEN excluded.gguf_path     ELSE hugs_models.gguf_path END,
			gpu_backend       = CASE WHEN excluded.gpu_backend   <> '' THEN excluded.gpu_backend   ELSE hugs_models.gpu_backend END,
			is_cuda_variant   = CASE WHEN excluded.is_cuda_variant <> 0 THEN excluded.is_cuda_variant ELSE hugs_models.is_cuda_variant END,
			context_size      = CASE WHEN excluded.context_size  > 0 THEN excluded.context_size  ELSE hugs_models.context_size END,
			capabilities_json = CASE WHEN excluded.capabilities_json <> '' THEN excluded.capabilities_json ELSE hugs_models.capabilities_json END,
			status            = CASE WHEN excluded.status <> '' THEN excluded.status ELSE hugs_models.status END,
			notes             = CASE WHEN excluded.notes <> '' THEN excluded.notes ELSE hugs_models.notes END,
			created_at        = hugs_models.created_at,
			updated_at        = excluded.updated_at`,
		in.ModelID, in.BaseModelID, in.DisplayName, in.SourceRepo, in.GGUFPath, in.GPUBackend,
		in.IsCudaVariant, in.ContextSize, in.CapabilitiesJSON, in.Status, in.Notes, in.CreatedAt, in.UpdatedAt,
	)
	if err != nil {
		return Model{}, fmt.Errorf("hugs: upsert model %q: %w", in.ModelID, err)
	}
	return GetModel(ctx, db, in.ModelID)
}

// GetModel returns one canonical model. Returns sql.ErrNoRows when absent
// (canonical models are explicit — unlike ModelMeta, nothing is lazy-created).
func GetModel(ctx context.Context, db *sql.DB, modelID string) (Model, error) {
	var m Model
	err := db.QueryRowContext(ctx,
		`SELECT `+modelColumns+` FROM hugs_models WHERE model_id = ?`, modelID,
	).Scan(&m.ModelID, &m.BaseModelID, &m.DisplayName, &m.SourceRepo, &m.GGUFPath,
		&m.GPUBackend, &m.IsCudaVariant, &m.ContextSize, &m.CapabilitiesJSON,
		&m.Status, &m.Notes, &m.CreatedAt, &m.UpdatedAt)
	if err != nil {
		return Model{}, err
	}
	return m, nil
}

// ListModels returns all canonical models ordered by model id.
func ListModels(ctx context.Context, db *sql.DB) ([]Model, error) {
	if err := ensureCanonical(ctx, db); err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(ctx,
		`SELECT `+modelColumns+` FROM hugs_models ORDER BY model_id`)
	if err != nil {
		return nil, fmt.Errorf("hugs: list models: %w", err)
	}
	defer rows.Close()
	out := []Model{}
	for rows.Next() {
		var m Model
		if err := rows.Scan(&m.ModelID, &m.BaseModelID, &m.DisplayName, &m.SourceRepo,
			&m.GGUFPath, &m.GPUBackend, &m.IsCudaVariant, &m.ContextSize,
			&m.CapabilitiesJSON, &m.Status, &m.Notes, &m.CreatedAt, &m.UpdatedAt); err != nil {
			return nil, fmt.Errorf("hugs: scan model: %w", err)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// Asset is one ancillary file of a model (mmproj, vision/text encoder, VAE,
// LoRA, ControlNet, draft model, chat template, tokenizer, ...).
type Asset struct {
	ID                     int64  `json:"id"`
	ModelID                string `json:"model_id"`
	AssetType              string `json:"asset_type"`
	AssetName              string `json:"asset_name,omitempty"`
	LocalPath              string `json:"local_path,omitempty"`
	LocalFilename          string `json:"local_filename,omitempty"`
	DiskSizeBytes          int64  `json:"disk_size_bytes"`
	VRAMRequiredBytes      int64  `json:"vram_required_bytes"`
	SystemRAMRequiredBytes int64  `json:"system_ram_required_bytes"`
	LoadTarget             string `json:"load_target,omitempty"`
	OffloadSupported       int64  `json:"offload_supported"`
	Purpose                string `json:"purpose,omitempty"`
	MeasurementSource      string `json:"measurement_source,omitempty"`
	SHA256                 string `json:"sha256,omitempty"`
	DetectedAt             int64  `json:"detected_at"`
	CreatedAt              int64  `json:"created_at"`
	UpdatedAt              int64  `json:"updated_at"`
}

const assetColumns = `id, model_id, asset_type, asset_name, local_path, local_filename,
	disk_size_bytes, vram_required_bytes, system_ram_required_bytes, load_target,
	offload_supported, purpose, measurement_source, sha256, detected_at, created_at, updated_at`

// modelRegistered reports whether modelID has a canonical row (the FK is
// declarative only — SQLite enforcement is off in this store — so the parent
// row is checked explicitly to keep assets and smoke tests from orphaning).
func modelRegistered(ctx context.Context, db *sql.DB, modelID string) error {
	var one int
	err := db.QueryRowContext(ctx, `SELECT 1 FROM hugs_models WHERE model_id = ?`, modelID).Scan(&one)
	if err == sql.ErrNoRows {
		return fmt.Errorf("%w: %q", ErrModelNotRegistered, modelID)
	}
	return err
}

// UpsertAsset records (or updates) one ancillary asset. Merge semantics for
// measured fields: zeros and empty strings leave stored values untouched, so
// partial discovery passes never clobber known measurements. The unique key
// is (model_id, asset_type, asset_name). The parent model must already be
// registered.
func UpsertAsset(ctx context.Context, db *sql.DB, in Asset) (Asset, error) {
	if err := ensureCanonical(ctx, db); err != nil {
		return Asset{}, err
	}
	in.ModelID = strings.TrimSpace(in.ModelID)
	in.AssetType = strings.TrimSpace(in.AssetType)
	in.AssetName = strings.TrimSpace(in.AssetName)
	if in.ModelID == "" || in.AssetType == "" {
		return Asset{}, fmt.Errorf("hugs: model_id and asset_type required")
	}
	if err := modelRegistered(ctx, db, in.ModelID); err != nil {
		return Asset{}, err
	}
	now := time.Now().Unix()
	if in.DetectedAt == 0 {
		in.DetectedAt = now
	}
	if in.CreatedAt == 0 {
		in.CreatedAt = now
	}
	in.UpdatedAt = now
	_, err := db.ExecContext(ctx, `
		INSERT INTO hugs_model_assets (model_id, asset_type, asset_name, local_path, local_filename,
			disk_size_bytes, vram_required_bytes, system_ram_required_bytes, load_target,
			offload_supported, purpose, measurement_source, sha256, detected_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(model_id, asset_type, asset_name) DO UPDATE SET
			local_path             = CASE WHEN excluded.local_path <> '' THEN excluded.local_path ELSE hugs_model_assets.local_path END,
			local_filename         = CASE WHEN excluded.local_filename <> '' THEN excluded.local_filename ELSE hugs_model_assets.local_filename END,
			disk_size_bytes        = CASE WHEN excluded.disk_size_bytes > 0 THEN excluded.disk_size_bytes ELSE hugs_model_assets.disk_size_bytes END,
			vram_required_bytes    = CASE WHEN excluded.vram_required_bytes > 0 THEN excluded.vram_required_bytes ELSE hugs_model_assets.vram_required_bytes END,
			system_ram_required_bytes = CASE WHEN excluded.system_ram_required_bytes > 0 THEN excluded.system_ram_required_bytes ELSE hugs_model_assets.system_ram_required_bytes END,
			load_target            = CASE WHEN excluded.load_target <> '' THEN excluded.load_target ELSE hugs_model_assets.load_target END,
			offload_supported      = CASE WHEN excluded.offload_supported <> 0 THEN excluded.offload_supported ELSE hugs_model_assets.offload_supported END,
			purpose                = CASE WHEN excluded.purpose <> '' THEN excluded.purpose ELSE hugs_model_assets.purpose END,
			measurement_source     = CASE WHEN excluded.measurement_source <> '' THEN excluded.measurement_source ELSE hugs_model_assets.measurement_source END,
			sha256                 = CASE WHEN excluded.sha256 <> '' THEN excluded.sha256 ELSE hugs_model_assets.sha256 END,
			detected_at            = CASE WHEN excluded.detected_at > 0 THEN excluded.detected_at ELSE hugs_model_assets.detected_at END,
			created_at             = hugs_model_assets.created_at,
			updated_at             = excluded.updated_at`,
		in.ModelID, in.AssetType, in.AssetName, in.LocalPath, in.LocalFilename,
		in.DiskSizeBytes, in.VRAMRequiredBytes, in.SystemRAMRequiredBytes, in.LoadTarget,
		in.OffloadSupported, in.Purpose, in.MeasurementSource, in.SHA256,
		in.DetectedAt, in.CreatedAt, in.UpdatedAt,
	)
	if err != nil {
		return Asset{}, fmt.Errorf("hugs: upsert asset %s/%s: %w", in.ModelID, in.AssetType, err)
	}
	return getAsset(ctx, db, in.ModelID, in.AssetType, in.AssetName)
}

func getAsset(ctx context.Context, db *sql.DB, modelID, assetType, assetName string) (Asset, error) {
	var a Asset
	err := db.QueryRowContext(ctx,
		`SELECT `+assetColumns+` FROM hugs_model_assets
		 WHERE model_id = ? AND asset_type = ? AND asset_name = ?`,
		modelID, assetType, assetName,
	).Scan(&a.ID, &a.ModelID, &a.AssetType, &a.AssetName, &a.LocalPath, &a.LocalFilename,
		&a.DiskSizeBytes, &a.VRAMRequiredBytes, &a.SystemRAMRequiredBytes, &a.LoadTarget,
		&a.OffloadSupported, &a.Purpose, &a.MeasurementSource, &a.SHA256,
		&a.DetectedAt, &a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		return Asset{}, err
	}
	return a, nil
}

// ListAssets returns assets for one model (modelID non-empty) or all assets
// (modelID empty), ordered by type then name.
func ListAssets(ctx context.Context, db *sql.DB, modelID string) ([]Asset, error) {
	if err := ensureCanonical(ctx, db); err != nil {
		return nil, err
	}
	query := `SELECT ` + assetColumns + ` FROM hugs_model_assets`
	args := []any{}
	if modelID = strings.TrimSpace(modelID); modelID != "" {
		query += ` WHERE model_id = ?`
		args = append(args, modelID)
	}
	query += ` ORDER BY asset_type, asset_name`
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("hugs: list assets: %w", err)
	}
	defer rows.Close()
	out := []Asset{}
	for rows.Next() {
		var a Asset
		if err := rows.Scan(&a.ID, &a.ModelID, &a.AssetType, &a.AssetName, &a.LocalPath,
			&a.LocalFilename, &a.DiskSizeBytes, &a.VRAMRequiredBytes, &a.SystemRAMRequiredBytes,
			&a.LoadTarget, &a.OffloadSupported, &a.Purpose, &a.MeasurementSource, &a.SHA256,
			&a.DetectedAt, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, fmt.Errorf("hugs: scan asset: %w", err)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// SmokeTest is one GPU smoke-test run for a model (load + completion probe at
// the production context, before/peak/after VRAM where the driver reports it).
type SmokeTest struct {
	ID                 int64  `json:"id"`
	ModelID            string `json:"model_id"`
	RunAt              int64  `json:"run_at"`
	GPUBackend         string `json:"gpu_backend,omitempty"`
	ContextSize        int64  `json:"context_size"`
	GPUVRAMBeforeBytes int64  `json:"gpu_vram_before_bytes"`
	GPUVRAMPeakBytes   int64  `json:"gpu_vram_peak_bytes"`
	GPUVRAMAfterBytes  int64  `json:"gpu_vram_after_bytes"`
	Success            int64  `json:"success"`
	Error              string `json:"error,omitempty"`
	Notes              string `json:"notes,omitempty"`
	CreatedAt          int64  `json:"created_at"`
}

const smokeTestColumns = `id, model_id, run_at, gpu_backend, context_size,
	gpu_vram_before_bytes, gpu_vram_peak_bytes, gpu_vram_after_bytes, success, error, notes, created_at`

// InsertSmokeTest records one smoke-test run (append-only history). The parent
// model must already be registered. success is normalized: any non-zero value
// stores 1.
func InsertSmokeTest(ctx context.Context, db *sql.DB, in SmokeTest) (SmokeTest, error) {
	if err := ensureCanonical(ctx, db); err != nil {
		return SmokeTest{}, err
	}
	in.ModelID = strings.TrimSpace(in.ModelID)
	if in.ModelID == "" {
		return SmokeTest{}, fmt.Errorf("hugs: model_id required")
	}
	if err := modelRegistered(ctx, db, in.ModelID); err != nil {
		return SmokeTest{}, err
	}
	now := time.Now().Unix()
	if in.RunAt == 0 {
		in.RunAt = now
	}
	in.CreatedAt = now
	if in.Success != 0 {
		in.Success = 1
	}
	res, err := db.ExecContext(ctx, `
		INSERT INTO hugs_smoke_tests (model_id, run_at, gpu_backend, context_size,
			gpu_vram_before_bytes, gpu_vram_peak_bytes, gpu_vram_after_bytes, success, error, notes, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		in.ModelID, in.RunAt, in.GPUBackend, in.ContextSize,
		in.GPUVRAMBeforeBytes, in.GPUVRAMPeakBytes, in.GPUVRAMAfterBytes,
		in.Success, in.Error, in.Notes, in.CreatedAt,
	)
	if err != nil {
		return SmokeTest{}, fmt.Errorf("hugs: insert smoke test: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return SmokeTest{}, fmt.Errorf("hugs: insert smoke test id: %w", err)
	}
	in.ID = id
	return in, nil
}

// ListSmokeTests returns smoke-test history for one model (modelID non-empty)
// or across all models (modelID empty), newest first.
func ListSmokeTests(ctx context.Context, db *sql.DB, modelID string) ([]SmokeTest, error) {
	if err := ensureCanonical(ctx, db); err != nil {
		return nil, err
	}
	query := `SELECT ` + smokeTestColumns + ` FROM hugs_smoke_tests`
	args := []any{}
	if modelID = strings.TrimSpace(modelID); modelID != "" {
		query += ` WHERE model_id = ?`
		args = append(args, modelID)
	}
	query += ` ORDER BY run_at DESC, id DESC`
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("hugs: list smoke tests: %w", err)
	}
	defer rows.Close()
	out := []SmokeTest{}
	for rows.Next() {
		var st SmokeTest
		if err := rows.Scan(&st.ID, &st.ModelID, &st.RunAt, &st.GPUBackend, &st.ContextSize,
			&st.GPUVRAMBeforeBytes, &st.GPUVRAMPeakBytes, &st.GPUVRAMAfterBytes,
			&st.Success, &st.Error, &st.Notes, &st.CreatedAt); err != nil {
			return nil, fmt.Errorf("hugs: scan smoke test: %w", err)
		}
		out = append(out, st)
	}
	return out, rows.Err()
}
