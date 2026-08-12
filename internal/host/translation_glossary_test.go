package host

import (
	"strings"
	"testing"

	"github.com/voocel/ainovel-cli/internal/translation"
)

func TestUpdateTranslationGlossaryIsAppendOnly(t *testing.T) {
	root := t.TempDir()
	store := translation.NewStore(root)
	if err := store.Init(); err != nil {
		t.Fatal(err)
	}
	h := &Host{translation: &translation.Controller{Store: store, Policy: translation.Policy{Enabled: true}}, events: make(chan Event, 4), eventSubs: make(map[chan Event]struct{})}
	result, err := h.UpdateTranslationGlossary(map[string]string{"云城": "Thành phố Mây"})
	if err != nil {
		t.Fatalf("save glossary: %v", err)
	}
	if result.Version != 1 || result.Terms["云城"].Vietnamese != "Thành phố Mây" {
		t.Fatalf("unexpected glossary: %#v", result)
	}
	_, err = h.UpdateTranslationGlossary(map[string]string{"云城": "Vân Thành"})
	if err == nil || !strings.Contains(err.Error(), "không thể ghi đè") {
		t.Fatalf("want append-only error, got %v", err)
	}
}
