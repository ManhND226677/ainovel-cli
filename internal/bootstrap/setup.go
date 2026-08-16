package bootstrap

import (
	"bufio"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"

	"github.com/voocel/ainovel-cli/internal/rules"
	"github.com/voocel/ainovel-cli/internal/utils"
)

// exampleConfig là mẫu có chú thích được ghi vào ~/.ainovel/config.example.jsonc
// sau khi hướng dẫn xong. File nhúng phải khớp config.example.jsonc ở gốc repo,
// test sẽ chặn việc lệch nhau.
//
//go:embed config.example.jsonc
var exampleConfig string

// NeedsSetup kiểm tra có cần hướng dẫn lần đầu hay không
// (chỉ chạy khi cả config toàn cầu lẫn config dự án đều chưa tồn tại).
func NeedsSetup() bool {
	if p := DefaultConfigPath(); p != "" {
		if _, err := os.Stat(p); err == nil {
			return false
		}
	}
	if _, err := os.Stat(projectConfigPath()); err == nil {
		return false
	}
	return true
}

type setupProvider struct {
	name           string
	label          string
	baseURL        string // địa chỉ mặc định đi kèm preset
	needType       bool   // proxy tự cấu hình cần hỏi thêm loại giao thức
	apiKeyOptional bool   // true: cho phép bỏ trống API Key
}

var setupProviders = []setupProvider{
	{name: "openrouter", label: "OpenRouter", baseURL: "https://openrouter.ai/api/v1"},
	{name: "anthropic", label: "Anthropic"},
	{name: "gemini", label: "Gemini"},
	{name: "openai", label: "OpenAI"},
	{name: "deepseek", label: "DeepSeek"},
	{name: "qwen", label: "Qwen"},
	{name: "glm", label: "GLM"},
	{name: "grok", label: "Grok"},
	{name: "ollama", label: "Ollama", baseURL: "http://localhost:11434/v1", apiKeyOptional: true},
	{name: "bedrock", label: "Bedrock", apiKeyOptional: true},
	{name: "custom", label: "Custom Proxy", needType: true, apiKeyOptional: true},
}

var apiTypeOptions = []struct{ name, label string }{
	{name: "openai", label: "OpenAI tương thích"},
	{name: "anthropic", label: "Anthropic tương thích"},
	{name: "gemini", label: "Gemini tương thích"},
}

// RunSetup chạy hướng dẫn lần đầu qua prompt stdin thường (không phụ thuộc TUI),
// trả về config đã sinh. Không tương tác được (EOF/pipe) sẽ báo lỗi kèm cách tự tạo config.
func RunSetup() (Config, error) {
	in := bufio.NewReader(os.Stdin)

	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "Chưa tìm thấy file cấu hình, bắt đầu thiết lập ban đầu...")
	fmt.Fprintf(os.Stderr, "  Vị trí config: %s\n", DefaultConfigPath())
	fmt.Fprintln(os.Stderr, "  Sau khi xong có thể sửa file này để tinh chỉnh nâng cao.")
	fmt.Fprintln(os.Stderr)

	// Bước 1: chọn provider
	sp, err := selectProvider(in)
	if err != nil {
		return Config{}, err
	}
	providerName := sp.name
	var pc ProviderConfig
	printStepDone("Provider", sp.label)

	// Proxy tự cấu hình: hỏi thêm tên và loại giao thức API
	if sp.needType {
		providerName, err = promptText(in, "Tên provider", "my-proxy", false)
		if err != nil {
			return Config{}, err
		}
		providerType, err := selectAPIType(in)
		if err != nil {
			return Config{}, err
		}
		pc.Type = providerType
	}

	// Bước 2: API Key
	apiKey, err := promptText(in, "[2/4] API Key", "sk-xxx", sp.apiKeyOptional)
	if err != nil {
		return Config{}, err
	}
	pc.APIKey = apiKey
	if apiKey == "" {
		printStepDone("API Key", "không đặt")
	} else {
		printStepDone("API Key", maskKey(apiKey))
	}

	// Bước 3: Base URL (Enter để dùng mặc định)
	baseDefault := sp.baseURL
	baseHint := "để trống dùng địa chỉ chính thức"
	if baseDefault != "" {
		baseHint = baseDefault
	}
	baseURL, err := promptText(in, "[3/4] Base URL (Enter = mặc định, người dùng proxy điền địa chỉ proxy)", baseHint, true)
	if err != nil {
		return Config{}, err
	}
	if baseURL == "" {
		baseURL = baseDefault
	}
	pc.BaseURL = baseURL
	if baseURL != "" {
		printStepDone("Base URL", baseURL)
	} else {
		printStepDone("Base URL", "mặc định")
	}

	// Bước 4: tên model (bắt buộc)
	modelName, err := promptText(in, "[4/4] Tên model", "ví dụ: gpt-4o / claude-sonnet-4 / gemini-2.5-pro", false)
	if err != nil {
		return Config{}, err
	}
	printStepDone("Model", modelName)
	pc.Models = []ModelConfig{{Name: modelName}}

	cfg := Config{
		Provider:  providerName,
		ModelName: modelName,
		Providers: map[string]ProviderConfig{providerName: pc},
		Roles:     map[string]RoleConfig{},
		Style:     "default",
	}

	path := DefaultConfigPath()
	if err := SaveConfig(path, cfg); err != nil {
		return cfg, fmt.Errorf("save config: %w", err)
	}
	saveExampleConfig()

	// Thư mục quy tắc riêng được tạo bởi luồng khởi động (runWithConfig), ở đây chỉ lấy đường dẫn để nhắc
	rulesDir := rules.DefaultHomeRulesDir()

	fmt.Fprintln(os.Stderr)
	fmt.Fprintf(os.Stderr, "✓ Đã lưu config vào %s\n", path)
	fmt.Fprintf(os.Stderr, "  Model mặc định: %s\n", modelName)
	fmt.Fprintln(os.Stderr, "  Muốn dùng model khác nhau theo vai trò, chỉnh trực tiếp trong file config.")
	if rulesDir != "" {
		fmt.Fprintf(os.Stderr, "  Quy tắc viết chung đặt trong %s dưới dạng file .md (xem README.txt ở đó)\n", rulesDir)
	}
	fmt.Fprintln(os.Stderr)

	return cfg, nil
}

