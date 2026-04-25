package main

import (
	"fmt"
	"log"
	"net/http"

	"github.com/theolujay/appa/internal/api"
	"github.com/theolujay/appa/internal/hub"
	"github.com/theolujay/appa/internal/pipeline"
	"github.com/theolujay/appa/internal/store"
)

func main() {

	s, err := store.New("deployments.db")
	if err != nil {
		log.Fatalf("failed to initialise store: %v", err)
	}

	broker := hub.New()
	go broker.Run()
	p := pipeline.New(s, broker)
	h := api.New(s, p, broker)

	mux := http.NewServeMux()

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "ok")
	})

	mux.HandleFunc("POST /deployments", h.CreateDeployment)
	mux.HandleFunc("GET /deployments/{id}/logs", h.StreamLogs)

	log.Println("server listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", mux))

}
