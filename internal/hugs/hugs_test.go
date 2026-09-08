package hugs

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

// osWriteFile writes n bytes to path (test helper).
func osWriteFile(path string, n int64) error {
	return os.WriteFile(path, make([]byte, n), 0o644)
}

func testDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	ctx := context.Background()
	// Apply the same DDL as the goose migration so tests match production.
	for _, stmt := range []string{
		`CREATE TABLE IF NOT EXISTS hugs_model_meta (
			model_id TEXT PRIMARY KEY, tags TEXT NOT NULL DEFAULT '',
			notes TEXT NOT NULL DEFAULT '', first_seen INTEGER NOT NULL DEFAULT 0,
			last_loaded_at INTEGER NOT NULL DEFAULT 0,
			last_request_at INTEGER NOT NULL DEFAULT 0,
			registered_at INTEGER NOT NULL DEFAULT 0)`,
		`CREATE TABLE IF NOT EXISTS hugs_settings (key TEXT PRIMARY KEY, value TEXT NOT NULL DEFAULT '')`,
	} {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("ddl: %v", err)
		}
	}
	return db
}

func TestModelMetaCRUD(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	m, err := GetModelMeta(ctx, db, "hugs-llama3.2-3b")
	if err != nil {
		t.Fatalf("get (creates default): %v", err)
	}
	if m.FirstSeen == 0 {
		t.Fatal("expected first_seen to be set on create")
	}

	m2, err := UpdateModelMeta(ctx, db, ModelMeta{ModelID: "hugs-llama3.2-3b", Tags: "scratch,small", Notes: "test model"})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if m2.Tags != "scratch,small" || m2.Notes != "test model" {
		t.Fatalf("update not applied: %+v", m2)
	}

	// Empty fields must NOT clobber stored values.
	m3, err := UpdateModelMeta(ctx, db, ModelMeta{ModelID: "hugs-llama3.2-3b"})
	if err != nil {
		t.Fatalf("update empty: %v", err)
	}
	if m3.Tags != "scratch,small" {
		t.Fatalf("empty update clobbered tags: %+v", m3)
	}

	all, err := ListModelMeta(ctx, db)
	if err != nil || len(all) != 1 {
		t.Fatalf("list: %v (%d rows)", err, len(all))
	}
}

func TestSettingsRoundTrip(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	if v, _ := Setting(ctx, db, "pricing_source"); v != "" {
		t.Fatalf("expected unset, got %q", v)
	}
	if err := SetSetting(ctx, db, "pricing_source", "/opt/llama-hugs/pricing.json"); err != nil {
		t.Fatalf("set: %v", err)
	}
	v, err := Setting(ctx, db, "pricing_source")
	if err != nil || v != "/opt/llama-hugs/pricing.json" {
		t.Fatalf("roundtrip: %q %v", v, err)
	}
}

func TestScanDirOrphansAndTotals(t *testing.T) {
	dir := t.TempDir()
	write := func(name string) {
		p := filepath.Join(dir, name)
		if err := osWriteFile(p, 1234); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
	write("model-a.gguf")
	write("model-b.gguf")
	write("notes.txt") // must be ignored

	rep, err := ScanDir(dir, true)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if rep.FileCount != 2 || rep.TotalBytes != 2468 {
		t.Fatalf("bad report: count=%d total=%d", rep.FileCount, rep.TotalBytes)
	}
	if len(rep.Files) != 2 {
		t.Fatalf("files not included: %d", len(rep.Files))
	}
}

func TestScanDirMissingRoot(t *testing.T) {
	if _, err := ScanDir("/nonexistent-path-lh-test", false); err == nil {
		t.Fatal("expected error for missing root")
	}
}
