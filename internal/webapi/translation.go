package webapi

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/voocel/ainovel-cli/internal/translation"
)

func (s *Server) translationSnapshot() *translation.Status {
	status, err := s.runtime.TranslationStatus()
	if err != nil {
		return nil
	}
	return &status
}

func (s *Server) translationStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	status, err := s.runtime.TranslationStatus()
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"online":     true,
		"status":     status,
		"updated_at": status.UpdatedAt,
	})
}

type translationRetryRequest struct {
	Chapters []int `json:"chapters"`
}

func (s *Server) translationRetry(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var request translationRetryRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if len(request.Chapters) == 0 {
		writeError(w, http.StatusBadRequest, "chapters is required")
		return
	}
	if err := s.runtime.RetryTranslations(request.Chapters); err != nil {
		status := http.StatusConflict
		if strings.Contains(err.Error(), "required") {
			status = http.StatusBadRequest
		}
		writeError(w, status, err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{
		"ok":       true,
		"chapters": request.Chapters,
		"message":  "Đã xếp batch retry; trạng thái sẽ cập nhật theo event live.",
	})
}
