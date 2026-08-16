package webapi

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/voocel/ainovel-cli/internal/domain"
	"github.com/voocel/ainovel-cli/internal/host"
	"github.com/voocel/ainovel-cli/internal/translation"
)

// Options configures the dashboard API. Defaults to loopback because the API can
// control the writing engine. For a VPS deploy, pass 0.0.0.0:PORT and set Token.
type Options struct {
	Addr  string
	Token string
	// UIDir is an optional directory of a built SPA (index.html + assets).
	// When set, non-/api routes are served from this directory.
	UIDir string
}

// Server exposes read-only telemetry plus the minimal lifecycle controls the
// browser dashboard needs. The TUI and headless paths remain unchanged.
type Server struct {
	runtime          *host.Host
	telemetryMu      sync.Mutex
	telemetryClients map[chan translation.LiveProgress]struct{}
	latestTelemetry  map[int]translation.LiveProgress
	cocreateMu       sync.Mutex
	cocreate         *webCoCreate
}

func New(runtime *host.Host) *Server {
	s := &Server{runtime: runtime, telemetryClients: make(map[chan translation.LiveProgress]struct{}), latestTelemetry: make(map[int]translation.LiveProgress)}
	if runtime != nil {
		runtime.SubscribeLiveProgress(s.publishLiveProgress)
	}
	return s
}

func (s *Server) publishLiveProgress(update translation.LiveProgress) {
	s.telemetryMu.Lock()
	defer s.telemetryMu.Unlock()
	cached := update
	if previous, ok := s.latestTelemetry[update.Chapter]; ok {
		if cached.SourcePreview == "" {
			cached.SourcePreview = previous.SourcePreview
		}
		if cached.Stage == "streaming" && cached.Preview != "" {
			cached.Preview = previous.Preview + cached.Preview
			if len(cached.Preview) > 24000 {
				cached.Preview = cached.Preview[len(cached.Preview)-24000:]
			}
		}
	}
	s.latestTelemetry[update.Chapter] = cached
	for client := range s.telemetryClients {
		select {
		case client <- update:
		default:
			// Slow browser tabs may miss a delta; completion and durable status
			// are still delivered through normal WebSocket state updates.
		}
	}
}

func (s *Server) liveProgressSnapshot() []translation.LiveProgress {
	s.telemetryMu.Lock()
	defer s.telemetryMu.Unlock()
	items := make([]translation.LiveProgress, 0, len(s.latestTelemetry))
	for _, update := range s.latestTelemetry {
		items = append(items, update)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Chapter < items[j].Chapter })
	return items
}

func (s *Server) subscribeLiveProgress() (<-chan translation.LiveProgress, func()) {
	updates := make(chan translation.LiveProgress, 128)
	s.telemetryMu.Lock()
	s.telemetryClients[updates] = struct{}{}
	s.telemetryMu.Unlock()
	return updates, func() {
		s.telemetryMu.Lock()
		delete(s.telemetryClients, updates)
		s.telemetryMu.Unlock()
	}
}

func (s *Server) Serve(opts Options) error {
	addr := strings.TrimSpace(opts.Addr)
	if addr == "" {
		addr = "127.0.0.1:10001"
	}
	handler := s.routes()
	if ui := strings.TrimSpace(opts.UIDir); ui != "" {
		// Ghi đè bằng thư mục trên đĩa — tiện khi phát triển frontend.
		handler = withSPA(ui, handler)
		fmt.Printf("Web UI (static) dir: %s\n", ui)
	} else {
		// Mặc định: dùng bản SPA nhúng sẵn trong binary.
		handler = withSPAFS(embeddedUI(), handler)
	}
	if opts.Token != "" {
		handler = withTokenAuth(opts.Token, handler)
	}
	server := &http.Server{Addr: addr, Handler: withCORS(handler)}
	fmt.Printf("Dashboard API running at http://%s/api/\n", addr)
	fmt.Printf("Open UI: http://%s/\n", displayAddr(addr))
	return server.ListenAndServe()
}

func displayAddr(addr string) string {
	host, port, ok := strings.Cut(addr, ":")
	if !ok {
		return addr
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		return "127.0.0.1:" + port
	}
	return addr
}

// withSPA phục vụ SPA từ thư mục trên đĩa (chế độ --ui-dir).
func withSPA(uiDir string, api http.Handler) http.Handler {
	return withSPAFS(os.DirFS(filepath.Clean(uiDir)), api)
}

