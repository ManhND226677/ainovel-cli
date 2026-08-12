package host

import (
	"os"
	"path/filepath"
	"testing"

	storepkg "github.com/voocel/ainovel-cli/internal/store"
)

func TestManuscriptSnapshotRoundTrip(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "chapters"), 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "chapters", "01.md")
	if err := os.WriteFile(path, []byte("bản thảo gốc"), 0o600); err != nil {
		t.Fatal(err)
	}
	h := &Host{store: storepkg.NewStore(root), events: make(chan Event, 4), eventSubs: make(map[chan Event]struct{})}
	snapshot, err := h.CreateManuscriptSnapshot("Trước khi sửa")
	if err != nil {
		t.Fatalf("create snapshot: %v", err)
	}
	if snapshot.Files != 1 {
		t.Fatalf("snapshot files = %d, want 1", snapshot.Files)
	}
	if err := os.WriteFile(path, []byte("bản thảo đã sửa"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := h.RestoreManuscriptSnapshot(snapshot.ID); err != nil {
		t.Fatalf("restore snapshot: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "bản thảo gốc" {
		t.Fatalf("restored contents = %q", string(got))
	}
	items, err := h.ListManuscriptSnapshots()
	if err != nil {
		t.Fatalf("list snapshots: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("snapshot count = %d, want original + pre-restore", len(items))
	}
}
