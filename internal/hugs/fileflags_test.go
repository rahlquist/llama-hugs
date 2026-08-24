package hugs

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
)

func flagsDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestFileFlagsRoundTrip(t *testing.T) {
	db := flagsDB(t)
	ctx := context.Background()

	if err := SetFileFlags(ctx, db, []string{"a.gguf", "b.gguf"}, true); err != nil {
		t.Fatalf("flag: %v", err)
	}
	got, err := ListFileFlags(ctx, db)
	if err != nil || len(got) != 2 {
		t.Fatalf("list: %v (%v)", err, got)
	}

	if err := SetFileFlags(ctx, db, []string{"a.gguf"}, false); err != nil {
		t.Fatalf("unflag: %v", err)
	}
	got, _ = ListFileFlags(ctx, db)
	if len(got) != 1 || got[0] != "b.gguf" {
		t.Fatalf("after unflag: %v", got)
	}
}

func TestSetFileFlagsRejectsPaths(t *testing.T) {
	db := flagsDB(t)
	ctx := context.Background()

	for _, bad := range []string{"../evil.gguf", "sub/dir.gguf", "", "a/b.gguf"} {
		if err := SetFileFlags(ctx, db, []string{bad}, true); err == nil {
			t.Fatalf("expected rejection for %q", bad)
		}
	}
}

func TestDeleteFilesOnlyInsideRoot(t *testing.T) {
	dir := t.TempDir()
	outside := t.TempDir()

	inFile := filepath.Join(dir, "inside.gguf")
	outFile := filepath.Join(outside, "outside.gguf")
	os.WriteFile(inFile, []byte("x"), 0o644)
	os.WriteFile(outFile, []byte("x"), 0o644)

	deleted, errs := DeleteFiles(context.Background(), nil, dir, []string{inFile, outFile})
	if len(errs) != 1 {
		t.Fatalf("want exactly one error (outside root), got %v", errs)
	}
	if _, err := os.Stat(outFile); err != nil {
		t.Fatal("outside file must survive")
	}
	if _, err := os.Stat(inFile); err == nil {
		t.Fatal("inside file must be deleted")
	}
	if len(deleted) != 1 || deleted[0] != inFile {
		t.Fatalf("deleted list wrong: %v", deleted)
	}
}
