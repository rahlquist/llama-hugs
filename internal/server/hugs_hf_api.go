package server

// HF hub rescan endpoint — see internal/hugs/hfscan.go for the scan, matching
// and conservative capability-inference logic. Divergence policy: this handler
// file is fork-added; the only upstream-file edit is the route registration in
// server.go.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rahlquist/llama-hugs/internal/config"
	"github.com/rahlquist/llama-hugs/internal/hugs"
)

// hfModelFetcher fetches the PUBLIC Hugging Face model metadata for repoID.
// It returns the parsed metadata on success (HTTP 200); httpStatus is the
// response status (200 on success, 401/403/404 on misses); on network or
// decode errors err is non-nil and httpStatus is 0. No Authorization header
// is ever set, so no token can leak into the request.
type hfModelFetcher func(ctx context.Context, repoID string) (*hugs.HFModelMeta, int, error)

// fetchHFModelMetaPublic is the default fetcher: GET to the PUBLIC HF Hub API
// endpoint https://huggingface.co/api/models/<repo>, body parsed into
// HFModelMeta. A 401/403/404 returns (nil, status, nil) so callers can treat
// misses as exact, tokenless verdicts rather than failures.
// hfModelURL builds the public HF model API URL for repoID, escaping each
// path segment separately. url.PathEscape on the whole id would escape the
// "/" separator as %2F, which the HF API rejects with 400.
func hfModelURL(repoID string) string {
	segs := strings.Split(repoID, "/")
	escaped := make([]string, 0, len(segs))
	for _, s := range segs {
		escaped = append(escaped, url.PathEscape(s))
	}
	return "https://huggingface.co/api/models/" + strings.Join(escaped, "/")
}

func fetchHFModelMetaPublic(ctx context.Context, repoID string) (*hugs.HFModelMeta, int, error) {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, hfModelURL(repoID), nil)
	if err != nil {
		return nil, 0, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, resp.StatusCode, nil
	}
	var meta hugs.HFModelMeta
	if err := json.NewDecoder(resp.Body).Decode(&meta); err != nil {
		return nil, resp.StatusCode, fmt.Errorf("decode hf metadata: %w", err)
	}
	return &meta, resp.StatusCode, nil
}

// hfSearchLimit bounds the number of candidates requested per HF Hub search.
const hfSearchLimit = 20

// hfModelSearcher searches the PUBLIC Hugging Face Hub model API
// (GET /api/models?search=...) for candidate repo ids matching query. It
// returns the raw result list on success (HTTP 200); httpStatus is the
// response status on misses (401/403/404); on network or decode errors err
// is non-nil and httpStatus is 0. No Authorization header is ever set, so no
// token can leak into the request.
type hfModelSearcher func(ctx context.Context, query string, limit int) ([]hugs.HFSearchResult, int, error)

// hfSearchURL builds the public HF Hub model search URL for query.
func hfSearchURL(query string, limit int) string {
	return "https://huggingface.co/api/models?search=" + url.QueryEscape(query) + "&limit=" + strconv.Itoa(limit)
}

func searchHFModelsPublic(ctx context.Context, query string, limit int) ([]hugs.HFSearchResult, int, error) {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, hfSearchURL(query, limit), nil)
	if err != nil {
		return nil, 0, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, resp.StatusCode, nil
	}
	var results []hugs.HFSearchResult
	if err := json.NewDecoder(resp.Body).Decode(&results); err != nil {
		return nil, resp.StatusCode, fmt.Errorf("decode hf search: %w", err)
	}
	return results, resp.StatusCode, nil
}

// hfVerifySummary aggregates the per-model HF verification outcomes.
type hfVerifySummary struct {
	Checked      int                  `json:"checked"`      // models queried against the public HF API
	Matched      int                  `json:"matched"`      // repo exists (200)
	Unmatched    int                  `json:"unmatched"`    // exact 404 misses and no-strong-match searches
	Unauthorized int                  `json:"unauthorized"` // 401/403 — gated or missing, unverifiable tokenless
	Errors       int                  `json:"errors"`       // network/timeout/unexpected
	NoRef        int                  `json:"no_ref"`       // no repo ref and no searchable name/alias (not queried)
	Models       []hugs.HFModelResult `json:"models"`       // one entry per configured model
}

