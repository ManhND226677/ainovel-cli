package host

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/voocel/ainovel-cli/internal/translation"
)

// TranslationGlossary returns the durable Vietnamese terminology ledger. It is
// intentionally separate from Status so status snapshots remain compact.
func (h *Host) TranslationGlossary() (translation.Glossary, error) {
	if h.translation == nil || !h.translation.Policy.Enabled {
		return translation.Glossary{}, fmt.Errorf("translation is not enabled for this book")
	}
	return h.translation.Store.LoadGlossary()
}

// UpdateTranslationGlossary adds explicit human terminology choices before a
// recovery batch. Existing committed choices are append-only and cannot be
// silently rewritten, preserving the glossary-version audit contract.
func (h *Host) UpdateTranslationGlossary(entries map[string]string) (translation.Glossary, error) {
	if h.translation == nil || !h.translation.Policy.Enabled {
		return translation.Glossary{}, fmt.Errorf("translation is not enabled for this book")
	}
	if len(entries) == 0 {
		return translation.Glossary{}, fmt.Errorf("glossary entries are required")
	}

	glossary, err := h.translation.Store.LoadGlossary()
	if err != nil {
		return translation.Glossary{}, err
	}
	if glossary.Terms == nil {
		glossary.Terms = make(map[string]translation.GlossaryEntry)
	}
	keys := make([]string, 0, len(entries))
	for source := range entries {
		keys = append(keys, source)
	}
	sort.Strings(keys)

	changed := false
	now := time.Now().UTC()
	for _, rawSource := range keys {
		source := strings.TrimSpace(rawSource)
		vietnamese := strings.TrimSpace(entries[rawSource])
		if source == "" || vietnamese == "" {
			return translation.Glossary{}, fmt.Errorf("glossary source and Vietnamese value are required")
		}
		if existing, found := glossary.Terms[source]; found {
			if existing.Vietnamese != vietnamese {
				return translation.Glossary{}, fmt.Errorf("thuật ngữ %q đã được chốt là %q; không thể ghi đè", source, existing.Vietnamese)
			}
			continue
		}
		glossary.Terms[source] = translation.GlossaryEntry{
			Source: source, Vietnamese: vietnamese, UpdatedAt: now,
		}
		changed = true
	}
	if changed {
		glossary.Version++
		if err := h.translation.Store.SaveGlossary(glossary); err != nil {
			return translation.Glossary{}, err
		}
		h.emitEvent(Event{Time: now, Agent: "translation_coordinator", Category: "TRANSLATION", Level: "info", Summary: fmt.Sprintf("Đã cập nhật %d thuật ngữ glossary từ dashboard", len(entries))})
	}
	return glossary, nil
}
