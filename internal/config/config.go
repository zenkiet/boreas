// Package config loads Boreas runtime configuration from the environment.
package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Port      int
	Postgres  PostgresConfig
	Admin     AdminConfig
	NotifyURL string
	FCM       FCMConfig
	AI        AIConfig
	Sourcebot SourcebotConfig
	S3        S3Config
}

type FCMConfig struct {
	Project string
	Keyfile string
}

type AIConfig struct {
	BaseURL string
	APIKey  string
	Model   string
}

func (c AIConfig) Enabled() bool { return c.BaseURL != "" && c.Model != "" }

type SourcebotConfig struct {
	URL    string
	APIKey string
}

func (c SourcebotConfig) Enabled() bool { return c.URL != "" && c.APIKey != "" }

type S3Config struct {
	Endpoint, Bucket, Region, AccessKeyID, SecretAccessKey string
}

func (c S3Config) Enabled() bool { return c.Endpoint != "" }

type PostgresConfig struct {
	Host     string
	Port     string
	User     string
	Password string
	DB       string
	SSLMode  string
	URL      string
}

type AdminConfig struct {
	Username string
	Email    string
	Password string
}

func (c AdminConfig) Provided() bool { return c.Username != "" && c.Email != "" && c.Password != "" }

func (p PostgresConfig) DSN() string {
	if p.URL != "" {
		return p.URL
	}
	dsn := url.URL{
		Scheme:   "postgres",
		User:     url.UserPassword(p.User, p.Password),
		Host:     p.Host + ":" + p.Port,
		Path:     "/" + p.DB,
		RawQuery: url.Values{"sslmode": {p.SSLMode}}.Encode(),
	}
	return dsn.String()
}

func Load() (*Config, error) {
	cfg := Config{
		Port: 8080,
		Postgres: PostgresConfig{
			Host: "localhost", Port: "5432", User: "postgres",
			Password: "postgres", DB: "boreas", SSLMode: "disable",
		},
	}

	setString("BOREAS_DB_HOST", &cfg.Postgres.Host)
	setString("BOREAS_DB_PORT", &cfg.Postgres.Port)
	setString("BOREAS_DB_USER", &cfg.Postgres.User)
	setString("BOREAS_DB_PASSWORD", &cfg.Postgres.Password)
	setString("BOREAS_DB_NAME", &cfg.Postgres.DB)
	setString("BOREAS_DB_SSLMODE", &cfg.Postgres.SSLMode)
	setString("BOREAS_DATABASE_URL", &cfg.Postgres.URL)
	setString("BOREAS_ADMIN_USERNAME", &cfg.Admin.Username)
	setString("BOREAS_ADMIN_EMAIL", &cfg.Admin.Email)
	setString("BOREAS_ADMIN_PASSWORD", &cfg.Admin.Password)
	setString("BOREAS_NOTIFY_URL", &cfg.NotifyURL)
	setString("BOREAS_FCM_PROJECT", &cfg.FCM.Project)
	setString("BOREAS_FCM_KEYFILE", &cfg.FCM.Keyfile)
	setString("BOREAS_AI_BASE_URL", &cfg.AI.BaseURL)
	setString("BOREAS_AI_API_KEY", &cfg.AI.APIKey)
	setString("BOREAS_AI_MODEL", &cfg.AI.Model)
	setString("BOREAS_SOURCEBOT_URL", &cfg.Sourcebot.URL)
	setString("BOREAS_SOURCEBOT_API_KEY", &cfg.Sourcebot.APIKey)
	setString("BOREAS_S3_ENDPOINT", &cfg.S3.Endpoint)
	setString("BOREAS_S3_BUCKET", &cfg.S3.Bucket)
	setString("BOREAS_S3_REGION", &cfg.S3.Region)
	setString("BOREAS_S3_ACCESS_KEY_ID", &cfg.S3.AccessKeyID)
	setString("BOREAS_S3_SECRET_ACCESS_KEY", &cfg.S3.SecretAccessKey)

	if value, ok := os.LookupEnv("BOREAS_PORT"); ok {
		port, err := strconv.Atoi(value)
		if err != nil {
			return nil, fmt.Errorf("BOREAS_PORT: %w", err)
		}
		cfg.Port = port
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func setString(name string, target *string) {
	if value, ok := os.LookupEnv(name); ok {
		*target = value
	}
}

func (c Config) validate() error {
	var result error
	if c.Port < 1 || c.Port > 65535 {
		result = errors.Join(result, errors.New("BOREAS_PORT must be between 1 and 65535"))
	}
	if c.Postgres.URL == "" && (strings.TrimSpace(c.Postgres.Host) == "" || strings.TrimSpace(c.Postgres.Port) == "" ||
		strings.TrimSpace(c.Postgres.User) == "" || strings.TrimSpace(c.Postgres.DB) == "") {
		result = errors.Join(result, errors.New("database host, port, user, and name are required"))
	}
	if (c.FCM.Project == "") != (c.FCM.Keyfile == "") {
		result = errors.Join(result, errors.New("BOREAS_FCM_PROJECT and BOREAS_FCM_KEYFILE must be set together"))
	}
	if (c.Sourcebot.URL == "") != (c.Sourcebot.APIKey == "") {
		result = errors.Join(result, errors.New("BOREAS_SOURCEBOT_URL and BOREAS_SOURCEBOT_API_KEY must be set together"))
	}
	if (c.AI.BaseURL == "") != (c.AI.Model == "") {
		result = errors.Join(result, errors.New("BOREAS_AI_BASE_URL and BOREAS_AI_MODEL must be set together"))
	}
	if s3 := c.S3; s3.Endpoint != "" && (s3.Bucket == "" || s3.AccessKeyID == "" || s3.SecretAccessKey == "") {
		result = errors.Join(result, errors.New("BOREAS_S3_ENDPOINT needs BOREAS_S3_BUCKET, BOREAS_S3_ACCESS_KEY_ID and BOREAS_S3_SECRET_ACCESS_KEY"))
	}
	return result
}

func (c Config) ListenAddr() string { return "0.0.0.0:" + strconv.Itoa(c.Port) }

func (c Config) TeamNotifyURL() string {
	if c.NotifyURL == "" {
		return ""
	}
	return strings.TrimSuffix(c.NotifyURL, "/") + "/boreas"
}
