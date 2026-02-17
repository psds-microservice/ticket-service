package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	AppHost  string
	HTTPPort string
	GRPCPort string
	AppEnv   string
	LogLevel string

	// SearchServiceURL — если задан, ticket-service отправляет тикеты в search-service для индексации (POST /search/index/ticket).
	SearchServiceURL string

	DB struct {
		Host     string
		Port     string
		User     string
		Password string
		Database string
		SSLMode  string
	}
}

func Load() (*Config, error) {
	_ = godotenv.Load(".env")
	_ = godotenv.Load("../.env")

	cfg := &Config{
		AppHost:          getEnv("APP_HOST", "0.0.0.0"),
		HTTPPort:         firstEnv("APP_PORT", "HTTP_PORT", "8097"),
		GRPCPort:         firstEnv("GRPC_PORT", "METRICS_PORT", "9097"),
		AppEnv:           getEnv("APP_ENV", "development"),
		LogLevel:         getEnv("LOG_LEVEL", "info"),
		SearchServiceURL: getEnv("SEARCH_SERVICE_URL", ""),
	}
	cfg.DB.Host = getEnv("DB_HOST", "localhost")
	cfg.DB.Port = getEnv("DB_PORT", "5432")
	cfg.DB.User = getEnv("DB_USER", "postgres")
	cfg.DB.Password = getEnv("DB_PASSWORD", "postgres")
	cfg.DB.Database = getEnv("DB_DATABASE", "ticket_service")
	cfg.DB.SSLMode = getEnv("DB_SSLMODE", "disable")
	return cfg, nil
}

func (c *Config) Validate() error {
	if c.DB.Host == "" || c.DB.Database == "" {
		return errors.New("config: DB_HOST and DB_DATABASE are required")
	}
	if c.AppEnv == "production" && c.DB.Password == "" {
		return errors.New("config: in production DB_PASSWORD is required")
	}
	return nil
}

func (c *Config) DSN() string {
	return fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		c.DB.Host, c.DB.Port, c.DB.User, c.DB.Password, c.DB.Database, c.DB.SSLMode)
}

func (c *Config) DatabaseURL() string {
	pass := url.QueryEscape(c.DB.Password)
	return fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=%s",
		c.DB.User, pass, c.DB.Host, c.DB.Port, c.DB.Database, c.DB.SSLMode)
}

func (c *Config) Addr() string {
	return c.AppHost + ":" + c.HTTPPort
}

func firstEnv(keysAndDef ...string) string {
	if len(keysAndDef) == 0 {
		return ""
	}
	def := keysAndDef[len(keysAndDef)-1]
	for _, k := range keysAndDef[:len(keysAndDef)-1] {
		if v := os.Getenv(k); v != "" {
			return v
		}
	}
	return def
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
