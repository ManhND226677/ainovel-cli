package host

import (
	"fmt"
	"sort"
	"time"

	"github.com/voocel/agentcore"
	"github.com/voocel/ainovel-cli/internal/bootstrap"
	"github.com/voocel/ainovel-cli/internal/domain"
	storepkg "github.com/voocel/ainovel-cli/internal/store"
	"github.com/voocel/ainovel-cli/internal/translation"
)

// translationSource is deliberately read-only. The translation package can
// consume committed Chinese artifacts but cannot mutate the creative Store.
type translationSource struct{ store *storepkg.Store }

func (s translationSource) CompletedChapters() ([]int, error) {
	progress, err := s.store.Progress.Load()
	if err != nil || progress == nil {
		return nil, err
	}
	return append([]int(nil), progress.CompletedChapters...), nil
}

func (s translationSource) PendingRewrites() ([]int, error) {
	progress, err := s.store.Progress.Load()
	if err != nil || progress == nil {
		return nil, err
	}
	return append([]int(nil), progress.PendingRewrites...), nil
}

func (s translationSource) LoadCommittedChapter(chapter int) (string, error) {
	return s.store.Drafts.LoadChapterText(chapter)
}

func (s translationSource) IsBookCompleted() (bool, error) {
	progress, err := s.store.Progress.Load()
	if err != nil || progress == nil {
		return false, err
	}
	return progress.Phase == domain.PhaseComplete, nil
}

func translationPolicy(cfg bootstrap.TranslationConfig) translation.Policy {
	return translation.Policy{
		Enabled:              cfg.Enabled,
		MinStableChapters:    cfg.MinStableChapters,
		MaxBatchChapters:     cfg.MaxBatchChapters,
		MaxLagChapters:       cfg.MaxLagChapters,
		MaxConcurrentBatches: cfg.MaxConcurrentBatches,
		MaxRetries:           cfg.MaxRetries,
		Debounce:             time.Duration(cfg.DebounceSeconds) * time.Second,
		AutoRetranslate:      cfg.AutoRetranslateOnRewrite,
	}.Normalize()
}

// translationRoleModel applies the documented fallbacks without modifying the
// global ModelSet: coordinator inherits editor when configured, otherwise the
// ModelSet default; translator inherits writer when configured, otherwise default.
func translationRoleModel(models *bootstrap.ModelSet, role, fallback string) agentcore.ChatModel {
	reportFailover := func(bootstrap.FailoverEvent) {}
	if _, _, explicit := models.CurrentSelection(role); explicit {
		return models.ForRoleWithFailover(role, reportFailover)
	}
	if _, _, explicit := models.CurrentSelection(fallback); explicit {
		return models.ForRoleWithFailover(fallback, reportFailover)
	}
	return models.Default
}

// rebindTranslationModelsLocked refreshes the Controller's ChatModel handles
// after /settings or SwitchModel. The controller keeps concrete model pointers
// from Host construction; without this, dashboard provider changes never reach
// an already-running translator worker (zyloo slug stuck forever).
//
// Caller must hold h.mu.
func (h *Host) rebindTranslationModelsLocked() {
	if h == nil || h.translation == nil || h.models == nil || h.usage == nil {
		return
	}
	coordinatorModel := newUsageTrackedModel(
		translationRoleModel(h.models, "translation_coordinator", "editor"),
		"translation_coordinator",
		h.usage.Record,
	)
	translatorModel := newUsageTrackedModel(
		translationRoleModel(h.models, "translator", "writer"),
		"translator",
		h.usage.Record,
	)
	h.translation.CoordinatorModel = coordinatorModel
	h.translation.TranslatorModel = translatorModel
	h.translation.CoordinatorMeta = modelMeta(h.models, "translation_coordinator", "editor")
	h.translation.TranslatorMeta = modelMeta(h.models, "translator", "writer")
}

func (h *Host) triggerTranslation(trigger string) {
	controller := h.translation
	if controller == nil || !controller.Policy.Enabled {
		return
	}
	ctx := h.runCtx
	h.launchAsync(func() {
		if err := controller.Run(ctx, trigger); err != nil && ctx.Err() == nil {
			h.emitTranslationEvent(Event{Time: time.Now(), Category: "ERROR", Level: "error", Summary: "Agent điều phối/dịch gặp lỗi: " + err.Error()})
		}
	})
}

func (h *Host) emitTranslationEvent(event Event) {
	if event.Agent == "" {
		event.Agent = "translation_coordinator"
	}
	h.emitEvent(event)
	// Translation Controller không đi qua agentcore observer. Persist event tại
	// đây để /api/events và WebSocket replay có cùng lịch sử với worker khác.
	if h.observer != nil {
		h.observer.persistEvent(event)
	}
}

func translationReporter(h *Host) translation.Reporter {
	return func(level, summary string) {
		eventLevel := "info"
		switch level {
		case "error":
			eventLevel = "error"
		case "warn":
			eventLevel = "warn"
		case "success":
			eventLevel = "success"
		}
		h.emitTranslationEvent(Event{Time: time.Now(), Category: "TRANSLATION", Level: eventLevel, Summary: summary})
	}
}

func modelMeta(models *bootstrap.ModelSet, role, fallback string) translation.ModelMeta {
	model := translationRoleModel(models, role, fallback)
	return translation.ModelMeta{Provider: bootstrap.ModelProvider(model), Name: bootstrap.ModelName(model)}
}

