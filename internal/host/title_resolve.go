package host

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/voocel/agentcore"
	"github.com/voocel/agentcore/schema"
	"github.com/voocel/ainovel-cli/internal/host/exp"
	"github.com/voocel/ainovel-cli/internal/llmcontract"
	"github.com/voocel/ainovel-cli/internal/translation"
)

// Export wires TitleResolver so Vietnamese EPUB/TXT always get full chapter titles.
func (h *Host) Export(ctx context.Context, opts exp.Options) (*exp.Result, error) {
	var translations *translation.Store
	if h.translation != nil {
		translations = h.translation.Store
	}
	author, desc, titleVI := h.ExportMeta()
	if opts.Author == "" {
		opts.Author = author
	}
	if opts.Description == "" {
		opts.Description = desc
	}
	zhName := ""
	if p, _ := h.store.Progress.Load(); p != nil {
		zhName = strings.TrimSpace(p.NovelName)
	}
	if opts.Title == "" {
		switch opts.Language {
		case exp.LanguageVietnamese:
			if t := ResolveVietnameseTitle(zhName, titleVI); t != "" {
				opts.Title = t
			} else if zhName != "" {
				opts.Title = zhName
			}
		default:
			opts.Title = zhName
		}
	}
	deps := exp.Deps{Store: h.store, Translation: translations}
	if opts.Language == exp.LanguageVietnamese {
		deps.TitleResolver = func(zh map[int]string) (map[int]string, error) {
			out, err := h.resolveChapterTitlesVI(zh)
			if err != nil {
				slog.Warn("dịch tiêu đề chương VI thất bại", "module", "export", "need", len(zh), "err", err)
				h.emitEvent(Event{
					Time:     time.Now(),
					Category: "SYSTEM",
					Level:    "warn",
					Summary:  fmt.Sprintf("Không dịch đủ tiêu đề chương VI (%d còn thiếu): %v", len(zh), err),
				})
			} else if n := len(out); n > 0 {
				slog.Info("đã dịch tiêu đề chương VI", "module", "export", "count", n)
				h.emitEvent(Event{
					Time:     time.Now(),
					Category: "SYSTEM",
					Level:    "info",
					Summary:  fmt.Sprintf("Đã điền %d tiêu đề chương tiếng Việt cho bản export", n),
				})
			}
			return out, err
		}
	}
	return exp.Run(ctx, deps, opts)
}

type titleBatchItem struct {
	Chapter int    `json:"chapter"`
	TitleZH string `json:"title_zh"`
}

type titleBatchResult struct {
	Titles []struct {
		Chapter int    `json:"chapter"`
		TitleVI string `json:"title_vi"`
	} `json:"titles"`
}

