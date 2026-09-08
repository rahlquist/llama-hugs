package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rahlquist/llama-hugs/internal/config"
	"github.com/rahlquist/llama-hugs/internal/hugs"
	"github.com/rahlquist/llama-hugs/internal/logmon"
	"github.com/rahlquist/llama-hugs/internal/store"
)

// canonicalAPIServer builds a Server (stub routers) over an in-memory store.
// store.New("") runs every goose migration, including 00003_hugs_canonical.
func canonicalAPIServer(t *testing.T) *Server {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	proxylog := logmon.NewWriter(io.Discard)
	st, err := store.New("")
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	s := &Server{
		cfg:         config.Config{},
		muxlog:      logmon.NewWriter(io.Discard),
		proxylog:    proxylog,
		upstreamlog: logmon.NewWriter(io.Discard),
		inflight:    newInflightTracker(),
		metrics:     newMetricsMonitor(proxylog, 0, 0, st),
		store:       st,
		shutdownCtx: ctx,
		shutdownFn:  cancel,
	}
	s.routes()
	return s
}

func doJSON(t *testing.T, s *Server, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var rdr io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		rdr = bytes.NewReader(data)
	}
	req := httptest.NewRequest(method, path, rdr)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	rr := httptest.NewRecorder()
	s.ServeHTTP(rr, req)
	return rr
}

func TestCanonicalModelsAPI(t *testing.T) {
	s := canonicalAPIServer(t)

	// Empty list before anything is registered.
	rr := doJSON(t, s, http.MethodGet, "/api/hugs/models", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", rr.Code, rr.Body.String())
	}
	var models []hugs.Model
	if err := json.Unmarshal(rr.Body.Bytes(), &models); err != nil || len(models) != 0 {
		t.Fatalf("empty list: %v (%d)", err, len(models))
	}

	// Upsert a canonical model.
	rr = doJSON(t, s, http.MethodPost, "/api/hugs/models/hugs-qwen3-27b", map[string]any{
		"display_name": "Qwen3 27B", "source_repo": "Qwen/Qwen3-27B",
		"gpu_backend": "ROCm", "context_size": 64000, "status": "pinned",
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("upsert status=%d body=%s", rr.Code, rr.Body.String())
	}
	var created hugs.Model
	if err := json.Unmarshal(rr.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode upsert: %v", err)
	}
	if created.ModelID != "hugs-qwen3-27b" || created.Status != "pinned" || created.CreatedAt == 0 {
		t.Fatalf("upsert result wrong: %+v", created)
	}

	// Get by id.
	rr = doJSON(t, s, http.MethodGet, "/api/hugs/models/hugs-qwen3-27b", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("get status=%d body=%s", rr.Code, rr.Body.String())
	}
	var got hugs.Model
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil || got.DisplayName != "Qwen3 27B" {
		t.Fatalf("get decode: %v %+v", err, got)
	}

	// Missing model → 404.
	rr = doJSON(t, s, http.MethodGet, "/api/hugs/models/nope", nil)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("missing get status=%d want 404", rr.Code)
	}

	// List now returns one.
	rr = doJSON(t, s, http.MethodGet, "/api/hugs/models", nil)
	if err := json.Unmarshal(rr.Body.Bytes(), &models); err != nil || len(models) != 1 || models[0].ModelID != "hugs-qwen3-27b" {
		t.Fatalf("list after upsert: %v (%+v)", err, models)
	}
}

func TestCanonicalAssetsAPISmokeAPI(t *testing.T) {
	s := canonicalAPIServer(t)

	// Assets for an unregistered model → 404.
	rr := doJSON(t, s, http.MethodPost, "/api/hugs/models/ghost/assets", map[string]any{
		"asset_type": "mmproj", "asset_name": "x.gguf",
	})
	if rr.Code != http.StatusNotFound {
		t.Fatalf("ghost asset status=%d want 404 body=%s", rr.Code, rr.Body.String())
	}

	// Register the model, then upsert an asset.
	if rr = doJSON(t, s, http.MethodPost, "/api/hugs/models/hugs-qwen3-vl", map[string]any{
		"display_name": "Qwen3 VL", "source_repo": "Qwen/Qwen3-VL",
	}); rr.Code != http.StatusOK {
		t.Fatalf("register status=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = doJSON(t, s, http.MethodPost, "/api/hugs/models/hugs-qwen3-vl/assets", map[string]any{
		"asset_type": "mmproj", "asset_name": "qwen3-vl-mmproj.gguf",
		"local_path":      "/home/u/.cache/llama.cpp/qwen3-vl-mmproj.gguf",
		"disk_size_bytes": 987654321, "load_target": "vram", "measurement_source": "observed",
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("asset upsert status=%d body=%s", rr.Code, rr.Body.String())
	}
	var asset hugs.Asset
	if err := json.Unmarshal(rr.Body.Bytes(), &asset); err != nil {
		t.Fatalf("decode asset: %v", err)
	}
	if asset.ModelID != "hugs-qwen3-vl" || asset.AssetType != "mmproj" || asset.DiskSizeBytes != 987654321 {
		t.Fatalf("asset result wrong: %+v", asset)
	}

	// List assets.
	rr = doJSON(t, s, http.MethodGet, "/api/hugs/models/hugs-qwen3-vl/assets", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("assets list status=%d", rr.Code)
	}
	var assets []hugs.Asset
	if err := json.Unmarshal(rr.Body.Bytes(), &assets); err != nil || len(assets) != 1 {
		t.Fatalf("assets list decode: %v (%d)", err, len(assets))
	}

	// Smoke test for an unregistered model → 404.
	rr = doJSON(t, s, http.MethodPost, "/api/hugs/models/ghost/smoke", map[string]any{"success": 1})
	if rr.Code != http.StatusNotFound {
		t.Fatalf("ghost smoke status=%d want 404 body=%s", rr.Code, rr.Body.String())
	}

	// Insert a smoke test for the registered model.
	rr = doJSON(t, s, http.MethodPost, "/api/hugs/models/hugs-qwen3-vl/smoke", map[string]any{
		"gpu_backend": "ROCm0", "context_size": 64000,
		"gpu_vram_before_bytes": 2000000000, "gpu_vram_peak_bytes": 6100000000,
		"success": 1, "notes": "ok",
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("smoke insert status=%d body=%s", rr.Code, rr.Body.String())
	}
	var st hugs.SmokeTest
	if err := json.Unmarshal(rr.Body.Bytes(), &st); err != nil {
		t.Fatalf("decode smoke: %v", err)
	}
	if st.ModelID != "hugs-qwen3-vl" || st.Success != 1 || st.ID == 0 {
		t.Fatalf("smoke result wrong: %+v", st)
	}

	// List smoke history.
	rr = doJSON(t, s, http.MethodGet, "/api/hugs/models/hugs-qwen3-vl/smoke", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("smoke list status=%d", rr.Code)
	}
	var tests []hugs.SmokeTest
	if err := json.Unmarshal(rr.Body.Bytes(), &tests); err != nil || len(tests) != 1 {
		t.Fatalf("smoke list decode: %v (%d)", err, len(tests))
	}

	// Bad JSON → 400.
	req := httptest.NewRequest(http.MethodPost, "/api/hugs/models/hugs-qwen3-vl/smoke", bytes.NewBufferString("{not json"))
	rr = httptest.NewRecorder()
	s.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("bad json status=%d want 400", rr.Code)
	}
}
