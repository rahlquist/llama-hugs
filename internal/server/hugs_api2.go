package server

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mostlygeek/llama-swap/internal/hugs"
)

// hugsPricingEntry is one model's pricing facts from the configured source.
type hugsPricingEntry struct {
	InputPerMTok   float64 `json:"input_per_mtok"`
	OutputPerMTok  float64 `json:"output_per_mtok"`
	Currency       string  `json:"currency,omitempty"`
	Source         string  `json:"source"`
	FetchedAtUnix  int64   `json:"fetched_at_unix"`
}

// handleHugsPricing GET /api/hugs/pricing
// Serves enrichment data from the file named by the pricing_source setting
// (settable via /api/hugs/settings). No network fetch happens in-process;
// the operator (or a future scheduled job) refreshes the file. Missing or
// unreadable source yields an empty list, never an error — the dashboard
// treats pricing as optional decoration.
func (s *Server) handleHugsPricing(w http.ResponseWriter, r *http.Request) {
	src, err := hugs.Setting(r.Context(), s.store.DB(), "pricing_source")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	out := map[string]any{"entries": map[string]hugsPricingEntry{}, "source": src}
	if src == "" {
		writeJSON(w, out)
		return
	}
	data, err := os.ReadFile(src)
	if err != nil {
		out["warning"] = "pricing source unreadable: " + err.Error()
		writeJSON(w, out)
		return
	}
	var entries map[string]hugsPricingEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		out["warning"] = "pricing source unparsable: " + err.Error()
		writeJSON(w, out)
		return
	}
	out["entries"] = entries
	writeJSON(w, out)
}

// hugsSettingsPayload is the GET/POST body for /api/hugs/settings.
type hugsSettingsPayload struct {
	PricingSource string `json:"pricing_source"`
}

func (s *Server) handleHugsSettingsGet(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	payload := hugsSettingsPayload{}
	var err error
	if payload.PricingSource, err = hugs.Setting(ctx, s.store.DB(), "pricing_source"); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, payload)
}

func (s *Server) handleHugsSettingsSet(w http.ResponseWriter, r *http.Request) {
	var in hugsSettingsPayload
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		http.Error(w, "bad json: "+err.Error(), http.StatusBadRequest)
		return
	}
	ctx := r.Context()
	if err := hugs.SetSetting(ctx, s.store.DB(), "pricing_source", strings.TrimSpace(in.PricingSource)); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.handleHugsSettingsGet(w, r)
}

// handleHugsBenchIngest POST /api/hugs/bench/ingest
// Body: {"source_dir": "/path"} — reads *.json result files produced by the
// nightly sweep into the hugs_bench table. Read-only against the files; the
// only write target is the Llama Hugs store.
func (s *Server) handleHugsBenchIngest(w http.ResponseWriter, r *http.Request) {
	var in struct {
		SourceDir  string `json:"source_dir"`
		SourceFile string `json:"source_file"` // optional: single .jsonl of records
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil ||
		(strings.TrimSpace(in.SourceDir) == "" && strings.TrimSpace(in.SourceFile) == "") {
		http.Error(w, `body must be {"source_dir": "/path"} or {"source_file": "/path.jsonl"}`, http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(in.SourceFile) != "" {
		s.hugsIngestJSONL(w, r, in.SourceFile)
		return
	}
	dir := in.SourceDir
	home, _ := os.UserHomeDir()
	if !strings.HasPrefix(dir, home) && !strings.HasPrefix(dir, "/opt/llama-hugs") {
		http.Error(w, "source_dir outside allowed roots", http.StatusForbidden)
		return
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		http.Error(w, "read dir: "+err.Error(), http.StatusBadRequest)
		return
	}
	inserted, skipped := 0, 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			skipped++
			continue
		}
		var rec hugs.BenchRecord
		if err := json.Unmarshal(data, &rec); err != nil || rec.Model == "" {
			skipped++
			continue
		}
		rec.SourceFile = filepath.Join(dir, e.Name())
		if rec.RunAtUnix == 0 {
			if info, ierr := e.Info(); ierr == nil {
				rec.RunAtUnix = info.ModTime().Unix()
			} else {
				rec.RunAtUnix = time.Now().Unix()
			}
		}
		if err := hugs.InsertBenchRecord(r.Context(), s.store.DB(), rec); err != nil {
			http.Error(w, "insert: "+err.Error(), http.StatusInternalServerError)
			return
		}
		inserted++
	}
	writeJSON(w, map[string]any{"inserted": inserted, "skipped": skipped})
}

// hugsIngestJSONL ingests a line-delimited JSON file of BenchRecords.
// Same root-safety rules as source_dir ingestion.
func (s *Server) hugsIngestJSONL(w http.ResponseWriter, r *http.Request, path string) {
	home, _ := os.UserHomeDir()
	if !strings.HasPrefix(path, home) && !strings.HasPrefix(path, "/opt/llama-hugs") {
		http.Error(w, "source_file outside allowed roots", http.StatusForbidden)
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		http.Error(w, "read file: "+err.Error(), http.StatusBadRequest)
		return
	}
	inserted, skipped := 0, 0
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var rec hugs.BenchRecord
		if err := json.Unmarshal([]byte(line), &rec); err != nil || rec.Model == "" {
			skipped++
			continue
		}
		rec.SourceFile = path
		if err := hugs.InsertBenchRecord(r.Context(), s.store.DB(), rec); err != nil {
			http.Error(w, "insert: "+err.Error(), http.StatusInternalServerError)
			return
		}
		inserted++
	}
	writeJSON(w, map[string]any{"inserted": inserted, "skipped": skipped})
}

// handleHugsBenchLeaderboard GET /api/hugs/bench/leaderboard
// Flat comparison: best tokens/s per model×task, ordered descending.
func (s *Server) handleHugsBenchLeaderboard(w http.ResponseWriter, r *http.Request) {
	rows, err := hugs.Leaderboard(r.Context(), s.store.DB())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{"leaderboard": rows})
}
