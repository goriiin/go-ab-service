package delivery

import (
	"encoding/json"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/goriiin/go-ab-service/internal/platform/database"
	"github.com/goriiin/go-ab-service/pkg/ab_types"
	"net/http"
)

type ExperimentHandler struct {
	repo *database.Repository
}

func NewExperimentHandler(r *database.Repository) *ExperimentHandler {
	return &ExperimentHandler{repo: r}
}

// CreateExperiment обрабатывает запрос на создание эксперимента.
func (h *ExperimentHandler) CreateExperiment(w http.ResponseWriter, r *http.Request) {
	var exp ab_types.Experiment
	if err := json.NewDecoder(r.Body).Decode(&exp); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Заполняем системные поля
	exp.ID = uuid.NewString()
	exp.ConfigVersion = uuid.New().String() // Используем UUIDv7, который гарантирует уникальность и хронологический порядок
	exp.Status = ab_types.StatusDraft       // Новые эксперименты всегда создаются как черновики

	// Для простоты, Salt тоже генерируется автоматически
	exp.Salt = uuid.NewString()

	if err := h.repo.CreateExperiment(&exp); err != nil {
		http.Error(w, "Failed to create experiment in database", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(exp)
}

// GetExperiment обрабатывает запрос на получение эксперимента.
func (h *ExperimentHandler) GetExperiment(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "experimentID")

	exp, err := h.repo.FindExperimentByID(id)
	if err != nil {
		// Проверяем, является ли ошибка "не найдено"
		if err.Error() == "experiment with id "+id+" not found" {
			http.Error(w, err.Error(), http.StatusNotFound)
		} else {
			http.Error(w, "Failed to retrieve experiment", http.StatusInternalServerError)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(exp)
}

func (h *ExperimentHandler) UpdateExperiment(w http.ResponseWriter, r *http.Request) {
	experimentID := chi.URLParam(r, "experimentID")
	if experimentID == "" {
		http.Error(w, "Experiment ID is required", http.StatusBadRequest)
		return
	}

	// Сначала найдем существующий эксперимент, чтобы убедиться, что он есть
	existingExp, err := h.repo.FindExperimentByID(experimentID)
	if err != nil {
		if err.Error() == "experiment with id "+experimentID+" not found" {
			http.Error(w, err.Error(), http.StatusNotFound)
		} else {
			http.Error(w, "Failed to retrieve experiment for update", http.StatusInternalServerError)
		}
		return
	}

	var updatedExp ab_types.Experiment
	if err := json.NewDecoder(r.Body).Decode(&updatedExp); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Сохраняем системные поля, которые не должны меняться через этот эндпоинт
	updatedExp.ID = existingExp.ID     // ID не меняется
	updatedExp.Salt = existingExp.Salt // Salt не меняется после создания

	// Генерируем новую версию конфигурации
	updatedExp.ConfigVersion = uuid.New().String()

	if err := h.repo.UpdateExperiment(&updatedExp); err != nil {
		http.Error(w, "Failed to update experiment in database", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(updatedExp)
}
