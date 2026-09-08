package server

// Package server — Llama Hugs extension endpoints.
// Divergence policy: all new handlers live in this file; upstream files only
// gain route registrations.

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/rahlquist/llama-hugs/internal/hugs"
)

// handleHugsMetaList GET /api/hugs/meta
func (s *Server) handleHugsMetaList(w http.ResponseWriter, r *http.Request) {
	metas, err := hugs.ListModelMeta(r.Context(), s.store.DB())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, metas)
}

// handleHugsMetaGet GET /api/hugs/meta/{model}
func (s *Server) handleHugsMetaGet(w http.ResponseWriter, r *http.Request) {
	modelID := r.PathValue("model")
	m, err := hugs.GetModelMeta(r.Context(), s.store.DB(), modelID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, m)
}

// handleHugsMetaUpdate POST /api/hugs/meta/{model}  body: ModelMeta JSON
func (s *Server) handleHugsMetaUpdate(w http.ResponseWriter, r *http.Request) {
	modelID := r.PathValue("model")
	var in hugs.ModelMeta
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		http.Error(w, "bad json: "+err.Error(), http.StatusBadRequest)
		return
	}
	in.ModelID = modelID
	m, err := hugs.UpdateModelMeta(r.Context(), s.store.DB(), in)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, m)
}

// handleHugsDisk GET /api/hugs/disk?dir=...&files=1
// Reports GGUF disk usage. dir defaults to ~/.cache/llama.cpp of the process
// user; only allowed when it resolves under that root (read-only safety).
func (s *Server) handleHugsDisk(w http.ResponseWriter, r *http.Request) {
	root := strings.TrimSpace(r.URL.Query().Get("dir"))
	if root == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			http.Error(w, "no home dir: "+err.Error(), http.StatusInternalServerError)
			return
		}
		root = filepath.Join(home, ".cache", "llama.cpp")
	} else {
		home, _ := os.UserHomeDir()
		allowed := filepath.Join(home, ".cache", "llama.cpp")
		if !strings.HasPrefix(root, allowed) {
			http.Error(w, "dir outside allowed root", http.StatusForbidden)
			return
		}
	}
	includeFiles := r.URL.Query().Get("files") == "1"
	rep, err := hugs.ScanDir(root, includeFiles)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Orphan detection: which on-disk files have no config entry? Config cmd
	// strings reference GGUF paths; build a set from all models' cmds.
	onDisk := map[string]bool{}
	if rep.Files != nil {
		for _, f := range rep.Files {
			onDisk[f.Name] = false // false = not referenced yet
		}
	}
	for _, mc := range s.cfg.Models {
		for name := range onDisk {
			if strings.Contains(mc.Cmd, name) {
				onDisk[name] = true
			}
		}
	}
	orphans := []string{}
	for name, referenced := range onDisk {
		if !referenced {
			orphans = append(orphans, name)
		}
	}

	writeJSON(w, map[string]any{
		"report":  rep,
		"orphans": orphans,
	})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}
