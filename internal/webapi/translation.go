package webapi

import (
	"encoding/json"
	"fmt"
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

func (s *Server) translationReport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	status, err := s.runtime.TranslationStatus()
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	format := strings.ToLower(r.URL.Query().Get("format"))
	if format == "" {
		format = "md"
	}
	jobID := strings.TrimSpace(r.URL.Query().Get("job_id"))
	if jobID != "" {
		if _, ok := status.Jobs[jobID]; !ok {
			writeError(w, http.StatusNotFound, "Job ID không tồn tại trong trạng thái bền vững.")
			return
		}
	}

	var sb strings.Builder
	if format == "txt" {
		sb.WriteString("=== BAO CAO TIEN DO DICH TRUYEN (AINOVEL-CLI) ===\n")
		sb.WriteString("Thoi gian xuat: " + status.UpdatedAt.Format("2006-01-02 15:04:05") + "\n\n")
		if jobID != "" {
			if job, ok := status.Jobs[jobID]; ok {
				sb.WriteString(fmt.Sprintf("Job ID: %s\nTrang thai: %s\nLy do: %s\nSo lan thu: %d\n\n", job.ID, job.State, job.Reason, job.Attempts))
			}
		}
		sb.WriteString("DANH SACH CHUONG:\n")
		for _, ch := range status.Chapters {
			if jobID != "" && ch.JobID != jobID {
				continue
			}
			sb.WriteString(strings.Repeat("-", 40) + "\n")
			sb.WriteString(fmt.Sprintf("Chuong: %d\nTrang thai: %s\nSo lan thu: %d\nVan tay nguon: %s\nCap nhat: %s\nLoi: %s\n",
				ch.Chapter, ch.State, ch.Attempts, ch.SourceSHA256, ch.UpdatedAt.Format("15:04:05"), ch.LastError))
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Content-Disposition", "attachment; filename=\"ainovel-translation-report.txt\"")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(sb.String()))
		return
	}

	// Mặc định Markdown
	sb.WriteString("# Báo cáo Tiến độ Dịch thuật (ainovel-cli)\n\n")
	sb.WriteString("- **Thời điểm xuất:** " + status.UpdatedAt.Format("02/01/2006 15:04:05 UTC") + "\n")
	if jobID != "" {
		sb.WriteString("- **Lọc theo Job ID:** `" + jobID + "`\n")
	}
	sb.WriteString("\n## Tổng quan các lô (Jobs)\n\n")
	sb.WriteString("| Job ID | Trạng thái | Lý do / Phạm vi | Số lần thử | Lỗi gần nhất |\n")
	sb.WriteString("|---|---|---|---|---|\n")
	for jid, job := range status.Jobs {
		if jobID != "" && jid != jobID {
			continue
		}
		chaptersStr := strings.Trim(strings.Join(strings.Fields(fmt.Sprintf("%v", job.Chapters)), ", "), "[]")
		sb.WriteString(fmt.Sprintf("| `%s` | **%s** | %s (chương: %s) | %d | %s |\n", jid[:8], job.State, job.Reason, chaptersStr, job.Attempts, job.LastError))
	}

	sb.WriteString("\n## Sổ chi tiết các chương\n\n")
	sb.WriteString("| Chương | Trạng thái | Lần thử | Dấu vân tay nguồn (SHA-256) | Cập nhật | Lỗi gần nhất |\n")
	sb.WriteString("|---|---|---|---|---|---|\n")
	for _, ch := range status.Chapters {
		if jobID != "" && ch.JobID != jobID {
			continue
		}
		sha := ch.SourceSHA256
		if len(sha) > 12 {
			sha = sha[:12] + "…"
		}
		sb.WriteString(fmt.Sprintf("| **%02d** | `%s` | %d | `%s` | %s | %s |\n", ch.Chapter, ch.State, ch.Attempts, sha, ch.UpdatedAt.Format("15:04:05"), ch.LastError))
	}
	sb.WriteString("\n> *Ghi chú: Bản tiếng Trung gốc được giữ nguyên bất biến; báo cáo này chỉ phản ánh các lô dịch sang tiếng Việt.*\n")

	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=\"ainovel-translation-report.md\"")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(sb.String()))
}
