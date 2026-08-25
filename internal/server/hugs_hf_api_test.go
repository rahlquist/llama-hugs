package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/rahlquist/llama-hugs/internal/config"
	"github.com/rahlquist/llama-hugs/internal/hugs"
	"github.com/rahlquist/llama-hugs/internal/logmon"
	"github.com/rahlquist/llama-hugs/internal/store"
)

// hfRescanServer builds a Server (stub routers) with the given models and an
// optional stub HF metadata fetcher, pointed at an isolated cache root via env.
func hfRescanServer(t *testing.T, models map[string]config.ModelConfig, fetch hfModelFetcher) *Server {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	proxylog := logmon.NewWriter(io.Discard)
	st, err := store.New("")
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	s := &Server{
		cfg:         config.Config{Models: models},
		muxlog:      logmon.NewWriter(io.Discard),
		proxylog:    proxylog,
		upstreamlog: logmon.NewWriter(io.Discard),
		inflight:    newInflightTracker(),
		metrics:     newMetricsMonitor(proxylog, 0, 0, st),
		store:       st,
		hfFetch:     fetch,
		shutdownCtx: ctx,
		shutdownFn:  cancel,
	}
	s.routes()
	return s
}

// writeHFRepo lays out a fake HF hub cache repo with one gguf file.
func writeHFRepo(t *testing.T, root, repoID string, size int) {
	t.Helper()
	encoded := strings.ReplaceAll(repoID, "/", "--")
	p := filepath.Join(root, "models--"+encoded, "snapshots", "rev1", "model.gguf")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(p, make([]byte, size), 0o644); err != nil {
		t.Fatalf("write %s: %v", p, err)
	}
}

func postHFRescan(t *testing.T, s *Server, verifyParam bool) *httptest.ResponseRecorder {
	t.Helper()
	path := "/api/hugs/hf/rescan"
	if verifyParam {
		path += "?verify=1"
	}
	req := httptest.NewRequest(http.MethodPost, path, nil)
	rr := httptest.NewRecorder()
	s.ServeHTTP(rr, req)
	return rr
}

func decodeHFRescan(t *testing.T, rr *httptest.ResponseRecorder) hfRescanResponse {
	t.Helper()
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var out hfRescanResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v body=%s", err, rr.Body.String())
	}
	return out
}

func TestHugsHFRescanMatchedAndUnmatched(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HF_HUB_CACHE", root)
	writeHFRepo(t, root, "org/used", 100)
	writeHFRepo(t, root, "org/untracked", 200)

	models := map[string]config.ModelConfig{
		"m1": {Cmd: "llama-server -hf org/used:model.gguf --port 8080"},
	}
	s := hfRescanServer(t, models, nil)
	out := decodeHFRescan(t, postHFRescan(t, s, false))

	if !out.CacheRootExists || out.CacheRoot != root {
		t.Fatalf("cache root: %+v", out)
	}
	if out.MatchedRepos != 1 || out.UnmatchedRepos != 1 {
		t.Fatalf("counts: %+v", out)
	}
	if !reflect.DeepEqual(out.Unmatched, []string{"org/untracked"}) {
		t.Fatalf("unmatched list: %+v", out.Unmatched)
	}
	if len(out.ConfigRefs) != 1 || out.ConfigRefs[0].RepoID != "org/used" || out.ConfigRefs[0].ModelID != "m1" {
		t.Fatalf("config refs: %+v", out.ConfigRefs)
	}
	byID := map[string]hugs.HFRepoReport{}
	for _, r := range out.Repos {
		byID[r.RepoID] = r
	}
	if r := byID["org/used"]; !r.Matched || r.ConfigModel != "m1" || !strings.Contains(r.Reason, `model "m1"`) {
		t.Fatalf("org/used report: %+v", r)
	}
	if r := byID["org/untracked"]; r.Matched || r.Reason != "no config reference" {
		t.Fatalf("org/untracked report: %+v", r)
	}
	// Scan summary persisted through the existing settings pattern.
	last, err := hugs.Setting(context.Background(), s.store.DB(), "hf_rescan_last")
	if err != nil || last == "" {
		t.Fatalf("hf_rescan_last not persisted: %q err=%v", last, err)
	}
	// Without verify=1 no meta rows are created for matched models.
	rows, err := hugs.ListModelMeta(context.Background(), s.store.DB())
	if err != nil || len(rows) != 0 {
		t.Fatalf("rescan created meta rows: %+v err=%v", rows, err)
	}
}

