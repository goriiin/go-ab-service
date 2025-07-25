package client_sdk

import (
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

type sdkMetrics struct {
	configVersion prometheus.Gauge
	decisions     *prometheus.CounterVec
	errors        *prometheus.CounterVec
}

func registerMetrics() *sdkMetrics {
	return &sdkMetrics{
		configVersion: promauto.NewGauge(prometheus.GaugeOpts{
			Name: "ab_client_config_version_timestamp_ms",
			Help: "The timestamp (in milliseconds) of the latest config version applied by the client.",
		}),
		decisions: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "ab_client_decisions_total",
			Help: "Total number of decisions made, partitioned by experiment and variant.",
		}, []string{"experiment_id", "variant_name"}),
		errors: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "ab_client_errors_total",
			Help: "Total number of errors encountered by the client.",
		}, []string{"type"}),
	}
}

// setVersionMetric безопасно парсит UUIDv7 и выставляет метрику.
func (m *sdkMetrics) setVersionMetric(versionStr string) {
	ver, err := uuid.Parse(versionStr)
	if err != nil {
		return // Не можем спарсить - не выставляем метрику
	}
	// UUIDv7 содержит 48-битный Unix-таймстемп в миллисекундах
	m.configVersion.Set(float64(ver.Time()))
}
