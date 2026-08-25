package hugs

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/rahlquist/llama-hugs/internal/config"
)

func TestExtractHFRefs(t *testing.T) {
	cases := []struct {
		name string
		cmd  string
		want []HFConfigRef
	}{
		{
			name: "llama-cpp -hf with file",
			cmd:  "llama-server -hf org/repo:model.gguf --port 8080",
			want: []HFConfigRef{{RepoID: "org/repo", File: "model.gguf", Raw: "org/repo:model.gguf"}},
		},
		{
			name: "double dash long form",
			cmd:  "llama-server --hf-repo org/repo",
			want: []HFConfigRef{{RepoID: "org/repo", Raw: "org/repo"}},
		},
		{
			name: "hf-file attaches to preceding hf-repo",
			cmd:  "llama-server --hf-repo org/repo --hf-file model.gguf",
			want: []HFConfigRef{{RepoID: "org/repo", File: "model.gguf", Raw: "org/repo"}},
		},
		{
			name: "bare repo token",
			cmd:  "llama-server -m org/repo:model.gguf --port 8080",
			want: []HFConfigRef{{RepoID: "org/repo", File: "model.gguf", Raw: "org/repo:model.gguf"}},
		},
		{
			name: "huggingface.co url",
			cmd:  "llama-server -m https://huggingface.co/org/repo/resolve/main/model.gguf",
			want: []HFConfigRef{{RepoID: "org/repo", Raw: "https://huggingface.co/org/repo/resolve/main/model.gguf"}},
		},
		{
			name: "local paths are ignored",
			cmd:  "llama-server -m /home/u/.cache/llama.cpp/model.gguf",
			want: []HFConfigRef{},
		},
		{
			name: "relative and home paths are ignored",
			cmd:  "llama-server -m ./models/foo.gguf -m ~/models/bar.gguf",
			want: []HFConfigRef{},
		},
		{
			name: "port-like tokens are ignored",
			cmd:  "llama-server --host 127.0.0.1 --port 8080 --proxy http://localhost:5800",
			want: []HFConfigRef{},
		},
		{
			name: "bare repo name without slash is ignored",
			cmd:  "llama-server -hf model.gguf",
			want: []HFConfigRef{},
		},
		{
			name: "dedupe identical bare and hf refs",
			cmd:  "-hf org/repo:model.gguf --port 1 org/repo:model.gguf",
			want: []HFConfigRef{{RepoID: "org/repo", File: "model.gguf", Raw: "org/repo:model.gguf"}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ExtractHFRefs(tc.cmd)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("ExtractHFRefs(%q)\n got: %+v\nwant: %+v", tc.cmd, got, tc.want)
			}
		})
	}
}

func TestExtractConfigHFRefsSortsByModelID(t *testing.T) {
	models := map[string]config.ModelConfig{
		"z-model": {Cmd: "llama-server -hf zz/repo:model.gguf"},
		"a-model": {Cmd: "llama-server -hf aa/repo"},
	}
	refs := ExtractConfigHFRefs(models)
	if len(refs) != 2 {
		t.Fatalf("want 2 refs, got %d: %+v", len(refs), refs)
	}
	if refs[0].ModelID != "a-model" || refs[1].ModelID != "z-model" {
		t.Fatalf("refs not sorted by model id: %+v", refs)
	}
}

func TestModelSearchNames(t *testing.T) {
	mc := config.ModelConfig{
		Name:    "Qwen3.5-9B",
		Aliases: []string{"qwen3.5-9b", "  qwen-9b  ", ""},
	}
	names := ModelSearchNames("qwen3.5-9b", mc)
	// Model id wins the dedupe against the identical display name and alias;
	// the distinct alias is kept, empties and whitespace are dropped.
	if !reflect.DeepEqual(names, []string{"qwen3.5-9b", "qwen-9b"}) {
		t.Fatalf("ModelSearchNames: %+v", names)
	}
	// Display name is used when the id is not a useful search string.
	names = ModelSearchNames("m1", config.ModelConfig{Name: "Hermes-4-70B"})
	if !reflect.DeepEqual(names, []string{"m1", "Hermes-4-70B"}) {
		t.Fatalf("ModelSearchNames with name: %+v", names)
	}
	// Aliases alone are searchable.
	names = ModelSearchNames("local-1", config.ModelConfig{Aliases: []string{"hermes-4"}})
	if !reflect.DeepEqual(names, []string{"local-1", "hermes-4"}) {
		t.Fatalf("ModelSearchNames with alias: %+v", names)
	}
	// Nothing searchable at all → nil (callers must report no_ref).
	if names := ModelSearchNames("", config.ModelConfig{}); names != nil {
		t.Fatalf("expected nil names, got %+v", names)
	}
}