// hfRescanResponse is the full JSON body of POST /api/hugs/hf/rescan.
type hfRescanResponse struct {
	CacheRoot       string              `json:"cache_root"`
	CacheRootExists bool                `json:"cache_root_exists"`
	ScannedAtUnix   int64               `json:"scanned_at_unix"`
	Repos           []hugs.HFRepoReport `json:"repos"`
	MatchedRepos    int                 `json:"matched_repos"`
	UnmatchedRepos  int                 `json:"unmatched_repos"`
	Unmatched       []string            `json:"unmatched"`
	ConfigRefs      []hugs.HFConfigRef  `json:"config_refs"`
	Verify          *hfVerifySummary    `json:"verify,omitempty"`
	MetaUpdated     []string            `json:"meta_updated,omitempty"` // registered_at stamped
	TagsUpdated     []string            `json:"tags_updated,omitempty"` // hf:* tags rewritten
	PersistedAtUnix int64               `json:"persisted_at_unix,omitempty"`
}

// handleHugsHFRescan POST /api/hugs/hf/rescan
//
// Rescans the local Hugging Face Hub cache (HF_HUB_CACHE, else HF_HOME/hub,
// else ~/.cache/huggingface/hub) for GGUF-bearing repos and matches them
// conservatively against the running config: a repo is matched only when
// some model's cmd references the exact repo id (-hf user/repo[:file],
// --hf-repo, --hf-file, a bare user/repo token, or a huggingface.co URL).
// Every unmatched repo is reported explicitly with reason "no config
// reference".
//
// ?verify=1 additionally queries the PUBLIC HF model API for each configured
// model. Models whose cmd references an HF repo are exact-matched by repo id;
// models without a repo reference fall back to HF Hub name search over their
// model id, display name and aliases, selecting the best strong match. For
// every queried model it derives conservative capability findings
// (vision/audio/image/tools/MTP) from the returned metadata, records them as
// namespaced hf:* tags on the per-model hugs_model_meta row (preserving user
// tags, never touching config), and reports unmatched exactly: 404 or no
// strong search match → unmatched, 401/403 → unauthorized (gated or missing,
// unverifiable tokenless). Models with no repo reference AND no searchable
// name/alias are reported as no_ref and never touch the network. No token is
// ever sent, and verification failures never fail the scan.
//
// Safety: this endpoint never writes config (the codebase has no config save
// path). Its only persistence is (1) a scan summary in hugs_settings via the
// existing SetSetting pattern, (2) a registered_at stamp on hugs_model_meta
// rows that already exist — MarkRegistered never creates rows, and (3) hf:*
// tags on per-model meta rows via ApplyHFTags (created through the normal
// GetModelMeta path — presentation metadata only).
func (s *Server) handleHugsHFRescan(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	root := hugs.HFHubCacheRoot()
	now := time.Now().Unix()

	out := hfRescanResponse{
		CacheRoot:       root,
		CacheRootExists: true,
		ScannedAtUnix:   now,
		Repos:           []hugs.HFRepoReport{},
		Unmatched:       []string{},
		ConfigRefs:      []hugs.HFConfigRef{},
	}

	repos, err := hugs.ScanHFHubCache(root)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			out.CacheRootExists = false
		} else {
			http.Error(w, "scan hf cache: "+err.Error(), http.StatusInternalServerError)
			return
		}
	}

	refs := hugs.ExtractConfigHFRefs(s.cfg.Models)
	out.ConfigRefs = refs

	var matched []hugs.HFRepoReport
	if repos != nil {
		out.Repos = hugs.MatchHFRepos(repos, refs)
	}
	for _, rep := range out.Repos {
		if rep.Matched {
			out.MatchedRepos++
			matched = append(matched, rep)
		} else {
			out.UnmatchedRepos++
			out.Unmatched = append(out.Unmatched, rep.RepoID)
		}
	}

	// ?verify=1: exact-match every configured model with an HF ref against the
	// public HF API, derive conservative capabilities, and record hf:* tags.
	if r.URL.Query().Get("verify") == "1" {
		summary := s.verifyModelsAgainstHF(ctx, s.cfg.Models, refs)
		out.Verify = summary
		out.TagsUpdated = s.persistHFFindings(ctx, summary)
	}

	// Persist a compact summary through the existing settings pattern. A
	// failure here is non-fatal: the scan result is still returned.
	summaryMap := map[string]any{
		"scanned_at_unix": now,
		"cache_root":      root,
		"matched_repos":   out.MatchedRepos,
		"unmatched_repos": out.UnmatchedRepos,
	}
	if out.Verify != nil {
		summaryMap["verified_models"] = out.Verify.Checked
		summaryMap["matched_models"] = out.Verify.Matched
		summaryMap["unmatched_models"] = out.Verify.Unmatched
		summaryMap["unauthorized_models"] = out.Verify.Unauthorized
		summaryMap["verify_errors"] = out.Verify.Errors
	}
	if err := hugs.SetSetting(ctx, s.store.DB(), "hf_rescan_last", mustJSON(summaryMap)); err == nil {
		out.PersistedAtUnix = time.Now().Unix()
	}

	// Stamp registered_at for matched models whose meta row already exists.
	// No rows are created and existing timestamps are never clobbered.
	for _, rep := range matched {
		if rep.ConfigModel == "" {
			continue
		}
		if ok, err := hugs.MarkRegistered(ctx, s.store.DB(), rep.ConfigModel, now); err == nil && ok {
			out.MetaUpdated = append(out.MetaUpdated, rep.ConfigModel)
		}
	}
	sort.Strings(out.MetaUpdated)
	sort.Strings(out.TagsUpdated)

	writeJSON(w, out)
}

