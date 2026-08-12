package host

import (
	"testing"

	storepkg "github.com/voocel/ainovel-cli/internal/store"
)

func TestTranslationReporterPersistsEventForReplay(t *testing.T) {
	store := storepkg.NewStore(t.TempDir())
	if err := store.Init(); err != nil {
		t.Fatal(err)
	}
	h := &Host{store: store, events: make(chan Event, 4), eventSubs: make(map[chan Event]struct{})}
	h.observer = newObserver(store, h.emitEvent, func(string) {}, func() {})

	translationReporter(h)("success", "Chương 01 đã hoàn tất dịch")

	items, err := h.ReplayQueue(0)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("replay items = %d, want 1", len(items))
	}
	item := items[0]
	if item.Agent != "translation_coordinator" || item.Category != "TRANSLATION" || item.Summary != "Chương 01 đã hoàn tất dịch" {
		t.Fatalf("unexpected persisted translation event: %+v", item)
	}
}
