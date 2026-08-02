package run

import (
	"regexp"
	"testing"
	"time"
)

func TestNewRunID(t *testing.T) {
	now := time.Date(2026, 8, 2, 10, 15, 30, 0, time.UTC)
	id := NewRunID(now)
	if !regexp.MustCompile(`^260802-101530-[a-z0-9]{4}$`).MatchString(id) {
		t.Fatalf("bad id %q", id)
	}
	if NewRunID(now) == id {
		t.Fatal("two ids from same second must differ (random suffix)")
	}
}