func TestSelectStrongHFMatch(t *testing.T) {
	results := func(ids ...string) []HFSearchResult {
		out := make([]HFSearchResult, 0, len(ids))
		for _, id := range ids {
			out = append(out, HFSearchResult{ID: id})
		}
		return out
	}
	cases := []struct {
		name    string
		query   string
		results []HFSearchResult
		want    string // expected repo id; "" means no match
	}{
		{
			name:    "exact repo id wins",
			query:   "Qwen/Qwen3.5-9B",
			results: results("Qwen/Qwen3.5-9B-Instruct", "qwen/qwen3.5-9b"),
			want:    "qwen/qwen3.5-9b",
		},
		{
			name:    "exact name segment beats prefix",
			query:   "qwen3.5-9b",
			results: results("Qwen/Qwen3.5-9B-Instruct", "someone/Qwen3.5-9B"),
			want:    "someone/Qwen3.5-9B",
		},
		{
			name:    "token-boundary prefix match",
			query:   "qwen3.5-9b",
			results: results("Qwen/Qwen3.5-9B-Instruct", "Qwen/Qwen3.5-9B-AWQ"),
			want:    "Qwen/Qwen3.5-9B-Instruct", // relevance order kept among equal ranks
		},
		{
			name:    "no token boundary on suffix is not a match",
			query:   "qwen3.5-9b",
			results: results("org/qwen3.5-9bx", "org/qwen3.5-9bber"),
			want:    "",
		},
		{
			name:    "unrelated results match nothing",
			query:   "qwen3.5-9b",
			results: results("meta-llama/Llama-3.1-8B", "mistralai/Mistral-7B-v0.3"),
			want:    "",
		},
		{
			name:    "case and whitespace insensitive",
			query:   "  QWEN3.5-9B ",
			results: results("Qwen/Qwen3.5-9B"),
			want:    "Qwen/Qwen3.5-9B",
		},
		{
			name:    "empty query never matches",
			query:   "",
			results: results("Qwen/Qwen3.5-9B"),
			want:    "",
		},
		{
			name:    "empty result set never matches",
			query:   "qwen3.5-9b",
			results: results(),
			want:    "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			best, rank, ok := SelectStrongHFMatch(tc.query, tc.results)
			if tc.want == "" {
				if ok {
					t.Fatalf("expected no match, got %+v (rank %d)", best, rank)
				}
				return
			}
			if !ok || best.ID != tc.want {
				t.Fatalf("SelectStrongHFMatch(%q, %+v) = %+v rank=%d ok=%v, want %q",
					tc.query, tc.results, best, rank, ok, tc.want)
			}
		})
	}
}