func saveExampleConfig() {
	dir, err := configDir()
	if err != nil {
		return
	}
	_ = os.WriteFile(filepath.Join(dir, "config.example.jsonc"), []byte(exampleConfig), 0o644)
}

func printStepDone(label, value string) {
	fmt.Fprintf(os.Stderr, "  ✓ %s: %s\n", label, value)
}

func maskKey(key string) string {
	if len(key) <= 8 {
		return "****"
	}
	return key[:4] + "****" + key[len(key)-4:]
}

// readLine đọc một dòng từ stdin; EOF ở dòng đầu coi như huỷ hướng dẫn.
func readLine(in *bufio.Reader) (string, error) {
	line, err := in.ReadString('\n')
	if err != nil && line == "" {
		return "", fmt.Errorf("không đọc được đầu vào (setup cancelled): %w", err)
	}
	return utils.CleanInputLine(line), nil
}

func selectProvider(in *bufio.Reader) (setupProvider, error) {
	fmt.Fprintln(os.Stderr, "[1/4] Chọn Provider (nhập số rồi Enter):")
	for i, p := range setupProviders {
		fmt.Fprintf(os.Stderr, "  %2d. %s\n", i+1, p.label)
	}
	for {
		fmt.Fprint(os.Stderr, "Lựa chọn [1]: ")
		line, err := readLine(in)
		if err != nil {
			return setupProvider{}, err
		}
		if line == "" {
			return setupProviders[0], nil
		}
		n := 0
		if _, err := fmt.Sscanf(line, "%d", &n); err == nil && n >= 1 && n <= len(setupProviders) {
			return setupProviders[n-1], nil
		}
		fmt.Fprintln(os.Stderr, "  Số không hợp lệ, thử lại.")
	}
}

func selectAPIType(in *bufio.Reader) (string, error) {
	fmt.Fprintln(os.Stderr, "Loại giao thức API:")
	for i, t := range apiTypeOptions {
		fmt.Fprintf(os.Stderr, "  %d. %s\n", i+1, t.label)
	}
	for {
		fmt.Fprint(os.Stderr, "Lựa chọn [1]: ")
		line, err := readLine(in)
		if err != nil {
			return "", err
		}
		if line == "" {
			return apiTypeOptions[0].name, nil
		}
		n := 0
		if _, err := fmt.Sscanf(line, "%d", &n); err == nil && n >= 1 && n <= len(apiTypeOptions) {
			return apiTypeOptions[n-1].name, nil
		}
		fmt.Fprintln(os.Stderr, "  Số không hợp lệ, thử lại.")
	}
}

// promptText in nhãn + gợi ý rồi đọc một dòng; allowEmpty quyết định
// có chấp nhận giá trị rỗng hay hỏi lại.
func promptText(in *bufio.Reader, label, placeholder string, allowEmpty bool) (string, error) {
	for {
		fmt.Fprintf(os.Stderr, "%s\n  gợi ý: %s\n> ", label, placeholder)
		line, err := readLine(in)
		if err != nil {
			return "", err
		}
		if line != "" || allowEmpty {
			return line, nil
		}
		fmt.Fprintln(os.Stderr, "  Giá trị này không được để trống.")
	}
}