func TestHugsHFRescanStampsExistingMetaRow(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HF_HUB_CACHE", root)
	writeHFRepo(t, root, "org/used", 100)
	models := map[string]config.ModelConfig{"m1": {Cmd: "llama-server -hf org/used"}}
	s := hfRescanServer(t, models, nil)

	// Pre-existing meta row (the normal path: GET /api/hugs/meta/{model}).
	if _, err := hugs.GetModelMeta(context.Background(), s.store.DB(), "m1"); err != nil {
		t.Fatalf("get meta: %v", err)
	}
	out := decodeHFRescan(t, postHFRescan(t, s, false))
	if !reflect.DeepEqual(out.MetaUpdated, []string{"m1"}) {
		t.Fatalf("meta_updated: %+v", out.MetaUpdated)
	}
	m, err := hugs.GetModelMeta(context.Background(), s.store.DB(), "m1")
	if err != nil || m.RegisteredAt == 0 {
		t.Fatalf("registered_at not stamped: %+v err=%v", m, err)
	}
	// Second scan must not clobber the existing timestamp.
	first := m.RegisteredAt
	decodeHFRescan(t, postHFRescan(t, s, false))
	m, err = hugs.GetModelMeta(context.Background(), s.store.DB(), "m1")
	if err != nil || m.RegisteredAt != first {
		t.Fatalf("registered_at clobbered: %+v err=%v", m, err)
	}
}

func TestHugsHFRescanMissingCacheRoot(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "nope")
	t.Setenv("HF_HUB_CACHE", missing)
	s := hfRescanServer(t, nil, nil)
	out := decodeHFRescan(t, postHFRescan(t, s, false))
	if out.CacheRootExists {
		t.Fatalf("expected cache_root_exists=false: %+v", out)
	}
	if len(out.Repos) != 0 || out.MatchedRepos != 0 || out.UnmatchedRepos != 0 || len(out.Unmatched) != 0 {
		t.Fatalf("expected empty scan: %+v", out)
	}
}

// hfMeta builds an HFModelMeta with the given pipeline tag and tags.
func hfMeta(repoID, pipeline string, tags ...string) *hugs.HFModelMeta {
	return &hugs.HFModelMeta{ID: repoID, PipelineTag: pipeline, Tags: tags}
}