// Ensure compile-time conformance to the read-only source boundary.
var _ translation.SourceReader = translationSource{}

func translationInitError(err error) error { return fmt.Errorf("init translation: %w", err) }

// RequestTranslation asks the Coordinator to re-evaluate current committed
// facts. It returns immediately; the agent work is registered with the Host
// lifecycle and reported through normal events.
func (h *Host) RequestTranslation() error {
	if h.translation == nil || !h.translation.Policy.Enabled {
		return fmt.Errorf("translation is not enabled for this book")
	}
	if !h.launchAsync(func() {
		if err := h.translation.RunFullQueue(h.runCtx); err != nil && h.runCtx.Err() == nil {
			h.emitTranslationEvent(Event{Time: time.Now(), Category: "ERROR", Level: "error", Summary: "Không thể chạy hàng đợi dịch: " + err.Error(), Detail: err.Error()})
		}
	}) {
		return fmt.Errorf("Host đang đóng, không thể khởi động hàng đợi dịch")
	}
	return nil
}

// TranslationStatus returns the durable status of Vietnamese artifacts.
func (h *Host) TranslationStatus() (translation.Status, error) {
	if h.translation == nil || !h.translation.Policy.Enabled {
		return translation.Status{}, fmt.Errorf("translation is not enabled for this book")
	}
	return h.translation.Store.LoadStatus()
}

// RetryTranslations explicitly requeues failed/stale chapters selected by the
// web dashboard. The actual LLM work runs under Host lifecycle ownership and
// reports progress through the normal event projection.
func (h *Host) RetryTranslations(chapters []int) error {
	if h.translation == nil || !h.translation.Policy.Enabled {
		return fmt.Errorf("translation is not enabled for this book")
	}
	chapters = append([]int(nil), chapters...)
	sort.Ints(chapters)
	if len(chapters) == 0 {
		return fmt.Errorf("retry requires at least one chapter")
	}
	if !h.launchAsync(func() {
		h.emitTranslationEvent(Event{Time: time.Now(), Category: "TRANSLATION", Level: "info", Summary: fmt.Sprintf("Dashboard yêu cầu retry %d chương dịch", len(chapters))})
		if err := h.translation.Retry(h.runCtx, chapters); err != nil && h.runCtx.Err() == nil {
			h.emitTranslationEvent(Event{Time: time.Now(), Category: "ERROR", Level: "error", Summary: "Retry batch dịch thất bại: " + err.Error(), Detail: err.Error()})
		}
	}) {
		return fmt.Errorf("Host đang đóng, không thể retry batch dịch")
	}
	return nil
}

// PauseTranslation prevents workers from claiming further queued chapters.
func (h *Host) PauseTranslation() error {
	if h.translation == nil || !h.translation.Policy.Enabled {
		return fmt.Errorf("translation is not enabled for this book")
	}
	h.translation.Pause()
	h.mu.Lock()
	h.translationPaused = true
	h.mu.Unlock()
	h.emitTranslationEvent(Event{Time: time.Now(), Category: "TRANSLATION", Level: "info", Summary: "Đã tạm dừng nhận chapter mới trong hàng đợi dịch"})
	return nil
}

func (h *Host) ResumeTranslation() error {
	if h.translation == nil || !h.translation.Policy.Enabled {
		return fmt.Errorf("translation is not enabled for this book")
	}
	h.translation.Resume()
	h.mu.Lock()
	h.translationPaused = false
	h.mu.Unlock()
	h.emitTranslationEvent(Event{Time: time.Now(), Category: "TRANSLATION", Level: "info", Summary: "Đã tiếp tục hàng đợi dịch"})
	return nil
}

func (h *Host) IsTranslationPaused() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.translationPaused
}

func (h *Host) StopTranslationChapter(chapter int) (bool, error) {
	if h.translation == nil || !h.translation.Policy.Enabled {
		return false, fmt.Errorf("translation is not enabled for this book")
	}
	stopped := h.translation.StopChapter(chapter)
	if stopped {
		h.emitTranslationEvent(Event{Time: time.Now(), Category: "TRANSLATION", Level: "warn", Summary: fmt.Sprintf("Đã yêu cầu dừng Translation Agent ở chapter %d", chapter)})
	}
	return stopped, nil
}

// SetTranslationInstruction durablely records user guidance for the next call
// to the Translator Agent. The Chinese source is never modified.
func (h *Host) SetTranslationInstruction(chapter int, instruction string) (translation.ChapterRecord, error) {
	if h.translation == nil || !h.translation.Policy.Enabled {
		return translation.ChapterRecord{}, fmt.Errorf("translation is not enabled for this book")
	}
	record, err := h.translation.Store.SetChapterInstruction(chapter, instruction)
	if err != nil {
		return translation.ChapterRecord{}, err
	}
	h.emitTranslationEvent(Event{Time: time.Now(), Category: "TRANSLATION", Level: "info", Summary: fmt.Sprintf("Đã lưu chỉ dẫn cho chapter %d", chapter)})
	return record, nil
}

// SubscribeLiveProgress installs the local presentation observer. It receives
// raw source/preview excerpts only through the loopback WebSocket bridge.
func (h *Host) SubscribeLiveProgress(fn translation.ProgressObserver) {
	if h.translation != nil {
		h.translation.Observe = fn
	}
}
