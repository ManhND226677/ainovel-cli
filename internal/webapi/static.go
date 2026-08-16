package webapi

import (
	"embed"
	"io/fs"
)

// staticFS là bản build của web-dashboard được nhúng sẵn vào binary
// (web-dashboard/dist/public copy vào đây bởi scripts/build-web).
// Nhờ vậy `ainovel-cli web` chạy ngay có giao diện, không cần --ui-dir.
//
//go:embed all:static
var staticFS embed.FS

// embeddedUI trả về filesystem gốc của bản SPA đã nhúng.
func embeddedUI() fs.FS {
	sub, err := fs.Sub(staticFS, "static")
	if err != nil {
		// Không thể xảy ra: thư mục static luôn tồn tại lúc build nhờ go:embed.
		panic(err)
	}
	return sub
}
