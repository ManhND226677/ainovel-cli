package webapi

import (
	"net/http"
	"strconv"
	"strings"
	"time"
)

func (s *Server) manuscriptOutline(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if s.runtime == nil {
		writeError(w, http.StatusServiceUnavailable, "engine runtime unavailable")
		return
	}
	items, err := s.runtime.ManuscriptOutline()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items":      items,
		"updated_at": time.Now(),
	})
}

func (s *Server) manuscriptChapter(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if s.runtime == nil {
		writeError(w, http.StatusServiceUnavailable, "engine runtime unavailable")
		return
	}
	raw := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/manuscript/chapters/"), "/")
	if raw == "" {
		writeError(w, http.StatusBadRequest, "chapter is required")
		return
	}
	chapter, err := strconv.Atoi(raw)
	if err != nil || chapter < 1 {
		writeError(w, http.StatusBadRequest, "invalid chapter number")
		return
	}
	item, err := s.runtime.LoadChapterManuscript(chapter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, item)
}
