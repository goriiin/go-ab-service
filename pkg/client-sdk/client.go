package client_sdk

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/goriiin/go-ab-service/internal/platform/queue"
	"github.com/goriiin/go-ab-service/pkg/ab_types"
	"github.com/segmentio/kafka-go"
	"io"
	"log"
	"math/rand"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// InMemoryCache хранит конфигурации экспериментов в памяти для сверхбыстрого доступа.
type InMemoryCache struct {
	// experiments - основное хранилище, индексированное по layerID.
	experiments map[string][]ab_types.Experiment
	// configVersion - последняя версия конфигурации, загруженная в кэш.
	configVersion string
	// rwMutex защищает кэш от одновременной записи и чтения.
	rwMutex sync.RWMutex
}

// Client - основной объект SDK.
type Client struct {
	config Config
	cache  *InMemoryCache
	// minioClient - клиент для работы с MinIO.
	minioClient *minio.Client

	kafkaReader *kafka.Reader
	// cancelFunc для грациозной остановки фонового процесса
	cancelFunc context.CancelFunc

	overrides map[string]string // Карта [experiment_id] -> variant_name
	metrics   *sdkMetrics

	assignmentProducer *queue.Producer // Переиспользуем наш платформенный пакет
}

type AssignmentEvent struct {
	UserID       string         `json:"user_id"`
	ExperimentID string         `json:"experiment_id"`
	VariantName  string         `json:"variant_name"`
	Timestamp    time.Time      `json:"timestamp"`
	Context      map[string]any `json:"context"`
}

// NewClient создает и инициализирует новый клиент A/B-платформы.
// Это блокирующая операция, которая не завершится, пока не будет загружена валидная конфигурация.
func NewClient(ctx context.Context, config Config) (*Client, error) {
	// 1. Добавляем jitter для предотвращения "эффекта толпы"
	jitter := time.Duration(rand.Intn(1000)) * time.Millisecond
	log.Printf("INFO: Applying startup jitter of %v", jitter)
	time.Sleep(jitter)

	// 2. Инициализируем зависимости
	minioClient, err := minio.New(config.MinIOEndpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(config.MinIOAccessKey, config.MinIOSecretKey, ""),
		Secure: config.MinIOUseSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create MinIO client: %w", err)
	}

	internalCtx, cancel := context.WithCancel(context.Background())

	client := &Client{
		config:             config,
		cache:              &InMemoryCache{experiments: make(map[string][]ab_types.Experiment)},
		minioClient:        minioClient,
		cancelFunc:         cancel,
		overrides:          make(map[string]string),
		metrics:            registerMetrics(), // Регистрируем метрики при старте
		assignmentProducer: queue.NewProducer(config.KafkaBrokers, config.AssignmentEventsTopic),
	}

	if client.config.OverridesFilePath != "" {
		overrides, err := client.loadOverrides()
		if err != nil {
			log.Printf("WARN: Could not load overrides file: %v", err)
		} else {
			client.overrides = overrides
			log.Printf("WARN: A/B client started with %d local overrides. THIS SHOULD NOT BE USED IN PRODUCTION.", len(client.overrides))
		}
	}

	// 3. Загружаем начальный снэпшот
	if err := client.loadInitialSnapshot(ctx); err != nil {
		return nil, fmt.Errorf("CRITICAL: failed to load initial configuration: %w", err)
	}

	// TODO: На следующих шагах здесь будет запущен фоновый потребитель Kafka.

	client.initKafkaReader()
	go client.runDeltaConsumer(internalCtx)

	log.Printf("INFO: A/B client initialized successfully with config version %s", client.cache.configVersion)
	return client, nil
}

func (c *Client) Close() error {
	log.Println("INFO: Shutting down A/B client...")
	c.cancelFunc() // Сигнализируем горутине-консьюмеру о необходимости завершиться

	// Закрываем оба соединения с Kafka
	if c.kafkaReader != nil {
		c.kafkaReader.Close()
	}
	if c.assignmentProducer != nil {
		return c.assignmentProducer.Close()
	}
	return nil
}

func (c *Client) initKafkaReader() {
	c.kafkaReader = kafka.NewReader(kafka.ReaderConfig{
		Brokers:  c.config.KafkaBrokers,
		GroupID:  c.config.KafkaGroupID,
		Topic:    "ab_deltas",
		MinBytes: 10e3, // 10KB
		MaxBytes: 10e6, // 10MB
	})
}