func TestScanHFHubCache(t *testing.T) {
	root := t.TempDir()
	write := func(rel string, n int) {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(p, make([]byte, n), 0o644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}
	// Two GGUF repos: one flat, one with a nested file and a non-gguf file.
	write("models--org--repo/snapshots/abc123/model.gguf", 100)
	write("models--org--repo/snapshots/abc123/notes.txt", 50)
	write("models--org--other/snapshots/def456/sub/model2.gguf", 200)
	// Repo with no gguf at all: must be skipped.
	write("models--org--empty/snapshots/rev1/readme.md", 10)
	// Non-repo dir: must be ignored.
	write("not-a-repo/model.gguf", 5)

	repos, err := ScanHFHubCache(root)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(repos) != 2 {
		t.Fatalf("want 2 repos, got %d: %+v", len(repos), repos)
	}
	if repos[0].RepoID != "org/other" || repos[1].RepoID != "org/repo" {
		t.Fatalf("repos not sorted or wrong ids: %+v", repos)
	}
	// org/other: nested gguf found, 200 bytes.
	if len(repos[0].Files) != 1 || repos[0].Files[0].Name != "model2.gguf" || repos[0].TotalBytes != 200 {
		t.Fatalf("org/other wrong: %+v", repos[0])
	}
	// org/repo: gguf only (notes.txt ignored), 100 bytes.
	if len(repos[1].Files) != 1 || repos[1].Files[0].Name != "model.gguf" || repos[1].TotalBytes != 100 {
		t.Fatalf("org/repo wrong: %+v", repos[1])
	}
}

func TestScanHFHubCacheMissingRoot(t *testing.T) {
	if _, err := ScanHFHubCache(filepath.Join(t.TempDir(), "does-not-exist")); err == nil {
		t.Fatal("expected error for missing root")
	}
}

func TestScanHFHubCacheDuplicateAcrossRevisions(t *testing.T) {
	root := t.TempDir()
	for _, rev := range []string{"rev1", "rev2"} {
		p := filepath.Join(root, "models--org--repo", "snapshots", rev, "model.gguf")
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(p, make([]byte, 42), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
	repos, err := ScanHFHubCache(root)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(repos) != 1 || len(repos[0].Files) != 1 || repos[0].TotalBytes != 42 {
		t.Fatalf("duplicate file across revisions not deduped: %+v", repos)
	}
}

func TestMatchHFRepos(t *testing.T) {
	repos := []CachedHFRepo{
		{RepoID: "org/untracked", Revision: "r1", Files: []HFRepoFile{{Name: "u.gguf"}}},
		{RepoID: "org/repo", Revision: "r1", Files: []HFRepoFile{{Name: "model.gguf"}}},
	}
	refs := []HFConfigRef{{ModelID: "m1", RepoID: "org/repo", File: "model.gguf", Raw: "org/repo:model.gguf"}}

	reports := MatchHFRepos(repos, refs)
	if len(reports) != 2 {
		t.Fatalf("want 2 reports, got %d", len(reports))
	}
	byID := map[string]HFRepoReport{}
	for _, r := range reports {
		byID[r.RepoID] = r
	}
	if r := byID["org/repo"]; !r.Matched || r.ConfigModel != "m1" || r.Reason == "" || r.Reason == "no config reference" {
		t.Fatalf("org/repo should match: %+v", r)
	}
	if r := byID["org/untracked"]; r.Matched || r.Reason != "no config reference" {
		t.Fatalf("org/untracked should be explicitly unmatched: %+v", r)
	}
}

func TestMatchHFReposCaseInsensitive(t *testing.T) {
	repos := []CachedHFRepo{{RepoID: "Org/Repo", Files: []HFRepoFile{{Name: "model.gguf"}}}}
	refs := []HFConfigRef{{ModelID: "m1", RepoID: "org/repo", Raw: "org/repo"}}
	reports := MatchHFRepos(repos, refs)
	if len(reports) != 1 || !reports[0].Matched {
		t.Fatalf("case-insensitive match failed: %+v", reports)
	}
}

func TestMarkRegisteredNeverCreatesOrClobbers(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	// No row yet: must NOT create one.
	ok, err := MarkRegistered(ctx, db, "ghost-model", 111)
	if err != nil {
		t.Fatalf("mark on missing row: %v", err)
	}
	if ok {
		t.Fatal("expected ok=false for missing row")
	}
	var count int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM hugs_model_meta WHERE model_id='ghost-model'`).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 0 {
		t.Fatal("MarkRegistered created a row — must never create")
	}

	// Existing row (via the normal create path): stamps once, never clobbers.
	if _, err := GetModelMeta(ctx, db, "real-model"); err != nil {
		t.Fatalf("get: %v", err)
	}
	ok, err = MarkRegistered(ctx, db, "real-model", 222)
	if err != nil || !ok {
		t.Fatalf("mark existing: ok=%v err=%v", ok, err)
	}
	m, err := GetModelMeta(ctx, db, "real-model")
	if err != nil || m.RegisteredAt != 222 {
		t.Fatalf("registered_at not stamped: %+v err=%v", m, err)
	}
	ok, err = MarkRegistered(ctx, db, "real-model", 333)
	if err != nil || !ok {
		t.Fatalf("second mark: ok=%v err=%v", ok, err)
	}
	m, err = GetModelMeta(ctx, db, "real-model")
	if err != nil || m.RegisteredAt != 222 {
		t.Fatalf("registered_at clobbered: %+v err=%v", m, err)
	}
}

func TestInferHFCapabilities(t *testing.T) {
	meta := func(repoID, pipeline string, tags []string, modelType string, archs []string) *HFModelMeta {
		m := &HFModelMeta{ID: repoID, PipelineTag: pipeline, Tags: tags}
		m.Config.ModelType = modelType
		m.Config.Architectures = archs
		return m
	}
	cases := []struct {
		name string
		meta *HFModelMeta
		want HFCapabilities
	}{
		{
			name: "vision-language model via pipeline tag",
			meta: meta("Qwen/Qwen2.5-VL-7B-Instruct", "image-text-to-text",
				[]string{"qwen2_5_vl", "image-text-to-text", "multimodal"}, "qwen2_5_vl",
				[]string{"Qwen2_5_VLForConditionalGeneration"}),
			want: HFCapabilities{Vision: true},
		},
		{
			name: "vision via architecture",
			meta: meta("org/llava", "text-generation", []string{"llava"}, "llava",
				[]string{"LlavaForConditionalGeneration"}),
			want: HFCapabilities{Vision: true},
		},
		{
			name: "audio via pipeline tag",
			meta: meta("openai/whisper-large-v3", "automatic-speech-recognition",
				[]string{"whisper", "audio"}, "whisper", []string{"WhisperForConditionalGeneration"}),
			want: HFCapabilities{Audio: true},
		},
		{
			name: "image generation via pipeline tag",
			meta: meta("stabilityai/stable-diffusion-xl-base-1.0", "text-to-image",
				[]string{"text-to-image"}, "", nil),
			want: HFCapabilities{Image: true},
		},
		{
			name: "tools via function-calling tag",
			meta: meta("NousResearch/Hermes-3-Llama-3.1-8B", "text-generation",
				[]string{"function calling"}, "llama", []string{"LlamaForCausalLM"}),
			want: HFCapabilities{Tools: true},
		},
		{
			name: "mtp via repo id token",
			meta: meta("Qwen/Qwen3-MTP-0.6B-Instruct-2507", "text-generation",
				nil, "qwen3", []string{"Qwen3ForCausalLM"}),
			want: HFCapabilities{MTP: true},
		},
		{
			name: "mtp via architecture token",
			meta: meta("org/m", "text-generation", []string{"mtp-head"},
				"qwen3", []string{"Qwen3ForCausalLM"}),
			want: HFCapabilities{MTP: true},
		},
		{
			name: "plain chat model claims nothing",
			meta: meta("Qwen/Qwen3-30B-A3B-Instruct-2507", "text-generation",
				nil, "qwen3_moe", []string{"Qwen3MoeForCausalLM"}),
			want: HFCapabilities{},
		},
		{
			name: "smtp does not trigger mtp",
			meta: meta("org/smtp-tools", "text-generation", []string{"smtp"}, "gpt2", nil),
			want: HFCapabilities{},
		},
		{
			name: "nil metadata claims nothing",
			meta: nil,
			want: HFCapabilities{},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, evidence := InferHFCapabilities(tc.meta)
			if got != tc.want {
				t.Fatalf("InferHFCapabilities(%v)\n got: %+v\nwant: %+v\nevidence: %v", tc.meta, got, tc.want, evidence)
			}
			if tc.want.HasAny() && len(evidence) == 0 {
				t.Fatalf("capabilities claimed without evidence: %+v", got)
			}
			if !tc.want.HasAny() && len(evidence) != 0 {
				t.Fatalf("no capabilities claimed but evidence present: %v", evidence)
			}
		})
	}
}

func TestInferHFCapabilitiesCardDataPipelineWins(t *testing.T) {
	// cardData.pipeline_tag is canonical: "text-to-image" there must win over
	// a stale top-level pipeline_tag.
	m := &HFModelMeta{ID: "org/sd", PipelineTag: "text-generation", Tags: []string{"diffusers"}}
	m.CardData.PipelineTag = "text-to-image"
	caps, evidence := InferHFCapabilities(m)
	if !caps.Image {
		t.Fatalf("cardData pipeline_tag not honored: %+v evidence=%v", caps, evidence)
	}
}

func TestApplyHFTags(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	// Fresh model: row created, only hf tags present.
	joined, err := ApplyHFTags(ctx, db, "m1", []string{"hf:checked", "hf:vision"})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if joined != "hf:checked,hf:vision" {
		t.Fatalf("fresh tags: %q", joined)
	}
	// Add a user tag, then rewrite hf namespace.
	if _, err := UpdateModelMeta(ctx, db, ModelMeta{ModelID: "m1", Tags: "favorite"}); err != nil {
		t.Fatalf("user tag: %v", err)
	}
	joined, err = ApplyHFTags(ctx, db, "m1", []string{"hf:checked", "hf:mtp"})
	if err != nil {
		t.Fatalf("apply 2: %v", err)
	}
	if joined != "favorite,hf:checked,hf:mtp" {
		t.Fatalf("merged tags: %q", joined)
	}
	// hf:vision from the first scan must be gone (stale findings never linger).
	if strings.Contains(joined, "hf:vision") {
		t.Fatalf("stale hf tag survived: %q", joined)
	}
	// Clearing the namespace keeps user tags.
	joined, err = ApplyHFTags(ctx, db, "m1", nil)
	if err != nil {
		t.Fatalf("apply 3: %v", err)
	}
	if joined != "favorite" {
		t.Fatalf("cleared tags: %q", joined)
	}
}
