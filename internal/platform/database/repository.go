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
	fullPayload, err := json.Marshal(exp)
	if err != nil {
		return fmt.Errorf("failed to marshal full experiment payload: %w", err)
	}
	batch := r.session.NewBatch(gocql.LoggedBatch)
	expQuery := `INSERT INTO experiments (id, layer_id, config_version, end_time, salt, status, targeting_rules, override_lists, variants) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`
	batch.Query(expQuery, exp.ID, exp.LayerID, exp.ConfigVersion, exp.EndTime, exp.Salt, exp.Status, string(rules), string(overrides), string(variants))
	outboxQuery := `INSERT INTO outbox (event_id, aggregate_id, event_type, payload, created_at) VALUES (?, ?, ?, ?, ?)`
	batch.Query(outboxQuery, gocql.TimeUUID(), exp.ID, "UPSERT", string(fullPayload), time.Now().UTC())
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
	_ = json.Unmarshal([]byte(rules), &exp.TargetingRules)
	_ = json.Unmarshal([]byte(overrides), &exp.OverrideLists)
	_ = json.Unmarshal([]byte(variants), &exp.Variants)
	if !endTime.IsZero() {
		exp.EndTime = &endTime
	}
	return &exp, nil
}

// FindAllActiveExperiments находит все активные эксперименты.
func (r *Repository) FindAllActiveExperiments() ([]ab_types.Experiment, error) {
	var experiments []ab_types.Experiment
	query := `SELECT id, layer_id, config_version, end_time, salt, status, targeting_rules, override_lists, variants FROM experiments WHERE status = ? ALLOW FILTERING`

	iter := r.session.Query(query, ab_types.StatusActive).Iter()
	var id, layerID, configVersion, salt, status, targetingRules, overrideLists, variantsStr string
	var endTime time.Time

	for iter.Scan(&id, &layerID, &configVersion, &endTime, &salt, &status, &targetingRules, &overrideLists, &variantsStr) {
		var exp ab_types.Experiment
		exp.ID = id
		exp.LayerID = layerID
		exp.ConfigVersion = configVersion
		exp.Salt = salt
		exp.Status = ab_types.ExperimentStatus(status)
		if !endTime.IsZero() {
			exp.EndTime = &endTime
		}
		_ = json.Unmarshal([]byte(targetingRules), &exp.TargetingRules)
		_ = json.Unmarshal([]byte(overrideLists), &exp.OverrideLists)
		_ = json.Unmarshal([]byte(variantsStr), &exp.Variants)
		experiments = append(experiments, exp)
	}

	if err := iter.Close(); err != nil {
		return nil, fmt.Errorf("failed to iterate over active experiments: %w", err)
	}

	return experiments, nil
}

// UpdateExperiment обновляет существующий эксперимент.
func (r *Repository) UpdateExperiment(exp *ab_types.Experiment) error {
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
	fullPayload, err := json.Marshal(exp)
	if err != nil {
		return fmt.Errorf("failed to marshal full experiment payload: %w", err)
	}
	batch := r.session.NewBatch(gocql.LoggedBatch)
	expQuery := `UPDATE experiments SET layer_id = ?, config_version = ?, end_time = ?, salt = ?, status = ?, targeting_rules = ?, override_lists = ?, variants = ? WHERE id = ?`
	batch.Query(expQuery, exp.LayerID, exp.ConfigVersion, exp.EndTime, exp.Salt, exp.Status, string(rules), string(overrides), string(variants), exp.ID)
	outboxQuery := `INSERT INTO outbox (event_id, aggregate_id, event_type, payload, created_at, processing_state) VALUES (?, ?, ?, ?, ?, ?)`
	batch.Query(outboxQuery, gocql.TimeUUID(), exp.ID, "UPSERT", string(fullPayload), time.Now().UTC(), "PENDING")
	if err := r.session.ExecuteBatch(batch); err != nil {
		return fmt.Errorf("failed to execute transactional outbox batch for update: %w", err)
	}
	return nil
}

// DeleteExperiment физически удаляет эксперимент и записывает событие в outbox.
func (r *Repository) DeleteExperiment(id string) error {
	batch := r.session.NewBatch(gocql.LoggedBatch)

	batch.Query(`DELETE FROM experiments WHERE id = ?`, id)

	deleteEventPayload := fmt.Sprintf(`{"id": "%s"}`, id)
	outboxQuery := `INSERT INTO outbox (event_id, aggregate_id, event_type, payload, created_at, processing_state) VALUES (?, ?, ?, ?, ?, ?)`
	batch.Query(outboxQuery, gocql.TimeUUID(), id, "DELETE", deleteEventPayload, time.Now().UTC(), "PENDING")

	if err := r.session.ExecuteBatch(batch); err != nil {
		return fmt.Errorf("failed to execute transactional delete batch: %w", err)
	}

	return nil
}