// runDeltaConsumer - основной цикл фонового процесса, читающего дельты.
func (c *Client) runDeltaConsumer(ctx context.Context) {
	for {
		select {
		case <-ctx.Done(): // Если контекст отменен, завершаем работу
			log.Println("INFO: Delta consumer shutting down.")
			return
		default:
			msg, err := c.kafkaReader.ReadMessage(ctx)
			if err != nil {
				c.metrics.errors.WithLabelValues("kafka_read_error").Inc()

				// Контекст был отменен во время чтения
				if errors.Is(err, context.Canceled) {
					return
				}
				log.Printf("ERROR: Failed to read delta message from Kafka: %v", err)
				continue
			}

			var exp ab_types.Experiment
			if err := json.Unmarshal(msg.Value, &exp); err != nil {
				c.metrics.errors.WithLabelValues("kafka_read_error").Inc()

				log.Printf("ERROR: Failed to unmarshal delta payload: %v", err)

				continue // Пропускаем битое сообщение
			}

			c.applyDelta(&exp)
		}
	}
}

// applyDelta атомарно применяет изменение к in-memory кэшу.
func (c *Client) applyDelta(exp *ab_types.Experiment) {
	c.cache.rwMutex.Lock()
	defer c.cache.rwMutex.Unlock()

	c.cache.configVersion = exp.ConfigVersion
	c.metrics.setVersionMetric(c.cache.configVersion)

	// Защита от устаревших сообщений: не применяем дельту, если ее версия старше, чем в кэше.
	if exp.ConfigVersion <= c.cache.configVersion {
		log.Printf("WARN: Skipping stale delta for experiment %s (delta version: %s, cache version: %s)", exp.ID, exp.ConfigVersion, c.cache.configVersion)
		return
	}

	// Проверяем, относится ли эксперимент к отслеживаемым слоям.
	useScoping := len(c.config.RelevantLayerIDs) > 0
	if useScoping {
		isRelevant := false
		for _, layerID := range c.config.RelevantLayerIDs {
			if exp.LayerID == layerID {
				isRelevant = true
				break
			}
		}
		if !isRelevant {
			return // Игнорируем дельту для нерелевантного слоя
		}
	}

	layerExperiments := c.cache.experiments[exp.LayerID]
	found := false
	for i, existingExp := range layerExperiments {
		if existingExp.ID == exp.ID {
			layerExperiments[i] = *exp // Обновляем существующий эксперимент
			found = true
			break
		}
	}

	if !found {
		// Добавляем новый эксперимент в слой
		c.cache.experiments[exp.LayerID] = append(layerExperiments, *exp)
	}

	// Обновляем глобальную версию конфигурации
	c.cache.configVersion = exp.ConfigVersion
	log.Printf("INFO: Applied delta for experiment %s. New config version: %s", exp.ID, c.cache.configVersion)

}

// loadInitialSnapshot реализует отказоустойчивую логику загрузки: MinIO -> Local Cache
func (c *Client) loadInitialSnapshot(ctx context.Context) error {
	// Попытка №1: Загрузить из MinIO
	snapshotData, err := c.fetchLatestSnapshotFromMinIO(ctx)
	if err == nil {
		log.Println("INFO: Successfully fetched latest snapshot from MinIO.")
		// Сохраняем свежий снэпшот локально для будущего отката
		if err := os.WriteFile(c.config.LocalCachePath, snapshotData, 0644); err != nil {
			log.Printf("WARN: Failed to save snapshot to local cache: %v", err)
		}
	} else {
		log.Printf("WARN: Failed to fetch snapshot from MinIO: %v. Falling back to local cache.", err)
		// Попытка №2: Загрузить с локального диска
		snapshotData, err = c.loadFromLocalCache()
		if err != nil {
			return fmt.Errorf("MinIO and local cache failed: %w", err)
		}
		log.Printf("INFO: Successfully loaded configuration from local cache file: %s", c.config.LocalCachePath)
	}

	// Если мы здесь, у нас есть данные. Парсим и заполняем кэш.
	return c.populateCacheFromSnapshot(snapshotData)
}

