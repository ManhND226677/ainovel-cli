package webapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

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
	Provider string  `json:"provider"`
	Model    string  `json:"model"`
	BaseURL  string  `json:"base_url"`
	APIKey   string  `json:"api_key"`
	TestOnly bool    `json:"test_only"`
	Role     string  `json:"role"`
	Temp     float64 `json:"temperature"`
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
	draft := host.ModelConfigurationDraft{
		Provider: provider, Type: current.Type, API: current.API, BaseURL: current.BaseURL,
		Models: current.Models, APIKeyAction: host.APIKeyKeep,
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
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": fmt.Sprintf("Đã lưu và áp dụng %s/%s.", provider, model)})
}

// Keep the compiler honest if the durable translation contract is refactored.
var _ translation.Glossary
