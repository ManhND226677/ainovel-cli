package startup

import (
	"fmt"
	"strings"
)

// PrepareQuick 将直接输入整理为可进入 Engine 的快速启动计划。
func PrepareQuick(req Request) (Plan, error) {
	prompt := strings.TrimSpace(req.UserPrompt)
	if prompt == "" {
		return Plan{}, fmt.Errorf("yêu cầu không được để trống")
	}
	return Plan{
		Mode:        ModeQuick,
		DisplayName: "Bắt đầu nhanh",
		RawPrompt:   prompt,
	}, nil
}
