package webapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/voocel/ainovel-cli/internal/host"
	"github.com/voocel/ainovel-cli/internal/library"
)

type webCoCreate struct {
	mu          sync.Mutex
	mode        string
	title       string
	history     []host.CoCreateMessage
	draft       string
	ready       bool
	suggestions []string
	lastReply   string
	busy        bool
	updatedAt   time.Time
}

func stripCoCreateDisplay(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	low := strings.ToLower(raw)
	if i := strings.Index(low, "<reply>"); i >= 0 {
		rest := raw[i+len("<reply>"):]
		low2 := strings.ToLower(rest)
		if j := strings.Index(low2, "</reply>"); j >= 0 {
			return strings.TrimSpace(rest[:j])
		}
	}
	return raw
}

func (s *Server) cocreateSnapshotLocked(sess *webCoCreate) map[string]any {
	if sess == nil {
		return map[string]any{"active": false}
	}
	uiHist := make([]map[string]string, 0, len(sess.history))
	for i, m := range sess.history {
		c := m.Content
		if strings.EqualFold(m.Role, "assistant") {
			if i == len(sess.history)-1 && strings.TrimSpace(sess.lastReply) != "" {
				c = sess.lastReply
			} else {
				c = stripCoCreateDisplay(c)
			}
		}
		uiHist = append(uiHist, map[string]string{"role": m.Role, "content": c})
	}
	return map[string]any{
		"active": true, "mode": sess.mode, "title": sess.title, "draft": sess.draft,
		"ready": sess.ready, "can_commit": strings.TrimSpace(sess.draft) != "",
		"suggestions": append([]string(nil), sess.suggestions...), "busy": sess.busy,
		"history": uiHist, "updated_at": sess.updatedAt,
		"stage_host": s.runtime != nil && s.runtime.IsCoCreating(),
	}
}

func (s *Server) cocreateHandler(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimSuffix(r.URL.Path, "/")
	switch path {
	case "/api/cocreate", "/api/cocreate/status":
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		s.cocreateMu.Lock()
		out := s.cocreateSnapshotLocked(s.cocreate)
		s.cocreateMu.Unlock()
		writeJSON(w, http.StatusOK, out)
	case "/api/cocreate/start":
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		s.cocreateStart(w, r)
	case "/api/cocreate/message":
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		s.cocreateMessage(w, r)
	case "/api/cocreate/commit":
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		s.cocreateCommit(w, r)
	case "/api/cocreate/cancel":
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		s.cocreateCancel(w, r)
	default:
		writeError(w, http.StatusNotFound, "cocreate route not found")
	}
}

func (s *Server) cocreateStart(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Mode    string `json:"mode"`
		Message string `json:"message"`
		Title   string `json:"title"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	mode := strings.ToLower(strings.TrimSpace(body.Mode))
	if mode == "" || mode == "cold" || mode == "new" {
		mode = "startup"
	}
	if mode != "startup" && mode != "stage" {
		writeError(w, http.StatusBadRequest, "mode must be startup or stage")
		return
	}
	msg := strings.TrimSpace(body.Message)
	if msg == "" {
		writeError(w, http.StatusBadRequest, "message is required")
		return
	}
	s.cocreateMu.Lock()
	if s.cocreate != nil && s.cocreate.busy {
		s.cocreateMu.Unlock()
		writeError(w, http.StatusConflict, "co-create is busy")
		return
	}
	if mode == "stage" {
		if s.runtime == nil || !s.runtime.PauseForCoCreate() {
			s.cocreateMu.Unlock()
			writeError(w, http.StatusConflict, "cannot enter stage co-create")
			return
		}
	}
	s.cocreate = &webCoCreate{mode: mode, title: strings.TrimSpace(body.Title), history: []host.CoCreateMessage{{Role: "user", Content: msg}}, updatedAt: time.Now().UTC(), busy: true}
	sess := s.cocreate
	s.cocreateMu.Unlock()
	reply, err := s.runCoCreate(sess)
	s.cocreateMu.Lock()
	defer s.cocreateMu.Unlock()
	if s.cocreate != sess {
		writeError(w, http.StatusConflict, "session replaced")
		return
	}
	sess.busy = false
	sess.updatedAt = time.Now().UTC()
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error(), "session": s.cocreateSnapshotLocked(sess)})
		return
	}
	applyCoCreateReply(sess, reply)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "session": s.cocreateSnapshotLocked(sess)})
}

func (s *Server) cocreateMessage(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Message string `json:"message"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	msg := strings.TrimSpace(body.Message)
	if msg == "" {
		writeError(w, http.StatusBadRequest, "message is required")
		return
	}
	s.cocreateMu.Lock()
	sess := s.cocreate
	if sess == nil {
		s.cocreateMu.Unlock()
		writeError(w, http.StatusConflict, "no active co-create session")
		return
	}
	if sess.busy {
		s.cocreateMu.Unlock()
		writeError(w, http.StatusConflict, "co-create is busy")
		return
	}
	sess.history = append(sess.history, host.CoCreateMessage{Role: "user", Content: msg})
	sess.suggestions = nil
	sess.busy = true
	sess.updatedAt = time.Now().UTC()
	s.cocreateMu.Unlock()
	reply, err := s.runCoCreate(sess)
	s.cocreateMu.Lock()
	defer s.cocreateMu.Unlock()
	if s.cocreate != sess {
		writeError(w, http.StatusConflict, "session replaced")
		return
	}
	sess.busy = false
	sess.updatedAt = time.Now().UTC()
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error(), "session": s.cocreateSnapshotLocked(sess)})
		return
	}
	applyCoCreateReply(sess, reply)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "session": s.cocreateSnapshotLocked(sess)})
}

