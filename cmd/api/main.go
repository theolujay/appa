package main

import (
	"flag"
	"log"
	"net/http"
	"os"

	"github.com/theolujay/appa/internal/hub"
	"github.com/theolujay/appa/internal/pipeline"
	"github.com/theolujay/appa/internal/store"
)

func main() {
	addr := flag.String("addr", ":8080", "HTTP network address")
	dsn := flag.String("dsn", "/data/deployments.db", "SQLite data source name")
	flag.Parse()

	infoLog := log.New(os.Stdout, "INFO\t", log.Ldate|log.Ltime|log.LUTC)
	errorLog := log.New(os.Stderr, "ERROR\t", log.Ldate|log.Ltime|log.LUTC|log.Lshortfile)

	store, err := store.New(*dsn)
	if err != nil {
		errorLog.Fatalf("failed to initialise store: %v", err)
	}

	hub := hub.New()
	go hub.Run()

	pipeline := pipeline.New(store, hub)
	// Sync active deployment routes with Caddy on startup
	if err := pipeline.SyncRoutes(); err != nil {
		errorLog.Printf("failed to sync routes: %v", err)
	}

	app := &application{
		errorLog: errorLog,
		infoLog:  infoLog,
		store:    store,
		pipeline: pipeline,
		hub:      hub,
	}

	srv := &http.Server{
		Addr:     *addr,
		ErrorLog: errorLog,
		Handler:  CORSMiddleware(app.routes()),
	}

	infoLog.Printf("Starting server on %s", *addr)
	err = srv.ListenAndServe()
	errorLog.Fatal(err)

}
