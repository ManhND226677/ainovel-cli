// Package i18n contains presentation strings for the Vietnamese fork.
//
// It deliberately does not translate prompts, tool contracts, or persisted story
// artifacts. Those remain Chinese-language inputs to the creative pipeline.
package i18n

import "fmt"

var vi = map[string]string{
	"command.help.name":             "tro-giup",
	"command.help.usage":            "/tro-giup",
	"command.help.description":      "Xem danh sách lệnh",
	"command.model.name":            "mo-hinh",
	"command.model.usage":           "/mo-hinh [vai-tro]",
	"command.model.description":     "Chuyển mô hình và mức suy luận theo vai trò",
	"command.config.name":           "cau-hinh",
	"command.config.usage":          "/cau-hinh",
	"command.config.description":    "Thêm hoặc chỉnh sửa nhà cung cấp, mô hình và cửa sổ ngữ cảnh",
	"command.diag.name":             "chan-doan",
	"command.diag.usage":            "/chan-doan",
	"command.diag.description":      "Chẩn đoán tình trạng sáng tác của tác phẩm",
	"command.review.name":           "duyet",
	"command.review.usage":          "/duyet bat|tat",
	"command.review.description":    "Bật hoặc tắt chế độ xét duyệt từng chương",
	"command.next.name":             "tiep-tuc",
	"command.next.usage":            "/tiep-tuc",
	"command.next.description":      "Cho phép viết thêm một chương sau khi xét duyệt",
	"command.import.name":           "nhap",
	"command.import.usage":          "/nhap <duong-dan> [--yes] [--story=open|closed] [--continue] [--guide=<huong-dan> ]",
	"command.import.description":    "Nhập truyện bên ngoài theo ngữ nghĩa; không có tham số sẽ tiếp tục lần nhập dở dang",
	"command.reopen.name":           "mo-lai",
	"command.reopen.usage":          "/mo-lai [huong-sang-tac]",
	"command.reopen.description":    "Mở lại tác phẩm đã hoàn tất để tiếp tục sáng tác",
	"command.cocreate.name":         "dong-sang-tac",
	"command.cocreate.usage":        "/dong-sang-tac",
	"command.cocreate.description":  "Tạm dừng để cùng lập kế hoạch cho giai đoạn tiếp theo",
	"command.simulate.name":         "mo-phong",
	"command.simulate.usage":        "/mo-phong",
	"command.simulate.description":  "Đọc ./simulate để tạo hoặc cập nhật hồ sơ mô phỏng văn phong",
	"command.importsim.name":        "nhap-mo-phong",
	"command.importsim.usage":       "/nhap-mo-phong <profile.json>",
	"command.importsim.description": "Nhập hồ sơ mô phỏng có sẵn và hợp nhất theo dấu vân tay ngữ liệu",
	"command.export.name":           "xuat",
	"command.export.usage":          "/xuat [duong-dan] [from=N] [to=M] [ngonngu=vi|zh] [--overwrite]",
	"command.export.description":    "Xuất các chương đã hoàn tất thành TXT/EPUB; dùng ngonngu=vi để xuất bản dịch Việt",
	"command.translate.name":        "dich",
	"command.translate.usage":       "/dich [trang-thai]",
	"command.translate.description": "Yêu cầu Agent điều phối đánh giá lô dịch hoặc xem trạng thái bản Việt",
	"help.title":                    "Trợ giúp lệnh",
	"help.alias":                    "bí danh",
	"help.usage":                    "Cách dùng",
	"help.shortcuts":                "Phím tắt",
	"modal.config.title":            "/cau-hinh · Cấu hình mô hình",
	"modal.config.hint":             "↑↓ chọn · Enter xác nhận · Esc hủy",
	"help.shortcut.search":          "Gõ / để tìm lệnh",
	"help.shortcut.select":          "↑↓ chọn lệnh gợi ý",
	"help.shortcut.complete":        "Tab/Enter chấp nhận gợi ý",
	"help.shortcut.close":           "Esc đóng bảng lệnh hiện tại",
	"help.shortcut.copy":            "Ctrl+R bật/tắt chế độ sao chép bằng chọn chuột",
	"help.footer":                   "  ↑↓ cuộn · Esc đóng",
	"event.unknown_role":            "Vai trò không xác định: %s",
	"event.usage":                   "Cách dùng: %s",
	"event.command_unknown":         "Lệnh không xác định: /%s",
	"event.command_needs_idle":      "Lệnh chỉ có thể chạy khi hệ thống đang rảnh: /%s",
	"event.export_starting":         "Đang xuất bản...",
	"event.export_started_failed":   "Không thể bắt đầu xuất bản: %s",
	"event.export_success":          "✓ Đã xuất %d chương / %s tới %s",
	"event.export_skipped":          " (bỏ qua %d chương chưa hoàn tất: %s)",
	"event.translation_requested":   "Đã yêu cầu Agent điều phối đánh giá lô dịch tiếng Việt.",
	"event.translation_status":      "Bản dịch Việt: %d hoàn tất, %d chờ/chạy, %d lỗi, %d stale.",
	"cli.error_headless_setup":      "chế độ headless không hỗ trợ thiết lập lần đầu; hãy chạy TUI một lần để hoàn tất cấu hình",
	"cli.error_positional_prompt":   "không còn hỗ trợ truyền yêu cầu truyện trực tiếp bằng dòng lệnh; hãy nhập trong TUI sau khi khởi động",
	"cli.error_prompt_headless":     "--prompt/--prompt-file chỉ dùng được trong chế độ --headless",
	"cli.error_prompt_read":         "không thể đọc prompt: %w",
	"cli.error_detail_written":      "(chi tiết lỗi đã được ghi vào %s)",
	"cli.press_enter_exit":          "\nNhấn Enter để thoát...",
}

// T returns a Vietnamese presentation string. A missing key deliberately falls
// back to the key, which makes omissions visible in development and tests.
func T(key string, args ...any) string {
	text, ok := vi[key]
	if !ok {
		return key
	}
	if len(args) == 0 {
		return text
	}
	return fmt.Sprintf(text, args...)
}
