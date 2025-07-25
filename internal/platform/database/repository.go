package database

import (
	"encoding/json"
	"fmt"
	"github.com/gocql/gocql"
	"github.com/goriiin/go-ab-service/pkg/ab_types"
	"time"
)

type Repository struct {
	session *gocql.Session
}

func NewRepository(s *gocql.Session) *Repository {
	return &Repository{session: s}
}

// CreateExperiment сохраняет новый эксперимент в базе данных.
func (r *Repository) CreateExperiment(exp *ab_types.Experiment) error {
	// Сериализуем сложные поля в JSON для хранения
	rules, err := json.Marshal(exp.TargetingRules)
	if err != nil {
		return fmt.Errorf("failed to marshal targeting rules: %w", err)
	}

	overrides, err := json.Marshal(exp.OverrideLists)
	if err != nil {
		return fmt.Errorf("failed to marshal override lists: %w", err)
	}

	variants, err := json.Marshal(exp.Variants)
	if err != nil {
		return fmt.Errorf("failed to marshal variants: %w", err)
	}

	// Сериализуем весь эксперимент для сохранения в outbox
	fullPayload, err := json.Marshal(exp)
	if err != nil {
		return fmt.Errorf("failed to marshal full experiment payload: %w", err)
	}

	// Создаем новый BATCH. LoggedBatch гарантирует атомарность операций внутри него.
	batch := r.session.NewBatch(gocql.LoggedBatch)

	// Операция 1: Запись в основную таблицу 'experiments'
	expQuery := `INSERT INTO experiments (id, layer_id, config_version, end_time, salt, status, targeting_rules, override_lists, variants) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`
	batch.Query(expQuery, exp.ID, exp.LayerID, exp.ConfigVersion, exp.EndTime, exp.Salt, exp.Status, string(rules), string(overrides), string(variants))

	// Операция 2: Запись "намерения отправить" в таблицу 'outbox'
	outboxQuery := `INSERT INTO outbox (event_id, aggregate_id, event_type, payload, created_at) VALUES (?, ?, ?, ?, ?)`
	batch.Query(outboxQuery, gocql.TimeUUID(), exp.ID, "UPSERT", string(fullPayload), time.Now().UTC())

	// Выполняем BATCH атомарно
	if err := r.session.ExecuteBatch(batch); err != nil {
		return fmt.Errorf("failed to execute transactional outbox batch: %w", err)
	}

	return nil
}

// FindExperimentByID находит эксперимент по его ID.
func (r *Repository) FindExperimentByID(id string) (*ab_types.Experiment, error) {
	var exp ab_types.Experiment
	var rules, overrides, variants string
	var endTime time.Time

	query := `SELECT id, layer_id, config_version, end_time, salt, status, targeting_rules, override_lists, variants FROM experiments WHERE id = ? LIMIT 1`
	if err := r.session.Query(query, id).Scan(&exp.ID, &exp.LayerID, &exp.ConfigVersion, &endTime, &exp.Salt, &exp.Status, &rules, &overrides, &variants); err != nil {
		if err == gocql.ErrNotFound {
			return nil, fmt.Errorf("experiment with id %s not found", id)
		}
		return nil, fmt.Errorf("failed to find experiment: %w", err)
	}

	// Десериализуем JSON обратно в структуры
	_ = json.Unmarshal([]byte(rules), &exp.TargetingRules)
	_ = json.Unmarshal([]byte(overrides), &exp.OverrideLists)
	_ = json.Unmarshal([]byte(variants), &exp.Variants)
	if !endTime.IsZero() {
		exp.EndTime = &endTime
	}

	return &exp, nil
}

func (r *Repository) UpdateExperiment(exp *ab_types.Experiment) error {
	// Сериализуем сложные поля в JSON для хранения
	rules, err := json.Marshal(exp.TargetingRules)
	if err != nil {
		return fmt.Errorf("failed to marshal targeting rules: %w", err)
	}

	overrides, err := json.Marshal(exp.OverrideLists)
	if err != nil {
		return fmt.Errorf("failed to marshal override lists: %w", err)
	}

	variants, err := json.Marshal(exp.Variants)
	if err != nil {
		return fmt.Errorf("failed to marshal variants: %w", err)
	}

	// Сериализуем весь эксперимент для сохранения в outbox
	fullPayload, err := json.Marshal(exp)
	if err != nil {
		return fmt.Errorf("failed to marshal full experiment payload: %w", err)
	}

	// Создаем новый BATCH
	batch := r.session.NewBatch(gocql.LoggedBatch)

	// Операция 1: Обновление основной таблицы 'experiments'
	// В CQL UPDATE используется для вставки и обновления, синтаксис идентичен.
	expQuery := `UPDATE experiments SET layer_id = ?, config_version = ?, end_time = ?, salt = ?, status = ?, targeting_rules = ?, override_lists = ?, variants = ? WHERE id = ?`
	batch.Query(expQuery, exp.LayerID, exp.ConfigVersion, exp.EndTime, exp.Salt, exp.Status, string(rules), string(overrides), string(variants), exp.ID)

	// Операция 2: Запись "намерения отправить" в таблицу 'outbox'
	outboxQuery := `INSERT INTO outbox (event_id, aggregate_id, event_type, payload, created_at, processing_state) VALUES (?, ?, ?, ?, ?, ?)`
	batch.Query(outboxQuery, gocql.TimeUUID(), exp.ID, "UPSERT", string(fullPayload), time.Now().UTC(), "PENDING")

	// Выполняем BATCH атомарно
	if err := r.session.ExecuteBatch(batch); err != nil {
		return fmt.Errorf("failed to execute transactional outbox batch for update: %w", err)
	}

	return nil
}