// verifyModelsAgainstHF verifies every configured model against the public HF
// API (via the injectable fetcher) with bounded concurrency, deduplicating
// fetches per unique repo id. Models whose cmd carries an HF repo reference
// are queried by exact repo id. Models without one fall back to HF Hub name
// search over their model id, display name and aliases (ModelSearchNames):
// the best strong match (exact id, exact name segment, or token-boundary
// prefix — see SelectStrongHFMatch) is selected, its metadata fetched, and
// capabilities inferred from the same signals as explicit refs. A model with
// no strong match is reported as unmatched; a model with no searchable
// name/alias at all is reported as no_ref and never touches the network.
func (s *Server) verifyModelsAgainstHF(ctx context.Context, models map[string]config.ModelConfig, refs []hugs.HFConfigRef) *hfVerifySummary {
	fetch := s.hfFetch
	if fetch == nil {
		fetch = fetchHFModelMetaPublic
	}
	search := s.hfSearch
	if search == nil {
		search = searchHFModelsPublic
	}

	// First ref per model is authoritative for the scan.
	refByModel := map[string]hugs.HFConfigRef{}
	for _, ref := range refs {
		if _, ok := refByModel[ref.ModelID]; !ok {
			refByModel[ref.ModelID] = ref
		}
	}

	// Per-model plan: an explicit repo ref, or the searchable names to fall
	// back to. A model with neither ref nor names is a true no_ref.
	type modelPlan struct {
		ref    hugs.HFConfigRef
		hasRef bool
		names  []string
	}
	plans := map[string]modelPlan{}
	for id, mc := range models {
		if ref, ok := refByModel[id]; ok {
			plans[id] = modelPlan{ref: ref, hasRef: true}
		} else {
			plans[id] = modelPlan{names: hugs.ModelSearchNames(id, mc)}
		}
	}

	// Unique explicit repos to fetch (case-insensitive dedupe).
	uniqueRepos := []string{}
	seenRepo := map[string]bool{}
	for _, p := range plans {
		if !p.hasRef {
			continue
		}
		key := strings.ToLower(p.ref.RepoID)
		if !seenRepo[key] {
			seenRepo[key] = true
			uniqueRepos = append(uniqueRepos, p.ref.RepoID)
		}
	}
	sort.Strings(uniqueRepos)

	// Unique search names for fallback discovery (normalized dedupe).
	uniqueNames := []string{}
	seenName := map[string]bool{}
	for _, p := range plans {
		for _, n := range p.names {
			key := hugs.NormalizeSearchName(n)
			if !seenName[key] {
				seenName[key] = true
				uniqueNames = append(uniqueNames, n)
			}
		}
	}
	sort.Strings(uniqueNames)

	// Bounded concurrent network phase: fetch explicit repos and search the
	// unique discovery names. Searches never carry an Authorization header.
	type outcome struct {
		meta   *hugs.HFModelMeta
		status int
		err    error
	}
	metaByRepo := map[string]outcome{} // key: lowercased repo id
	resultsByName := map[string][]hugs.HFSearchResult{}
	searchErrByName := map[string]error{}
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, 4)
	for _, repoID := range uniqueRepos {
		repoID := repoID
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			meta, status, err := fetch(ctx, repoID)
			mu.Lock()
			metaByRepo[strings.ToLower(repoID)] = outcome{meta: meta, status: status, err: err}
			mu.Unlock()
		}()
	}
	for _, name := range uniqueNames {
		name := name
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			results, status, err := search(ctx, name, hfSearchLimit)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				searchErrByName[name] = err
				return
			}
			if status != http.StatusOK {
				searchErrByName[name] = fmt.Errorf("search %q: unexpected http status %d", name, status)
				return
			}
			resultsByName[name] = results
		}()
	}
	wg.Wait()

	// Choose the best discovered repo per no-ref model: lowest match rank
	// across its names (names are ordered model id, display name, aliases;
	// exact beats token-boundary prefix).
	chosenRepo := map[string]string{}
	chosenName := map[string]string{}
	for id, p := range plans {
		if p.hasRef || len(p.names) == 0 {
			continue
		}
		best, bestRank, bestName := "", 99, ""
		for _, n := range p.names {
			if res, rank, ok := hugs.SelectStrongHFMatch(n, resultsByName[n]); ok && rank < bestRank {
				best, bestRank, bestName = res.ID, rank, n
			}
		}
		if best != "" {
			chosenRepo[id] = best
			chosenName[id] = bestName
		}
	}

	// Fetch metadata for the discovered repos, deduplicated against the
	// explicit fetches above (two models can discover the same repo).
	discoveredRepos := []string{}
	seenDiscovered := map[string]bool{}
	for _, repoID := range chosenRepo {
		key := strings.ToLower(repoID)
		if !seenDiscovered[key] {
			seenDiscovered[key] = true
			discoveredRepos = append(discoveredRepos, repoID)
		}
	}
	sort.Strings(discoveredRepos)
	for _, repoID := range discoveredRepos {
		key := strings.ToLower(repoID)
		mu.Lock()
		_, already := metaByRepo[key]
		mu.Unlock()
		if already {
			continue
		}
		repoID := repoID
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			meta, status, err := fetch(ctx, repoID)
			mu.Lock()
			metaByRepo[key] = outcome{meta: meta, status: status, err: err}
			mu.Unlock()
		}()
	}
	wg.Wait()

	// Build one result per configured model, sorted by model id.
	modelIDs := make([]string, 0, len(models))
	for id := range models {
		modelIDs = append(modelIDs, id)
	}
	sort.Strings(modelIDs)

	applyOutcome := func(res *hugs.HFModelResult, oc outcome) {
		res.HTTPStatus = oc.status
		switch {
		case oc.status == http.StatusOK && oc.meta != nil:
			res.Status = "matched"
			res.PipelineTag = oc.meta.PipelineTag
			res.Capabilities, res.Evidence = hugs.InferHFCapabilities(oc.meta)
		case oc.status == http.StatusNotFound:
			res.Status = "unmatched"
			res.Reason = "repo not found on Hugging Face (exact match)"
		case oc.status == http.StatusUnauthorized || oc.status == http.StatusForbidden:
			res.Status = "unauthorized"
			res.Reason = "repo is gated or missing; cannot be verified without a token"
		default:
			res.Status = "error"
			res.Error = fmt.Sprintf("unexpected http status %d", oc.status)
		}
	}

	summary := &hfVerifySummary{Models: []hugs.HFModelResult{}}
	for _, modelID := range modelIDs {
		p := plans[modelID]
		res := hugs.HFModelResult{ModelID: modelID}

		if !p.hasRef && len(p.names) == 0 {
			res.Status = "no_ref"
			res.Reason = "no Hugging Face repo reference in cmd and no searchable name/alias"
			summary.Models = append(summary.Models, res)
			summary.NoRef++
			continue
		}

		if p.hasRef {
			res.RepoID = p.ref.RepoID
			oc, ok := metaByRepo[strings.ToLower(p.ref.RepoID)]
			if !ok {
				res.Status = "error"
				res.Error = "no fetch outcome"
			} else if oc.err != nil {
				res.Status = "error"
				res.Error = oc.err.Error()
			} else {
				applyOutcome(&res, oc)
			}
			summary.Models = append(summary.Models, res)
			countHFResult(summary, res)
			continue
		}

		// Fallback discovery: search already ran; report the verdict.
		repoID, ok := chosenRepo[modelID]
		if !ok {
			if errMsg := firstSearchError(p.names, searchErrByName); errMsg != "" {
				res.Status = "error"
				res.Error = errMsg
			} else {
				res.Status = "unmatched"
				res.Reason = fmt.Sprintf("no strong Hugging Face match for %q", strings.Join(p.names, `", "`))
			}
			summary.Models = append(summary.Models, res)
			countHFResult(summary, res)
			continue
		}
		res.RepoID = repoID
		res.Reason = fmt.Sprintf("discovered by HF Hub search for %q", chosenName[modelID])
		oc, ok := metaByRepo[strings.ToLower(repoID)]
		if !ok {
			res.Status = "error"
			res.Error = "no fetch outcome"
		} else if oc.err != nil {
			res.Status = "error"
			res.Error = oc.err.Error()
		} else {
			applyOutcome(&res, oc)
		}
		summary.Models = append(summary.Models, res)
		countHFResult(summary, res)
	}
	return summary
}