func TestHugsHFRescanVerifyDerivesPerModelResults(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HF_HUB_CACHE", root)

	models := map[string]config.ModelConfig{
		"vision":  {Cmd: "llama-server -hf org/vl:model.gguf"},
		"tools":   {Cmd: "llama-server -hf org/tools"},
		"missing": {Cmd: "llama-server -hf org/gone"},
		"network": {Cmd: "llama-server -hf org/down"},
		"local":   {Cmd: "llama-server -m /home/u/model.gguf"},
	}
	stub := func(ctx context.Context, repoID string) (*hugs.HFModelMeta, int, error) {
		switch repoID {
		case "org/vl":
			return hfMeta("org/vl", "image-text-to-text", "multimodal", "vision"), 200, nil
		case "org/tools":
			return hfMeta("org/tools", "text-generation", "function calling"), 200, nil
		case "org/gone":
			return nil, 404, nil
		case "org/down":
			return nil, 0, errors.New("network down")
		default:
			t.Fatalf("unexpected repo %q", repoID)
			return nil, 0, nil
		}
	}
	s := hfRescanServer(t, models, stub)
	out := decodeHFRescan(t, postHFRescan(t, s, true))
	v := out.Verify
	if v == nil {
		t.Fatal("expected verify summary")
	}
	if v.Checked != 4 || v.Matched != 2 || v.Unmatched != 1 || v.Errors != 1 || v.NoRef != 1 {
		t.Fatalf("summary counts: %+v", v)
	}
	byModel := map[string]hugs.HFModelResult{}
	for _, m := range v.Models {
		byModel[m.ModelID] = m
	}
	if m := byModel["vision"]; m.Status != "matched" || m.RepoID != "org/vl" || !m.Capabilities.Vision || m.Capabilities.Audio || m.Capabilities.Image || m.Capabilities.Tools || m.Capabilities.MTP {
		t.Fatalf("vision result: %+v", m)
	}
	if m := byModel["tools"]; m.Status != "matched" || !m.Capabilities.Tools || m.Capabilities.Vision {
		t.Fatalf("tools result: %+v", m)
	}
	if m := byModel["missing"]; m.Status != "unmatched" || m.HTTPStatus != 404 || m.Reason == "" {
		t.Fatalf("missing result must be exact unmatched: %+v", m)
	}
	if m := byModel["network"]; m.Status != "error" || m.Error == "" {
		t.Fatalf("network result: %+v", m)
	}
	if m := byModel["local"]; m.Status != "no_ref" || m.RepoID != "" {
		t.Fatalf("local model must be no_ref: %+v", m)
	}
	// Findings persisted as hf:* tags on the queried models' meta rows.
	meta, err := hugs.GetModelMeta(context.Background(), s.store.DB(), "vision")
	if err != nil || !strings.Contains(meta.Tags, "hf:checked") || !strings.Contains(meta.Tags, "hf:vision") {
		t.Fatalf("vision meta tags: %+v err=%v", meta, err)
	}
	meta, err = hugs.GetModelMeta(context.Background(), s.store.DB(), "missing")
	if err != nil || !strings.Contains(meta.Tags, "hf:unmatched") {
		t.Fatalf("missing meta tags: %+v err=%v", meta, err)
	}
	// Local-only models get no meta row at all: verify must not fabricate one.
	rows, err := hugs.ListModelMeta(context.Background(), s.store.DB())
	if err != nil {
		t.Fatalf("list meta: %v", err)
	}
	for _, r := range rows {
		if r.ModelID == "local" {
			t.Fatal("no_ref model must not get a meta row from the rescan")
		}
	}
	if !reflect.DeepEqual(out.TagsUpdated, []string{"missing", "network", "tools", "vision"}) {
		t.Fatalf("tags_updated: %+v", out.TagsUpdated)
	}
}

func TestHugsHFRescanVerifyPreservesUserTags(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HF_HUB_CACHE", root)
	models := map[string]config.ModelConfig{"m1": {Cmd: "llama-server -hf org/vl"}}
	stub := func(ctx context.Context, repoID string) (*hugs.HFModelMeta, int, error) {
		return hfMeta("org/vl", "image-text-to-text", "multimodal"), 200, nil
	}
	s := hfRescanServer(t, models, stub)

	// Pre-seed a row with user tags via the normal meta path.
	if _, err := hugs.UpdateModelMeta(context.Background(), s.store.DB(),
		hugs.ModelMeta{ModelID: "m1", Tags: "favorite,big", Notes: "my notes"}); err != nil {
		t.Fatalf("seed meta: %v", err)
	}
	decodeHFRescan(t, postHFRescan(t, s, true))

	meta, err := hugs.GetModelMeta(context.Background(), s.store.DB(), "m1")
	if err != nil {
		t.Fatalf("get meta: %v", err)
	}
	if !strings.Contains(meta.Tags, "favorite") || !strings.Contains(meta.Tags, "big") {
		t.Fatalf("user tags lost: %q", meta.Tags)
	}
	if !strings.Contains(meta.Tags, "hf:vision") || !strings.Contains(meta.Tags, "hf:checked") {
		t.Fatalf("hf tags missing: %q", meta.Tags)
	}
	if meta.Notes != "my notes" {
		t.Fatalf("notes clobbered: %q", meta.Notes)
	}

	// A second scan must not duplicate or leave stale hf:* tags behind.
	decodeHFRescan(t, postHFRescan(t, s, true))
	meta, err = hugs.GetModelMeta(context.Background(), s.store.DB(), "m1")
	if err != nil {
		t.Fatalf("get meta 2: %v", err)
	}
	if strings.Count(meta.Tags, "hf:vision") != 1 || strings.Count(meta.Tags, "favorite") != 1 {
		t.Fatalf("tags duplicated after rescan: %q", meta.Tags)
	}
}

