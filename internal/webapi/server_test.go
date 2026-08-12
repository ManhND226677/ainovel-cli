package webapi

import (
	"testing"
	"time"
)

func TestCanonicalAgent(t *testing.T) {
	tests := map[string]string{
		"architect_long":    "Architect",
		"writer":            "Writer",
		"editor":            "Editor",
		"arbiter":           "Arbiter",
		"translator":        "Translation Agent",
		"translation_agent": "Translation Agent",
	}
	for input, want := range tests {
		if got := canonicalAgent(input); got != want {
			t.Fatalf("canonicalAgent(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestLastSeq(t *testing.T) {
	when := time.Date(2026, 8, 12, 8, 30, 0, 0, time.UTC)
	items := []eventResponse{{Seq: 17, Time: when}, {Seq: 18, Time: when.Add(time.Second)}}
	if got := lastSeq(items); got != 18 {
		t.Fatalf("lastSeq = %d, want 18", got)
	}
	if got := lastSeq(nil); got != 0 {
		t.Fatalf("lastSeq(nil) = %d, want 0", got)
	}
}
