package api

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

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
	var input struct {
		Source  string `json:"source"`
		EnvVars string `json:"env_vars"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if input.Source == "" {
		http.Error(w, "source is required", http.StatusBadRequest)
		return
	}
	id := uuid.New().String()
	// Persist the deployment record immediately
	if err := h.store.CreateDeploymentWithEnv(id, input.Source, input.EnvVars); err != nil {
		http.Error(w, "failed to create deployment", http.StatusInternalServerError)
		return
	}

	go h.pipeline.Run(id, input.Source)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]string{
		"id":     id,
		"status": "pending",
	})
}

func (h *Handler) UploadProject(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(100 << 20); err != nil { // 100MB max
		http.Error(w, "file too large", http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "missing file", http.StatusBadRequest)
		return
	}
	defer file.Close()

	envVars := r.FormValue("env_vars")

	id := uuid.New().String()
	uploadDir := filepath.Join("/tmp", "appa-upload", id)
	if err := os.MkdirAll(uploadDir, 0755); err != nil {
		http.Error(w, "failed to create upload dir", http.StatusInternalServerError)
		return
	}
	if err := unzip(file, header.Size, uploadDir); err != nil {
		http.Error(w, fmt.Sprintf("failed to unzip: %v", err), http.StatusBadRequest)
		return
	}
	if err := h.store.CreateDeploymentWithEnv(id, "uploaded-project", envVars); err != nil {
		http.Error(w, "failed to create deployment", http.StatusInternalServerError)
		return
	}

	go h.pipeline.Run(id, uploadDir)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]string{
		"id":     id,
		"status": "pending",
	})
}

func unzip(r io.ReaderAt, size int64, dest string) error {
	reader, err := zip.NewReader(r, size)
	if err != nil {
		return err
	}

	for _, f := range reader.File {
		fpath := filepath.Join(dest, f.Name)

		if f.FileInfo().IsDir() {
			os.MkdirAll(fpath, os.ModePerm)
			continue
		}

		if err := os.MkdirAll(filepath.Dir(fpath), os.ModePerm); err != nil {
			return err
		}

		outFile, err := os.OpenFile(fpath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			return err
		}

		rc, err := f.Open()
		if err != nil {
			outFile.Close()
			return err
		}

		_, err = io.Copy(outFile, rc)
		outFile.Close()
		rc.Close()

		if err != nil {
			return err
		}
	}
	return nil
}

func (h *Handler) CancelDeployment(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		http.Error(w, "missing deployment id", http.StatusBadRequest)
		return
	}

	if err := h.pipeline.Cancel(id); err != nil {
		msg := fmt.Sprintf("failed to cancel deployment: %v", err)
		http.Error(w, msg, http.StatusInternalServerError)
		return
	}

}

func (h *Handler) ListDeployments(w http.ResponseWriter, r *http.Request) {
	deployments, err := h.store.ListDeployments()
	if err != nil {
		http.Error(w, "failed to fetch deployments", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(deployments)
}

func CORSMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}
