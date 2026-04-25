package api

import (
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
	"github.com/theolujay/appa/internal/hub"
	"github.com/theolujay/appa/internal/pipeline"
	"github.com/theolujay/appa/internal/store"
)

type Handler struct {
	store    *store.Store
	pipeline *pipeline.Pipeline
	hub      *hub.Hub
}

func New(s *store.Store, p *pipeline.Pipeline, h *hub.Hub) *Handler {
	return &Handler{
		store:    s,
		pipeline: p,
		hub:      h,
	}
}

func (h *Handler) CreateDeployment(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Source string `json:"source"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if body.Source == "" {
		http.Error(w, "source is required", http.StatusBadRequest)
		return
	}
	id := uuid.New().String()
	// Persist the deployment record immediately
	// Pipeline hasn't started yet -- we're only recording intent
	if err := h.store.CreateDeployment(id, body.Source); err != nil {
		http.Error(w, "failed to create deployment", http.StatusInternalServerError)
		return
	}
	// Launch the pipeline in a goroutine.
	go h.pipeline.Run(id, body.Source)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]string{
		"id":     id,
		"status": "pending",
	})
}