func (s *Server) runCoCreate(sess *webCoCreate) (host.CoCreateReply, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	history := append([]host.CoCreateMessage(nil), sess.history...)
	if sess.mode == "stage" {
		return s.runtime.StageCoCreateStream(ctx, history, nil)
	}
	return s.runtime.CoCreateStream(ctx, history, nil)
}

func applyCoCreateReply(sess *webCoCreate, reply host.CoCreateReply) {
	raw := strings.TrimSpace(reply.Raw)
	if raw == "" {
		raw = strings.TrimSpace(reply.Message)
	}
	if raw != "" {
		sess.history = append(sess.history, host.CoCreateMessage{Role: "assistant", Content: raw})
	}
	if p := strings.TrimSpace(reply.Prompt); p != "" {
		sess.draft = p
	}
	sess.ready = reply.Ready
	sess.suggestions = append([]string(nil), reply.Suggestions...)
	sess.lastReply = strings.TrimSpace(reply.Message)
	if sess.lastReply == "" {
		sess.lastReply = stripCoCreateDisplay(raw)
	}
}

func (s *Server) cocreateCommit(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Title string `json:"title"`
		Draft string `json:"draft"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	s.cocreateMu.Lock()
	sess := s.cocreate
	if sess == nil {
		s.cocreateMu.Unlock()
		writeError(w, http.StatusConflict, "no active co-create session")
		return
	}
	if sess.busy {
		s.cocreateMu.Unlock()
		writeError(w, http.StatusConflict, "co-create is busy")
		return
	}
	draft := strings.TrimSpace(body.Draft)
	if draft == "" {
		draft = strings.TrimSpace(sess.draft)
	}
	mode := sess.mode
	title := strings.TrimSpace(body.Title)
	if title == "" {
		title = strings.TrimSpace(sess.title)
	}
	s.cocreateMu.Unlock()
	if draft == "" {
		writeError(w, http.StatusBadRequest, "draft is empty")
		return
	}
	if mode == "stage" {
		if err := s.runtime.ResumeFromCoCreate(draft); err != nil {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		s.cocreateMu.Lock()
		s.cocreate = nil
		s.cocreateMu.Unlock()
		writeJSON(w, http.StatusAccepted, map[string]any{"ok": true, "mode": "stage", "message": "Đã chốt hướng và tiếp tục sáng tác"})
		return
	}
	if title == "" {
		runes := []rune(strings.Split(draft, "\n")[0])
		if len(runes) > 36 {
			title = string(runes[:36])
		} else if len(runes) > 0 {
			title = string(runes)
		} else {
			title = "Truyện đồng sáng tác"
		}
	}
	if _, err := s.runtime.CreateBookAndOpen(s.libraryRoot(), library.CreateInput{Title: title}); err != nil {
		if inspect := s.runtime.InspectActiveBook(); inspect.BlocksNewStart {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
	}
	if err := s.runtime.PrepareUserRules(draft); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	if err := s.runtime.StartPrepared(draft); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	s.cocreateMu.Lock()
	s.cocreate = nil
	s.cocreateMu.Unlock()
	writeJSON(w, http.StatusAccepted, map[string]any{"ok": true, "mode": "startup", "message": "Đã chốt brief và bắt đầu sáng tác", "title": title, "dir": s.runtime.ActiveBookDir()})
}

func (s *Server) cocreateCancel(w http.ResponseWriter, r *http.Request) {
	s.cocreateMu.Lock()
	mode := ""
	if s.cocreate != nil {
		mode = s.cocreate.mode
	}
	s.cocreate = nil
	s.cocreateMu.Unlock()
	if mode == "stage" && s.runtime != nil {
		s.runtime.CancelCoCreate()
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