// withSPAFS phục vụ SPA từ một fs.FS bất kỳ (bản nhúng trong binary hoặc os.DirFS).
// /api/* được chuyển tiếp cho handler API; các đường dẫn còn lại resolve file tĩnh,
// không có thì fallback về index.html cho client-side routing.
func withSPAFS(root fs.FS, api http.Handler) http.Handler {
	fileServer := http.FileServer(http.FS(root))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") || r.URL.Path == "/api" {
			api.ServeHTTP(w, r)
			return
		}
		name := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
		if name == "" || name == "." {
			name = "index.html"
		}
		if st, err := fs.Stat(root, name); err == nil && !st.IsDir() {
			fileServer.ServeHTTP(w, r)
			return
		}
		if st, err := fs.Stat(root, path.Join(name, "index.html")); err == nil && !st.IsDir() {
			r2 := new(http.Request)
			*r2 = *r
			r2.URL = new(url.URL)
			*r2.URL = *r.URL
			r2.URL.Path += "/"
			fileServer.ServeHTTP(w, r2)
			return
		}
		index, err := fs.ReadFile(root, "index.html")
		if err != nil {
			http.Error(w, "web UI index.html not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.Write(index)
	})
}

func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/health", s.health)
	mux.HandleFunc("/api/state", s.state)
	mux.HandleFunc("/api/events", s.events)
	mux.HandleFunc("/api/ws", s.websocket)
	mux.HandleFunc("/api/agents/", s.agent)
	mux.HandleFunc("/api/translation/status", s.translationStatus)
	mux.HandleFunc("/api/translation/request", s.translationRequest)
	mux.HandleFunc("/api/translation/pause", s.translationPause)
	mux.HandleFunc("/api/translation/resume", s.translationResume)
	mux.HandleFunc("/api/translation/chapter/", s.translationChapterControl)
	mux.HandleFunc("/api/translation/retry", s.translationRetry)
	mux.HandleFunc("/api/translation/report", s.translationReport)
	mux.HandleFunc("/api/translation/glossary", s.translationGlossaryManagement)
	mux.HandleFunc("/api/snapshots", s.manuscriptSnapshots)
	mux.HandleFunc("/api/snapshots/restore", s.restoreManuscriptSnapshot)
	mux.HandleFunc("/api/settings/model", s.modelSettingsManagement)
	mux.HandleFunc("/api/library", s.libraryHandler)
	mux.HandleFunc("/api/library/open", s.libraryOpen)
	mux.HandleFunc("/api/library/archive", s.libraryArchive)
	mux.HandleFunc("/api/library/active", s.libraryActive)
	mux.HandleFunc("/api/library/create-and-start", s.libraryCreateAndStart)
	mux.HandleFunc("/api/export", s.exportHandler)
	mux.HandleFunc("/api/manuscript/outline", s.manuscriptOutline)
	mux.HandleFunc("/api/manuscript/chapters/", s.manuscriptChapter)
	mux.HandleFunc("/api/engine/start", s.start)
	mux.HandleFunc("/api/engine/resume", s.resume)
	mux.HandleFunc("/api/engine/continue", s.continueEngine)
	mux.HandleFunc("/api/engine/steer", s.steer)
	mux.HandleFunc("/api/engine/abort", s.abort)
	mux.HandleFunc("/api/engine/advance", s.advance)
	mux.HandleFunc("/api/engine/advance/next", s.advanceNext)
	mux.HandleFunc("/api/cocreate", s.cocreateHandler)
	mux.HandleFunc("/api/cocreate/", s.cocreateHandler)
	return mux
}

type stateResponse struct {
	Online            bool                `json:"online"`
	Snapshot          snapshotResponse    `json:"snapshot"`
	Agents            []agentResponse     `json:"agents"`
	Translation       *translation.Status `json:"translation,omitempty"`
	TranslationPaused bool                `json:"translation_paused"`
	UpdatedAt         time.Time           `json:"updated_at"`
}

