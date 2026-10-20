package core

import (
	"errors"
	"slices"
	"strings"
	"testing"
)

func TestValidateTaskName(t *testing.T) {
	valid := []string{"a", "A-1_b.c", strings.Repeat("x", 63)}
	for _, name := range valid {
		if err := ValidateTaskName(name); err != nil {
			t.Fatalf("ValidateTaskName(%q) = %v", name, err)
		}
	}
	invalid := []string{"", "-starts-dash", "has space", "slash/name", strings.Repeat("x", 64)}
	for _, name := range invalid {
		if err := ValidateTaskName(name); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("ValidateTaskName(%q) = %v, want ErrInvalidInput", name, err)
		}
	}
}

func TestProjectRoleRank(t *testing.T) {
	ordered := []ProjectRole{ProjectRoleViewer, ProjectRoleOperator, ProjectRoleMember, ProjectRoleOwner}
	for i := 1; i < len(ordered); i++ {
		if ordered[i-1].Rank() >= ordered[i].Rank() {
			t.Fatalf("%q must rank below %q", ordered[i-1], ordered[i])
		}
	}
	// An unknown or absent role must never satisfy a requirement.
	for _, role := range []ProjectRole{"", "root", "Owner"} {
		if role.Rank() != 0 {
			t.Fatalf("Rank(%q) = %d, want 0", role, role.Rank())
		}
	}
	if len(ProjectRole("").Enum()) != len(ordered) {
		t.Fatal("Enum must list every ranked role")
	}
}

func TestValidateProjectSlug(t *testing.T) {
	valid := []string{"a", "team-alpha", "a1-b2", strings.Repeat("x", 63)}
	for _, slug := range valid {
		if err := ValidateProjectSlug(slug); err != nil {
			t.Fatalf("ValidateProjectSlug(%q) = %v", slug, err)
		}
	}
	invalid := []string{"", "-dash", "Upper", "under_score", "dot.dot", strings.Repeat("x", 64)}
	for _, slug := range invalid {
		if err := ValidateProjectSlug(slug); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("ValidateProjectSlug(%q) = %v, want ErrInvalidInput", slug, err)
		}
	}
	for _, slug := range []string{"api", "health", "metrics", "static", "admin"} {
		if err := ValidateProjectSlug(slug); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("reserved slug %q was accepted", slug)
		}
	}
}

func TestContainerSpecValidation(t *testing.T) {
	base := ContainerSpec{
		Project: "team", Name: "task-1", Image: "image:tag", Port: 8080,
		Volumes: map[string]string{"/app/certs": "certs"},
	}
	if err := base.Validate(); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*ContainerSpec){
		"project":         func(s *ContainerSpec) { s.Project = "API" },
		"reserved slug":   func(s *ContainerSpec) { s.Project = "api" },
		"name":            func(s *ContainerSpec) { s.Name = "bad name" },
		"image":           func(s *ContainerSpec) { s.Image = " " },
		"port":            func(s *ContainerSpec) { s.Port = 65536 },
		"env":             func(s *ContainerSpec) { s.Env = map[string]string{"A=B": "x"} },
		"reserved env":    func(s *ContainerSpec) { s.Env = map[string]string{"BOREAS_PORT": "1"} },
		"reserved label":  func(s *ContainerSpec) { s.Labels = map[string]string{"task": "other"} },
		"reserved label2": func(s *ContainerSpec) { s.Labels = map[string]string{"project": "other"} },
		"relative target": func(s *ContainerSpec) { s.Volumes = map[string]string{"app": "certs"} },
		"root target":     func(s *ContainerSpec) { s.Volumes = map[string]string{"/": "certs"} },
		"unclean target":  func(s *ContainerSpec) { s.Volumes = map[string]string{"/a/../b": "certs"} },
		"nested folder":   func(s *ContainerSpec) { s.Volumes = map[string]string{"/a": "configs/nginx"} },
		"parent folder":   func(s *ContainerSpec) { s.Volumes = map[string]string{"/a": ".."} },
		"empty folder":    func(s *ContainerSpec) { s.Volumes = map[string]string{"/a": ""} },
	} {
		t.Run(name, func(t *testing.T) {
			s := base
			mutate(&s)
			if err := s.Validate(); !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("got %v, want ErrInvalidInput", err)
			}
		})
	}
}

func TestTaskCloneDoesNotAliasMaps(t *testing.T) {
	original := Task{Labels: map[string]string{"a": "b"}, Env: map[string]string{"X": "1"}}
	clone := original.Clone()
	clone.Labels["a"] = "changed"
	clone.Env["X"] = "2"
	if original.Labels["a"] != "b" || original.Env["X"] != "1" {
		t.Fatal("Clone aliases task maps")
	}
}

func TestBuildValidate(t *testing.T) {
	if err := (Build{State: BuildRunning}).Validate(); err != nil {
		t.Fatal(err)
	}
	for name, b := range map[string]Build{
		"state":         {State: "stale"},
		"progress":      {State: BuildRunning, Progress: 101},
		"long stage":    {State: BuildRunning, Stage: strings.Repeat("x", 101)},
		"control stage": {State: BuildRunning, Stage: "a\x00b"},
		"script url":    {State: BuildRunning, URL: "javascript:alert(1)"},
		"relative url":  {State: BuildRunning, URL: "/job/42"},
	} {
		if err := b.Validate(); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("%s: got %v, want ErrInvalidInput", name, err)
		}
	}
}

