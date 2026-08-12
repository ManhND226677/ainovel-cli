package webapi

import (
	"testing"
	"time"

	"github.com/voocel/ainovel-cli/internal/domain"
	"github.com/voocel/ainovel-cli/internal/host"
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

func TestRuntimeEventPreservesDiagnostics(t *testing.T) {
	item := domain.RuntimeQueueItem{
		Seq: 44, Time: time.Date(2026, 8, 12, 8, 45, 0, 0, time.UTC),
		Agent: "translator", Category: "ERROR", Summary: "Dịch thất bại",
		Payload: map[string]any{"kind": "provider_timeout", "level": "error", "detail": "provider timed out", "failed": true},
	}
	got := runtimeEvent(item)
	if got.Agent != "Translation Agent" || got.Kind != "provider_timeout" || got.Level != "error" || got.Detail != "provider timed out" || !got.Failed {
		t.Fatalf("runtimeEvent lost diagnostics: %+v", got)
	}
}

func TestLiveEventMapsAgentAndPriority(t *testing.T) {
	got := liveEvent(host.Event{Time: time.Now(), Agent: "translation_coordinator", Category: "ERROR", Level: "error", Summary: "batch failed"})
	if got.Agent != "Translation Agent" || got.Priority != "control" || got.Level != "error" {
		t.Fatalf("liveEvent = %+v", got)
	}
}
