package host

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/voocel/ainovel-cli/internal/translation"
)

// ChapterManuscript is a read-only projection for the web chapter reader.
// Chinese is the creative source of truth; Vietnamese is an isolated artifact.
type ChapterManuscript struct {
	Chapter          int    `json:"chapter"`
	Title            string `json:"title,omitempty"`
	CoreEvent        string `json:"core_event,omitempty"`
	Chinese          string `json:"chinese,omitempty"`
	Vietnamese       string `json:"vietnamese,omitempty"`
	ChineseChars     int    `json:"chinese_chars"`
	VietnameseChars  int    `json:"vietnamese_chars"`
	Completed        bool   `json:"completed"`
	TranslationState string `json:"translation_state,omitempty"`
	TranslationError string `json:"translation_error,omitempty"`
	SourceSHA256     string `json:"source_sha256,omitempty"`
}

// ManuscriptChapterMeta is one row in the outline/rail without full body text.
type ManuscriptChapterMeta struct {
	Chapter          int    `json:"chapter"`
	Title            string `json:"title,omitempty"`
	CoreEvent        string `json:"core_event,omitempty"`
	Completed        bool   `json:"completed"`
	WordCount        int    `json:"word_count,omitempty"`
	TranslationState string `json:"translation_state,omitempty"`
}

// ManuscriptOutline returns planned + completed chapter metadata for the reader rail.
// It reads store facts only — never depends on UI Snapshot assembly (models/usage).
func (h *Host) ManuscriptOutline() ([]ManuscriptChapterMeta, error) {
	if h == nil || h.store == nil {
		return nil, fmt.Errorf("host store unavailable")
	}
	progress, err := h.store.Progress.Load()
	if err != nil {
		return nil, err
	}
	completed := map[int]struct{}{}
	wordCounts := map[int]int{}
	if progress != nil {
		for _, ch := range progress.CompletedChapters {
			completed[ch] = struct{}{}
		}
		for ch, n := range progress.ChapterWordCounts {
			wordCounts[ch] = n
		}
	}
	viState := map[int]string{}
	if h.translation != nil && h.translation.Policy.Enabled {
		if status, stErr := h.translation.Store.LoadStatus(); stErr == nil {
			for ch, rec := range status.Chapters {
				viState[ch] = string(rec.State)
			}
		}
	}

	type row struct {
		chapter   int
		title     string
		coreEvent string
	}
	byChapter := map[int]row{}
	if outline, err := h.store.Outline.LoadOutline(); err == nil {
		for _, e := range outline {
			byChapter[e.Chapter] = row{chapter: e.Chapter, title: e.Title, coreEvent: e.CoreEvent}
		}
	}
	if volumes, err := h.store.Outline.LoadLayeredOutline(); err == nil {
		ch := 0
		for _, vol := range volumes {
			for _, arc := range vol.Arcs {
				for _, e := range arc.Chapters {
					ch++
					num := e.Chapter
					if num <= 0 {
						num = ch
					}
					if _, exists := byChapter[num]; exists {
						continue
					}
					byChapter[num] = row{chapter: num, title: e.Title, coreEvent: e.CoreEvent}
				}
			}
		}
	}
	for ch := range completed {
		if _, ok := byChapter[ch]; !ok {
			byChapter[ch] = row{chapter: ch, title: fmt.Sprintf("第%d章", ch)}
		}
	}

	// Prefer committed summary titles for completed chapters (same rule as TUI projection).
	for ch, item := range byChapter {
		if _, done := completed[ch]; !done {
			continue
		}
		if summary, err := h.store.Summaries.LoadSummary(ch); err == nil && summary != nil && strings.TrimSpace(summary.Title) != "" {
			item.title = strings.TrimSpace(summary.Title)
			byChapter[ch] = item
		}
	}

	nums := make([]int, 0, len(byChapter))
	for ch := range byChapter {
		nums = append(nums, ch)
	}
	sort.Ints(nums)

	out := make([]ManuscriptChapterMeta, 0, len(nums))
	for _, ch := range nums {
		item := byChapter[ch]
		_, done := completed[ch]
		out = append(out, ManuscriptChapterMeta{
			Chapter:          ch,
			Title:            item.title,
			CoreEvent:        item.coreEvent,
			Completed:        done,
			WordCount:        wordCounts[ch],
			TranslationState: viState[ch],
		})
	}
	return out, nil
}

// LoadChapterManuscript loads Chinese final text and optional Vietnamese translation.
func (h *Host) LoadChapterManuscript(chapter int) (ChapterManuscript, error) {
	if h == nil || h.store == nil {
		return ChapterManuscript{}, fmt.Errorf("host store unavailable")
	}
	if chapter < 1 {
		return ChapterManuscript{}, fmt.Errorf("chapter must be >= 1")
	}
	result := ChapterManuscript{Chapter: chapter}
	if outline, err := h.store.Outline.GetChapterOutline(chapter); err == nil && outline != nil {
		result.Title = strings.TrimSpace(outline.Title)
		result.CoreEvent = strings.TrimSpace(outline.CoreEvent)
	} else if layered, err := h.store.Outline.GetChapterFromLayered(chapter); err == nil && layered != nil {
		result.Title = strings.TrimSpace(layered.Title)
		result.CoreEvent = strings.TrimSpace(layered.CoreEvent)
	}
	if result.Title == "" {
		if summary, err := h.store.Summaries.LoadSummary(chapter); err == nil && summary != nil && strings.TrimSpace(summary.Title) != "" {
			result.Title = strings.TrimSpace(summary.Title)
		}
	}
	if result.Title == "" {
		result.Title = fmt.Sprintf("第%d章", chapter)
	}

	zh, err := h.store.Drafts.LoadChapterText(chapter)
	if err != nil {
		return ChapterManuscript{}, err
	}
	result.Chinese = zh
	result.ChineseChars = utf8.RuneCountInString(zh)

	if progress, err := h.store.Progress.Load(); err == nil && progress != nil {
		for _, ch := range progress.CompletedChapters {
			if ch == chapter {
				result.Completed = true
				break
			}
		}
	}

	if h.translation != nil && h.translation.Policy.Enabled && h.translation.Store != nil {
		text, rec, loadErr := h.translation.Store.LoadChapter(chapter)
		if loadErr == nil {
			result.Vietnamese = text
			result.VietnameseChars = utf8.RuneCountInString(text)
			result.TranslationState = string(rec.State)
			result.TranslationError = rec.LastError
			result.SourceSHA256 = rec.SourceSHA256
		} else if status, stErr := h.translation.Store.LoadStatus(); stErr == nil {
			if rec, ok := status.Chapters[chapter]; ok {
				result.TranslationState = string(rec.State)
				result.TranslationError = rec.LastError
				result.SourceSHA256 = rec.SourceSHA256
			}
			if loadErr != nil && !os.IsNotExist(loadErr) && result.TranslationState == "" {
				return result, loadErr
			}
		} else if loadErr != nil && !os.IsNotExist(loadErr) {
			return result, loadErr
		}
	}
	if result.TranslationState == "" && result.Chinese != "" {
		result.TranslationState = string(translation.ChapterPending)
	}
	return result, nil
}
