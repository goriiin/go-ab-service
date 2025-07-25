package delivery

import (
	"encoding/json"
	"github.com/goriiin/go-ab-service/pkg/ab_types"
	"log"
	"net/http"
	"slices"
	"strconv"
	"time"

	"github.com/cespare/xxhash/v2"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/goriiin/go-ab-service/internal/platform/database"

	"github.com/hashicorp/go-version"
)

// DecisionRequest определяет тело запроса для эндпоинта /decide.
type DecisionRequest struct {
	UserID     string         `json:"user_id"`
	Attributes map[string]any `json:"attributes"`
}

type ExperimentHandler struct {
	repo *database.Repository
}

func NewExperimentHandler(r *database.Repository) *ExperimentHandler {
	return &ExperimentHandler{repo: r}
}

// Decide обрабатывает запрос на получение назначений для пользователя.
func (h *ExperimentHandler) Decide(w http.ResponseWriter, r *http.Request) {
	var req DecisionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	if req.UserID == "" {
		http.Error(w, "user_id is required", http.StatusBadRequest)
		return
	}

	activeExperiments, err := h.repo.FindAllActiveExperiments()
	if err != nil {
		http.Error(w, "Failed to fetch experiments", http.StatusInternalServerError)
		return
	}

	assignments := evaluateUserAssignments(&req, activeExperiments)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(assignments)
}

// evaluateUserAssignments инкапсулирует логику назначения пользователя в эксперименты.
func evaluateUserAssignments(req *DecisionRequest, experiments []ab_types.Experiment) map[string]string {
	assignments := make(map[string]string)
	layers := make(map[string][]ab_types.Experiment)

	// Группировка экспериментов по слоям
	for _, exp := range experiments {
		layers[exp.LayerID] = append(layers[exp.LayerID], exp)
	}

	// Итерация по каждому слою для обеспечения взаимного исключения
	for _, expsInLayer := range layers {
		for _, exp := range expsInLayer {
			isAssigned, variantName := evaluateSingleExperiment(req, &exp)
			if isAssigned {
				assignments[exp.ID] = variantName
				break // Пользователь назначен в один эксперимент в слое, переходим к следующему слою.
			}
		}
	}

	return assignments
}

// evaluateSingleExperiment выполняет полную проверку одного эксперимента для пользователя.
func evaluateSingleExperiment(req *DecisionRequest, exp *ab_types.Experiment) (bool, string) {
	// 1. Предварительная фильтрация
	if exp.Status != ab_types.StatusActive || (exp.EndTime != nil && exp.EndTime.Before(time.Now())) {
		return false, ""
	}

	// 2. Проверка ручных исключений
	if slices.Contains(exp.OverrideLists.ForceExclude, req.UserID) {
		return false, ""
	}

	// 3. Проверка ручных включений (пропускает таргетинг)
	isForceIncluded := slices.Contains(exp.OverrideLists.ForceInclude, req.UserID)
	if !isForceIncluded {
		// 4. Проверка правил таргетинга
		if !checkTargetingRules(req, exp.TargetingRules) {
			return false, ""
		}
	}

	// 5. Финальное распределение (бакетирование)
	hashKey := []byte(req.UserID + exp.Salt)
	bucket := xxhash.Sum64(hashKey) % 1000

	for _, variant := range exp.Variants {
		if bucket >= uint64(variant.BucketRange[0]) && bucket <= uint64(variant.BucketRange[1]) {
			return true, variant.Name
		}
	}

	return false, ""
}

// checkTargetingRules проверяет, удовлетворяет ли пользователь ВСЕМ правилам таргетинга.
func checkTargetingRules(req *DecisionRequest, rules []ab_types.TargetingRule) bool {
	for _, rule := range rules {
		if !evaluateRule(req, &rule) {
			return false
		}
	}
	return true
}

// evaluateRule - ядро логики, проверяющее одно конкретное правило.
func evaluateRule(req *DecisionRequest, rule *ab_types.TargetingRule) bool {
	userValue, ok := req.Attributes[rule.Attribute]
	if !ok {
		return false
	}

	switch rule.Operator {
	case ab_types.OpEquals:
		userStr, ok1 := userValue.(string)
		ruleStr, ok2 := rule.Value.(string)
		return ok1 && ok2 && userStr == ruleStr
	case ab_types.OpGreaterThan:
		userNum, ok1 := toFloat64(userValue)
		ruleNum, ok2 := toFloat64(rule.Value)
		return ok1 && ok2 && userNum > ruleNum
	case ab_types.OpInList:
		userStr, ok1 := userValue.(string)
		ruleList, ok2 := rule.Value.([]interface{})
		if !ok1 || !ok2 {
			return false
		}
		for _, item := range ruleList {
			if strItem, ok := item.(string); ok && strItem == userStr {
				return true
			}
		}
		return false
	case ab_types.OpVersionGreaterThan:
		userVerStr, ok1 := userValue.(string)
		ruleVerStr, ok2 := rule.Value.(string)
		if !ok1 || !ok2 {
			return false
		}
		userV, err1 := version.NewVersion(userVerStr)
		ruleV, err2 := version.NewVersion(ruleVerStr)
		return err1 == nil && err2 == nil && userV.GreaterThan(ruleV)
	default:
		log.Printf("WARN: Unknown operator used: %s", rule.Operator)
		return false
	}
}

func toFloat64(v any) (float64, bool) {
	switch i := v.(type) {
	case float64:
		return i, true
	case int:
		return float64(i), true
	case string:
		f, err := strconv.ParseFloat(i, 64)
		return f, err == nil
	default:
		return 0, false
	}
}

// CreateExperiment обрабатывает запрос на создание эксперимента.
func (h *ExperimentHandler) CreateExperiment(w http.ResponseWriter, r *http.Request) {
	var exp ab_types.Experiment
	if err := json.NewDecoder(r.Body).Decode(&exp); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	exp.ID = uuid.NewString()
	exp.ConfigVersion = uuid.New().String()
	exp.Status = ab_types.StatusDraft
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

// UpdateExperiment обрабатывает обновление эксперимента.
func (h *ExperimentHandler) UpdateExperiment(w http.ResponseWriter, r *http.Request) {
	experimentID := chi.URLParam(r, "experimentID")
	existingExp, err := h.repo.FindExperimentByID(experimentID)
	if err != nil {
		http.Error(w, "Failed to retrieve experiment for update", http.StatusInternalServerError)
		return
	}
	var updatedExp ab_types.Experiment
	if err := json.NewDecoder(r.Body).Decode(&updatedExp); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	updatedExp.ID = existingExp.ID
	updatedExp.Salt = existingExp.Salt
	updatedExp.ConfigVersion = uuid.New().String()
	if err := h.repo.UpdateExperiment(&updatedExp); err != nil {
		http.Error(w, "Failed to update experiment in database", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(updatedExp)
}

// DeleteExperiment обрабатывает физическое удаление эксперимента.
func (h *ExperimentHandler) DeleteExperiment(w http.ResponseWriter, r *http.Request) {
	experimentID := chi.URLParam(r, "experimentID")
	if experimentID == "" {
		http.Error(w, "Experiment ID is required", http.StatusBadRequest)
		return
	}

	if err := h.repo.DeleteExperiment(experimentID); err != nil {
		http.Error(w, "Failed to delete experiment", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
