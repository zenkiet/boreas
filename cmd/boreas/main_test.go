package main

import (
	"testing"

	"github.com/google/uuid"
)

func TestGateCapsAnswersPerUserAndInTotal(t *testing.T) {
	g, alice := &gate{users: map[uuid.UUID]int{}}, uuid.New()
	for range answersPerUser {
		if !g.enter(alice) {
			t.Fatal("refused within the per-user cap")
		}
	}
	if g.enter(alice) {
		t.Fatal("admitted past the per-user cap")
	}
	g.leave(alice)
	if !g.enter(alice) {
		t.Fatal("a finished answer kept its slot")
	}
	for g.enter(uuid.New()) {
	}
	if g.total != answersInFlight {
		t.Fatalf("admitted %d answers in total, want %d", g.total, answersInFlight)
	}
}
