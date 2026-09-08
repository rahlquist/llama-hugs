package server

// Llama Hugs canonical-registry endpoints: models, ancillary model assets,
// and GPU smoke-test history. Handlers live in this file; routes are
// registered in server.go alongside the other /api/hugs extensions.

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/rahlquist/llama-hugs/internal/hugs"
)

// handleHugsModelsList GET /api/hugs/models
func (s *Server) handleHugsModelsList(w http.ResponseWriter, r *http.Request) {
	models, err := hugs.ListModels(r.Context(), s.store.DB())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, models)
}

// handleHugsModelsGet GET /api/hugs/models/{model}
// 404 when the model has no canonical row (canonical models are explicit;
// nothing is lazy-created here).
func (s *Server) handleHugsModelsGet(w http.ResponseWriter, r *http.Request) {
	modelID := r.PathValue("model")
	m, err := hugs.GetModel(r.Context(), s.store.DB(), modelID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "model not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, m)
}

// handleHugsModelsUpsert POST /api/hugs/models/{model}  body: Model JSON
// Registers or updates one canonical model (merge semantics: empty fields
// leave stored values untouched).
func (s *Server) handleHugsModelsUpsert(w http.ResponseWriter, r *http.Request) {
	modelID := r.PathValue("model")
	var in hugs.Model
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		http.Error(w, "bad json: "+err.Error(), http.StatusBadRequest)
		return
	}
	in.ModelID = modelID
	m, err := hugs.UpsertModel(r.Context(), s.store.DB(), in)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, m)
}

// handleHugsModelsAssetsList GET /api/hugs/models/{model}/assets
func (s *Server) handleHugsModelsAssetsList(w http.ResponseWriter, r *http.Request) {
	assets, err := hugs.ListAssets(r.Context(), s.store.DB(), r.PathValue("model"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, assets)
}

// handleHugsModelsAssetsUpsert POST /api/hugs/models/{model}/assets
// body: Asset JSON (model_id is taken from the path; asset_type required).
// Upsert key: (model_id, asset_type, asset_name). 404 when the model is not
// registered.
func (s *Server) handleHugsModelsAssetsUpsert(w http.ResponseWriter, r *http.Request) {
	modelID := r.PathValue("model")
	var in hugs.Asset
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		http.Error(w, "bad json: "+err.Error(), http.StatusBadRequest)
		return
	}
	in.ModelID = modelID
	a, err := hugs.UpsertAsset(r.Context(), s.store.DB(), in)
	if err != nil {
		if errors.Is(err, hugs.ErrModelNotRegistered) {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, a)
}

// handleHugsModelsSmokeList GET /api/hugs/models/{model}/smoke
func (s *Server) handleHugsModelsSmokeList(w http.ResponseWriter, r *http.Request) {
	tests, err := hugs.ListSmokeTests(r.Context(), s.store.DB(), r.PathValue("model"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, tests)
}

// handleHugsModelsSmokeInsert POST /api/hugs/models/{model}/smoke
// body: SmokeTest JSON (model_id from the path). Append-only history;
// success normalizes to 0/1. 404 when the model is not registered.
func (s *Server) handleHugsModelsSmokeInsert(w http.ResponseWriter, r *http.Request) {
	modelID := r.PathValue("model")
	var in hugs.SmokeTest
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		http.Error(w, "bad json: "+err.Error(), http.StatusBadRequest)
		return
	}
	in.ModelID = modelID
	st, err := hugs.InsertSmokeTest(r.Context(), s.store.DB(), in)
	if err != nil {
		if errors.Is(err, hugs.ErrModelNotRegistered) {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, st)
}
