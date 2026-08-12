package webapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/voocel/ainovel-cli/internal/host"
	"github.com/voocel/ainovel-cli/internal/translation"
)

// Options configures the local dashboard API. It intentionally binds to
// loopback by default because the API can control the local writing engine.
type Options struct {
	Addr string
}

// Server exposes read-only telemetry plus the minimal lifecycle controls the
// browser dashboard needs. The TUI and headless paths remain unchanged.
type Server struct {
	runtime *host.Host
}

func New(runtime *host.Host) *Server { return &Server{runtime: runtime} }

func (s *Server) Serve(opts Options) error {
	addr := strings.TrimSpace(opts.Addr)
	if addr == "" {
		addr = "127.0.0.1:8090"
	}
	server := &http.Server{Addr: addr, Handler: withCORS(s.routes())}
	fmt.Printf("Local dashboard API đang chạy tại http://%s/api/\n", addr)
	return server.ListenAndServe()
}

func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/health", s.health)
	mux.HandleFunc("/api/state", s.state)
	mux.HandleFunc("/api/events", s.events)
	mux.HandleFunc("/api/ws", s.websocket)
	mux.HandleFunc("/api/agents/", s.agent)
	mux.HandleFunc("/api/translation/status", s.translationStatus)
	mux.HandleFunc("/api/translation/retry", s.translationRetry)
	mux.HandleFunc("/api/engine/start", s.start)
	mux.HandleFunc("/api/engine/resume", s.resume)
	mux.HandleFunc("/api/engine/continue", s.continueEngine)
	mux.HandleFunc("/api/engine/abort", s.abort)
	return mux
}

type stateResponse struct {
	Online      bool                `json:"online"`
	Snapshot    snapshotResponse    `json:"snapshot"`
	Agents      []agentResponse     `json:"agents"`
	Translation *translation.Status `json:"translation,omitempty"`
	UpdatedAt   time.Time           `json:"updated_at"`
}

type snapshotResponse struct {
	NovelName        string  `json:"novel_name"`
	RuntimeState     string  `json:"runtime_state"`
	StatusLabel      string  `json:"status_label"`
	Phase            string  `json:"phase"`
	Flow             string  `json:"flow"`
	CurrentChapter   int     `json:"current_chapter"`
	TotalChapters    int     `json:"total_chapters"`
	CompletedCount   int     `json:"completed_count"`
	TotalWordCount   int     `json:"total_word_count"`
	TotalCostUSD     float64 `json:"total_cost_usd"`
	BudgetLimitUSD   float64 `json:"budget_limit_usd"`
	IsRunning        bool    `json:"is_running"`
	CurrentVolumeArc string  `json:"current_volume_arc"`
	NextVolumeTitle  string  `json:"next_volume_title"`
	LastCommit       string  `json:"last_commit_summary"`
	LastReview       string  `json:"last_review_summary"`
}

type agentResponse struct {
	Name      string          `json:"name"`
	Role      string          `json:"role"`
	State     string          `json:"state"`
	TaskID    string          `json:"task_id,omitempty"`
	TaskKind  string          `json:"task_kind,omitempty"`
	Summary   string          `json:"summary,omitempty"`
	Tool      string          `json:"tool,omitempty"`
	Turn      int             `json:"turn"`
	UpdatedAt time.Time       `json:"updated_at"`
	Context   contextResponse `json:"context"`
}

type contextResponse struct {
	Tokens    int     `json:"tokens"`
	Window    int     `json:"window"`
	Percent   float64 `json:"percent"`
	Scope     string  `json:"scope"`
	Strategy  string  `json:"strategy"`
	Active    int     `json:"active_messages"`
	Summary   int     `json:"summary_messages"`
	Compacted int     `json:"compacted_count"`
	Kept      int     `json:"kept_count"`
}

type eventResponse struct {
	Seq        int64     `json:"seq"`
	Time       time.Time `json:"time"`
	TaskID     string    `json:"task_id,omitempty"`
	Agent      string    `json:"agent,omitempty"`
	Category   string    `json:"category,omitempty"`
	Kind       string    `json:"kind,omitempty"`
	Summary    string    `json:"summary,omitempty"`
	Priority   string    `json:"priority,omitempty"`
	Level      string    `json:"level,omitempty"`
	Detail     string    `json:"detail,omitempty"`
	Failed     bool      `json:"failed,omitempty"`
	FinishedAt time.Time `json:"finished_at,omitempty"`
	RetryAt    time.Time `json:"retry_at,omitempty"`
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "service": "ainovel-engine", "time": time.Now()})
}

func (s *Server) state(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	snap := s.runtime.Snapshot()
	agents := make([]agentResponse, 0, len(snap.Agents))
	for _, agent := range snap.Agents {
		agents = append(agents, toAgent(agent))
	}
	writeJSON(w, http.StatusOK, stateResponse{
		Online: true, Snapshot: toSnapshot(snap), Agents: agents, Translation: s.translationSnapshot(), UpdatedAt: time.Now(),
	})
}