type snapshotResponse struct {
	NovelName            string  `json:"novel_name"`
	RuntimeState         string  `json:"runtime_state"`
	StatusLabel          string  `json:"status_label"`
	Phase                string  `json:"phase"`
	Flow                 string  `json:"flow"`
	CurrentChapter       int     `json:"current_chapter"`
	TotalChapters        int     `json:"total_chapters"`
	CompletedCount       int     `json:"completed_count"`
	TotalWordCount       int     `json:"total_word_count"`
	TotalCostUSD         float64 `json:"total_cost_usd"`
	BudgetLimitUSD       float64 `json:"budget_limit_usd"`
	IsRunning            bool    `json:"is_running"`
	CurrentVolumeArc     string  `json:"current_volume_arc"`
	NextVolumeTitle      string  `json:"next_volume_title"`
	LastCommit           string  `json:"last_commit_summary"`
	LastReview           string  `json:"last_review_summary"`
	AdvanceMode          string  `json:"advance_mode,omitempty"`
	AdvancePermitChapter int     `json:"advance_permit_chapter,omitempty"`
	HasAdvanceHold       bool    `json:"has_advance_hold,omitempty"`
	AdvanceHoldReason    string  `json:"advance_hold_reason,omitempty"`
	PendingSteer         string  `json:"pending_steer,omitempty"`
	BookDir              string  `json:"book_dir,omitempty"`
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
	out := toSnapshot(snap)
	out.BookDir = s.runtime.ActiveBookDir()
	writeJSON(w, http.StatusOK, stateResponse{
		Online: true, Snapshot: out, Agents: agents, Translation: s.translationSnapshot(), TranslationPaused: s.runtime.IsTranslationPaused(), UpdatedAt: time.Now(),
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
	prompt := strings.TrimSpace(req.Prompt)
	if prompt == "" {
		writeError(w, http.StatusBadRequest, "prompt is required")
		return
	}
	if err := s.runtime.PrepareUserRules(prompt); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	if err := s.runtime.StartPrepared(prompt); err != nil {
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

func (s *Server) steer(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req promptRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	text := strings.TrimSpace(req.Prompt)
	if text == "" {
		writeError(w, http.StatusBadRequest, "prompt is required")
		return
	}
	if err := s.runtime.Steer(text); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"ok": true})
}

func (s *Server) advance(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		snap := s.runtime.Snapshot()
		writeJSON(w, http.StatusOK, map[string]any{
			"mode":           snap.AdvanceMode,
			"permit_chapter": snap.AdvancePermitChapter,
			"has_hold":       snap.HasAdvanceHold,
			"hold_reason":    snap.AdvanceHoldReason,
			"is_running":     snap.IsRunning,
		})
	case http.MethodPost:
		var body struct {
			Mode string `json:"mode"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		mode := strings.ToLower(strings.TrimSpace(body.Mode))
		// Accept VI aliases used in TUI.
		switch mode {
		case "bat", "on", "review":
			mode = string(domain.ChapterAdvanceReview)
		case "tat", "off", "auto":
			mode = string(domain.ChapterAdvanceAuto)
		}
		if err := s.runtime.SetAdvanceMode(domain.ChapterAdvanceMode(mode)); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		snap := s.runtime.Snapshot()
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "mode": snap.AdvanceMode})
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) advanceNext(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if err := s.runtime.AdvanceOneChapter(); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"ok": true})
}

func toSnapshot(s host.UISnapshot) snapshotResponse {
	return snapshotResponse{
		NovelName: s.NovelName, RuntimeState: s.RuntimeState, StatusLabel: s.StatusLabel,
		Phase: s.Phase, Flow: s.Flow, CurrentChapter: s.CurrentChapter, TotalChapters: s.TotalChapters,
		CompletedCount: s.CompletedCount, TotalWordCount: s.TotalWordCount, TotalCostUSD: s.TotalCostUSD,
		BudgetLimitUSD: s.BudgetLimitUSD, IsRunning: s.IsRunning, CurrentVolumeArc: s.CurrentVolumeArc,
		NextVolumeTitle: s.NextVolumeTitle, LastCommit: s.LastCommitSummary, LastReview: s.LastReviewSummary,
		AdvanceMode: s.AdvanceMode, AdvancePermitChapter: s.AdvancePermitChapter,
		HasAdvanceHold: s.HasAdvanceHold, AdvanceHoldReason: s.AdvanceHoldReason,
		PendingSteer: s.PendingSteer,
	}
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

func withTokenAuth(token string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Chỉ yêu cầu token đối với các action thay đổi trạng thái / điều khiển
		path := r.URL.Path
		isControlAction := strings.HasPrefix(path, "/api/engine/") ||
			strings.HasPrefix(path, "/api/translation/chapter/") ||
			(r.Method == http.MethodPost && (path == "/api/translation/request" || path == "/api/translation/retry" ||
				path == "/api/translation/pause" || path == "/api/translation/resume" ||
				path == "/api/translation/glossary" || path == "/api/snapshots" ||
				path == "/api/snapshots/restore" || path == "/api/settings/model" ||
				path == "/api/library" || path == "/api/library/open" ||
				path == "/api/library/archive" || path == "/api/library/create-and-start" ||
				path == "/api/export" || strings.HasPrefix(path, "/api/cocreate/")))
		if isControlAction {
			authHeader := r.Header.Get("Authorization")
			queryToken := r.URL.Query().Get("token")
			provided := ""
			if strings.HasPrefix(authHeader, "Bearer ") {
				provided = strings.TrimPrefix(authHeader, "Bearer ")
			} else if queryToken != "" {
				provided = queryToken
			}
			if provided != token {
				writeError(w, http.StatusUnauthorized, "Unauthorized: invalid or missing control token")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		// Let the dashboard read download filenames (filename* / X-Export-Filename).
		w.Header().Set("Access-Control-Expose-Headers", "Content-Disposition, X-Export-Filename")
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
