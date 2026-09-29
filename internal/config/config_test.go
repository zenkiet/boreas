package config

import (
	"strings"
	"testing"
)

func TestLoadDefaults(t *testing.T) {
	cfg, err := Load()
	if err != nil || cfg.Port != 8080 || cfg.ListenAddr() != "0.0.0.0:8080" || cfg.Admin.Provided() {
		t.Fatalf("defaults = %+v, %v", cfg, err)
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
	if err != nil || cfg.Port != 9090 || cfg.Postgres.Host != "db.internal" || cfg.Postgres.DB != "staging" || !cfg.Admin.Provided() {
		t.Fatalf("overrides not applied: %+v, %v", cfg, err)
	}
}

func TestLoadRejectsInvalidPort(t *testing.T) {
	for _, port := range []string{"70000", "not-a-number"} {
		t.Setenv("BOREAS_PORT", port)
		if _, err := Load(); err == nil {
			t.Fatalf("BOREAS_PORT=%q must be rejected", port)
		}
	}
}

func TestDSNEscapesPasswordAndPrefersURL(t *testing.T) {
	p := PostgresConfig{
		Host: "localhost", Port: "5432", User: "boreas",
		Password: "p@ss:word/1", DB: "boreas", SSLMode: "require",
	}
	dsn := p.DSN()
	if strings.Contains(dsn, "p@ss:word/1") || !strings.Contains(dsn, "sslmode=require") || !strings.Contains(dsn, "/boreas") {
		t.Fatalf("unexpected DSN: %s", dsn)
	}

	p.URL = "postgres://override/db"
	if p.DSN() != "postgres://override/db" {
		t.Fatalf("URL should take precedence, got %s", p.DSN())
	}
}

func TestLoadRejectsFCMWithKeyedNotifyURL(t *testing.T) {
	// A keyed URL stays valid on its own: only enabling FCM makes it a misconfiguration.
	t.Setenv("BOREAS_NOTIFY_URL", "http://boreas-noti:8000/notify/boreas")
	if _, err := Load(); err != nil {
		t.Fatalf("a keyed URL without FCM must load: %v", err)
	}
	t.Setenv("BOREAS_FCM_PROJECT", "boreas")
	t.Setenv("BOREAS_FCM_KEYFILE", "/config/key.json")

	for _, notifyURL := range []string{
		"http://boreas-noti:8000/notify/boreas",
		"",
	} {
		t.Setenv("BOREAS_NOTIFY_URL", notifyURL)
		_, err := Load()
		if err == nil {
			t.Fatalf("BOREAS_NOTIFY_URL=%q must be rejected while FCM is enabled", notifyURL)
		}
		if !strings.Contains(err.Error(), "BOREAS_NOTIFY_URL") {
			t.Fatalf("error must name the offending variable: %v", err)
		}
	}

	for _, notifyURL := range []string{
		"http://boreas-noti:8000/notify",
		"http://boreas-noti:8000/notify/",
	} {
		t.Setenv("BOREAS_NOTIFY_URL", notifyURL)
		if _, err := Load(); err != nil {
			t.Fatalf("BOREAS_NOTIFY_URL=%q is the stateless endpoint, got %v", notifyURL, err)
		}
	}
}

func TestLoadRejectsPartialFCMConfig(t *testing.T) {
	for name, value := range map[string]string{
		"BOREAS_FCM_PROJECT": "boreas",
		"BOREAS_FCM_KEYFILE": "/config/key.json",
	} {
		t.Run(name, func(t *testing.T) {
			t.Setenv(name, value)
			if _, err := Load(); err == nil {
				t.Fatal("partial FCM configuration must be rejected")
			}
		})
	}
}

func TestTeamNotifyURLDerivedFromNotifyURL(t *testing.T) {
	if got := (Config{}).TeamNotifyURL(); got != "" {
		t.Fatalf("unset notify URL must derive empty, got %q", got)
	}
	cfg := Config{NotifyURL: "http://boreas-noti:8000/notify/"}
	if got := cfg.TeamNotifyURL(); got != "http://boreas-noti:8000/notify/boreas" {
		t.Fatalf("got %q", got)
	}
}
