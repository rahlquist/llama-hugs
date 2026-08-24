package server

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"

	"github.com/rahlquist/llama-hugs/internal/hugs"
)

// hugsModelsCacheRoot resolves the allowed file-operation root (the model
// cache dir of the process user).
func hugsModelsCacheRoot() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".cache", "llama.cpp")
}

// handleHugsFileFlagsGet GET /api/hugs/files/flags → {"flags": [name...]}
func (s *Server) handleHugsFileFlagsGet(w http.ResponseWriter, r *http.Request) {
	flags, err := hugs.ListFileFlags(r.Context(), s.store.DB())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{"flags": flags})
}

// handleHugsFileFlagsSet POST /api/hugs/files/flags
// Body: {"names": ["a.gguf", ...], "flagged": true|false}
// Names are bare filenames inside the models cache root.
func (s *Server) handleHugsFileFlagsSet(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Names   []string `json:"names"`
		Flagged bool     `json:"flagged"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || len(in.Names) == 0 {
		http.Error(w, `body must be {"names": [...], "flagged": bool}`, http.StatusBadRequest)
		return
	}
	if err := hugs.SetFileFlags(r.Context(), s.store.DB(), in.Names, in.Flagged); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	flags, _ := hugs.ListFileFlags(r.Context(), s.store.DB())
	writeJSON(w, map[string]any{"flags": flags})
}

// handleHugsFilesDelete POST /api/hugs/files/delete
// Body: {"paths": ["/home/.../.cache/llama.cpp/a.gguf", ...]}
// Irreversible. Every path must be a regular .gguf directly under the cache
// root; anything else is refused per-file and nothing outside is touched.
func (s *Server) handleHugsFilesDelete(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Paths []string `json:"paths"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || len(in.Paths) == 0 {
		http.Error(w, `body must be {"paths": [...]}`, http.StatusBadRequest)
		return
	}
	root := hugsModelsCacheRoot()
	deleted, errs := hugs.DeleteFiles(r.Context(), s.store.DB(), root, in.Paths)
	status := http.StatusOK
	if len(errs) > 0 && len(deleted) == 0 {
		status = http.StatusBadRequest
	}
	errMsgs := make([]string, len(errs))
	for i, e := range errs {
		errMsgs[i] = e.Error()
	}
	w.WriteHeader(status)
	writeJSON(w, map[string]any{"deleted": deleted, "errors": errMsgs})
}
