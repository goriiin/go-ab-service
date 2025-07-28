package main

import (
	"context"
	"log"
	"time"

	"github.com/google/uuid"
	"github.com/goriiin/go-ab-service/internal/config"
	"github.com/goriiin/go-ab-service/internal/platform/database"
	"github.com/goriiin/go-ab-service/internal/platform/queue"
	"github.com/jackc/pgx/v5/pgxpool"
)

type OutboxEvent struct {
	EventID     uuid.UUID `json:"event_id"`
	AggregateID string    `json:"aggregate_id"`
	Payload     []byte    `json:"payload"`
}

func main() {
	const deltasTopic = "ab_deltas"
	kafkaBrokers := []string{"kafka:9092"}

	dbCfg := config.NewDBConfig()
	dbPool, err := database.NewPostgresConnection(dbCfg.ConnectionString())
	if err != nil {
		log.Fatalf("FATAL: Failed to connect to PostgreSQL: %v", err)
	}
	defer dbPool.Close()
	log.Println("INFO: Outbox worker connected to PostgreSQL")

	producer := queue.NewProducer(kafkaBrokers, deltasTopic)
	defer producer.Close()
	log.Println("INFO: Outbox worker connected to Kafka")

	log.Println("INFO: Starting outbox processing loop...")
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		processEvents(context.Background(), dbPool, producer)
	}
}

func processEvents(ctx context.Context, pool *pgxpool.Pool, producer *queue.Producer) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		log.Printf("ERROR: could not begin transaction: %v", err)
		return
	}
	defer tx.Rollback(ctx)

	query := `
		SELECT event_id, aggregate_id, payload
		FROM outbox
		WHERE processing_state = 'PENDING'
		ORDER BY created_at
		LIMIT 10
		FOR UPDATE SKIP LOCKED`

	rows, err := tx.Query(ctx, query)
	if err != nil {
		log.Printf("ERROR: could not query outbox events: %v", err)
		return
	}

	var eventsToProcess []OutboxEvent
	var eventIDsToDelete []uuid.UUID

	for rows.Next() {
		var event OutboxEvent
		if err := rows.Scan(&event.EventID, &event.AggregateID, &event.Payload); err != nil {
			log.Printf("ERROR: failed to scan outbox event: %v", err)
			continue
		}
		eventsToProcess = append(eventsToProcess, event)
		eventIDsToDelete = append(eventIDsToDelete, event.EventID)
	}
	rows.Close()

	if len(eventsToProcess) == 0 {
		return
	}

	log.Printf("INFO: Locked %d events for processing.", len(eventsToProcess))

	for _, event := range eventsToProcess {
		err = producer.Publish(ctx, []byte(event.AggregateID), event.Payload)
		if err != nil {
			log.Printf("ERROR: Failed to publish event %s to Kafka: %v. Transaction will be rolled back.", event.EventID, err)
			return
		}
		log.Printf("INFO: Successfully published event for aggregate %s.", event.AggregateID)
	}

	deleteQuery := "DELETE FROM outbox WHERE event_id = ANY($1)"
	cmdTag, err := tx.Exec(ctx, deleteQuery, eventIDsToDelete)
	if err != nil {
		log.Printf("CRITICAL: Failed to delete processed events: %v. Manual intervention might be required.", err)
		return
	}

	if err = tx.Commit(ctx); err != nil {
		log.Printf("ERROR: Failed to commit transaction: %v", err)
		return
	}

	log.Printf("INFO: Completed processing for %d events.", cmdTag.RowsAffected())
}
