package translation

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/voocel/agentcore"
	"github.com/voocel/agentcore/schema"
	"github.com/voocel/ainovel-cli/internal/llmcontract"
)

const translatorMaxTokens = 16384

var translatorContract = llmcontract.Contract{
	Name:        "translator_zh_vi",
	Description: "Dịch một chương tiểu thuyết từ tiếng Trung sang tiếng Việt",
	Schema: schema.Object(
		schema.Property("text", schema.String("Toàn bộ nội dung chương tiếng Việt, không có lời giải thích")).Required(),
		schema.Property("glossary", schema.Array("Thuật ngữ mới cần khóa cho các chương sau", schema.Object(
			schema.Property("source", schema.String("Thuật ngữ tiếng Trung")).Required(),
			schema.Property("vietnamese", schema.String("Cách dịch tiếng Việt đã dùng")).Required(),
		))).Required(),
	),
}

// TranslationRequest contains only immutable source facts plus the terminology
// context needed for an agent to translate consistently.
type TranslationRequest struct {
	Chapter            int                      `json:"chapter"`
	ChineseText        string                   `json:"chinese_text"`
	Glossary           map[string]GlossaryEntry `json:"glossary"`
	PreviousVietnamese string                   `json:"previous_vietnamese,omitempty"`
	Instruction        string                   `json:"instruction,omitempty"`
}

// GlossaryCandidate is a proposed new locked term from a translated chapter.
type GlossaryCandidate struct {
	Source     string `json:"source"`
	Vietnamese string `json:"vietnamese"`
}

// TranslationResult is the structured result of one Translator Agent call.
type TranslationResult struct {
	Text     string              `json:"text"`
	Glossary []GlossaryCandidate `json:"glossary"`
}

type translationResponse = TranslationResult

// Translate invokes the Translation Agent. It cannot access the creative Store;
// a caller decides whether the source fingerprint is still current before commit.
func Translate(ctx context.Context, model agentcore.ChatModel, systemPrompt string, request TranslationRequest) (TranslationResult, error) {
	if model == nil {
		return TranslationResult{}, fmt.Errorf("translator model is nil")
	}
	if request.Chapter <= 0 || strings.TrimSpace(request.ChineseText) == "" {
		return TranslationResult{}, fmt.Errorf("invalid translation request")
	}
	payload, err := json.MarshalIndent(request, "", "  ")
	if err != nil {
		return TranslationResult{}, fmt.Errorf("marshal translation request: %w", err)
	}
	response, err := llmcontract.Execute(ctx, model, llmcontract.Request[translationResponse]{
		Contract:     translatorContract,
		SystemPrompt: systemPrompt,
		Payload:      string(payload),
		Options:      []agentcore.CallOption{agentcore.WithMaxTokens(translatorMaxTokens)},
		Agent:        "translator",
		Validate: func(response *translationResponse) error {
			if err := ValidateText(request.ChineseText, response.Text); err != nil {
				return err
			}
			return ValidateGlossary(response.Glossary)
		},
	})
	if err != nil {
		return TranslationResult{}, fmt.Errorf("translator: %w", err)
	}
	return TranslationResult(response), nil
}

// TranslateWithPreview mirrors Translate but forwards raw text deltas to a
// local observer. The final JSON is still schema-decoded and mechanically
// validated before it can become a Vietnamese artifact. Providers that do not
// support streaming transparently fall back to the durable non-stream path.
func TranslateWithPreview(ctx context.Context, model agentcore.ChatModel, systemPrompt string, request TranslationRequest, onDelta func(string)) (TranslationResult, error) {
	if onDelta == nil {
		return Translate(ctx, model, systemPrompt, request)
	}
	if model == nil || request.Chapter <= 0 || strings.TrimSpace(request.ChineseText) == "" {
		return Translate(ctx, model, systemPrompt, request)
	}
	payload, err := json.MarshalIndent(request, "", "  ")
	if err != nil {
		return TranslationResult{}, fmt.Errorf("marshal translation request: %w", err)
	}
	schemaOptions, resolution := llmcontract.Plan(model, translatorContract)
	prompt, err := llmcontract.PreparePrompt(systemPrompt, translatorContract, resolution)
	if err != nil {
		return TranslationResult{}, fmt.Errorf("prepare translation stream: %w", err)
	}
	stream, err := model.GenerateStream(ctx, []agentcore.Message{agentcore.SystemMsg(prompt), agentcore.UserMsg(string(payload))}, nil, append(schemaOptions, agentcore.WithMaxTokens(translatorMaxTokens))...)
	if err != nil {
		return Translate(ctx, model, systemPrompt, request)
	}
	var raw strings.Builder
	for event := range stream {
		switch event.Type {
		case agentcore.StreamEventTextDelta:
			raw.WriteString(event.Delta)
			onDelta(event.Delta)
		case agentcore.StreamEventDone:
			if raw.Len() == 0 {
				raw.WriteString(event.Message.TextContent())
				onDelta(event.Message.TextContent())
			}
			if event.StopReason != "" && event.StopReason != agentcore.StopReasonStop {
				return Translate(ctx, model, systemPrompt, request)
			}
		case agentcore.StreamEventError:
			return Translate(ctx, model, systemPrompt, request)
		}
	}
	body := strings.TrimSpace(raw.String())
	if resolution.Mode != llmcontract.ModeNativeJSONSchema {
		body = llmcontract.ExtractJSONObject(body)
	}
	var response translationResponse
	if body == "" || llmcontract.ValidateJSON(translatorContract.Schema, []byte(body)) != nil || json.Unmarshal([]byte(body), &response) != nil || ValidateText(request.ChineseText, response.Text) != nil || ValidateGlossary(response.Glossary) != nil {
		return Translate(ctx, model, systemPrompt, request)
	}
	return TranslationResult(response), nil
}

// ValidateText is intentionally mechanical. Literary adequacy remains a model
// task; code only rejects empty/corrupt results before they become artifacts.
// ValidateGlossary guards the mechanical integrity of proposals. Existing
// entries remain immutable in Store.MergeGlossary, so an LLM cannot rewrite a
// previously locked translation choice.
func ValidateGlossary(candidates []GlossaryCandidate) error {
	seen := make(map[string]bool, len(candidates))
	for _, candidate := range candidates {
		source := strings.TrimSpace(candidate.Source)
		vietnamese := strings.TrimSpace(candidate.Vietnamese)
		if source == "" || vietnamese == "" {
			return fmt.Errorf("glossary terms must be non-empty")
		}
		if seen[source] {
			return fmt.Errorf("glossary has duplicate source term %q", source)
		}
		seen[source] = true
	}
	return nil
}

func ValidateText(source, translated string) error {
	if strings.TrimSpace(translated) == "" {
		return fmt.Errorf("translation is empty")
	}
	if strings.TrimSpace(translated) == strings.TrimSpace(source) {
		return fmt.Errorf("translation is identical to Chinese source")
	}
	if strings.Contains(translated, "```json") || strings.Contains(translated, `"text":`) {
		return fmt.Errorf("translation contains structured response wrapper")
	}
	return nil
}