// countHFResult increments the summary counters for one model result.
func countHFResult(summary *hfVerifySummary, res hugs.HFModelResult) {
	switch res.Status {
	case "matched", "unmatched", "unauthorized", "error":
		summary.Checked++
	}
	switch res.Status {
	case "matched":
		summary.Matched++
	case "unmatched":
		summary.Unmatched++
	case "unauthorized":
		summary.Unauthorized++
	case "error":
		summary.Errors++
	case "no_ref":
		summary.NoRef++
	}
}

// firstSearchError returns the first search error for any of the model's
// names, or "" when every search succeeded (even with no strong match).
func firstSearchError(names []string, searchErrByName map[string]error) string {
	for _, n := range names {
		if err := searchErrByName[n]; err != nil {
			return err.Error()
		}
	}
	return ""
}

// persistHFFindings records each queried model's outcome as namespaced hf:*
// tags on its hugs_model_meta row (created via the normal meta path — never
// config). Returns the models whose tags were rewritten. Findings are:
//
//	hf:checked             — model was queried against HF
//	hf:vision/audio/image/tools/mtp — conservative capability findings
//	hf:unmatched           — exact 404 miss
//	hf:unauthorized        — gated or missing, unverifiable tokenless
//	hf:error               — network/timeout/unexpected
//
// no_ref models are left untouched: there is nothing to record.
func (s *Server) persistHFFindings(ctx context.Context, summary *hfVerifySummary) []string {
	if summary == nil {
		return nil
	}
	updated := []string{}
	for _, res := range summary.Models {
		if res.Status == "no_ref" {
			continue
		}
		hfTags := []string{"hf:checked"}
		switch res.Status {
		case "matched":
			if res.Capabilities.Vision {
				hfTags = append(hfTags, "hf:vision")
			}
			if res.Capabilities.Audio {
				hfTags = append(hfTags, "hf:audio")
			}
			if res.Capabilities.Image {
				hfTags = append(hfTags, "hf:image")
			}
			if res.Capabilities.Tools {
				hfTags = append(hfTags, "hf:tools")
			}
			if res.Capabilities.MTP {
				hfTags = append(hfTags, "hf:mtp")
			}
			if res.Capabilities.Uncensored {
				hfTags = append(hfTags, "hf:uncensored")
			}
		case "unmatched":
			hfTags = append(hfTags, "hf:unmatched")
		case "unauthorized":
			hfTags = append(hfTags, "hf:unauthorized")
		case "error":
			hfTags = append(hfTags, "hf:error")
		}
		if _, err := hugs.ApplyHFTags(ctx, s.store.DB(), res.ModelID, hfTags); err != nil {
			s.proxylog.Warnf("hf rescan: record tags for %s: %v", res.ModelID, err)
			continue
		}
		updated = append(updated, res.ModelID)
	}
	return updated
}

// mustJSON marshals v to a JSON string; on the (internal, non-user-facing)
// path where marshaling cannot fail it returns "".
func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(b)
}
