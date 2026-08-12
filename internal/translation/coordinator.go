package translation

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/voocel/agentcore"
	"github.com/voocel/agentcore/schema"
	"github.com/voocel/ainovel-cli/internal/llmcontract"
)

const coordinatorMaxTokens = 4096

var coordinatorContract = llmcontract.Contract{
	Name:        "translation_coordinator",
	Description: "Quyết định thời điểm và lô chương dịch Trung-Việt",
	Schema: schema.Object(
		schema.Property("action", schema.Enum("Hành động điều phối", string(DecisionWait), string(DecisionTranslate), string(DecisionResume), string(DecisionRetranslate), string(DecisionFinalize))).Required(),
		schema.Property("chapters", schema.Array("Danh sách chương tăng dần", schema.Int("Số chương"))).Required(),
		schema.Property("job_id", schema.String("ID job cần tiếp tục; các hành động khác để chuỗi rỗng")).Required(),
		schema.Property("reason", schema.String("Lý do ngắn gọn dựa trên facts")).Required(),
	),
}

// Decide asks the Translation Coordinator Agent for a structured decision. The
// caller must still call Decision.Validate and persist the audit record before
// creating any job.
func Decide(ctx context.Context, model agentcore.ChatModel, systemPrompt string, snapshot Snapshot) (Decision, error) {
	if model == nil {
		return Decision{}, fmt.Errorf("translation coordinator model is nil")
	}
	snapshot.Policy = snapshot.Policy.Normalize()
	payload, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return Decision{}, fmt.Errorf("marshal translation snapshot: %w", err)
	}
	decision, err := llmcontract.Execute(ctx, model, llmcontract.Request[Decision]{
		Contract:     coordinatorContract,
		SystemPrompt: systemPrompt,
		Payload:      string(payload),
		Options:      []agentcore.CallOption{agentcore.WithMaxTokens(coordinatorMaxTokens)},
		Agent:        "translation_coordinator",
		Validate: func(decision *Decision) error {
			return decision.Validate(snapshot)
		},
		Hooks: llmcontract.Hooks{
			Resolved: func(res llmcontract.Resolution) {
				slog.Debug("translation coordinator contract resolved", "module", "translation", "mode", res.Mode, "provider", res.Provider, "model", res.Model)
			},
		},
	})
	if err != nil {
		return Decision{}, fmt.Errorf("translation coordinator: %w", err)
	}
	return decision, nil
}

// AuditDecision is an append-only explanation of a validated coordinator call.
type AuditDecision struct {
	SnapshotDigest string   `json:"snapshot_digest"`
	Trigger        string   `json:"trigger"`
	Decision       Decision `json:"decision"`
	Valid          bool     `json:"valid"`
	ValidationErr  string   `json:"validation_error,omitempty"`
	Provider       string   `json:"provider,omitempty"`
	Model          string   `json:"model,omitempty"`
}

// SnapshotDigest produces a stable audit fingerprint without storing chapter
// bodies in the decision journal.
func SnapshotDigest(snapshot Snapshot) string {
	copy := snapshot
	for i := range copy.Completed {
		copy.Completed[i].Text = ""
	}
	data, err := json.Marshal(copy)
	if err != nil {
		return ""
	}
	return Digest(string(data))
}
