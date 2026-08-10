package diag

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/voocel/ainovel-cli/internal/store"
)

// ExportRelPath 是脱敏诊断文件相对 output 目录的固定位置（覆盖式一份）。
const ExportRelPath = "meta/diag-export.md"

// Export 完整诊断 + 渲染 + 落盘，返回写出的绝对路径。供 headless / 外部调用。
func Export(s *store.Store) (string, error) {
	rep, rc := Diagnose(s)
	return WriteExport(s, rep, rc)
}

// WriteExport 把已算好的 Report + RuntimeCapture 渲染落盘，不重复抓取。
// 供 /diag 命令复用 Diagnose 的结果。
func WriteExport(s *store.Store, rep Report, rc RuntimeCapture) (string, error) {
	data := RenderExport(rep, rc)
	abs := filepath.Join(s.Dir(), filepath.FromSlash(ExportRelPath))
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(abs, data, 0o644); err != nil {
		return "", err
	}
	return abs, nil
}

// RenderExport 把创作 Report + 运行时抓取组合成脱敏 Markdown。
func RenderExport(rep Report, rc RuntimeCapture) []byte {
	var b strings.Builder
	st := rep.Stats

	b.WriteString("# diag-export\n\n")
	fmt.Fprintf(&b, "> Thời gian tạo %s · %s/%s\n", time.Now().Format("2006-01-02 15:04:05"), rc.GoOS, rc.GoArch)
	b.WriteString("> ⚠️ Đã ẩn danh: Nội dung chính/lệnh (prompt)/quá trình suy nghĩ đã bị loại bỏ, chỉ giữ lại khung hành vi. Có thể gửi trực tiếp vào issue.\n\n")

	// 1. 环境
	b.WriteString("## 1. Môi trường\n\n")
	fmt.Fprintf(&b, "- Giai đoạn `%s`", orDash(st.Phase))
	if st.Flow != "" {
		fmt.Fprintf(&b, " / luồng `%s`", st.Flow)
	}
	fmt.Fprintf(&b, " · Chương %d/%d · Số từ %d\n", st.CompletedChapters, st.TotalChapters, st.TotalWords)
	if st.PlanningTier != "" {
		fmt.Fprintf(&b, "- Kế hoạch `%s`\n", st.PlanningTier)
	}
	for _, m := range rc.Models {
		fmt.Fprintf(&b, "- %s → `%s` / `%s`\n", m.Agent, orDash(m.Provider), orDash(m.Model))
	}

	// 2. 诊断发现（仅运行时；创作类诊断含剧情/伏笔，留在 /diag 屏上报告，不进可分享导出）
	b.WriteString("\n## 2. Phát hiện chẩn đoán (Lúc chạy)\n\n")
	rf := runtimeFindings(&rc)
	sortFindings(rf)
	if len(rf) == 0 {
		b.WriteString("Không có cảnh báo khi chạy.\n")
	} else {
		for _, f := range rf {
			fmt.Fprintf(&b, "- [%s] %s\n", f.Severity, f.Title)
			if f.Evidence != "" {
				fmt.Fprintf(&b, "  - Dấu vết: %s\n", f.Evidence)
			}
			if f.Suggestion != "" {
				fmt.Fprintf(&b, "  - → %s\n", f.Suggestion)
			}
		}
	}

	// 3. 运行时信号（原始聚合）
	b.WriteString("\n## 3. Tín hiệu thời gian chạy\n\n")
	wrote := false
	if rc.CurrentStep != "" {
		fmt.Fprintf(&b, "- Bước hiện tại `%s`\n", rc.CurrentStep)
		wrote = true
	}
	if rc.StuckStep != "" {
		fmt.Fprintf(&b, "- ⚠️ Bị kẹt: liên tục kẹt ở `%s` ×%d\n", rc.StuckStep, rc.StuckCount)
		wrote = true
	}
	if len(rc.Repeats) > 0 {
		b.WriteString("- Chữ ký tần số cao (cửa sổ gần ≥3 lần, bao gồm công cụ lặp lại bình thường, chỉ mang tính chất tham khảo):\n")
		for _, r := range rc.Repeats {
			fmt.Fprintf(&b, "  - `%s` ×%d\n", r.Sig, r.Count)
		}
		wrote = true
	}
	if len(rc.DupContent) > 0 {
		b.WriteString("- Tạo nội dung trùng lặp lặp đi lặp lại (cùng một sha):\n")
		for _, d := range rc.DupContent {
			fmt.Fprintf(&b, "  - sha=%s ×%d\n", d.Sha, d.Count)
		}
		wrote = true
	}
	if len(rc.LogKinds) > 0 {
		b.WriteString("- Phân loại lỗi nhật ký: ")
		b.WriteString(joinKinds(rc.LogKinds))
		b.WriteString("\n")
		wrote = true
	}
	if rc.LogErrors > 0 || rc.LogWarns > 0 {
		fmt.Fprintf(&b, "- Nhật ký lỗi (error) ×%d · cảnh báo (warn) ×%d\n", rc.LogErrors, rc.LogWarns)
		wrote = true
	}
	if rc.StopGuard > 0 {
		fmt.Fprintf(&b, "- StopGuard đánh chặn ×%d\n", rc.StopGuard)
		wrote = true
	}
	if !wrote {
		b.WriteString("- Không có tín hiệu bất thường rõ ràng khi chạy.\n")
	}

	// 4. 行为骨架尾巴
	fmt.Fprintf(&b, "\n## 4. Phần đuôi khung hành vi (Cuối %d dòng)\n\n", len(rc.Tail))
	if len(rc.Tail) == 0 {
		b.WriteString("(Không có lịch sử phiên)\n")
	} else {
		b.WriteString("```\n")
		for _, ev := range rc.Tail {
			b.WriteString(formatSkel(ev))
			b.WriteString("\n")
		}
		b.WriteString("```\n")
	}

	// 5. 脱敏自检
	b.WriteString("\n## 5. Tự kiểm tra che giấu dữ liệu\n\n")
	fmt.Fprintf(&b, "- %d khối văn bản bị ẩn danh · 0 phần văn bản tiểu thuyết bị lộ\n", rc.RedactedTexts)
	if len(rc.Sources) > 0 {
		fmt.Fprintf(&b, "- Nguồn dữ liệu: %s\n", strings.Join(rc.Sources, " · "))
	}

	return []byte(b.String())
}

