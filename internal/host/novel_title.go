package host

import "strings"

// knownVietnameseTitles maps Chinese novel names (progress.NovelName) to the
// Vietnamese display title used in the dashboard and VI exports.
// Keep in sync with web-dashboard/client/src/lib/novelTitle.ts when adding books.
var knownVietnameseTitles = map[string]string{
	"仙道畸变：我能锁定理智": "Tiên Đạo Dị Biến Ta Có Thể Khóa Chặt Lý Trí",
}

// ResolveVietnameseTitle returns a Vietnamese book title for export/UI.
// Priority: explicit override → known map → empty (caller may fall back to ZH).
func ResolveVietnameseTitle(chineseName, overrideVI string) string {
	if t := strings.TrimSpace(overrideVI); t != "" {
		return t
	}
	key := strings.TrimSpace(chineseName)
	if key == "" {
		return ""
	}
	if t, ok := knownVietnameseTitles[key]; ok {
		return t
	}
	// Also try fullwidth/colon variants
	alt := strings.ReplaceAll(key, ":", "：")
	if t, ok := knownVietnameseTitles[alt]; ok {
		return t
	}
	alt = strings.ReplaceAll(key, "：", ":")
	if t, ok := knownVietnameseTitles[alt]; ok {
		return t
	}
	return ""
}
