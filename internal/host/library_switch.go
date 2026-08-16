package host

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/voocel/ainovel-cli/internal/library"
	storepkg "github.com/voocel/ainovel-cli/internal/store"
	"github.com/voocel/ainovel-cli/internal/translation"
)

// ActiveBookDir returns the current store root (absolute when possible).
func (h *Host) ActiveBookDir() string {
	if h == nil || h.store == nil {
		return ""
	}
	return h.store.Dir()
}

// SwitchBook rebinds the Host to another book directory.
// Requires idle lifecycle (not running/paused mid-job). Releases the old lock,
// acquires the new one, and rebuilds store + translation controller handles.
func (h *Host) SwitchBook(dir string) error {
	if h == nil {
		return fmt.Errorf("host is nil")
	}
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return fmt.Errorf("book directory is required")
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return fmt.Errorf("resolve book dir: %w", err)
	}
	if st, err := os.Stat(abs); err != nil || !st.IsDir() {
		return fmt.Errorf("book directory does not exist: %s", abs)
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	if h.closing {
		return fmt.Errorf("host is closing")
	}
	if h.lifecycle == lifecycleRunning {
		return fmt.Errorf("engine is running — pause/stop before switching novels")
	}
	if h.exclusive != "" {
		return fmt.Errorf("background job %q is active — wait or abort before switching", h.exclusive)
	}
	if h.cocreating {
		return fmt.Errorf("co-create session is open — finish it before switching novels")
	}
	if cur := h.store.Dir(); sameBookPath(cur, abs) {
		return nil
	}

	// Persist usage for the outgoing book before rebinding the tracker path.
	if h.usage != nil {
		_ = h.usage.SaveNow()
	}

	newLease, err := acquireBookLease(abs)
	if err != nil {
		return err
	}

	newStore := storepkg.NewStore(abs)
	if err := newStore.Init(); err != nil {
		_ = newLease.Close()
		return fmt.Errorf("init store at %s: %w", abs, err)
	}

	// Release old lock only after new lease succeeds.
	oldLease := h.bookLease
	h.bookLease = newLease
	if oldLease != nil {
		if err := oldLease.Close(); err != nil {
			slog.Warn("release previous book lock", "module", "host", "err", err)
		}
	}

	h.store = newStore
	h.cfg.OutputDir = abs
	if h.observer != nil {
		h.observer.rebindStore(newStore)
	}
	if h.usage != nil {
		h.usage.RebindStore(newStore)
		if _, err := h.usage.LoadFromStore(); err != nil {
			slog.Warn("load usage for new book", "module", "host", "err", err)
		}
	}
	if h.gate != nil {
		h.gate.store = newStore
	}

	// Rebuild translation controller against the new root when enabled.
	if h.cfg.Translation.IsEnabled() {
		translationStore := translation.NewStore(abs)
		if err := translationStore.Init(); err != nil {
			return fmt.Errorf("init translation store: %w", err)
		}
		if h.translation == nil {
			coordinatorModel := newUsageTrackedModel(translationRoleModel(h.models, "translation_coordinator", "editor"), "translation_coordinator", h.usage.Record)
			translatorModel := newUsageTrackedModel(translationRoleModel(h.models, "translator", "writer"), "translator", h.usage.Record)
			h.translation = &translation.Controller{
				Store:             translationStore,
				Source:            translationSource{store: newStore},
				CoordinatorModel:  coordinatorModel,
				TranslatorModel:   translatorModel,
				CoordinatorPrompt: h.bundle.Prompts.TranslationCoordinator,
				TranslatorPrompt:  h.bundle.Prompts.Translator,
				CoordinatorMeta:   modelMeta(h.models, "translation_coordinator", "editor"),
				TranslatorMeta:    modelMeta(h.models, "translator", "writer"),
				Policy:            translationPolicy(h.cfg.Translation),
				Report:            translationReporter(h),
			}
		} else {
			h.translation.Store = translationStore
			h.translation.Source = translationSource{store: newStore}
			h.rebindTranslationModelsLocked()
		}
		h.translationPaused = false
	} else {
		h.translation = nil
	}

	// Reset agent chrome for the new book.
	if h.observer != nil {
		h.observer.agentMu.Lock()
		h.observer.agents = make(map[string]*agentState)
		h.observer.agentMu.Unlock()
	}
	h.lifecycle = lifecycleIdle
	h.writerRestore = nil

	title := filepath.Base(abs)
	if p, _ := newStore.Progress.Load(); p != nil && strings.TrimSpace(p.NovelName) != "" {
		title = strings.TrimSpace(p.NovelName)
	}
	h.emitEvent(Event{
		Time:     time.Now(),
		Category: "SYSTEM",
		Summary:  fmt.Sprintf("Đã chuyển truyện: %s", title),
		Level:    "info",
	})
	slog.Info("switched book", "module", "host", "dir", abs, "title", title)
	return nil
}

// ExportMeta fills author/description/titleVI from library book meta when present.
func (h *Host) ExportMeta() (author, description, titleVI string) {
	if h == nil || h.store == nil {
		return "", "", ""
	}
	return library.MetaFromBookDir(h.store.Dir())
}

