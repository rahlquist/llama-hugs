package hugs

import (
	"context"
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

func benchDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestBenchInsertAndLeaderboard(t *testing.T) {
	db := benchDB(t)
	ctx := context.Background()

	if _, err := db.ExecContext(ctx, ensureBenchDDL); err != nil {
		t.Fatalf("ddl: %v", err)
	}

	recs := []BenchRecord{
		{Model: "m1", Task: "pp", TokensPerS: 100, RunAtUnix: 100},
		{Model: "m1", Task: "pp", TokensPerS: 150, RunAtUnix: 200}, // best
		{Model: "m1", Task: "tg", TokensPerS: 40, RunAtUnix: 200},
		{Model: "m2", Task: "pp", TokensPerS: 120, RunAtUnix: 300},
	}
	for _, r := range recs {
		if err := InsertBenchRecord(ctx, db, r); err != nil {
			t.Fatalf("insert %+v: %v", r, err)
		}
	}

	rows, err := Leaderboard(ctx, db)
	if err != nil {
		t.Fatalf("leaderboard: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("want 3 rows, got %d", len(rows))
	}
	// Sorted desc by best tps.
	if rows[0].Model != "m1" || rows[0].Task != "pp" || rows[0].BestTPS != 150 || rows[0].RunCount != 2 {
		t.Fatalf("row0 wrong: %+v", rows[0])
	}
	if rows[1].BestTPS != 120 || rows[2].BestTPS != 40 {
		t.Fatalf("ordering wrong: %+v", rows)
	}

	trend, err := TrendRows(ctx, db, "m1", "pp")
	if err != nil || len(trend) != 2 {
		t.Fatalf("trend: %v (%d)", err, len(trend))
	}
	if trend[0].TokensPerS > trend[1].TokensPerS && trend[0].RunAtUnix > trend[1].RunAtUnix {
		t.Fatal("trend not time-ordered")
	}
}