func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	after, _ := strconv.ParseInt(r.URL.Query().Get("after"), 10, 64)
	items, err := s.runtime.ReplayQueue(after)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	agentFilter := strings.TrimSpace(r.URL.Query().Get("agent"))
	categoryFilter := strings.TrimSpace(r.URL.Query().Get("category"))
	kindFilter := strings.TrimSpace(r.URL.Query().Get("kind"))
	levelFilter := strings.TrimSpace(r.URL.Query().Get("level"))
	result := make([]eventResponse, 0, len(items))
	for _, item := range items {
		kind := payloadString(item.Payload, "kind")
		level := payloadString(item.Payload, "level")
		if agentFilter != "" && canonicalAgent(item.Agent) != canonicalAgent(agentFilter) {
			continue
		}
		if categoryFilter != "" && !strings.EqualFold(item.Category, categoryFilter) {
			continue
		}
		if kindFilter != "" && !strings.EqualFold(kind, kindFilter) {
			continue
		}
		if levelFilter != "" && !strings.EqualFold(level, levelFilter) {
			continue
		}
		converted := runtimeEvent(item)
		converted.Kind = kind
		converted.Level = level
		result = append(result, converted)
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": result, "next": lastSeq(result)})
}

func (s *Server) agent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	role := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/agents/"), "/")
	role = strings.ReplaceAll(role, "-", " ")
	snap := s.runtime.Snapshot()
	var found *agentResponse
	for _, item := range snap.Agents {
		if canonicalAgent(item.Name) == canonicalAgent(role) {
			copy := toAgent(item)
			found = &copy
			break
		}
	}
	items, err := s.runtime.ReplayQueue(0)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	history := make([]eventResponse, 0, 64)
	for _, item := range items {
		if canonicalAgent(item.Agent) != canonicalAgent(role) {
			continue
		}
		history = append(history, eventResponse{Seq: item.Seq, Time: item.Time, TaskID: item.TaskID, Agent: canonicalAgent(item.Agent), Category: item.Category, Kind: payloadString(item.Payload, "kind"), Summary: item.Summary, Priority: string(item.Priority)})
		if len(history) >= 100 {
			break
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"agent": found, "role": canonicalAgent(role), "history": history})
}

type promptRequest struct {
	Prompt string `json:"prompt"`
}

func (s *Server) start(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req promptRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if strings.TrimSpace(req.Prompt) == "" {
		writeError(w, http.StatusBadRequest, "prompt is required")
		return
	}
	if err := s.runtime.StartPrepared(req.Prompt); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"ok": true})
}

func (s *Server) resume(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if _, err := s.runtime.Resume(); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"ok": true})
}

func (s *Server) continueEngine(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req promptRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if err := s.runtime.Continue(req.Prompt); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"ok": true})
}

func (s *Server) abort(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"ok": s.runtime.Abort()})
}

func toSnapshot(s host.UISnapshot) snapshotResponse {
	return snapshotResponse{NovelName: s.NovelName, RuntimeState: s.RuntimeState, StatusLabel: s.StatusLabel, Phase: s.Phase, Flow: s.Flow, CurrentChapter: s.CurrentChapter, TotalChapters: s.TotalChapters, CompletedCount: s.CompletedCount, TotalWordCount: s.TotalWordCount, TotalCostUSD: s.TotalCostUSD, BudgetLimitUSD: s.BudgetLimitUSD, IsRunning: s.IsRunning, CurrentVolumeArc: s.CurrentVolumeArc, NextVolumeTitle: s.NextVolumeTitle, LastCommit: s.LastCommitSummary, LastReview: s.LastReviewSummary}
}

func toAgent(a host.AgentSnapshot) agentResponse {
	return agentResponse{Name: canonicalAgent(a.Name), Role: canonicalAgent(a.Name), State: a.State, TaskID: a.TaskID, TaskKind: a.TaskKind, Summary: a.Summary, Tool: a.Tool, Turn: a.Turn, UpdatedAt: a.UpdatedAt, Context: contextResponse{Tokens: a.Context.Tokens, Window: a.Context.ContextWindow, Percent: a.Context.Percent, Scope: a.Context.Scope, Strategy: a.Context.Strategy, Active: a.Context.ActiveMessages, Summary: a.Context.SummaryMessages, Compacted: a.Context.CompactedCount, Kept: a.Context.KeptCount}}
}

func canonicalAgent(name string) string {
	lower := strings.ToLower(strings.TrimSpace(name))
	switch {
	case strings.HasPrefix(lower, "architect"):
		return "Architect"
	case strings.HasPrefix(lower, "writer"):
		return "Writer"
	case strings.HasPrefix(lower, "editor"):
		return "Editor"
	case strings.Contains(lower, "arbiter"):
		return "Arbiter"
	case strings.Contains(lower, "translator"), strings.Contains(lower, "translation"):
		return "Translation Agent"
	default:
		return strings.TrimSpace(name)
	}
}

func payloadString(payload any, key string) string {
	if values, ok := payload.(map[string]any); ok {
		for _, candidate := range []string{key, strings.ToUpper(key[:1]) + key[1:]} {
			if value, ok := values[candidate].(string); ok {
				return value
			}
		}
	}
	return ""
}

func payloadBool(payload any, key string) bool {
	if values, ok := payload.(map[string]any); ok {
		for _, candidate := range []string{key, strings.ToUpper(key[:1]) + key[1:]} {
			if value, ok := values[candidate].(bool); ok {
				return value
			}
		}
	}
	return false
}

func payloadTime(payload any, key string) time.Time {
	if values, ok := payload.(map[string]any); ok {
		for _, candidate := range []string{key, strings.ToUpper(key[:1]) + key[1:]} {
			if value, ok := values[candidate].(string); ok {
				if parsed, err := time.Parse(time.RFC3339Nano, value); err == nil {
					return parsed
				}
			}
		}
	}
	return time.Time{}
}

func lastSeq(items []eventResponse) int64 {
	if len(items) == 0 {
		return 0
	}
	return items[len(items)-1].Seq
}

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]any{"error": message})
}
