package database

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/goriiin/go-ab-service/pkg/ab_types"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(p *pgxpool.Pool) *Repository {
	return &Repository{pool: p}
}

// CreateExperiment сохраняет новый эксперимент и событие в outbox в одной транзакции.
func (r *Repository) CreateExperiment(exp *ab_types.Experiment) error {
	fullPayload, err := json.Marshal(exp)
	if err != nil {
		return fmt.Errorf("failed to marshal full experiment payload: %w", err)
	}

	tx, err := r.pool.Begin(context.Background())
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(context.Background())

	expQuery := `
		INSERT INTO experiments (id, layer_id, config_version, end_time, salt, status, targeting_rules, override_lists, variants)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`
	_, err = tx.Exec(context.Background(), expQuery,
		exp.ID, exp.LayerID, exp.ConfigVersion, exp.EndTime, exp.Salt, exp.Status,
		exp.TargetingRules, exp.OverrideLists, exp.Variants)
	if err != nil {
		return fmt.Errorf("failed to insert experiment: %w", err)
	}

	outboxQuery := `
		INSERT INTO outbox (event_id, aggregate_id, event_type, payload, created_at, processing_state)
		VALUES ($1, $2, $3, $4, $5, 'PENDING')`
	_, err = tx.Exec(context.Background(), outboxQuery,
		uuid.New(), exp.ID, "UPSERT", fullPayload, time.Now().UTC())
	if err != nil {
		return fmt.Errorf("failed to insert outbox event: %w", err)
	}

	return tx.Commit(context.Background())
}

// FindExperimentByID находит эксперимент по его ID.
func (r *Repository) FindExperimentByID(id string) (*ab_types.Experiment, error) {
	var exp ab_types.Experiment

	query := `
		SELECT id, layer_id, config_version, end_time, salt, status, targeting_rules, override_lists, variants
		FROM experiments WHERE id = $1 LIMIT 1`

	err := r.pool.QueryRow(context.Background(), query, id).Scan(
		&exp.ID, &exp.LayerID, &exp.ConfigVersion, &exp.EndTime, &exp.Salt, &exp.Status,
		&exp.TargetingRules, &exp.OverrideLists, &exp.Variants)

	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("experiment with id %s not found", id)
		}
		return nil, fmt.Errorf("failed to find experiment: %w", err)
	}

	return &exp, nil
}

// FindAllActiveExperiments находит все активные эксперименты.
func (r *Repository) FindAllActiveExperiments() ([]ab_types.Experiment, error) {
	var experiments []ab_types.Experiment
	query := `
		SELECT id, layer_id, config_version, end_time, salt, status, targeting_rules, override_lists, variants
		FROM experiments WHERE status = $1`

	rows, err := r.pool.Query(context.Background(), query, ab_types.StatusActive)
	if err != nil {
		return nil, fmt.Errorf("failed to query active experiments: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var exp ab_types.Experiment
		err := rows.Scan(
			&exp.ID, &exp.LayerID, &exp.ConfigVersion, &exp.EndTime, &exp.Salt, &exp.Status,
			&exp.TargetingRules, &exp.OverrideLists, &exp.Variants)
		if err != nil {
			return nil, fmt.Errorf("failed to scan experiment row: %w", err)
		}
		experiments = append(experiments, exp)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating over active experiments: %w", err)
	}

	return experiments, nil
}

// UpdateExperiment обновляет существующий эксперимент и событие в outbox в одной транзакции.
func (r *Repository) UpdateExperiment(exp *ab_types.Experiment) error {
	fullPayload, err := json.Marshal(exp)
	if err != nil {
		return fmt.Errorf("failed to marshal full experiment payload: %w", err)
	}

	tx, err := r.pool.Begin(context.Background())
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(context.Background())

	expQuery := `
		UPDATE experiments
		SET layer_id = $1, config_version = $2, end_time = $3, salt = $4, status = $5,
		    targeting_rules = $6, override_lists = $7, variants = $8
		WHERE id = $9`
	_, err = tx.Exec(context.Background(), expQuery,
		exp.LayerID, exp.ConfigVersion, exp.EndTime, exp.Salt, exp.Status,
		exp.TargetingRules, exp.OverrideLists, exp.Variants, exp.ID)
	if err != nil {
		return fmt.Errorf("failed to update experiment: %w", err)
	}

	outboxQuery := `
		INSERT INTO outbox (event_id, aggregate_id, event_type, payload, created_at, processing_state)
		VALUES ($1, $2, $3, $4, $5, 'PENDING')`
	_, err = tx.Exec(context.Background(), outboxQuery,
		uuid.New(), exp.ID, "UPSERT", fullPayload, time.Now().UTC())
	if err != nil {
		return fmt.Errorf("failed to insert outbox event for update: %w", err)
	}

	return tx.Commit(context.Background())
}

// DeleteExperiment удаляет эксперимент и записывает событие в outbox в одной транзакции.
func (r *Repository) DeleteExperiment(id string) error {
	tx, err := r.pool.Begin(context.Background())
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(context.Background())

	tag, err := tx.Exec(context.Background(), `DELETE FROM experiments WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("failed to execute delete on experiment: %w", err)
	}

	if tag.RowsAffected() == 0 {
		return fmt.Errorf("experiment not found")
	}

	deleteEventPayload, err := json.Marshal(map[string]string{"id": id})
	if err != nil {
		return fmt.Errorf("failed to marshal delete event payload: %w", err)
	}

	outboxQuery := `
		INSERT INTO outbox (event_id, aggregate_id, event_type, payload, created_at, processing_state)
		VALUES ($1, $2, $3, $4, $5, 'PENDING')`
	_, err = tx.Exec(context.Background(), outboxQuery,
		uuid.New(), id, "DELETE", deleteEventPayload, time.Now().UTC())
	if err != nil {
		return fmt.Errorf("failed to insert delete event into outbox: %w", err)
	}

	return tx.Commit(context.Background())
}
