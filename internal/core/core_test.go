package core

import (
	"errors"
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
	base := ContainerSpec{Project: "team", Name: "task-1", Image: "image:tag", Port: 8080}
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
