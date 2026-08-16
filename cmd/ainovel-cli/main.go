package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/voocel/ainovel-cli/assets"
	"github.com/voocel/ainovel-cli/internal/bootstrap"
	"github.com/voocel/ainovel-cli/internal/host"
	"github.com/voocel/ainovel-cli/internal/rules"
	buildversion "github.com/voocel/ainovel-cli/internal/version"
	"github.com/voocel/ainovel-cli/internal/webapi"
)

var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

// Ứng dụng chỉ còn chế độ web dashboard: chạy không tham số là khởi động server
// tại 127.0.0.1:10001. `web`/`--web` vẫn được chấp nhận cho tương thích lệnh cũ.
func main() {
	opts, args, err := parseCLIOptions(os.Args[1:])
	if err != nil {
		die("flags: %v", err)
	}
	if opts.Version {
		buildversion.Print(os.Stdout, versionInfo())
		return
	}
	if opts.Update {
		if err := runSelfUpdate(opts.UpdateVersion); err != nil {
			fmt.Fprintf(os.Stderr, "update: %v\n", err)
			os.Exit(1)
		}
		return
	}

	// Hướng dẫn lần đầu
	if bootstrap.NeedsSetup() {
		setupCfg, err := bootstrap.RunSetup()
		if err != nil {
			die("setup: %v", err)
		}
		runWithConfig(setupCfg, opts, args)
		return
	}

	cfg, err := bootstrap.LoadConfig()
	if err != nil {
		die("config: %v", err)
	}

	runWithConfig(cfg, opts, args)
}

// die xử lý lỗi thoát: in ra stderr, ghi vào ~/.ainovel/last-error.log,
// và tạm dừng chờ Enter khi chạy trong terminal tương tác — khởi động bằng
// double-click thì console đóng theo process, không tạm dừng người dùng sẽ
// không kịp đọc lỗi.
func die(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintln(os.Stderr, msg)
	if path := bootstrap.WriteStartupError(msg); path != "" {
		fmt.Fprintf(os.Stderr, "(chi tiết lỗi đã được ghi vào %s)\n", path)
	}
	if stdinIsTerminal() {
		fmt.Fprint(os.Stderr, "\nNhấn Enter để thoát...")
		fmt.Fscanln(os.Stdin)
	}
	os.Exit(1)
}

// stdinIsTerminal kiểm tra stdin có nối với terminal không. Double-click /
// terminal tương tác là true; pipe, redirect, CI là false.
func stdinIsTerminal() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

func runWithConfig(cfg bootstrap.Config, opts cliOptions, args []string) {
	rules.EnsureHomeRulesDir()

	if len(args) > 0 {
		die("Lỗi: tham số không hỗ trợ %q; các flag hợp lệ: --addr --token --ui-dir --book --output-dir", args[0])
	}

	// FillDefaults phải chạy trước khi nạp assets: OutputDir là field runtime,
	// giá trị mặc định được chuẩn hoá ở đây — nếu không thì style/ cấp sách
	// (book-level style override) sẽ không bao giờ được tải.
	cfg.FillDefaults()
	// Multi-book: --output-dir thắng; nếu không có thì --book phân giải/tạo trong library/.
	if dir, err := resolveCLIBookDir(opts); err != nil {
		die("book: %v", err)
	} else if dir != "" {
		cfg.OutputDir = dir
	}
	bundle := assets.Load(cfg.Style, assets.DefaultLoadOptions(cfg.OutputDir))
	if err := webapi.Run(cfg, bundle, webapi.Options{
		Addr:  opts.WebAddr,
		Token: opts.WebToken,
		UIDir: opts.WebUIDir,
	}); err != nil {
		die("web: %v", err)
	}
}

type cliOptions struct {
	Web           bool // tương thích lệnh cũ, web luôn là chế độ chạy
	WebAddr       string
	WebToken      string
	WebUIDir      string
	Book          string // library id/slug/path hoặc tên sách mới
	OutputDir     string // thư mục sách tường minh (thắng --book)
	Version       bool
	Update        bool
	UpdateVersion string
}

// parseCLIOptions tách CLI flag, trả về options và tham số dư.
func parseCLIOptions(argv []string) (cliOptions, []string, error) {
	var opts cliOptions
	var args []string
	for i := 0; i < len(argv); i++ {
		switch argv[i] {
		case "--version", "-v":
			opts.Version = true
		case "version":
			if i+1 < len(argv) {
				return opts, nil, fmt.Errorf("version 不接受参数")
			}
			opts.Version = true
		case "update":
			if opts.Update {
				return opts, nil, fmt.Errorf("update 只能指定一次")
			}
			opts.Update = true
			if i+1 < len(argv) {
				if strings.HasPrefix(argv[i+1], "-") {
					return opts, nil, fmt.Errorf("update 只接受一个可选版本参数")
				}
				opts.UpdateVersion = argv[i+1]
				i++
			}
			if i+1 < len(argv) {
				return opts, nil, fmt.Errorf("update 只接受一个可选版本参数")
			}
		case "web", "--web":
			opts.Web = true
		case "--addr":
			if i+1 >= len(argv) {
				return opts, nil, fmt.Errorf("--addr thiếu giá trị")
			}
			opts.WebAddr = argv[i+1]
			i++
		case "--token":
			if i+1 >= len(argv) {
				return opts, nil, fmt.Errorf("--token thiếu giá trị")
			}
			opts.WebToken = argv[i+1]
			i++
		case "--ui-dir":
			if i+1 >= len(argv) {
				return opts, nil, fmt.Errorf("--ui-dir thiếu giá trị")
			}
			opts.WebUIDir = argv[i+1]
			i++
		case "--book":
			if i+1 >= len(argv) {
				return opts, nil, fmt.Errorf("--book thiếu giá trị")
			}
			opts.Book = argv[i+1]
			i++
		case "--output-dir":
			if i+1 >= len(argv) {
				return opts, nil, fmt.Errorf("--output-dir thiếu giá trị")
			}
			opts.OutputDir = argv[i+1]
			i++
		default:
			args = append(args, argv[i])
		}
	}
	if opts.Version && (opts.Update || opts.Web || len(args) > 0) {
		return opts, nil, fmt.Errorf("version 不能与其他启动参数混用")
	}
	if opts.Update && (opts.Web || len(args) > 0) {
		return opts, nil, fmt.Errorf("update 不能与其他启动参数混用")
	}
	return opts, args, nil
}

func versionInfo() buildversion.Info {
	return buildversion.Resolve(buildversion.Info{
		Version: version,
		Commit:  commit,
		Date:    date,
	})
}

func runSelfUpdate(target string) error {
	info := versionInfo()
	result, err := buildversion.Update(context.Background(), buildversion.UpdateOptions{
		Repo:           "ManhND226677/ainovel-cli",
		BinaryName:     "ainovel-cli",
		TargetVersion:  target,
		CurrentVersion: info.Version,
	})
	if err != nil {
		return err
	}
	if !result.Updated {
		fmt.Printf("ainovel-cli đang ở phiên bản mới nhất %s\n", result.Version)
		return nil
	}
	fmt.Printf("ainovel-cli đã cập nhật lên %s\n", result.Version)
	fmt.Printf("Vị trí cài đặt: %s\n", result.Path)
	return nil
}

func resolveCLIBookDir(opts cliOptions) (string, error) {
	return host.ResolveBookDir(opts.OutputDir, opts.Book, "")
}
