package webapi

import (
	"encoding/json"
	"net/http"
	"strings"
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

func TestRuntimeEventSerializesPersistedTranslationAgent(t *testing.T) {
	item := domain.RuntimeQueueItem{
		Seq: 45, Time: time.Date(2026, 8, 12, 9, 0, 0, 0, time.UTC),
		Agent: "translation_coordinator", Category: "TRANSLATION", Summary: "Hoàn tất lô dịch chương 2",
		Payload: map[string]any{"level": "success"},
	}
	encoded, err := json.Marshal(runtimeEvent(item))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), `"agent":"Translation Agent"`) {
		t.Fatalf("serialized translation event lost agent: %s", encoded)
	}
}

func TestLiveEventMapsAgentAndPriority(t *testing.T) {
	got := liveEvent(host.Event{Time: time.Now(), Agent: "translation_coordinator", Category: "ERROR", Level: "error", Summary: "batch failed"})
	if got.Agent != "Translation Agent" || got.Priority != "control" || got.Level != "error" {
		t.Fatalf("liveEvent = %+v", got)
	}
}

func TestTokenAuthMiddleware(t *testing.T) {
	handler := withTokenAuth("secret-123", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))

	// GET health nên được pass qua không cần token
	reqGet, _ := http.NewRequest("GET", "/api/health", nil)
	recGet := httptestNewRecorder()
	handler.ServeHTTP(recGet, reqGet)
	if recGet.Code != http.StatusOK {
		t.Fatalf("GET /api/health should not require token, got %d", recGet.Code)
	}

	// Mọi POST control thiếu token phải trả 401, bao gồm các workspace quản trị mới.
	for _, path := range []string{
		"/api/engine/abort",
		"/api/translation/request",
		"/api/translation/retry",
		"/api/translation/glossary",
		"/api/snapshots",
		"/api/snapshots/restore",
		"/api/settings/model",
	} {
		reqPostNoAuth, _ := http.NewRequest("POST", path, nil)
		recPostNoAuth := httptestNewRecorder()
		handler.ServeHTTP(recPostNoAuth, reqPostNoAuth)
		if recPostNoAuth.Code != http.StatusUnauthorized {
			t.Fatalf("POST %s without token should return 401, got %d", path, recPostNoAuth.Code)
		}
	}

	// Bearer token đúng phải cho phép toàn bộ POST control đi qua handler.
	for _, path := range []string{
		"/api/engine/abort",
		"/api/translation/request",
		"/api/translation/retry",
		"/api/translation/glossary",
		"/api/snapshots",
		"/api/snapshots/restore",
		"/api/settings/model",
	} {
		reqPostAuth, _ := http.NewRequest("POST", path, nil)
		reqPostAuth.Header.Set("Authorization", "Bearer secret-123")
		recPostAuth := httptestNewRecorder()
		handler.ServeHTTP(recPostAuth, reqPostAuth)
		if recPostAuth.Code != http.StatusOK {
			t.Fatalf("POST %s with valid Bearer token should pass, got %d", path, recPostAuth.Code)
		}
	}
}

func httptestNewRecorder() *httptestResponseRecorder {
	return &httptestResponseRecorder{HeaderMap: make(http.Header), Code: 200}
}

type httptestResponseRecorder struct {
	HeaderMap http.Header
	Body      []byte
	Code      int
}

func (r *httptestResponseRecorder) Header() http.Header { return r.HeaderMap }
func (r *httptestResponseRecorder) Write(b []byte) (int, error) {
	r.Body = append(r.Body, b...)
	return len(b), nil
}
func (r *httptestResponseRecorder) WriteHeader(statusCode int) { r.Code = statusCode }
