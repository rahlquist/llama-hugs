package hugs

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"time"
)

// BenchRecord is one benchmark datapoint (model × task × run).
type BenchRecord struct {
	ID         int64   `json:"id"`
	Model      string  `json:"model"`
	Task       string  `json:"task"`
	TokensPerS float64 `json:"tokens_per_s"`
	RunAtUnix  int64   `json:"run_at_unix"`
	Notes      string  `json:"notes,omitempty"`
	SourceFile string  `json:"source_file,omitempty"`
}

// EnsureBenchSchema creates the bench table if missing (additive, idempotent —
// mirrors the goose-migration DDL for environments where tests open a raw db).
const ensureBenchDDL = `
CREATE TABLE IF NOT EXISTS hugs_bench (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    model TEXT NOT NULL,
    task TEXT NOT NULL DEFAULT '',
    tokens_per_s REAL NOT NULL DEFAULT 0,
    run_at INTEGER NOT NULL DEFAULT 0,
    notes TEXT NOT NULL DEFAULT '',
    source_file TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_hugs_bench_model_task
    ON hugs_bench (model, task, tokens_per_s DESC);
`

// InsertBenchRecord stores one datapoint.
func InsertBenchRecord(ctx context.Context, db *sql.DB, r BenchRecord) error {
	// The bench table is created additively on first use (idempotent), so no
	// goose migration ordering issues arise for existing stores.
	if _, err := db.ExecContext(ctx, ensureBenchDDL); err != nil {
		return fmt.Errorf("hugs: ensure bench schema: %w", err)
	}
	if r.RunAtUnix == 0 {
		r.RunAtUnix = time.Now().Unix()
	}
	_, err := db.ExecContext(ctx, `
		INSERT INTO hugs_bench (model, task, tokens_per_s, run_at, notes, source_file)
		VALUES (?, ?, ?, ?, ?, ?)`,
		r.Model, r.Task, r.TokensPerS, r.RunAtUnix, r.Notes, r.SourceFile)
	if err != nil {
		return fmt.Errorf("hugs: insert bench: %w", err)
	}
	return nil
}

// LeaderboardRow is one line of the flat comparison: best tokens/s seen for
// a model×task pair plus when it was measured.
type LeaderboardRow struct {
	Model              string  `json:"model"`
	Task               string  `json:"task"`
	BestTPS            float64 `json:"best_tokens_per_s"`
	RunAtUnix          int64   `json:"run_at_unix"`
	RunCount           int     `json:"run_count"`
	MaxMemoryFootprint int64   `json:"max_memory_footprint"`
}

// Leaderboard returns best-per-model×task rows sorted by BestTPS desc.
func Leaderboard(ctx context.Context, db *sql.DB) ([]LeaderboardRow, error) {
	// Smoke history is optional in older stores; create the minimal table so
	// benchmark queries remain backwards-compatible.
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS hugs_models (model_id TEXT PRIMARY KEY); CREATE TABLE IF NOT EXISTS hugs_smoke_tests (model_id TEXT NOT NULL, gpu_vram_peak_bytes INTEGER NOT NULL DEFAULT 0, gpu_vram_before_bytes INTEGER NOT NULL DEFAULT 0)`); err != nil {
		return nil, fmt.Errorf("hugs: ensure smoke schema: %w", err)
	}
	rows, err := db.QueryContext(ctx, `
		SELECT b.model, b.task, MAX(b.tokens_per_s), MAX(b.run_at), COUNT(*),
			COALESCE((SELECT MAX(s.gpu_vram_peak_bytes - COALESCE(s.gpu_vram_before_bytes, 0)) FROM hugs_smoke_tests s
				JOIN hugs_models m ON m.model_id = s.model_id
				WHERE m.model_id = 'hugs-' || b.model OR m.model_id = b.model), 0)
		FROM hugs_bench b
		GROUP BY b.model, b.task`)
	if err != nil {
		return nil, fmt.Errorf("hugs: leaderboard: %w", err)
	}
	defer rows.Close()
	out := []LeaderboardRow{}
	for rows.Next() {
		var row LeaderboardRow
		var task string
		var best sql.NullFloat64
		var runAt sql.NullInt64
		if err := rows.Scan(&row.Model, &task, &best, &runAt, &row.RunCount, &row.MaxMemoryFootprint); err != nil {
			return nil, fmt.Errorf("hugs: leaderboard scan: %w", err)
		}
		row.Task = task
		row.BestTPS = best.Float64
		row.RunAtUnix = runAt.Int64
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool { return out[i].BestTPS > out[j].BestTPS })
	return out, nil
}

// TrendRows returns all records for one model×task ordered by time, for the
// UI's regression view.
func TrendRows(ctx context.Context, db *sql.DB, model, task string) ([]BenchRecord, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, model, task, tokens_per_s, run_at, notes, source_file
		FROM hugs_bench WHERE model = ? AND task = ?
		ORDER BY run_at ASC`, model, task)
	if err != nil {
		return nil, fmt.Errorf("hugs: trend: %w", err)
	}
	defer rows.Close()
	out := []BenchRecord{}
	for rows.Next() {
		var r BenchRecord
		if err := rows.Scan(&r.ID, &r.Model, &r.Task, &r.TokensPerS, &r.RunAtUnix, &r.Notes, &r.SourceFile); err != nil {
			return nil, fmt.Errorf("hugs: trend scan: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
