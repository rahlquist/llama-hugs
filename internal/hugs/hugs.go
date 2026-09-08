// Package hugs implements Llama Hugs platform extensions on top of the
// llama-swap fork: per-model metadata (tags/notes/lifecycle), disk-usage
// reporting, and enrichment hooks.
//
// Divergence policy: everything lives in this package; the only upstream-file
// edits are route registrations and a store handle passthrough in server.go.
package hugs

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// ModelMeta is one row of per-model presentation metadata.
type ModelMeta struct {
	ModelID       string `json:"model_id"`
	Tags          string `json:"tags"`
	Notes         string `json:"notes"`
	FirstSeen     int64  `json:"first_seen"`
	LastLoadedAt  int64  `json:"last_loaded_at"`
	LastRequestAt int64  `json:"last_request_at"`
	RegisteredAt  int64  `json:"registered_at"`
}

// GetModelMeta returns metadata for a model, creating a default row if absent.
func GetModelMeta(ctx context.Context, db *sql.DB, modelID string) (ModelMeta, error) {
	m := ModelMeta{ModelID: modelID}
	err := db.QueryRowContext(ctx,
		`SELECT tags, notes, first_seen, last_loaded_at, last_request_at, registered_at
		 FROM hugs_model_meta WHERE model_id = ?`, modelID,
	).Scan(&m.Tags, &m.Notes, &m.FirstSeen, &m.LastLoadedAt, &m.LastRequestAt, &m.RegisteredAt)
	if err == sql.ErrNoRows {
		_, ierr := db.ExecContext(ctx,
			`INSERT OR IGNORE INTO hugs_model_meta (model_id, first_seen) VALUES (?, ?)`,
			modelID, time.Now().Unix())
		if ierr != nil {
			return m, fmt.Errorf("hugs: insert default meta: %w", ierr)
		}
		m.FirstSeen = time.Now().Unix()
		return m, nil
	}
	return m, err
}

// UpdateModelMeta merges non-empty fields into the stored row.
func UpdateModelMeta(ctx context.Context, db *sql.DB, in ModelMeta) (ModelMeta, error) {
	// Ensure row exists first.
	if _, err := GetModelMeta(ctx, db, in.ModelID); err != nil {
		return ModelMeta{}, err
	}
	_, err := db.ExecContext(ctx, `
		UPDATE hugs_model_meta SET
			tags       = CASE WHEN ? <> '' THEN ? ELSE tags END,
			notes      = CASE WHEN ? <> '' THEN ? ELSE notes END,
			first_seen = CASE WHEN ? > 0   THEN ? ELSE first_seen END,
			last_loaded_at  = CASE WHEN ? > 0 THEN ? ELSE last_loaded_at END,
			last_request_at = CASE WHEN ? > 0 THEN ? ELSE last_request_at END,
			registered_at   = CASE WHEN ? > 0 THEN ? ELSE registered_at END
		WHERE model_id = ?`,
		in.Tags, in.Tags,
		in.Notes, in.Notes,
		in.FirstSeen, in.FirstSeen,
		in.LastLoadedAt, in.LastLoadedAt,
		in.LastRequestAt, in.LastRequestAt,
		in.RegisteredAt, in.RegisteredAt,
		in.ModelID,
	)
	if err != nil {
		return ModelMeta{}, fmt.Errorf("hugs: update meta: %w", err)
	}
	return GetModelMeta(ctx, db, in.ModelID)
}

// ListModelMeta returns all rows with metadata.
func ListModelMeta(ctx context.Context, db *sql.DB) ([]ModelMeta, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT model_id, tags, notes, first_seen, last_loaded_at, last_request_at, registered_at
		 FROM hugs_model_meta ORDER BY model_id`)
	if err != nil {
		return nil, fmt.Errorf("hugs: list meta: %w", err)
	}
	defer rows.Close()
	out := []ModelMeta{}
	for rows.Next() {
		var m ModelMeta
		if err := rows.Scan(&m.ModelID, &m.Tags, &m.Notes, &m.FirstSeen, &m.LastLoadedAt, &m.LastRequestAt, &m.RegisteredAt); err != nil {
			return nil, fmt.Errorf("hugs: scan meta: %w", err)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// FileEntry is one file reported by ScanDir.
type FileEntry struct {
	Path      string `json:"path"`
	Name      string `json:"name"`
	SizeBytes int64  `json:"size_bytes"`
	MtimeUnix int64  `json:"mtime_unix"`
}

// DirReport summarizes a scanned directory of model files.
type DirReport struct {
	Root        string      `json:"root"`
	TotalBytes  int64       `json:"total_bytes"`
	FileCount   int         `json:"file_count"`
	Files       []FileEntry `json:"files,omitempty"` // populated only when includeFiles
	ScannedAtMs int64       `json:"scanned_at_ms"`
}

// ScanDir walks root for *.gguf files (non-recursive beyond root itself —
// upstream keeps a flat cache dir). Read-only.
func ScanDir(root string, includeFiles bool) (*DirReport, error) {
	rep := &DirReport{Root: root, ScannedAtMs: time.Now().UnixMilli()}
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("hugs: scan dir: %w", err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(strings.ToLower(e.Name()), ".gguf") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue // raced deletion — skip, never fail the whole scan
		}
		fe := FileEntry{
			Path:      filepath.Join(root, e.Name()),
			Name:      e.Name(),
			SizeBytes: info.Size(),
			MtimeUnix: info.ModTime().Unix(),
		}
		rep.TotalBytes += fe.SizeBytes
		rep.FileCount++
		if includeFiles {
			rep.Files = append(rep.Files, fe)
		}
	}
	sort.Slice(rep.Files, func(i, j int) bool { return rep.Files[i].SizeBytes > rep.Files[j].SizeBytes })
	return rep, nil
}

// Setting reads one settings value; empty string when unset.
func Setting(ctx context.Context, db *sql.DB, key string) (string, error) {
	var v string
	err := db.QueryRowContext(ctx, `SELECT value FROM hugs_settings WHERE key = ?`, key).Scan(&v)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return v, err
}

// SetSetting upserts one settings value.
func SetSetting(ctx context.Context, db *sql.DB, key, value string) error {
	_, err := db.ExecContext(ctx,
		`INSERT INTO hugs_settings (key, value) VALUES (?, ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value)
	if err != nil {
		return fmt.Errorf("hugs: set setting: %w", err)
	}
	return nil
}