func TestRedactHidesSecretsNotRules(t *testing.T) {
	text := `const apiKey = "AbC123xyz789";
db := "Server=db;User Id=sa;Password=Pa55word;"
url := "postgres://boreas:s3cretpw@db/boreas"
aws := "AKIAABCDEFGHIJKLMNOP"
error := "Password must be 8 characters"
if (password.length < 8 && task-management-component-list) {}
DB_PASSWORD=hunter2xyz
STRIPE_KEY=zzqq778899
api_key: abcdef123
password = "ab12"
Authorization: Bearer abcDEF123opaque456
cache := "redis://:s3cretpw@cache"
JWT_SECRET = qwerty77
Authorization: Bearer abcdefghijklmnop
SECRET_KEY_BASE=kb77base
DB_PASSWORD_PROD=prodpw99
Authorization: Basic dXNlcjpwYXNz
mysql --password hunter3abc -h db
run --password "abc123q" --verbose
run --password 'two wordsq'
args := ["--password", "jsonpw77"]
curl --api-key "apikey99" https://x
password:colonpw1
API_SECRET="correct horse battery"
2026-10-12T07:59:59Z Loading the signing key from disk
2026-10-12T08:00:00Z MIIEvQIBADANBgkqhkiG9w0BAQEFAASCBKcwggSjAgEAAoIBAQC7
2026-10-12T08:00:00Z VJTUt9Us8cKjMzEfYyjiWA4R4/M2bS1GB4t7NXp98C3SC6dVMvDu
2026-10-12T08:00:00Z -----END PRIVATE KEY-----
LOG_LEVEL=debug
if token == nil {
Send a Bearer token in the header.`
	got := Redact(text)
	for _, secret := range []string{
		"AbC123xyz789", "Pa55word", "s3cretpw", "AKIAABCDEFGHIJKLMNOP", "hunter2xyz", "zzqq778899", "abcdef123", "ab12",
		"abcDEF123opaque456", "qwerty77", "abcdefghijklmnop", "kb77base", "prodpw99", "dXNlcjpwYXNz", "hunter3abc",
		"colonpw1", "horse", "MIIEvQIBADAN", "VJTUt9Us8cKj", "abc123q", "wordsq", "jsonpw77", "apikey99",
	} {
		if strings.Contains(got, secret) {
			t.Errorf("%s leaked:\n%s", secret, got)
		}
	}
	for _, rule := range []string{
		"Password must be 8 characters", "password.length < 8", "task-management-component-list", "-h db",
		"Loading the signing key from disk", "LOG_LEVEL=debug", "token == nil", "Bearer token",
	} {
		if !strings.Contains(got, rule) {
			t.Errorf("%q lost:\n%s", rule, got)
		}
	}
}

func TestCleanRoutes(t *testing.T) {
	got, err := CleanRoutes([]Route{
		{Path: "/pay/", URL: "https://pay.lan/x/../base/", AnyCert: true},
		{Path: "/api", URL: "http://10.20.0.15:8080/"},
	})
	want := []Route{
		{Path: "/api", URL: "http://10.20.0.15:8080"},
		{Path: "/pay", URL: "https://pay.lan/base", AnyCert: true},
	}
	if err != nil || !slices.Equal(got, want) {
		t.Fatalf("CleanRoutes = %v, %v", got, err)
	}

	badPath := ": path must start with / and use letters, digits, - and _"
	long := "/" + strings.Repeat("x", 64)
	for _, c := range []struct {
		routes  []Route
		message string
	}{
		{[]Route{{Path: "/", URL: "http://a"}}, "/: / is the task itself"},
		{[]Route{{Path: "api", URL: "http://a"}}, "api" + badPath},
		{[]Route{{Path: "/a b", URL: "http://a"}}, "/a b" + badPath},
		{[]Route{{Path: long, URL: "http://a"}}, long + badPath},
		{[]Route{{Path: "/ok", URL: "http://a"}, {Path: "/ok/", URL: "http://b"}}, "/ok/: path is taken"},
		{[]Route{{Path: "/x", URL: "ftp://a"}}, "/x: " + badRouteURL},
		{[]Route{{Path: "/x", URL: "http://u:p@a"}}, "/x: " + badRouteURL},
		{[]Route{{Path: "/x", URL: "http://a?q=1"}}, "/x: " + badRouteURL},
		{[]Route{{Path: "/x", URL: "http://a#f"}}, "/x: " + badRouteURL},
		{[]Route{{Path: "/x", URL: "/relative"}}, "/x: " + badRouteURL},
		{make([]Route, 21), "at most 20 routes"},
	} {
		_, err := CleanRoutes(c.routes)
		if !errors.Is(err, ErrInvalidInput) || !strings.HasSuffix(err.Error(), "\n"+c.message) {
			t.Fatalf("CleanRoutes(%v) = %v, want %q", c.routes, err, c.message)
		}
	}
}
