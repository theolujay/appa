package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/theolujay/appa/internal/api"
	"github.com/theolujay/appa/internal/hub"
	"github.com/theolujay/appa/internal/pipeline"
	"github.com/theolujay/appa/internal/store"
)

func main() {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "deployments.db"
	}

	s, err := store.New(dsn)

	if err != nil {
		log.Fatalf("failed to initialise store: %v", err)
	}

	broker := hub.New()
	go broker.Run()
	p := pipeline.New(s, broker)

	// Sync active deployment routes with Caddy on startup
	if err := p.SyncRoutes(); err != nil {
		log.Printf("failed to sync routes: %v", err)
	}

	h := api.New(s, p, broker)

	mux := http.NewServeMux()

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "ok")
	})

	mux.HandleFunc("GET /deployments", h.ListDeployments)
	mux.HandleFunc("POST /deployments", h.CreateDeployment)
	mux.HandleFunc("POST /deployments/upload", h.UploadProject)
	mux.HandleFunc("PATCH /deployments/{id}", h.CancelDeployment)
	mux.HandleFunc("GET /deployments/{id}/logs", h.StreamLogs)

	log.Println("server listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", api.CORSMiddleware(mux)))

}
