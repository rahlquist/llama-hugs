package hugs

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	_ "modernc.org/sqlite"
)

// canonicalTestDB opens an in-memory db and applies the canonical DDL via the
// same ensure helper the production functions use (mirrors the goose
// migration, exactly like testDB does for hugs_model_meta/hugs_settings).
func canonicalTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := ensureCanonical(context.Background(), db); err != nil {
		t.Fatalf("ensure canonical ddl: %v", err)
	}
	return db
}

func TestModelsUpsertGetList(t *testing.T) {
	db := canonicalTestDB(t)
	ctx := context.Background()

	m, err := UpsertModel(ctx, db, Model{
		ModelID: "hugs-qwen3-27b", DisplayName: "Qwen3 27B", SourceRepo: "Qwen/Qwen3-27B",
		GGUFPath: "/home/u/.cache/llama.cpp/qwen3-27b.gguf", GPUBackend: "ROCm",
		ContextSize: 64000, Status: "pinned", Notes: "primary",
	})
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if m.ModelID != "hugs-qwen3-27b" || m.Status != "pinned" || m.CreatedAt == 0 || m.UpdatedAt == 0 {
		t.Fatalf("upsert result wrong: %+v", m)
	}

	// Merge: empty fields must not clobber stored values.
	m2, err := UpsertModel(ctx, db, Model{ModelID: "hugs-qwen3-27b", Notes: "updated"})
	if err != nil {
		t.Fatalf("upsert merge: %v", err)
	}
	if m2.DisplayName != "Qwen3 27B" || m2.Status != "pinned" || m2.Notes != "updated" {
		t.Fatalf("merge clobbered fields: %+v", m2)
	}
	if m2.CreatedAt != m.CreatedAt {
		t.Fatalf("created_at not preserved: %d -> %d", m.CreatedAt, m2.CreatedAt)
	}

	got, err := GetModel(ctx, db, "hugs-qwen3-27b")
	if err != nil || got.GGUFPath != m.GGUFPath {
		t.Fatalf("get: %v %+v", err, got)
	}

	all, err := ListModels(ctx, db)
	if err != nil || len(all) != 1 || all[0].ModelID != "hugs-qwen3-27b" {
		t.Fatalf("list: %v (%d rows)", err, len(all))
	}

	if _, err := GetModel(ctx, db, "missing-model"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected ErrNoRows for missing model, got %v", err)
	}
	if _, err := UpsertModel(ctx, db, Model{}); err == nil {
		t.Fatal("expected error for empty model_id")
	}
}

func TestAssetsUpsertList(t *testing.T) {
	db := canonicalTestDB(t)
	ctx := context.Background()

	if _, err := UpsertModel(ctx, db, Model{ModelID: "hugs-qwen3-vl"}); err != nil {
		t.Fatalf("register model: %v", err)
	}

	// Assets for an unregistered model are refused.
	if _, err := UpsertAsset(ctx, db, Asset{ModelID: "ghost", AssetType: "mmproj"}); !errors.Is(err, ErrModelNotRegistered) {
		t.Fatalf("expected ErrModelNotRegistered, got %v", err)
	}

	a, err := UpsertAsset(ctx, db, Asset{
		ModelID: "hugs-qwen3-vl", AssetType: "mmproj", AssetName: "qwen3-vl-mmproj.gguf",
		LocalPath:     "/home/u/.cache/llama.cpp/qwen3-vl-mmproj.gguf",
		LocalFilename: "qwen3-vl-mmproj.gguf", DiskSizeBytes: 1_234_567_890,
		VRAMRequiredBytes: 1_500_000_000, LoadTarget: "vram", OffloadSupported: 1,
		MeasurementSource: "observed", SHA256: "abc123", Purpose: "vision projector",
	})
	if err != nil {
		t.Fatalf("upsert asset: %v", err)
	}
	if a.ID == 0 || a.ModelID != "hugs-qwen3-vl" || a.DiskSizeBytes != 1_234_567_890 {
		t.Fatalf("asset result wrong: %+v", a)
	}

	// Re-upsert on the same key updates measured fields; zeros don't clobber.
	a2, err := UpsertAsset(ctx, db, Asset{
		ModelID: "hugs-qwen3-vl", AssetType: "mmproj", AssetName: "qwen3-vl-mmproj.gguf",
		VRAMRequiredBytes: 1_600_000_000, MeasurementSource: "observed",
	})
	if err != nil {
		t.Fatalf("re-upsert asset: %v", err)
	}
	if a2.ID != a.ID {
		t.Fatalf("expected same row id, got %d != %d", a2.ID, a.ID)
	}
	if a2.VRAMRequiredBytes != 1_600_000_000 || a2.DiskSizeBytes != 1_234_567_890 || a2.SHA256 != "abc123" {
		t.Fatalf("merge wrong: %+v", a2)
	}
	if a2.CreatedAt != a.CreatedAt {
		t.Fatalf("created_at not preserved: %d -> %d", a.CreatedAt, a2.CreatedAt)
	}

	// A second asset type on the same model.
	if _, err := UpsertAsset(ctx, db, Asset{ModelID: "hugs-qwen3-vl", AssetType: "chat_template", AssetName: "qwen3-vl.jinja", LocalFilename: "qwen3-vl.jinja"}); err != nil {
		t.Fatalf("upsert second asset: %v", err)
	}

	byModel, err := ListAssets(ctx, db, "hugs-qwen3-vl")
	if err != nil || len(byModel) != 2 {
		t.Fatalf("list by model: %v (%d rows)", err, len(byModel))
	}
	// Ordered by asset_type then name: chat_template before mmproj.
	if byModel[0].AssetType != "chat_template" || byModel[1].AssetType != "mmproj" {
		t.Fatalf("asset ordering wrong: %+v", byModel)
	}

	all, err := ListAssets(ctx, db, "")
	if err != nil || len(all) != 2 {
		t.Fatalf("list all: %v (%d rows)", err, len(all))
	}

	if _, err := UpsertAsset(ctx, db, Asset{ModelID: "hugs-qwen3-vl"}); err == nil {
		t.Fatal("expected error for missing asset_type")
	}
}

