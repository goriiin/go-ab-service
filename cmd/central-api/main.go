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
	// В реальном приложении эти значения будут браться из конфигурационных файлов или переменных окружения.
	const (
		cassandraHost = "cassandra:9042" // Имя сервиса из docker-compose
		keyspace      = "ab_platform"
		apiPort       = ":8080"
	)

	// 1. Установка соединения с базой данных
	session, err := database.NewCassandraSession(cassandraHost, keyspace)
	if err != nil {
		log.Fatalf("FATAL: Failed to connect to Cassandra: %v", err)
	}
	defer session.Close()
	log.Println("INFO: Successfully connected to Cassandra")

	// 2. Инициализация слоев приложения
	experimentRepo := database.NewRepository(session)
	experimentHandler := delivery.NewExperimentHandler(experimentRepo)

	// 3. Инициализация роутера с использованием chi
	r := chi.NewRouter()

	// 4. Подключение базовых middleware для надежности и удобства отладки
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)                    // Логирует каждый запрос
	r.Use(middleware.Recoverer)                 // Восстанавливается после паник, возвращая 500 ошибку
	r.Use(middleware.Timeout(60 * time.Second)) // Устанавливает таймаут на запрос

	// 5. Определение маршрутов API

	// Эндпоинт для проверки работоспособности
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		// В будущем можно добавить проверку соединения с Cassandra, например session.Query("SELECT release_version FROM system.local").Exec()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	// Группа маршрутов для управления экспериментами
	r.Route("/experiments", func(r chi.Router) {
		r.Post("/", experimentHandler.CreateExperiment)
		r.Get("/{experimentID}", experimentHandler.GetExperiment)
		r.Put("/{experimentID}", experimentHandler.UpdateExperiment)
	})

	// Добавление эндпоинта для метрик
	r.Handle("/metrics", promhttp.Handler())

	// 6. Запуск HTTP-сервера
	log.Printf("INFO: Starting Central API Service on port %s", apiPort)
	if err := http.ListenAndServe(apiPort, r); err != nil {
		log.Fatalf("FATAL: Failed to start server: %v", err)
	}
}
