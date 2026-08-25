package hugs

// Hugging Face Hub cache rescan, used by POST /api/hugs/hf/rescan.
//
// The scan is read-only against the filesystem. Matching is deliberately
// conservative: a cached repo is considered matched only when some model's
// cmd references the exact repo id (see ExtractHFRefs); everything else is
// reported explicitly as unmatched. When a model's cmd carries no repo
// reference, the verify path (internal/server) falls back to HF Hub name
// search over the model id, display name and aliases (see ModelSearchNames
// and SelectStrongHFMatch). The only database write in this package is
// MarkRegistered, which never creates rows.

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/rahlquist/llama-hugs/internal/config"
)

// HFRepoFile is one GGUF file inside a cached HF repo snapshot.
type HFRepoFile struct {
	Name      string `json:"name"`
	Path      string `json:"path"`
	SizeBytes int64  `json:"size_bytes"`
}

// CachedHFRepo is one downloaded repo found in the HF hub cache.
type CachedHFRepo struct {
	RepoID     string       `json:"repo_id"`
	Revision   string       `json:"revision,omitempty"`
	TotalBytes int64        `json:"total_bytes"`
	Files      []HFRepoFile `json:"files"`
}

// HFConfigRef is one Hugging Face repo reference extracted from a model's cmd.
type HFConfigRef struct {
	ModelID string `json:"model_id"`
	RepoID  string `json:"repo_id"`
	File    string `json:"file,omitempty"` // optional file filter, e.g. -hf org/name:file.gguf
	Raw     string `json:"raw,omitempty"`  // the exact cmd token it came from
}

// HFRepoReport is one cached repo with its (conservative) match verdict.
type HFRepoReport struct {
	RepoID      string       `json:"repo_id"`
	Matched     bool         `json:"matched"`
	ConfigModel string       `json:"config_model,omitempty"` // matched config model id
	Reason      string       `json:"reason,omitempty"`
	Revision    string       `json:"revision,omitempty"`
	TotalBytes  int64        `json:"total_bytes"`
	Files       []HFRepoFile `json:"files"`
}

// HFHubCacheRoot resolves the Hugging Face Hub cache directory, honoring the
// same precedence as huggingface_hub: HF_HUB_CACHE, then HF_HOME/hub, then
// ~/.cache/huggingface/hub.
func HFHubCacheRoot() string {
	if c := strings.TrimSpace(os.Getenv("HF_HUB_CACHE")); c != "" {
		return c
	}
	if h := strings.TrimSpace(os.Getenv("HF_HOME")); h != "" {
		return filepath.Join(h, "hub")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".cache", "huggingface", "hub")
}

// ScanHFHubCache walks the HF hub cache for repos that contain at least one
// *.gguf file. Repos without any GGUF are skipped entirely. Symlinked blobs
// are reported by their snapshot-relative path. Read-only; raced deletions
// are skipped, never fatal.
func ScanHFHubCache(root string) ([]CachedHFRepo, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("hugs: scan hf cache: %w", err)
	}
	repos := []CachedHFRepo{}
	for _, e := range entries {
		if !e.IsDir() || !strings.HasPrefix(e.Name(), "models--") {
			continue
		}
		repoID := strings.ReplaceAll(strings.TrimPrefix(e.Name(), "models--"), "--", "/")
		if repoID == "" {
			continue
		}
		repo := CachedHFRepo{RepoID: repoID}
		snapDir := filepath.Join(root, e.Name(), "snapshots")
		revs, err := os.ReadDir(snapDir)
		if err != nil {
			continue // no snapshots yet — nothing downloaded
		}
		seen := map[string]bool{}
		for _, rev := range revs {
			if !rev.IsDir() {
				continue
			}
			files := scanSnapshotGGUF(filepath.Join(snapDir, rev.Name()))
			if len(files) == 0 {
				continue
			}
			if repo.Revision == "" {
				repo.Revision = rev.Name()
			}
			for _, f := range files {
				if seen[f.Name] {
					continue // same file across revisions — keep the first
				}
				seen[f.Name] = true
				repo.TotalBytes += f.SizeBytes
				repo.Files = append(repo.Files, f)
			}
		}
		if len(repo.Files) == 0 {
			continue
		}
		sort.Slice(repo.Files, func(i, j int) bool {
			return repo.Files[i].SizeBytes > repo.Files[j].SizeBytes
		})
		repos = append(repos, repo)
	}
	sort.Slice(repos, func(i, j int) bool { return repos[i].RepoID < repos[j].RepoID })
	return repos, nil
}