func TestSmokeTestsInsertList(t *testing.T) {
	db := canonicalTestDB(t)
	ctx := context.Background()

	if _, err := UpsertModel(ctx, db, Model{ModelID: "hugs-llama3.2-3b"}); err != nil {
		t.Fatalf("register model: %v", err)
	}

	if _, err := InsertSmokeTest(ctx, db, SmokeTest{ModelID: "ghost"}); !errors.Is(err, ErrModelNotRegistered) {
		t.Fatalf("expected ErrModelNotRegistered, got %v", err)
	}

	pass, err := InsertSmokeTest(ctx, db, SmokeTest{
		ModelID: "hugs-llama3.2-3b", RunAt: 100, GPUBackend: "ROCm0", ContextSize: 64000,
		GPUVRAMBeforeBytes: 2_000_000_000, GPUVRAMPeakBytes: 6_100_000_000, GPUVRAMAfterBytes: 2_000_000_000,
		Success: 1, Notes: "full-context load ok",
	})
	if err != nil {
		t.Fatalf("insert pass: %v", err)
	}
	if pass.ID == 0 || pass.Success != 1 || pass.CreatedAt == 0 {
		t.Fatalf("pass result wrong: %+v", pass)
	}

	fail, err := InsertSmokeTest(ctx, db, SmokeTest{
		ModelID: "hugs-llama3.2-3b", RunAt: 200, GPUBackend: "CUDA0", ContextSize: 64000,
		Success: 0, Error: "hipMalloc out of memory",
	})
	if err != nil {
		t.Fatalf("insert fail: %v", err)
	}
	if fail.Success != 0 || fail.Error == "" {
		t.Fatalf("fail result wrong: %+v", fail)
	}

	// Non-zero success normalizes to 1.
	odd, err := InsertSmokeTest(ctx, db, SmokeTest{ModelID: "hugs-llama3.2-3b", RunAt: 300, Success: 42})
	if err != nil {
		t.Fatalf("insert odd success: %v", err)
	}
	if odd.Success != 1 {
		t.Fatalf("success not normalized: %+v", odd)
	}

	// Newest first.
	tests, err := ListSmokeTests(ctx, db, "hugs-llama3.2-3b")
	if err != nil || len(tests) != 3 {
		t.Fatalf("list by model: %v (%d rows)", err, len(tests))
	}
	if tests[0].RunAt != 300 || tests[1].RunAt != 200 || tests[2].RunAt != 100 {
		t.Fatalf("not newest first: %+v", tests)
	}

	all, err := ListSmokeTests(ctx, db, "")
	if err != nil || len(all) != 3 {
		t.Fatalf("list all: %v (%d rows)", err, len(all))
	}

	if _, err := InsertSmokeTest(ctx, db, SmokeTest{}); err == nil {
		t.Fatal("expected error for empty model_id")
	}
}
