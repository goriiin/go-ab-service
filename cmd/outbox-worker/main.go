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

// OutboxEvent представляет структуру записи в таблице outbox.
type OutboxEvent struct {
	EventID         gocql.UUID `cql:"event_id"`
	AggregateID     string     `cql:"aggregate_id"`
	Payload         string     `cql:"payload"`
	ProcessingState string     `cql:"processing_state"`
}

func main() {
	// Конфигурация (в реальном приложении - из env или файлов)
	const cassandraHost = "cassandra:9042"
	const keyspace = "ab_platform"
	kafkaBrokers := []string{"kafka:9092"}
	const deltasTopic = "ab_deltas"

	// Подключение к зависимостям
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

	// Основной цикл воркера
	ticker := time.NewTicker(5 * time.Second) // Опрашивать каждые 5 секунд
	defer ticker.Stop()

	for range ticker.C {
		processEvents(context.Background(), session, producer)
	}
}

func processEvents(ctx context.Context, session *gocql.Session, producer *queue.Producer) {
	// 1. Найти события, ожидающие обработки
	iter := session.Query(`SELECT event_id, aggregate_id, payload, processing_state FROM outbox WHERE processing_state = ? LIMIT 10 ALLOW FILTERING`, "PENDING").Iter()

	var event OutboxEvent
	// ИСПРАВЛЕННЫЙ ЦИКЛ: Используем iter.Scan() для прямого сканирования в поля структуры.
	// Переменные должны передаваться в том же порядке, что и колонки в SELECT.
	for iter.Scan(&event.EventID, &event.AggregateID, &event.Payload, &event.ProcessingState) {
		// 2. Попытаться "захватить" событие с помощью LWT
		applied, err := session.Query(`UPDATE outbox SET processing_state = ? WHERE event_id = ? IF processing_state = ?`,
			"LOCKED", event.EventID, "PENDING").ScanCAS()
		if err != nil {
			log.Printf("ERROR: Failed to lock event %s: %v", event.EventID, err)
			continue
		}

		if !applied {
			// Другой воркер уже захватил это событие, пропускаем
			log.Printf("INFO: Event %s was locked by another worker, skipping.", event.EventID)
			// Сбрасываем переменную для следующей итерации, чтобы избежать обработки старых данных
			event = OutboxEvent{}
			continue
		}

		log.Printf("INFO: Locked event %s for processing.", event.EventID)

		// 3. Отправить событие в Kafka
		err = producer.Publish(ctx, []byte(event.AggregateID), []byte(event.Payload))
		if err != nil {
			log.Printf("ERROR: Failed to publish event %s to Kafka: %v. Releasing lock.", event.EventID, err)
			// Освободить "замок", чтобы можно было попробовать еще раз
			session.Query(`UPDATE outbox SET processing_state = ? WHERE event_id = ?`, "PENDING", event.EventID).Exec()
			event = OutboxEvent{} // Сброс перед выходом из итерации
			continue
		}

		log.Printf("INFO: Successfully published event for aggregate %s.", event.AggregateID)

		// 4. Удалить обработанное событие из outbox
		if err := session.Query(`DELETE FROM outbox WHERE event_id = ?`, event.EventID).Exec(); err != nil {
			log.Printf("CRITICAL: Failed to delete processed event %s: %v. Manual intervention might be required.", event.EventID, err)
			event = OutboxEvent{} // Сброс перед выходом из итерации
			continue
		}
		log.Printf("INFO: Completed processing for event %s.", event.EventID)

		// Сбросить переменную для следующей итерации
		event = OutboxEvent{}
	}

	// Проверяем на наличие ошибок во время итерации
	if err := iter.Close(); err != nil {
		log.Printf("ERROR: Failed to close iterator: %v", err)
	}
}
