// File: pkg/ab_types/models.go

package ab_types

import "time"

// ExperimentStatus определяет возможные статусы эксперимента.
type ExperimentStatus string

const (
	StatusDraft    ExperimentStatus = "DRAFT"
	StatusActive   ExperimentStatus = "ACTIVE"
	StatusPaused   ExperimentStatus = "PAUSED"
	StatusFinished ExperimentStatus = "FINISHED"
)

// Operator определяет операторы, используемые в правилах таргетинга.
type Operator string

const (
	// Строковые операторы
	OpEquals      Operator = "EQUALS"
	OpNotEquals   Operator = "NOT_EQUALS"
	OpContains    Operator = "CONTAINS"
	OpNotContains Operator = "NOT_CONTAINS"

	// Числовые операторы
	OpGreaterThan        Operator = "GREATER_THAN"
	OpLessThan           Operator = "LESS_THAN"
	OpGreaterThanOrEqual Operator = "GREATER_THAN_OR_EQUAL"
	OpLessThanOrEqual    Operator = "LESS_THAN_OR_EQUAL"

	// Операторы для семантических версий (SemVer)
	OpVersionGreaterThan Operator = "VERSION_GREATER_THAN"
	OpVersionLessThan    Operator = "VERSION_LESS_THAN"
	OpVersionEquals      Operator = "VERSION_EQUALS"

	// Операторы для списков/массивов
	OpInList    Operator = "IN_LIST"
	OpNotInList Operator = "NOT_IN_LIST"
)

// Experiment представляет полную конфигурацию A/B-эксперимента.
// Эта структура является основным контрактом данных в системе.
type Experiment struct {
	// ID - уникальный, неизменяемый идентификатор эксперимента.
	ID string `json:"id"`

	// LayerID - идентификатор слоя для управления взаимоисключением.
	LayerID string `json:"layer_id"`

	// ConfigVersion - версия конфигурации (UUIDv7), обеспечивает хронологический порядок.
	ConfigVersion string `json:"config_version"`

	// EndTime - время автоматического завершения эксперимента (опционально).
	EndTime *time.Time `json:"end_time,omitempty"`

	// Salt - уникальная строка для хеширования, обеспечивает статистическую независимость.
	Salt string `json:"salt"`

	// Status - текущий жизненный цикл эксперимента.
	Status ExperimentStatus `json:"status"`

	// TargetingRules - массив правил для выборки аудитории.
	// Пользователь должен удовлетворять ВСЕМ правилам.
	TargetingRules []TargetingRule `json:"targeting_rules"`

	// OverrideLists - списки для ручного включения или исключения пользователей.
	OverrideLists OverrideLists `json:"override_lists"`

	// Variants - массив вариантов (групп) теста.
	Variants []Variant `json:"variants"`
}

// TargetingRule определяет одно правило для таргетинга.
type TargetingRule struct {
	// Attribute - атрибут пользователя для проверки (например, "country", "app_version").
	Attribute string `json:"attribute"`
	// Operator - операция для сравнения.
	Operator Operator `json:"operator"`
	// Value - значение, с которым сравнивается атрибут пользователя.
	Value any `json:"value"`
}

// OverrideLists содержит списки пользователей для принудительного включения/исключения.
// На основе ответа на вопрос №6, логика ForceInclude теперь означает 100% участие.
type OverrideLists struct {
	// ForceInclude - список ID пользователей для принудительного включения в эксперимент.
	// Эти пользователи пропускают проверку правил таргетинга и сразу переходят к бакетированию.
	ForceInclude []string `json:"force_include"`

	// ForceExclude - список ID пользователей для принудительного исключения из эксперимента.
	ForceExclude []string `json:"force_exclude"`
}

// Variant определяет один из вариантов в эксперименте (контрольный или тестовый).
type Variant struct {
	// Name - уникальное в рамках эксперимента имя варианта (например, "control", "treatment_A").
	Name string `json:"name"`
	// BucketRange - диапазон бакетов [от, до] (включительно), от 0 до 999.
	BucketRange [2]int `json:"bucket_range"`
}
