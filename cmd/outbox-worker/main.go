// File: cmd/outbox-worker/main.go
package main

import (
	"context"
	"github.com/goriiin/go-ab-service/internal/platform/database"
	"github.com/goriiin/go-ab-service/internal/platform/queue"
	"log"
	"time"

	"github.com/gocql/gocql"
)

type OutboxEvent struct {
	EventID         gocql.UUID `cql:"event_id"`
	AggregateID     string     `cql:"aggregate_id"`
	Payload         string     `cql:"payload"`
	ProcessingState string     `cql:"processing_state"`
}

func main() {
	const (
		cassandraHost = "cassandra:9042"
		keyspace      = "ab_platform"
		deltasTopic   = "ab_deltas"
	)

	kafkaBrokers := []string{"kafka:9092"}

	session, err := database.NewCassandraSession(cassandraHost, keyspace)
	if err != nil {
		log.Fatalf("FATAL: Failed to connect to Cassandra: %v", err)
	}
	defer session.Close()

	log.Println("INFO: Outbox worker connected to Cassandra")

	producer := queue.NewProducer(kafkaBrokers, deltasTopic)
	defer producer.Close()

	log.Println("INFO: Outbox worker connected to Kafka")

	log.Println("INFO: Starting outbox processing loop...")

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		processEvents(context.Background(), session, producer)
	}
}

func processEvents(ctx context.Context, session *gocql.Session, producer *queue.Producer) {
	iter := session.Query(`SELECT event_id, aggregate_id, payload, processing_state FROM outbox WHERE processing_state = ? LIMIT 10 ALLOW FILTERING`, "PENDING").Iter()

	var event OutboxEvent
	for iter.Scan(&event.EventID, &event.AggregateID, &event.Payload, &event.ProcessingState) {
		applied, err := session.Query(`UPDATE outbox SET processing_state = ? WHERE event_id = ? IF processing_state = ?`,
			"LOCKED", event.EventID, "PENDING").ScanCAS()
		if err != nil {
			log.Printf("ERROR: Failed to lock event %s: %v", event.EventID, err)
			continue
		}

		if !applied {
			log.Printf("INFO: Event %s was locked by another worker, skipping.", event.EventID)
			event = OutboxEvent{}

			continue
		}

		log.Printf("INFO: Locked event %s for processing.", event.EventID)

		err = producer.Publish(ctx, []byte(event.AggregateID), []byte(event.Payload))
		if err != nil {
			log.Printf("ERROR: Failed to publish event %s to Kafka: %v. Releasing lock.", event.EventID, err)
			session.Query(`UPDATE outbox SET processing_state = ? WHERE event_id = ?`, "PENDING", event.EventID).Exec()
			event = OutboxEvent{} // Сброс перед выходом из итерации
			continue
		}

		log.Printf("INFO: Successfully published event for aggregate %s.", event.AggregateID)

		if err = session.Query(
			`DELETE FROM outbox WHERE event_id = ?`,
			event.EventID).
			Exec(); err != nil {

			log.Printf("CRITICAL: Failed to delete processed event %s: %v. Manual intervention might be required.", event.EventID, err)
			event = OutboxEvent{}

			continue
		}

		log.Printf("INFO: Completed processing for event %s.", event.EventID)

		event = OutboxEvent{}
	}

	if err := iter.Close(); err != nil {
		log.Printf("ERROR: Failed to close iterator: %v", err)
	}
}
