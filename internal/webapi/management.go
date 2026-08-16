package webapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/voocel/ainovel-cli/internal/bootstrap"
	"github.com/voocel/ainovel-cli/internal/host"
	"github.com/voocel/ainovel-cli/internal/translation"
)

type managementGlossaryRequest struct {
	Entries map[string]string `json:"entries"`
}

func (s *Server) translationRequest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if err := s.runtime.RequestTranslation(); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{
		"ok":      true,
		"message": "Translation Coordinator đang đánh giá các chương đã chốt để mở lô dịch.",
	})
}

func (s *Server) translationPause(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if err := s.runtime.PauseTranslation(); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "paused": true, "message": "Đã tạm dừng nhận chapter mới."})
}

func (s *Server) translationResume(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if err := s.runtime.ResumeTranslation(); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "paused": false, "message": "Đã tiếp tục hàng đợi dịch."})
}

type chapterInstructionRequest struct {
	Instruction string `json:"instruction"`
}

// translationChapterControl handles POST /chapter/{n}/stop and
// POST /chapter/{n}/instruction. The token middleware protects both routes.
func (s *Server) translationChapterControl(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/translation/chapter/"), "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		writeError(w, http.StatusNotFound, "translation chapter control not found")
		return
	}
	chapter, err := strconv.Atoi(parts[0])
	if err != nil || chapter <= 0 {
		writeError(w, http.StatusBadRequest, "chapter must be a positive integer")
		return
	}
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	switch parts[1] {
	case "stop":
		stopped, err := s.runtime.StopTranslationChapter(chapter)
		if err != nil {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		if !stopped {
			writeError(w, http.StatusConflict, "chapter is not currently being translated")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": fmt.Sprintf("Đã yêu cầu dừng chapter %d.", chapter)})
	case "instruction":
		var request chapterInstructionRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		if len([]rune(strings.TrimSpace(request.Instruction))) > 4000 {
			writeError(w, http.StatusBadRequest, "instruction must not exceed 4000 characters")
			return
		}
		record, err := s.runtime.SetTranslationInstruction(chapter, request.Instruction)
		if err != nil {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, record)
	default:
		writeError(w, http.StatusNotFound, "translation chapter control not found")
	}
}

func (s *Server) translationGlossaryManagement(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		glossary, err := s.runtime.TranslationGlossary()
		if err != nil {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, glossary)
	case http.MethodPost:
		var request managementGlossaryRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		glossary, err := s.runtime.UpdateTranslationGlossary(request.Entries)
		if err != nil {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, glossary)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

type snapshotCreateRequest struct {
	Title string `json:"title"`
}

func (s *Server) manuscriptSnapshots(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		items, err := s.runtime.ListManuscriptSnapshots()
		if err != nil {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": items})
	case http.MethodPost:
		var request snapshotCreateRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		item, err := s.runtime.CreateManuscriptSnapshot(request.Title)
		if err != nil {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, item)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

type restoreSnapshotRequest struct {
	ID      string `json:"id"`
	Confirm bool   `json:"confirm"`
}

func (s *Server) restoreManuscriptSnapshot(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var request restoreSnapshotRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if strings.TrimSpace(request.ID) == "" || !request.Confirm {
		writeError(w, http.StatusBadRequest, "snapshot id and confirm=true are required")
		return
	}
	if err := s.runtime.RestoreManuscriptSnapshot(request.ID); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": "Đã khôi phục snapshot và tạo bản sao an toàn trước đó."})
}

type managementModelSettingsRequest struct {
	Provider          string  `json:"provider"`
	Model             string  `json:"model"`
	BaseURL           string  `json:"base_url"`
	APIKey            string  `json:"api_key"`
	TestOnly          bool    `json:"test_only"`
	Role              string  `json:"role"`
	Temp              float64 `json:"temperature"`
	ContextWindow     int     `json:"context_window"`
	AddModelIfMissing bool    `json:"add_model_if_missing"`
}

func (s *Server) modelSettingsManagement(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		writeJSON(w, http.StatusOK, s.runtime.ModelConfiguration())
		return
	}
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var request managementModelSettingsRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	snapshot := s.runtime.ModelConfiguration()
	provider := strings.TrimSpace(request.Provider)
	if provider == "" {
		provider = snapshot.DefaultProvider
	}
	var current *host.ProviderSnapshot
	for i := range snapshot.Providers {
		if snapshot.Providers[i].Name == provider {
			current = &snapshot.Providers[i]
			break
		}
	}
	if current == nil {
		writeError(w, http.StatusBadRequest, "provider not found in current configuration")
		return
	}
	model := strings.TrimSpace(request.Model)
	if model == "" {
		model = snapshot.DefaultModel
	}
	if model == "" {
		writeError(w, http.StatusBadRequest, "model is required")
		return
	}

	// Start from the provider's registered model library and ensure the chosen
	// model is present. Dashboard users must be able to type a free-form model
	// id (OpenRouter/zyloo slugs, etc.) instead of being locked to a <select>.
	models := append([]bootstrap.ModelConfig(nil), current.Models...)
	found := false
	for i := range models {
		if strings.TrimSpace(models[i].Name) == model {
			found = true
			if request.ContextWindow > 0 {
				models[i].ContextWindow = request.ContextWindow
			}
			break
		}
	}
	addMissing := request.AddModelIfMissing
	if !request.TestOnly {
		// Saving always registers an unknown model so /model and CandidateModels
		// keep showing it after reload.
		addMissing = true
	}
	if !found && addMissing {
		entry := bootstrap.ModelConfig{Name: model}
		if request.ContextWindow > 0 {
			entry.ContextWindow = request.ContextWindow
		}
		models = append(models, entry)
	}

	draft := host.ModelConfigurationDraft{
		Provider: provider, Type: current.Type, API: current.API, BaseURL: current.BaseURL,
		Models: models, APIKeyAction: host.APIKeyKeep,
	}
	if strings.TrimSpace(request.BaseURL) != "" {
		draft.BaseURL = strings.TrimSpace(request.BaseURL)
	}
	if strings.TrimSpace(request.APIKey) != "" {
		draft.APIKeyAction = host.APIKeyReplace
		draft.APIKey = request.APIKey
	}
	if request.TestOnly {
		if err := s.runtime.TestModelConnection(r.Context(), draft, model); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": "Kết nối model thành công; cấu hình chưa được lưu."})
		return
	}
	if err := s.runtime.ConfigureModels(draft); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.runtime.SwitchModel(request.Role, provider, model); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"message": fmt.Sprintf("Đã lưu và áp dụng %s/%s.", provider, model),
		"model":   model,
		"added":   !found,
	})
}

// Keep the compiler honest if the durable translation contract is refactored.
var _ translation.Glossary