// scanSnapshotGGUF recursively collects *.gguf files under a snapshot dir.
func scanSnapshotGGUF(root string) []HFRepoFile {
	var files []HFRepoFile
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil // skip unreadable entries, never fail the scan
		}
		if !strings.HasSuffix(strings.ToLower(d.Name()), ".gguf") {
			return nil
		}
		info, ierr := d.Info()
		if ierr != nil {
			return nil
		}
		files = append(files, HFRepoFile{Name: d.Name(), Path: path, SizeBytes: info.Size()})
		return nil
	})
	return files
}

// hfSpecTokenRe matches user/repo[:file] tokens. Both segments must be
// alphanumeric-ish (letters, digits, _, -, .) which excludes local paths
// like ./x, ../x, /abs/path, ~/x and URL scheme prefixes.
var hfSpecTokenRe = regexp.MustCompile(`^([A-Za-z0-9_.-]+)/([A-Za-z0-9_.-]+)(?::([^/]+))?$`)

// hfSpecFromToken parses a single cmd token into repo id and optional file.
// Returns ok=false for anything ambiguous (no slash, path-traversal-ish
// segments, flag-looking orgs). Conservative: when in doubt, not a reference.
func hfSpecFromToken(tok string) (repo, file string, ok bool) {
	m := hfSpecTokenRe.FindStringSubmatch(tok)
	if m == nil {
		return "", "", false
	}
	org, name, f := m[1], m[2], m[3]
	if org == "." || org == ".." || name == "." || name == ".." {
		return "", "", false
	}
	if strings.HasPrefix(org, ".") || strings.HasPrefix(name, ".") {
		return "", "", false
	}
	if strings.HasPrefix(org, "-") || strings.HasPrefix(name, "-") {
		return "", "", false
	}
	return org + "/" + name, f, true
}

// hfURLRepo extracts user/repo from a huggingface.co URL token, e.g.
// https://huggingface.co/org/name/resolve/main/file.gguf → org/name.
func hfURLRepo(tok string) (repo string, ok bool) {
	idx := strings.Index(tok, "huggingface.co/")
	if idx < 0 {
		return "", false
	}
	rest := tok[idx+len("huggingface.co/"):]
	parts := strings.SplitN(rest, "/", 3)
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return "", false
	}
	org, name := parts[0], parts[1]
	if strings.ContainsAny(name, "?:") || strings.HasPrefix(org, "-") || strings.HasPrefix(name, "-") {
		return "", false
	}
	return org + "/" + name, true
}

// ExtractHFRefs extracts Hugging Face repo references from one model cmd.
// Recognized forms (llama.cpp syntax and bare tokens):
//
//	-hf user/repo[:file]        --hf-repo user/repo[:file]
//	--hf-file file.gguf         (attached to the most recent -hf/--hf-repo)
//	user/repo[:file]            (bare, exact token)
//	https://huggingface.co/user/repo/...
//
// A bare repo name without a slash is never treated as a reference. Results
// are deduplicated.
func ExtractHFRefs(cmd string) []HFConfigRef {
	refs := []HFConfigRef{}
	lastRepoIdx := -1

	// Straightforward pass over the token slice.
	tokens := splitCmdTokens(cmd)
	for i := 0; i < len(tokens); i++ {
		tok := tokens[i]
		switch tok {
		case "-hf", "--hf-repo":
			if i+1 < len(tokens) {
				i++
				if repo, file, ok := hfSpecFromToken(tokens[i]); ok {
					refs = append(refs, HFConfigRef{RepoID: repo, File: file, Raw: tokens[i]})
					lastRepoIdx = len(refs) - 1
				}
			}
		case "--hf-file":
			if i+1 < len(tokens) {
				i++
				if lastRepoIdx >= 0 && refs[lastRepoIdx].File == "" {
					refs[lastRepoIdx].File = tokens[i]
				}
			}
		default:
			if repo, file, ok := hfSpecFromToken(tok); ok {
				refs = append(refs, HFConfigRef{RepoID: repo, File: file, Raw: tok})
			} else if repo, ok := hfURLRepo(tok); ok {
				refs = append(refs, HFConfigRef{RepoID: repo, Raw: tok})
			}
		}
	}
	return dedupeHFRefs(refs)
}

// splitCmdTokens splits a cmd on whitespace and strips surrounding quotes.
func splitCmdTokens(cmd string) []string {
	var out []string
	for _, tok := range strings.Fields(cmd) {
		tok = strings.Trim(tok, "\"'`")
		if tok != "" {
			out = append(out, tok)
		}
	}
	return out
}

