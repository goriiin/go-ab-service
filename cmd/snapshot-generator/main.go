package main

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/goriiin/go-ab-service/internal/platform/database"
	"github.com/goriiin/go-ab-service/internal/platform/queue"
	"github.com/goriiin/go-ab-service/internal/platform/storage"
	"github.com/goriiin/go-ab-service/pkg/ab_types"
	"log"
	"time"
)

// SnapshotMeta описывает сообщение, отправляемое в Kafka после создания снэпшота.
type SnapshotMeta struct {
	SnapshotVersion string `json:"snapshot_version"`
	Path            string `json:"path"`
	CreatedAt       string `json:"created_at"`
}

func main() {
	// Конфигурация
	const cassandraHost = "cassandra:9042"
	const keyspace = "ab_platform"
	kafkaBrokers := []string{"kafka:9092"}
	const snapshotsTopic = "ab_snapshots_meta"
	const minioEndpoint = "minio:9000"
	const minioAccessKey = "minioadmin"
	const minioSecretKey = "minioadmin"
	const snapshotBucket = "ab-snapshots"

	// 1. Инициализация зависимостей
	session, err := database.NewCassandraSession(cassandraHost, keyspace)
	if err != nil {
		log.Fatalf("FATAL: Cannot connect to Cassandra: %v", err)
	}
	defer session.Close()

	minioClient, err := storage.NewMinIOClient(minioEndpoint, minioAccessKey, minioSecretKey, false)
	if err != nil {
		log.Fatalf("FATAL: Cannot connect to MinIO: %v", err)
	}

	producer := queue.NewProducer(kafkaBrokers, snapshotsTopic)
	defer producer.Close()

	log.Println("INFO: Starting snapshot generation process...")

	// 2. Получение всех активных экспериментов
	iter := session.Query(`SELECT id, layer_id, config_version, end_time, salt, status, targeting_rules, override_lists, variants FROM experiments WHERE status = ? ALLOW FILTERING`, ab_types.StatusActive).Iter()

	var experiments []ab_types.Experiment
	var latestVersion string

	var id, layerID, configVersion, salt, status, targetingRules, overrideLists, variants string
	var endTime time.Time

	// ИСПРАВЛЕННЫЙ ЦИКЛ: Используем iter.Scan() для чтения данных в переменные.
	for iter.Scan(&id, &layerID, &configVersion, &endTime, &salt, &status, &targetingRules, &overrideLists, &variants) {
		var exp ab_types.Experiment
		exp.ID = id
		exp.LayerID = layerID
		exp.ConfigVersion = configVersion
		exp.Salt = salt
		exp.Status = ab_types.ExperimentStatus(status)

		if !endTime.IsZero() {
			exp.EndTime = &endTime
		}

		// Десериализуем JSON-строки в соответствующие структуры
		if err := json.Unmarshal([]byte(targetingRules), &exp.TargetingRules); err != nil {
			log.Printf("WARN: Failed to unmarshal targeting_rules for exp %s: %v", exp.ID, err)
		}
		if err := json.Unmarshal([]byte(overrideLists), &exp.OverrideLists); err != nil {
			log.Printf("WARN: Failed to unmarshal override_lists for exp %s: %v", exp.ID, err)
		}
		if err := json.Unmarshal([]byte(variants), &exp.Variants); err != nil {
			log.Printf("WARN: Failed to unmarshal variants for exp %s: %v", exp.ID, err)
		}

		experiments = append(experiments, exp)

		// Находим самую последнюю версию конфигурации в снэпшоте
		if exp.ConfigVersion > latestVersion {
			latestVersion = exp.ConfigVersion
		}
	}

	if err := iter.Close(); err != nil {
		log.Fatalf("FATAL: Failed to iterate over experiments: %v", err)
	}

	if len(experiments) == 0 {
		log.Println("INFO: No active experiments found. Snapshot not generated.")
		return
	}
	log.Printf("INFO: Found %d active experiments to include in snapshot.", len(experiments))

	// 3. Формирование и загрузка снэпшота
	snapshotData, err := json.Marshal(experiments)
	if err != nil {
		log.Fatalf("FATAL: Failed to marshal experiments to JSON: %v", err)
	}

	objectName := "snapshot-" + latestVersion + ".json"
	_, err = minioClient.Upload(context.Background(), snapshotBucket, objectName, "application/json", bytes.NewReader(snapshotData), int64(len(snapshotData)))
	if err != nil {
		log.Fatalf("FATAL: Failed to upload snapshot to MinIO: %v", err)
	}
	log.Printf("INFO: Successfully uploaded snapshot '%s' to bucket '%s'.", objectName, snapshotBucket)

	// 4. Отправка уведомления в Kafka
	meta := SnapshotMeta{
		SnapshotVersion: latestVersion,
		Path:            objectName,
		CreatedAt:       time.Now().UTC().Format(time.RFC3339),
	}
	metaData, _ := json.Marshal(meta)

	err = producer.Publish(context.Background(), []byte(latestVersion), metaData)
	if err != nil {
		log.Fatalf("FATAL: Failed to publish snapshot metadata to Kafka: %v", err)
	}

	log.Printf("INFO: Successfully published snapshot metadata for version %s.", latestVersion)
	log.Println("INFO: Snapshot generation process completed successfully.")
}
