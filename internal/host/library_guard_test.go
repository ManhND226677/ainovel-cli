package host

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/voocel/ainovel-cli/internal/bootstrap"
	"github.com/voocel/ainovel-cli/internal/domain"
	"github.com/voocel/ainovel-cli/internal/library"
	storepkg "github.com/voocel/ainovel-cli/internal/store"
)

func TestRefuseNewBookAllowsEmptyAndInitOnly(t *testing.T) {
	dir := t.TempDir()
	st := storepkg.NewStore(dir)
	if err := st.Init(); err != nil {
		t.Fatal(err)
	}
	h := &Host{store: st}
	if err := h.refuseNewBookOverExisting(); err != nil {
		t.Fatalf("empty store should allow new start: %v", err)
	}
	if err := st.Progress.Init("Nháp", 0); err != nil {
		t.Fatal(err)
	}
	if err := h.refuseNewBookOverExisting(); err != nil {
		t.Fatalf("phase init without chapters should allow retry: %v", err)
	}
}

func TestRefuseNewBookBlocksCompletedAndFoundation(t *testing.T) {
	dir := t.TempDir()
	st := storepkg.NewStore(dir)
	if err := st.Init(); err != nil {
		t.Fatal(err)
	}
	h := &Host{store: st}

	p := &domain.Progress{
		NovelName:         "Cũ",
		Phase:             domain.PhaseWriting,
		CompletedChapters: []int{1, 2},
	}
	if err := st.Progress.Save(p); err != nil {
		t.Fatal(err)
	}
	err := h.refuseNewBookOverExisting()
	if err == nil || !strings.Contains(err.Error(), "chương hoàn tất") {
		t.Fatalf("expected completed guard, got %v", err)
	}

	p2 := &domain.Progress{NovelName: "Outline", Phase: domain.PhaseOutline}
	if err := st.Progress.Save(p2); err != nil {
		t.Fatal(err)
	}
	err = h.refuseNewBookOverExisting()
	if err == nil || !strings.Contains(err.Error(), "phase=outline") {
		t.Fatalf("expected foundation guard, got %v", err)
	}
}

func TestRefuseNewBookBlocksChapterFiles(t *testing.T) {
	dir := t.TempDir()
	st := storepkg.NewStore(dir)
	if err := st.Init(); err != nil {
		t.Fatal(err)
	}
	chDir := filepath.Join(dir, "chapters")
	if err := os.MkdirAll(chDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(chDir, "01.md"), []byte("# hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	h := &Host{store: st}
	err := h.refuseNewBookOverExisting()
	if err == nil || !strings.Contains(err.Error(), "chapters") {
		t.Fatalf("expected chapter file guard, got %v", err)
	}
}

func TestCreateBookAndOpenLeavesPreviousDirIntact(t *testing.T) {
	libRoot := t.TempDir()
	oldDir := t.TempDir()
	oldStore := storepkg.NewStore(oldDir)
	if err := oldStore.Init(); err != nil {
		t.Fatal(err)
	}
	if err := oldStore.Progress.Save(&domain.Progress{
		NovelName:         "Sách A",
		Phase:             domain.PhaseWriting,
		CompletedChapters: []int{1},
	}); err != nil {
		t.Fatal(err)
	}
	chDir := filepath.Join(oldDir, "chapters")
	if err := os.MkdirAll(chDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(chDir, "01.md"), []byte("A"), 0o644); err != nil {
		t.Fatal(err)
	}

	lease, err := acquireBookLease(oldDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = lease.Close() })

	h := &Host{
		store:     oldStore,
		bookLease: lease,
		events:    make(chan Event, 8),
		cfg:       bootstrap.Config{OutputDir: oldDir},
	}

	book, err := h.CreateBookAndOpen(libRoot, library.CreateInput{Title: "Sách B"})
	if err != nil {
		t.Fatalf("CreateBookAndOpen: %v", err)
	}
	t.Cleanup(func() {
		if h.bookLease != nil {
			_ = h.bookLease.Close()
		}
	})
	if sameBookPath(h.store.Dir(), oldDir) {
		t.Fatal("host still on old book")
	}
	if !sameBookPath(h.store.Dir(), book.Path) {
		t.Fatalf("active=%s book=%s", h.store.Dir(), book.Path)
	}
	data, err := os.ReadFile(filepath.Join(oldDir, "chapters", "01.md"))
	if err != nil || string(data) != "A" {
		t.Fatalf("old book corrupted: %v %q", err, data)
	}
	if err := h.refuseNewBookOverExisting(); err != nil {
		t.Fatalf("new book should allow start: %v", err)
	}
	oldHost := &Host{store: oldStore}
	if err := oldHost.refuseNewBookOverExisting(); err == nil {
		t.Fatal("old book should still block new start")
	}
}

func TestResolveBookDirCreatesLibraryEntry(t *testing.T) {
	libRoot := t.TempDir()
	dir, err := ResolveBookDir("", "Truyện Test Resolve", libRoot)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Fatal(err)
	}
	idx, err := library.Open(libRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(idx.Books) == 0 {
		t.Fatal("expected library registration")
	}
}
