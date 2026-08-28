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

func TestLoadRejectsFCMWithKeyedNotifyURL(t *testing.T) {
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

// A keyed URL stays valid on its own: only enabling FCM makes it a misconfiguration.
func TestLoadAllowsKeyedNotifyURLWithoutFCM(t *testing.T) {
	t.Setenv("BOREAS_NOTIFY_URL", "http://boreas-noti:8000/notify/boreas")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.FCM.Enabled() {
		t.Fatal("FCM must not be considered enabled without both variables")
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