// fetchLatestSnapshotFromMinIO находит и загружает самый последний снэпшот.
func (c *Client) fetchLatestSnapshotFromMinIO(ctx context.Context) ([]byte, error) {
	objectCh := c.minioClient.ListObjects(ctx, c.config.SnapshotBucket, minio.ListObjectsOptions{})
	var objectNames []string
	for object := range objectCh {
		if object.Err != nil {
			return nil, object.Err
		}
		objectNames = append(objectNames, object.Key)
	}

	if len(objectNames) == 0 {
		return nil, errors.New("no snapshots found in bucket")
	}

	// Сортируем имена файлов. Т.к. версия - это UUIDv7, лексикографическая сортировка верна.
	sort.Strings(objectNames)
	latestSnapshotName := objectNames[len(objectNames)-1]
	log.Printf("INFO: Found latest snapshot: %s", latestSnapshotName)

	obj, err := c.minioClient.GetObject(ctx, c.config.SnapshotBucket, latestSnapshotName, minio.GetObjectOptions{})
	if err != nil {
		return nil, err
	}
	defer obj.Close()

	buf := new(bytes.Buffer)
	if _, err := io.Copy(buf, obj); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// loadFromLocalCache загружает снэпшот с диска и проверяет его TTL.
func (c *Client) loadFromLocalCache() ([]byte, error) {
	info, err := os.Stat(c.config.LocalCachePath)
	if os.IsNotExist(err) {
		return nil, errors.New("local cache file does not exist")
	}
	if time.Since(info.ModTime()) > c.config.LocalCacheTTL {
		return nil, fmt.Errorf("local cache is stale (older than %v)", c.config.LocalCacheTTL)
	}
	return os.ReadFile(c.config.LocalCachePath)
}

// populateCacheFromSnapshot парсит JSON и заполняет in-memory кэш.
func (c *Client) populateCacheFromSnapshot(data []byte) error {
	var experiments []ab_types.Experiment
	if err := json.Unmarshal(data, &experiments); err != nil {
		return fmt.Errorf("failed to unmarshal snapshot JSON: %w", err)
	}

	c.cache.rwMutex.Lock()
	defer c.cache.rwMutex.Unlock()

	// Очищаем старый кэш
	c.cache.experiments = make(map[string][]ab_types.Experiment)
	c.cache.configVersion = ""

	useScoping := len(c.config.RelevantLayerIDs) > 0
	relevantLayers := make(map[string]bool)
	for _, id := range c.config.RelevantLayerIDs {
		relevantLayers[id] = true
	}

	loadedCount := 0
	for _, exp := range experiments {
		// Применяем скоупинг, если он настроен
		if useScoping && !relevantLayers[exp.LayerID] {
			continue
		}

		c.cache.experiments[exp.LayerID] = append(c.cache.experiments[exp.LayerID], exp)
		if exp.ConfigVersion > c.cache.configVersion {
			c.cache.configVersion = exp.ConfigVersion
		}
		loadedCount++
	}

	log.Printf("INFO: Populated cache with %d experiments across %d layers.", loadedCount, len(c.cache.experiments))
	c.metrics.setVersionMetric(c.cache.configVersion)

	return nil
}

type DecisionContext struct {
	UserID     string
	Attributes map[string]any
}

// Decide принимает решение для пользователя на основе его ID и атрибутов.
// Возвращает map[experiment_id]variant_name для всех экспериментов, в которые попал пользователь.
func (c *Client) Decide(userID string, attributes map[string]any) map[string]string {
	if userID == "" {
		return map[string]string{}
	}

	assignments := make(map[string]string)

	ctx := &DecisionContext{
		UserID:     userID,
		Attributes: attributes,
	}

	c.cache.rwMutex.RLock() // Блокируем кэш только на чтение
	defer c.cache.rwMutex.RUnlock()

	// Итерируемся по каждому слою в кэше
	for _, experimentsInLayer := range c.cache.experiments {
		for _, exp := range experimentsInLayer {
			// Проверяем, подходит ли пользователь для данного эксперимента
			isAssigned, variantName := c.evaluateExperiment(ctx, &exp)
			if isAssigned {
				assignments[exp.ID] = variantName
				// Ключевой момент: как только пользователь попал в один эксперимент в слое,
				// мы прекращаем обработку этого слоя и переходим к следующему.
				// Это обеспечивает взаимную исключительность.
				break
			}
		}
	}

	if len(c.overrides) > 0 {
		for expID, variantName := range c.overrides {
			assignments[expID] = variantName
		}
	}

	return assignments
}

/*
Пример

	{
	  "exp_unique_id_123": "treatment_A",
	  "another_exp_456": "control"
	}
*/
func (c *Client) loadOverrides() (map[string]string, error) {
	data, err := os.ReadFile(c.config.OverridesFilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read overrides file: %w", err)
	}
	var overrides map[string]string
	if err := json.Unmarshal(data, &overrides); err != nil {
		return nil, fmt.Errorf("failed to parse overrides JSON: %w", err)
	}
	return overrides, nil
}