func sameBookPath(a, b string) bool {
	aa, e1 := filepath.Abs(a)
	bb, e2 := filepath.Abs(b)
	if e1 != nil || e2 != nil {
		return filepath.Clean(a) == filepath.Clean(b)
	}
	return filepath.Clean(aa) == filepath.Clean(bb)
}

// BookWorkSummary is a short projection for UI guards (new book vs resume).
type BookWorkSummary struct {
	Dir             string `json:"dir"`
	Title           string `json:"title,omitempty"`
	Phase           string `json:"phase,omitempty"`
	Completed       int    `json:"completed"`
	HasChapterFiles bool   `json:"has_chapter_files"`
	BlocksNewStart  bool   `json:"blocks_new_start"`
	Message         string `json:"message,omitempty"`
}

// InspectActiveBook reports whether StartPrepared would be refused on the active dir.
func (h *Host) InspectActiveBook() BookWorkSummary {
	sum := BookWorkSummary{}
	if h == nil || h.store == nil {
		return sum
	}
	sum.Dir = h.store.Dir()
	if p, _ := h.store.Progress.Load(); p != nil {
		sum.Title = strings.TrimSpace(p.NovelName)
		sum.Phase = string(p.Phase)
		sum.Completed = len(p.CompletedChapters)
	}
	sum.HasChapterFiles = countFinalChapters(sum.Dir) > 0
	if err := h.refuseNewBookOverExisting(); err != nil {
		sum.BlocksNewStart = true
		sum.Message = err.Error()
	}
	return sum
}

// OpenLibrary opens the project library index (creates root/index if needed).
func OpenLibrary(root string) (*library.Index, error) {
	if strings.TrimSpace(root) == "" {
		root = library.DefaultRoot
	}
	return library.Open(root)
}

// CreateBook registers a new empty book under the library root without switching.
func CreateBook(root string, in library.CreateInput) (library.Book, error) {
	idx, err := OpenLibrary(root)
	if err != nil {
		return library.Book{}, err
	}
	return idx.Create(in)
}

// CreateBookAndOpen creates an empty book directory and switches the Host to it.
// The previous active book files are left untouched on disk.
func (h *Host) CreateBookAndOpen(root string, in library.CreateInput) (library.Book, error) {
	if h == nil {
		return library.Book{}, fmt.Errorf("host is nil")
	}
	idx, err := OpenLibrary(root)
	if err != nil {
		return library.Book{}, err
	}
	book, err := idx.Create(in)
	if err != nil {
		return library.Book{}, err
	}
	if err := h.SwitchBook(book.Path); err != nil {
		return book, fmt.Errorf("created %s but switch failed: %w", book.Path, err)
	}
	_ = idx.SetActive(book.ID)
	book.Active = true
	return book, nil
}

// OpenBookByIDOrPath switches to a registered book (id/slug) or filesystem path.
func (h *Host) OpenBookByIDOrPath(root, idOrPath string) (library.Book, error) {
	if h == nil {
		return library.Book{}, fmt.Errorf("host is nil")
	}
	idOrPath = strings.TrimSpace(idOrPath)
	if idOrPath == "" {
		return library.Book{}, fmt.Errorf("book id or path is required")
	}
	idx, err := OpenLibrary(root)
	if err != nil {
		return library.Book{}, err
	}
	if b, ok := idx.Find(idOrPath); ok {
		if err := h.SwitchBook(b.Path); err != nil {
			return b, err
		}
		_ = idx.SetActive(b.ID)
		b.Active = true
		return b, nil
	}
	abs, err := filepath.Abs(idOrPath)
	if err != nil {
		return library.Book{}, err
	}
	if err := h.SwitchBook(abs); err != nil {
		return library.Book{}, err
	}
	b, err := idx.EnsureRegistered(abs, "")
	if err != nil {
		return library.Book{}, err
	}
	_ = idx.SetActive(b.ID)
	b.Active = true
	return b, nil
}

// ResolveBookDir picks an output directory for CLI/headless:
// explicit outputDir > library book id/slug/path > empty (caller keeps default).
func ResolveBookDir(outputDir, bookRef, libraryRoot string) (string, error) {
	outputDir = strings.TrimSpace(outputDir)
	if outputDir != "" {
		abs, err := filepath.Abs(outputDir)
		if err != nil {
			return "", err
		}
		if err := os.MkdirAll(abs, 0o755); err != nil {
			return "", err
		}
		return abs, nil
	}
	bookRef = strings.TrimSpace(bookRef)
	if bookRef == "" {
		return "", nil
	}
	if st, err := os.Stat(bookRef); err == nil && st.IsDir() {
		return filepath.Abs(bookRef)
	}
	idx, err := OpenLibrary(libraryRoot)
	if err != nil {
		return "", err
	}
	if b, ok := idx.Find(bookRef); ok {
		return b.Path, nil
	}
	book, err := idx.Create(library.CreateInput{Title: bookRef, Slug: bookRef})
	if err != nil {
		book, err = idx.Create(library.CreateInput{Title: bookRef})
		if err != nil {
			return "", fmt.Errorf("book %q not found and create failed: %w", bookRef, err)
		}
	}
	_ = idx.SetActive(book.ID)
	return book.Path, nil
}
