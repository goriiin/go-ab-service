package config

import (
	"fmt"
	"os"
)

// DBConfig содержит параметры подключения к базе данных PostgreSQL.
type DBConfig struct {
	Host     string
	Port     string
	User     string
	Password string
	DBName   string
	SSLMode  string
}

// NewDBConfig создает конфигурацию базы данных, читая переменные окружения
// и используя значения по умолчанию, если они не установлены.
func NewDBConfig() *DBConfig {
	return &DBConfig{
		Host:     getEnv("DB_HOST", "postgres"),
		Port:     getEnv("DB_PORT", "5432"),
		User:     getEnv("DB_USER", "user"),
		Password: getEnv("DB_PASSWORD", "password"),
		DBName:   getEnv("DB_NAME", "ab_platform"),
		SSLMode:  getEnv("DB_SSLMODE", "disable"),
	}
}

// ConnectionString возвращает DSN (Data Source Name) для подключения к PostgreSQL.
func (c *DBConfig) ConnectionString() string {
	return fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=%s",
		c.User, c.Password, c.Host, c.Port, c.DBName, c.SSLMode)
}

// getEnv - вспомогательная функция для чтения переменной окружения с fallback-значением.
func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}