// dedupeHFRefs removes duplicate refs (same repo, file, and raw token).
func dedupeHFRefs(refs []HFConfigRef) []HFConfigRef {
	seen := map[string]bool{}
	out := []HFConfigRef{}
	for _, r := range refs {
		key := strings.ToLower(r.RepoID) + "\x00" + r.File + "\x00" + r.Raw
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, r)
	}
	return out
}

// ExtractConfigHFRefs collects every HF repo reference across all configured
// models' cmd strings, sorted by model id then occurrence.
func ExtractConfigHFRefs(models map[string]config.ModelConfig) []HFConfigRef {
	ids := make([]string, 0, len(models))
	for id := range models {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := []HFConfigRef{}
	for _, id := range ids {
		for _, ref := range ExtractHFRefs(models[id].Cmd) {
			ref.ModelID = id
			out = append(out, ref)
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// HF Hub name search — fallback discovery when a model's cmd has no repo ref
// ---------------------------------------------------------------------------

// HFSearchResult is one entry of the PUBLIC HF Hub model search response
// (GET /api/models?search=...&limit=...). Only the repo id is needed for
// candidate selection; full metadata for the chosen repo is fetched
// separately so capabilities can be inferred from the same signals as
// explicit refs.
type HFSearchResult struct {
	ID string `json:"id"`
}

// NormalizeSearchName lowercases and trims a repo id or display name for
// stable comparison. Punctuation is preserved: exact equality plus
// token-boundary prefix rules (see SelectStrongHFMatch) decide the verdict.
func NormalizeSearchName(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

// ModelSearchNames returns the non-empty, de-duplicated searchable names for
// a configured model: its model id, display name (config "name"), and
// aliases, in that order. A model with nothing searchable returns nil —
// callers must report it as no_ref rather than fabricate a search.
func ModelSearchNames(modelID string, mc config.ModelConfig) []string {
	var out []string
	seen := map[string]bool{}
	add := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" {
			return
		}
		key := NormalizeSearchName(s)
		if seen[key] {
			return
		}
		seen[key] = true
		out = append(out, s)
	}
	add(modelID)
	add(mc.Name)
	for _, a := range mc.Aliases {
		add(a)
	}
	return out
}

// strongMatchRank scores how strongly a search result matches a query name:
//
//	0 — normalized repo id equals the query
//	1 — normalized name segment (text after the last "/") equals the query
//	2 — query is a token-boundary prefix of the name segment
//	   ("qwen3.5-9b" → "Qwen/Qwen3.5-9B-Instruct")
//
// A prefix only counts when the following character is "-", "_" or ".",
// so "qwen3.5-9b" never matches "qwen3.5-9bx". Returns ok=false when the
// candidate is not a strong match at all.
func strongMatchRank(query, candidateID string) (rank int, ok bool) {
	q := NormalizeSearchName(query)
	if q == "" {
		return 0, false
	}
	cid := NormalizeSearchName(candidateID)
	if cid == q {
		return 0, true
	}
	name := candidateID
	if i := strings.LastIndex(name, "/"); i >= 0 {
		name = name[i+1:]
	}
	cn := NormalizeSearchName(name)
	if cn == q {
		return 1, true
	}
	if strings.HasPrefix(cn, q) {
		rest := cn[len(q):]
		if rest != "" && (rest[0] == '-' || rest[0] == '_' || rest[0] == '.') {
			return 2, true
		}
	}
	return 0, false
}

// SelectStrongHFMatch picks the best candidate from a HF Hub model search
// response for one query name. Exact matches win over token-boundary prefix
// matches; ties keep search (relevance) order. The returned rank is the
// match strength (lower is stronger: 0 exact id, 1 exact name segment,
// 2 token-boundary prefix). Returns ok=false when no result is a strong
// match.
func SelectStrongHFMatch(query string, results []HFSearchResult) (best HFSearchResult, rank int, ok bool) {
	bestRank := 0
	found := false
	for _, r := range results {
		if r.ID == "" {
			continue
		}
		rk, m := strongMatchRank(query, r.ID)
		if !m {
			continue
		}
		if !found || rk < bestRank {
			best, bestRank, found = r, rk, true
		}
	}
	return best, bestRank, found
}

// normalizeRepoID lowercases a repo id for conservative case-insensitive
// comparison (HF Hub treats repo ids case-insensitively).
func normalizeRepoID(repoID string) string {
	return strings.ToLower(strings.TrimSpace(repoID))
}

// MatchHFRepos assigns each cached repo a conservative verdict against the
// config refs: matched only when some ref's repo id equals the cached repo id
// (case-insensitive). Unmatched repos carry the explicit reason
// "no config reference". Results are sorted by repo id.
func MatchHFRepos(repos []CachedHFRepo, refs []HFConfigRef) []HFRepoReport {
	refByRepo := map[string][]HFConfigRef{}
	for _, r := range refs {
		key := normalizeRepoID(r.RepoID)
		refByRepo[key] = append(refByRepo[key], r)
	}
	reports := make([]HFRepoReport, 0, len(repos))
	for _, repo := range repos {
		rep := HFRepoReport{
			RepoID:     repo.RepoID,
			Revision:   repo.Revision,
			TotalBytes: repo.TotalBytes,
			Files:      repo.Files,
		}
		matchRefs := refByRepo[normalizeRepoID(repo.RepoID)]
		if len(matchRefs) > 0 {
			rep.Matched = true
			rep.ConfigModel = matchRefs[0].ModelID // refs are sorted by model id
			rep.Reason = describeRefs(matchRefs)
		} else {
			rep.Matched = false
			rep.Reason = "no config reference"
		}
		reports = append(reports, rep)
	}
	sort.Slice(reports, func(i, j int) bool { return reports[i].RepoID < reports[j].RepoID })
	return reports
}

// describeRefs builds a human-readable match reason from the config refs,
// e.g. `config model "m1" references "-hf org/name:file.gguf"`.
func describeRefs(refs []HFConfigRef) string {
	parts := make([]string, 0, len(refs))
	for _, r := range refs {
		raw := r.Raw
		if raw == "" {
			raw = r.RepoID
		}
		parts = append(parts, fmt.Sprintf("model %q references %q", r.ModelID, raw))
	}
	return strings.Join(parts, "; ")
}

// ---------------------------------------------------------------------------
// Hugging Face metadata & conservative capability inference
// ---------------------------------------------------------------------------

// HFModelMeta is the subset of the PUBLIC Hugging Face model API response
// (GET https://huggingface.co/api/models/<repo>) used for conservative
// capability inference. Unknown fields are ignored.
type HFModelMeta struct {
	ID          string   `json:"id"`
	PipelineTag string   `json:"pipeline_tag"`
	Tags        []string `json:"tags"`
	LibraryName string   `json:"library_name"`
	Config      struct {
		ModelType     string   `json:"model_type"`
		Architectures []string `json:"architectures"`
	} `json:"config"`
	CardData struct {
		PipelineTag string `json:"pipeline_tag"`
	} `json:"cardData"`
}

// HFCapabilities is the conservative capability finding set derived from HF
// model metadata. Every field defaults to false: a capability is claimed only
// when the metadata positively signals it (see InferHFCapabilities), so an
// absent signal never fabricates a finding.
type HFCapabilities struct {
	Vision bool `json:"vision"` // image/video input understanding
	Audio  bool `json:"audio"`  // audio/speech input understanding
	Image  bool `json:"image"`  // image/video generation (output modality)
	Tools  bool `json:"tools"`  // function calling / tool use
	MTP    bool `json:"mtp"`    // multi-token prediction (--mtp support)
}

// HasAny reports whether at least one capability was derived.
func (c HFCapabilities) HasAny() bool {
	return c.Vision || c.Audio || c.Image || c.Tools || c.MTP
}

// HFModelResult is one configured model's verification outcome against the
// public HF API. Status is one of:
//
//	matched      — repo exists (200); Capabilities holds the conservative findings
//	unmatched    — repo not found on HF (404) or no strong name-search match
//	unauthorized — 401/403: gated or nonexistent; cannot be verified tokenless
//	error        — network/timeout/unexpected response; no finding possible
//	no_ref       — cmd has no HF repo reference AND no searchable name/alias;
//	               not queried
type HFModelResult struct {
	ModelID      string         `json:"model_id"`
	RepoID       string         `json:"repo_id,omitempty"`
	Status       string         `json:"status"`
	HTTPStatus   int            `json:"http_status,omitempty"`
	Error        string         `json:"error,omitempty"`
	Reason       string         `json:"reason,omitempty"`
	Capabilities HFCapabilities `json:"capabilities,omitempty"`
	PipelineTag  string         `json:"pipeline_tag,omitempty"`
	Evidence     []string       `json:"evidence,omitempty"` // matched metadata signals
}

// Capability evidence hint groups. Every hint is matched case-insensitively
// as a substring of the lower-cased metadata field, except MTP which uses a
// token-boundary match to avoid "smtp"-style false positives.
var (
	visionPipelineTags = map[string]bool{
		"image-text-to-text": true, "image-to-text": true,
		"visual-question-answering": true, "document-question-answering": true,
		"video-text-to-text": true, "zero-shot-image-classification": true,
	}
	visionHints = []string{
		"image-text-to-text", "image-to-text", "vision",
		"visual-question-answering", "document-question-answering",
		"video-text-to-text",
	}
	visionArchHints = []string{"vl", "llava", "vision", "mllama", "internvl", "qwen2-vl", "pixtral", "molmo", "smolvlm"}

	audioPipelineTags = map[string]bool{
		"automatic-speech-recognition": true, "audio-to-text": true,
		"speech-recognition": true, "speech-translation": true,
		"text-to-speech": true, "text-to-audio": true, "audio-to-audio": true,
		"audio-classification": true, "music-generation": true,
		"audio-text-to-text": true,
	}
	audioHints = []string{
		"automatic-speech-recognition", "audio-to-text", "speech-recognition",
		"speech-translation", "text-to-speech", "text-to-audio",
		"audio-classification", "audio-to-audio", "speech", "whisper",
	}
	audioArchHints = []string{
		"whisper", "wav2vec", "hubert", "clap", "encodec", "bark", "vits",
		"seamless", "speecht5", "audio", "speech",
	}

	imagePipelineTags = map[string]bool{
		"text-to-image": true, "image-to-image": true, "text-to-video": true,
		"image-to-video": true, "video-to-video": true,
	}
	imageHints = []string{
		"text-to-image", "image-to-image", "text-to-video", "image-to-video",
		"stable-diffusion", "sdxl", "controlnet", "diffusers", "pixart", "flux",
	}
	imageArchHints = []string{"unet", "diffusion", "flux", "pixart", "cogvideo", "stable-diffusion", "deepfloyd"}

	toolHints = []string{
		"tools", "tool-use", "tool use", "tool_use", "tool-calling",
		"tool calling", "function calling", "function-calling",
		"function_calling", "agent", "agents", "agentic",
	}
)

// mtpTokenRe matches an "mtp" token at a word boundary (e.g. "mtp-head",
// "qwen3-mtp-0.6b") without matching "smtp" or "mtp" inside a longer word.
var mtpTokenRe = regexp.MustCompile(`(?i)(^|[_-])(mtp|mtp-head)([_-]|$)`)

// hasAnyFold reports whether any hint is a case-insensitive substring of any
// field. The first hit is returned so callers can cite evidence.
func hasAnyFold(fields, hints []string) (string, bool) {
	for _, f := range fields {
		lf := strings.ToLower(f)
		for _, h := range hints {
			if strings.Contains(lf, h) {
				return h, true
			}
		}
	}
	return "", false
}

// InferHFCapabilities conservatively derives capability findings from HF
// model metadata. A capability is claimed only when metadata positively
// signals it (pipeline_tag, tags, config.model_type/architectures). The
// returned evidence lists the specific signals that fired, for display.
func InferHFCapabilities(meta *HFModelMeta) (HFCapabilities, []string) {
	var caps HFCapabilities
	var evidence []string
	if meta == nil {
		return caps, evidence
	}
	add := func(capKey, signal string) {
		evidence = append(evidence, capKey+": "+signal)
	}
	lowerFields := func(ss []string) []string {
		out := make([]string, 0, len(ss))
		for _, s := range ss {
			out = append(out, strings.ToLower(strings.TrimSpace(s)))
		}
		return out
	}

	pipeline := strings.ToLower(strings.TrimSpace(meta.PipelineTag))
	if cd := strings.ToLower(strings.TrimSpace(meta.CardData.PipelineTag)); cd != "" {
		pipeline = cd // cardData.pipeline_tag is the canonical card value
	}
	tags := lowerFields(meta.Tags)
	archs := lowerFields(meta.Config.Architectures)
	modelType := strings.ToLower(strings.TrimSpace(meta.Config.ModelType))

	// Vision: image/video input understanding.
	if visionPipelineTags[pipeline] {
		caps.Vision = true
		add("vision", "pipeline_tag="+pipeline)
	} else if hint, ok := hasAnyFold(tags, visionHints); ok {
		caps.Vision = true
		add("vision", "tag="+hint)
	} else if hint, ok := hasAnyFold(archs, visionArchHints); ok {
		caps.Vision = true
		add("vision", "arch="+hint)
	} else if hint, ok := hasAnyFold([]string{modelType}, visionArchHints); ok {
		caps.Vision = true
		add("vision", "model_type="+hint)
	}

	// Audio: audio/speech input understanding.
	if audioPipelineTags[pipeline] {
		caps.Audio = true
		add("audio", "pipeline_tag="+pipeline)
	} else if hint, ok := hasAnyFold(tags, audioHints); ok {
		caps.Audio = true
		add("audio", "tag="+hint)
	} else if hint, ok := hasAnyFold(archs, audioArchHints); ok {
		caps.Audio = true
		add("audio", "arch="+hint)
	} else if hint, ok := hasAnyFold([]string{modelType}, audioArchHints); ok {
		caps.Audio = true
		add("audio", "model_type="+hint)
	}

	// Image: image/video generation (output modality).
	if imagePipelineTags[pipeline] {
		caps.Image = true
		add("image", "pipeline_tag="+pipeline)
	} else if hint, ok := hasAnyFold(tags, imageHints); ok {
		caps.Image = true
		add("image", "tag="+hint)
	} else if hint, ok := hasAnyFold(archs, imageArchHints); ok {
		caps.Image = true
		add("image", "arch="+hint)
	} else if hint, ok := hasAnyFold([]string{modelType}, imageArchHints); ok {
		caps.Image = true
		add("image", "model_type="+hint)
	}

	// Tools: function calling / tool use. Only tag/card signals are reliable.
	if hint, ok := hasAnyFold(tags, toolHints); ok {
		caps.Tools = true
		add("tools", "tag="+hint)
	}

	// MTP: multi-token prediction. Token-boundary match on repo id, tags,
	// model_type and architectures; never claimed from a loose substring.
	mtpFields := append([]string{meta.ID, modelType}, archs...)
	mtpFields = append(mtpFields, tags...)
	for _, f := range mtpFields {
		if mtpTokenRe.MatchString(f) {
			caps.MTP = true
			add("mtp", "signal="+f)
			break
		}
	}

	return caps, evidence
}

// hfTagPrefix namespaces rescan-derived tags inside the freeform
// comma-separated hugs_model_meta.tags column. ApplyHFTags rewrites the whole
// hf:* namespace on each rescan so stale findings never linger; user tags are
// always preserved.
const hfTagPrefix = "hf:"

// ApplyHFTags rewrites the hf:* tag namespace on a model's meta row to
// exactly hfTags, preserving user tags. The row is created via the normal
// GetModelMeta path when absent (per-model presentation metadata — never
// config). Returns the final comma-separated tags.
func ApplyHFTags(ctx context.Context, db *sql.DB, modelID string, hfTags []string) (string, error) {
	cur, err := GetModelMeta(ctx, db, modelID)
	if err != nil {
		return "", fmt.Errorf("hugs: apply hf tags: get: %w", err)
	}
	kept := []string{}
	for _, t := range strings.Split(cur.Tags, ",") {
		t = strings.TrimSpace(t)
		if t == "" || strings.HasPrefix(t, hfTagPrefix) {
			continue
		}
		kept = append(kept, t)
	}
	seen := map[string]bool{}
	all := append([]string{}, kept...)
	all = append(all, hfTags...)
	out := make([]string, 0, len(all))
	for _, t := range all {
		t = strings.TrimSpace(t)
		if t == "" || seen[t] {
			continue
		}
		seen[t] = true
		out = append(out, t)
	}
	joined := strings.Join(out, ",")
	if _, err := UpdateModelMeta(ctx, db, ModelMeta{ModelID: modelID, Tags: joined}); err != nil {
		return "", fmt.Errorf("hugs: apply hf tags: update: %w", err)
	}
	return joined, nil
}

// MarkRegistered stamps registered_at on an existing hugs_model_meta row,
// only when it is currently unset. Unlike GetModelMeta it never creates a
// row, so a rescan cannot fabricate metadata. Returns whether a row existed.
func MarkRegistered(ctx context.Context, db *sql.DB, modelID string, ts int64) (bool, error) {
	res, err := db.ExecContext(ctx,
		`UPDATE hugs_model_meta
		 SET registered_at = CASE WHEN registered_at = 0 THEN ? ELSE registered_at END
		 WHERE model_id = ?`, ts, modelID)
	if err != nil {
		return false, fmt.Errorf("hugs: mark registered: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}