func TestHugsHFRescanVerifyUnauthorizedIsExplicit(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HF_HUB_CACHE", root)
	models := map[string]config.ModelConfig{"gated": {Cmd: "llama-server -hf meta/gated"}}
	stub := func(ctx context.Context, repoID string) (*hugs.HFModelMeta, int, error) {
		return nil, 401, nil
	}
	s := hfRescanServer(t, models, stub)
	out := decodeHFRescan(t, postHFRescan(t, s, true))
	v := out.Verify
	if v == nil || v.Unauthorized != 1 || v.Matched != 0 || v.Unmatched != 0 {
		t.Fatalf("summary: %+v", v)
	}
	if m := v.Models[0]; m.Status != "unauthorized" || m.HTTPStatus != 401 || m.Reason == "" {
		t.Fatalf("gated result: %+v", m)
	}
	meta, err := hugs.GetModelMeta(context.Background(), s.store.DB(), "gated")
	if err != nil || !strings.Contains(meta.Tags, "hf:unauthorized") {
		t.Fatalf("gated meta tags: %+v err=%v", meta, err)
	}
}

func TestHugsHFRescanVerifyWithoutParamSkipsNetwork(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HF_HUB_CACHE", root)
	writeHFRepo(t, root, "org/a", 10)
	called := false
	s := hfRescanServer(t, nil, func(ctx context.Context, repoID string) (*hugs.HFModelMeta, int, error) {
		called = true
		return hfMeta(repoID, "text-generation"), 200, nil
	})
	out := decodeHFRescan(t, postHFRescan(t, s, false))
	if called {
		t.Fatal("fetcher must not run without verify=1")
	}
	if out.Verify != nil {
		t.Fatalf("verify summary present without param: %+v", out.Verify)
	}
}

func TestHugsHFRescanVerifyDedupesSharedRepo(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HF_HUB_CACHE", root)
	models := map[string]config.ModelConfig{
		"a1": {Cmd: "llama-server -hf shared/repo"},
		"a2": {Cmd: "llama-server -hf shared/repo"},
	}
	calls := 0
	stub := func(ctx context.Context, repoID string) (*hugs.HFModelMeta, int, error) {
		calls++
		return hfMeta("shared/repo", "text-generation"), 200, nil
	}
	s := hfRescanServer(t, models, stub)
	out := decodeHFRescan(t, postHFRescan(t, s, true))
	if calls != 1 {
		t.Fatalf("shared repo fetched %d times, want 1", calls)
	}
	if out.Verify.Matched != 2 || out.Verify.Checked != 2 {
		t.Fatalf("summary: %+v", out.Verify)
	}
}

func TestHugsHFRescanNeverMutatesConfig(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HF_HUB_CACHE", root)
	writeHFRepo(t, root, "org/a", 10)
	models := map[string]config.ModelConfig{"m1": {Cmd: "llama-server -hf org/a"}}
	s := hfRescanServer(t, models, nil)
	before := s.cfg.Models["m1"]
	decodeHFRescan(t, postHFRescan(t, s, false))
	if !reflect.DeepEqual(before, s.cfg.Models["m1"]) {
		t.Fatalf("rescan mutated config: before=%+v after=%+v", before, s.cfg.Models["m1"])
	}
}

func TestHFModelURLKeepsSlashSeparator(t *testing.T) {
	u := hfModelURL("Qwen/Qwen2.5-VL-7B-Instruct")
	if !strings.Contains(u, "/api/models/Qwen/Qwen2.5-VL-7B-Instruct") {
		t.Fatalf("url: %s", u)
	}
	if strings.Contains(u, "%2F") {
		t.Fatalf("slash separator escaped: %s", u)
	}
	// Odd-but-safe repo ids still get their segments escaped.
	u2 := hfModelURL("we ird/re po")
	if !strings.Contains(u2, "we%20ird/re%20po") || strings.Contains(u2, "%2F") {
		t.Fatalf("segment escaping: %s", u2)
	}
}

func TestHugsHFRescanRouteIsPostOnly(t *testing.T) {
	s := hfRescanServer(t, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/hugs/hf/rescan", nil)
	rr := httptest.NewRecorder()
	s.ServeHTTP(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET on POST-only route: status=%d", rr.Code)
	}
}
