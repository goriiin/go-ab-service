// File: cmd/central-api/main.go
package main

import (
	"encoding/json"
	"github.com/goriiin/go-ab-service/internal/delivery"
	"github.com/goriiin/go-ab-service/internal/platform/database"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"log"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func main() {
	const (
		cassandraHost = "cassandra:9042"
		keyspace      = "ab_platform"
		apiPort       = ":8080"
	)

	session, err := database.NewCassandraSession(cassandraHost, keyspace)
	if err != nil {
		log.Fatalf("FATAL: Failed to connect to Cassandra: %v", err)
	}
	defer session.Close()
	log.Println("INFO: Successfully connected to Cassandra")

	experimentRepo := database.NewRepository(session)
	experimentHandler := delivery.NewExperimentHandler(experimentRepo)

	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)                    // Логирует каждый запрос
	r.Use(middleware.Recoverer)                 // Восстанавливается после паник, возвращая 500 ошибку
	r.Use(middleware.Timeout(60 * time.Second)) // Устанавливает таймаут на запрос

	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	// Новый эндпоинт для принятия решений
	r.Post("/decide", experimentHandler.Decide)

	r.Route("/experiments", func(r chi.Router) {
		r.Post("/", experimentHandler.CreateExperiment)
		r.Get("/{experimentID}", experimentHandler.GetExperiment)
		r.Put("/{experimentID}", experimentHandler.UpdateExperiment)
		r.Delete("/{experimentID}", experimentHandler.DeleteExperiment) // Добавлен маршрут для удаления
	})

	r.Handle("/metrics", promhttp.Handler())

	log.Printf("INFO: Starting Central API Service on port %s", apiPort)
	if err = http.ListenAndServe(apiPort, r); err != nil {
		log.Fatalf("FATAL: Failed to start server: %v", err)
	}
}
