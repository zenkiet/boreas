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
}

type FCMConfig struct {
	Project string
	Keyfile string
}

func (c FCMConfig) Enabled() bool {
	return c.Project != "" && c.Keyfile != ""
}

type PostgresConfig struct {
	Host     string
	Port     string
	User     string
	Password string
	DB       string
	SSLMode  string
	// URL, when set, is used verbatim and the fields above are ignored.
	URL string
}

// AdminConfig seeds the first administrator only while the users table is empty.
type AdminConfig struct {
	Username string
	Email    string
	Password string
}

func (c AdminConfig) Provided() bool {
	return c.Username != "" && c.Email != "" && c.Password != ""
}

// DSN uses url.URL so reserved characters in passwords are escaped.
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
	if c.Postgres.URL == "" {
		if strings.TrimSpace(c.Postgres.Host) == "" || strings.TrimSpace(c.Postgres.Port) == "" ||
			strings.TrimSpace(c.Postgres.User) == "" || strings.TrimSpace(c.Postgres.DB) == "" {
			result = errors.Join(result, errors.New("database host, port, user, and name are required"))
		}
	}
	if (c.FCM.Project == "") != (c.FCM.Keyfile == "") {
		result = errors.Join(result, errors.New("BOREAS_FCM_PROJECT and BOREAS_FCM_KEYFILE must be set together"))
	}
	if c.FCM.Enabled() && !statelessNotifyURL(c.NotifyURL) {
		result = errors.Join(result, errors.New(
			"BOREAS_FCM_PROJECT and BOREAS_FCM_KEYFILE require BOREAS_NOTIFY_URL to end in /notify, "+
				"without an Apprise configuration key: the keyed endpoint ignores per-request targets, "+
				"so every subscribed browser would be dropped without an error"))
	}
	return result
}

func statelessNotifyURL(notifyURL string) bool {
	parsed, err := url.Parse(notifyURL)
	if err != nil {
		return false
	}
	return strings.HasSuffix(strings.TrimSuffix(parsed.Path, "/"), "/notify")
}

// ListenAddr binds all interfaces; deployment controls external exposure.
func (c Config) ListenAddr() string { return "0.0.0.0:" + strconv.Itoa(c.Port) }

func (c Config) TeamNotifyURL() string {
	if c.NotifyURL == "" {
		return ""
	}
	return strings.TrimSuffix(c.NotifyURL, "/") + "/boreas"
}