// formatSkel 把一条骨架渲染成单行，看派发先后顺序。
func formatSkel(ev SkelEvent) string {
	var parts []string
	parts = append(parts, "["+ev.Agent+"/"+ev.Role+"]")
	for _, t := range ev.Tools {
		parts = append(parts, t.Name+formatArgs(t.Args)+invalidTag(t))
	}
	if ev.ErrClass != "" {
		parts = append(parts, "lỗi: "+ev.ErrClass)
	}
	if len(ev.Tools) == 0 && ev.ErrClass == "" && ev.TextSha != "" {
		parts = append(parts, "text<sha="+ev.TextSha+">")
	}
	return strings.Join(parts, " ")
}

func formatArgs(args map[string]string) string {
	if len(args) == 0 {
		return ""
	}
	keys := make([]string, 0, len(args))
	for k := range args {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	pairs := make([]string, 0, len(keys))
	for _, k := range keys {
		pairs = append(pairs, k+": "+args[k])
	}
	return "{" + strings.Join(pairs, ", ") + "}"
}

func invalidTag(t SkelTool) string {
	if !t.Invalid {
		return ""
	}
	if t.ParseErr != "" {
		return " ⚠️tham số không hợp lệ(" + firstLine(t.ParseErr, 80) + ")"
	}
	return " ⚠️tham số không hợp lệ"
}

func joinKinds(m map[string]int) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s ×%d", k, m[k]))
	}
	return strings.Join(parts, " · ")
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
