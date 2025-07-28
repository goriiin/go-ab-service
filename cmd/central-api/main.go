package main

import (
	"encoding/json"
	"github.com/goriiin/go-ab-service/internal/config"
	"log"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/goriiin/go-ab-service/internal/delivery"
	"github.com/goriiin/go-ab-service/internal/platform/database"
)

func main() {
	const apiPort = ":8080"

	dbCfg := config.NewDBConfig()
	dbPool, err := database.NewPostgresConnection(dbCfg.ConnectionString())
	if err != nil {
		log.Fatalf("FATAL: Failed to connect to PostgreSQL: %v", err)
	}
	defer dbPool.Close()

	repo := database.NewRepository(dbPool)
	handler := delivery.NewExperimentHandler(repo)

	r := chi.NewRouter()
	r.Use(middleware.RequestID, middleware.RealIP, middleware.Logger, middleware.Recoverer)
	r.Use(middleware.Timeout(60 * time.Second))

	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	r.Post("/decide", handler.Decide)

	r.Route("/experiments", func(r chi.Router) {
		r.Post("/", handler.CreateExperiment)
		r.Get("/{experimentID}", handler.GetExperiment)
		r.Put("/{experimentID}", handler.UpdateExperiment)
		r.Delete("/{experimentID}", handler.DeleteExperiment)
	})

	r.Handle("/metrics", promhttp.Handler())

	log.Printf("INFO: Starting Central API Service on port %s", apiPort)
	if err := http.ListenAndServe(apiPort, r); err != nil {
		log.Fatalf("FATAL: Failed to start server: %v", err)
	}
}