// resolveChapterTitlesVI translates leftover Chinese chapter titles to Vietnamese
// in one structured LLM call. Results are suitable for TOC; not full prose.
func (h *Host) resolveChapterTitlesVI(zhTitles map[int]string) (map[int]string, error) {
	if h == nil || len(zhTitles) == 0 {
		return map[int]string{}, nil
	}
	h.mu.Lock()
	model := translationRoleModel(h.models, "translator", "writer")
	h.mu.Unlock()
	if model == nil {
		return nil, fmt.Errorf("no translator model configured for title backfill")
	}
	slog.Info("resolving VI chapter titles", "module", "export", "count", len(zhTitles))

	items := make([]titleBatchItem, 0, len(zhTitles))
	for ch, zh := range zhTitles {
		zh = strings.TrimSpace(zh)
		if ch <= 0 || zh == "" {
			continue
		}
		items = append(items, titleBatchItem{Chapter: ch, TitleZH: zh})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Chapter < items[j].Chapter })
	if len(items) == 0 {
		return map[int]string{}, nil
	}

	// Smaller chunks = higher fill rate and fewer truncated JSON responses.
	const chunkSize = 25
	out := make(map[int]string, len(items))
	var firstErr error
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()

	contract := llmcontract.Contract{
		Name:        "chapter_title_zh_vi",
		Description: "Dịch hàng loạt tiêu đề chương",
		Schema: schema.Object(
			schema.Property("titles", schema.Array("Danh sách tiêu đề đã dịch — đủ mọi chapter trong input", schema.Object(
				schema.Property("chapter", schema.Int("Số chương")).Required(),
				schema.Property("title_vi", schema.String("Tiêu đề tiếng Việt thuần, không số chương")).Required(),
			))).Required(),
		),
	}

	for start := 0; start < len(items); start += chunkSize {
		end := start + chunkSize
		if end > len(items) {
			end = len(items)
		}
		chunk := items[start:end]
		// Retry each chunk a couple of times — title fill must be complete.
		var chunkErr error
		for attempt := 1; attempt <= 3; attempt++ {
			payload, err := json.MarshalIndent(map[string]any{
				"task": "Dịch TẤT CẢ tiêu đề chương Trung → Việt. Mỗi phần tử input phải có đúng một title_vi. " +
					"Văn phong tiêu đề tiểu thuyết, ngắn, không giải thích.",
				"count": len(chunk),
				"items": chunk,
			}, "", "  ")
			if err != nil {
				return out, err
			}
			want := make(map[int]struct{}, len(chunk))
			for _, it := range chunk {
				want[it.Chapter] = struct{}{}
			}
			resp, err := llmcontract.Execute(ctx, model, llmcontract.Request[titleBatchResult]{
				Contract: contract,
				SystemPrompt: "Bạn là biên dịch tiêu đề chương tiểu thuyết Trung→Việt. " +
					"Chỉ trả JSON theo schema. Phải trả ĐỦ số chương trong input. " +
					"Mỗi title_vi: ngắn ≤40 ký tự, tên chương thật, " +
					"KHÔNG viết 'Chương N', KHÔNG số chương, KHÔNG tiếng Trung, KHÔNG giải thích.",
				Payload: string(payload),
				Options: []agentcore.CallOption{agentcore.WithMaxTokens(4096)},
				Agent:   "translator",
				Validate: func(r *titleBatchResult) error {
					if r == nil || len(r.Titles) == 0 {
						return fmt.Errorf("empty titles")
					}
					return nil
				},
			})
			if err != nil {
				chunkErr = fmt.Errorf("title batch %d-%d attempt %d: %w", start, end, attempt, err)
				continue
			}
			got := 0
			for _, row := range resp.Titles {
				t := strings.TrimSpace(row.TitleVI)
				if row.Chapter <= 0 || t == "" {
					continue
				}
				if _, ok := want[row.Chapter]; !ok {
					continue
				}
				low := strings.ToLower(t)
				if strings.HasPrefix(low, "chương") || strings.HasPrefix(low, "chuong") {
					fields := strings.Fields(t)
					if len(fields) >= 3 {
						t = strings.TrimSpace(strings.Join(fields[2:], " "))
						t = strings.TrimLeft(t, ":.-–— ")
					} else {
						continue
					}
				}
				if t == "" {
					continue
				}
				out[row.Chapter] = t
				got++
			}
			// Success if we filled most of the chunk; retry if too sparse.
			if got >= (len(chunk)*3)/4 || got == len(chunk) {
				chunkErr = nil
				break
			}
			chunkErr = fmt.Errorf("title batch %d-%d attempt %d: only %d/%d titles", start, end, attempt, got, len(chunk))
		}
		if chunkErr != nil && firstErr == nil {
			firstErr = chunkErr
		}
	}
	// Report how many input titles remain missing.
	missing := 0
	for _, it := range items {
		if _, ok := out[it.Chapter]; !ok {
			missing++
		}
	}
	if missing > 0 {
		if firstErr != nil {
			return out, fmt.Errorf("still missing %d titles: %w", missing, firstErr)
		}
		return out, fmt.Errorf("still missing %d chapter titles after LLM fill", missing)
	}
	return out, nil
}
