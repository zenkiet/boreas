package config

import (
	"strings"
	"testing"
)

func TestLoadDefaults(t *testing.T) {
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Port != 8080 {
		t.Fatalf("port = %d, want 8080", cfg.Port)
	}
	if cfg.ListenAddr() != "0.0.0.0:8080" {
		t.Fatalf("listen address = %q", cfg.ListenAddr())
	}
	if cfg.Admin.Provided() {
		t.Fatal("admin should not be considered provided without environment variables")
	}
}

func TestLoadEnvironmentOverrides(t *testing.T) {
	t.Setenv("BOREAS_PORT", "9090")
	t.Setenv("BOREAS_DB_HOST", "db.internal")
	t.Setenv("BOREAS_DB_NAME", "staging")
	t.Setenv("BOREAS_ADMIN_USERNAME", "root")
	t.Setenv("BOREAS_ADMIN_EMAIL", "root@example.com")
	t.Setenv("BOREAS_ADMIN_PASSWORD", "supersecret")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Port != 9090 || cfg.Postgres.Host != "db.internal" || cfg.Postgres.DB != "staging" {
		t.Fatalf("overrides not applied: %+v", cfg)
	}
	if !cfg.Admin.Provided() {
		t.Fatal("admin credentials should be considered provided")
	}
}

func TestLoadRejectsInvalidPort(t *testing.T) {
	t.Setenv("BOREAS_PORT", "70000")
	if _, err := Load(); err == nil {
		t.Fatal("expected validation error for out-of-range port")
	}
	t.Setenv("BOREAS_PORT", "not-a-number")
	if _, err := Load(); err == nil {
		t.Fatal("expected parse error for non-numeric port")
	}
}

func TestDSNEscapesPasswordAndPrefersURL(t *testing.T) {
	p := PostgresConfig{
		Host: "localhost", Port: "5432", User: "boreas",
		Password: "p@ss:word/1", DB: "boreas", SSLMode: "require",
	}
	dsn := p.DSN()
	if strings.Contains(dsn, "p@ss:word/1") {
		t.Fatalf("password was not escaped: %s", dsn)
	}
	if !strings.Contains(dsn, "sslmode=require") || !strings.Contains(dsn, "/boreas") {
		t.Fatalf("unexpected DSN: %s", dsn)
	}

	p.URL = "postgres://override/db"
	if p.DSN() != "postgres://override/db" {
		t.Fatalf("URL should take precedence, got %s", p.DSN())
	}
}
