package client_sdk

import "time"

// Config содержит все параметры, необходимые для инициализации и работы клиентской библиотеки.
type Config struct {
	// A list of layer IDs this client instance cares about.
	// Если список пуст, будут загружены все эксперименты.
	// Это ключевой механизм для скоупинга и экономии памяти.
	RelevantLayerIDs []string

	// Kafka configuration for receiving deltas
	KafkaBrokers []string
	KafkaGroupID string // Уникальный ID для группы потребителей

	// MinIO configuration for fetching snapshots
	MinIOEndpoint  string
	MinIOAccessKey string
	MinIOSecretKey string
	MinIOUseSSL    bool
	SnapshotBucket string

	// Local fallback cache settings
	LocalCachePath string        // Путь к файлу для кэширования снэпшота на диске
	LocalCacheTTL  time.Duration // Максимальное время жизни локального кэша

	OverridesFilePath string

	AssignmentEventsTopic string
}
