package hugs

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// File deletion flags: filenames marked by an operator in the UI as pending
// deletion. Flags persist in the store so a browser refresh doesn't lose them;
// actual deletion is a separate explicit action.

const fileFlagsDDL = `
CREATE TABLE IF NOT EXISTS hugs_file_flags (
    filename   TEXT PRIMARY KEY,
    flagged_at INTEGER NOT NULL
);
`

// ensureFileFlags creates the flags table if missing (idempotent).
func ensureFileFlags(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, fileFlagsDDL); err != nil {
		return fmt.Errorf("hugs: ensure file flags schema: %w", err)
	}
	return nil
}

// SetFileFlags flags (or unflags) the named files. Names must be bare
// filenames — no path separators.
func SetFileFlags(ctx context.Context, db *sql.DB, names []string, flagged bool) error {
	if err := ensureFileFlags(ctx, db); err != nil {
		return err
	}
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" || strings.ContainsRune(name, '/') || strings.Contains(name, string([]rune{0})) {
			return fmt.Errorf("hugs: invalid filename %q", name)
		}
		var err error
		if flagged {
			_, err = db.ExecContext(ctx,
				`INSERT INTO hugs_file_flags (filename, flagged_at) VALUES (?, ?)
				 ON CONFLICT(filename) DO UPDATE SET flagged_at = excluded.flagged_at`,
				name, time.Now().Unix())
		} else {
			_, err = db.ExecContext(ctx, `DELETE FROM hugs_file_flags WHERE filename = ?`, name)
		}
		if err != nil {
			return fmt.Errorf("hugs: set file flag %q: %w", name, err)
		}
	}
	return nil
}

// ListFileFlags returns all currently flagged filenames.
func ListFileFlags(ctx context.Context, db *sql.DB) ([]string, error) {
	if err := ensureFileFlags(ctx, db); err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(ctx, `SELECT filename FROM hugs_file_flags ORDER BY filename`)
	if err != nil {
		return nil, fmt.Errorf("hugs: list file flags: %w", err)
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, fmt.Errorf("hugs: scan file flag: %w", err)
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// ClearFileFlags removes flag rows for the named files (used after deletion).
func ClearFileFlags(ctx context.Context, db *sql.DB, names []string) error {
	return SetFileFlags(ctx, db, names, false)
}

// DeleteFiles removes the given absolute paths from disk. Every path must be
// a regular file directly inside root (no subdirectories, no traversal) and
// must end in .gguf — belt and braces for an irreversible operation. Paths
// outside root are refused and reported per-file. The db handle is optional;
// when provided, corresponding flags are cleared for successfully deleted
// files.
func DeleteFiles(ctx context.Context, db *sql.DB, root string, paths []string) (deleted []string, errs []error) {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return nil, []error{fmt.Errorf("hugs: resolve root: %w", err)}
	}
	for _, p := range paths {
		p = strings.TrimSpace(p)
		abs, err := filepath.Abs(p)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: resolve: %w", p, err))
			continue
		}
		if !strings.HasPrefix(abs, rootAbs+string(filepath.Separator)) {
			errs = append(errs, fmt.Errorf("%s: outside allowed root", p))
			continue
		}
		if strings.ToLower(filepath.Ext(abs)) != ".gguf" {
			errs = append(errs, fmt.Errorf("%s: not a .gguf file", p))
			continue
		}
		info, err := os.Lstat(abs)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: stat: %w", p, err))
			continue
		}
		if !info.Mode().IsRegular() {
			errs = append(errs, fmt.Errorf("%s: not a regular file", p))
			continue
		}
		select {
		case <-ctx.Done():
			errs = append(errs, ctx.Err())
			return deleted, errs
		default:
		}
		if err := os.Remove(abs); err != nil {
			errs = append(errs, fmt.Errorf("%s: remove: %w", p, err))
			continue
		}
		deleted = append(deleted, abs)
	}
	if db != nil && len(deleted) > 0 {
		names := make([]string, len(deleted))
		for i, d := range deleted {
			names[i] = filepath.Base(d)
		}
		_ = ClearFileFlags(ctx, db, names)
	}
	return deleted, errs
}
