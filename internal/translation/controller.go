package translation

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/voocel/agentcore"
)

// SourceReader exposes committed Chinese facts to the translation branch. It
// deliberately has no write methods, so Coordinator and Translator cannot alter
// the creative source of truth.
type SourceReader interface {
	CompletedChapters() ([]int, error)
	PendingRewrites() ([]int, error)
	LoadCommittedChapter(chapter int) (string, error)
	IsBookCompleted() (bool, error)
}

// Reporter receives short presentation-safe Vietnamese status messages. Raw
// coordinator facts and decisions are persisted separately in the audit log.
type Reporter func(level, summary string)

// Controller combines the Coordinator Agent, Translator Agent, isolated Store,
// and source reader. Run is safe to invoke concurrently from commit/review/
// rewrite events; it coalesces extra triggers rather than running two batches.
type Controller struct {
	Store  *Store
	Source SourceReader

	CoordinatorModel  agentcore.ChatModel
	TranslatorModel   agentcore.ChatModel
	CoordinatorPrompt string
	TranslatorPrompt  string
	CoordinatorMeta   ModelMeta
	TranslatorMeta    ModelMeta
	Policy            Policy
	Report            Reporter
	Observe           ProgressObserver

	mu           sync.Mutex
	running      bool
	pending      bool
	trigger      string
	lastDecision time.Time
	controlMu    sync.Mutex
	paused       bool
	resumeCh     chan struct{}
	cancels      map[int]context.CancelFunc
}

// ModelMeta is persisted on artifacts for audit without coupling this package
// to the bootstrap/model configuration implementation.
type ModelMeta struct {
	Provider string
	Name     string
}

// Run evaluates a durable snapshot and, when approved by the Coordinator and
// mechanics, executes one translation batch. Concurrent calls collapse into a
// final re-evaluation after the active run ends.
func (c *Controller) Run(ctx context.Context, trigger string) error {
	if c == nil || !c.Policy.Enabled {
		return nil
	}
	if c.Store == nil || c.Source == nil {
		return fmt.Errorf("translation controller requires store and source reader")
	}
	c.mu.Lock()
	if c.running {
		c.pending = true
		c.trigger = trigger
		c.mu.Unlock()
		return nil
	}
	c.running = true
	c.trigger = trigger
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		c.running = false
		c.mu.Unlock()
	}()

	for {
		c.mu.Lock()
		currentTrigger := c.trigger
		c.pending = false
		c.mu.Unlock()

		if err := c.runOnce(ctx, currentTrigger); err != nil {
			return err
		}
		c.mu.Lock()
		again := c.pending && ctx.Err() == nil
		c.mu.Unlock()
		if !again {
			return nil
		}
	}
}

// Pause prevents workers from taking another queued chapter. In-flight calls
// finish normally so valid output is not discarded mid-stream.
func (c *Controller) Pause() {
	c.controlMu.Lock()
	defer c.controlMu.Unlock()
	if c.paused {
		return
	}
	c.paused = true
	c.resumeCh = make(chan struct{})
}

func (c *Controller) Resume() {
	c.controlMu.Lock()
	defer c.controlMu.Unlock()
	if !c.paused {
		return
	}
	close(c.resumeCh)
	c.paused = false
	c.resumeCh = nil
}

func (c *Controller) IsPaused() bool {
	c.controlMu.Lock()
	defer c.controlMu.Unlock()
	return c.paused
}

// StopChapter cancels one active model call. Its durable record becomes failed
// and may safely be retried without changing the Chinese source artifact.
func (c *Controller) StopChapter(chapter int) bool {
	c.controlMu.Lock()
	cancel := c.cancels[chapter]
	c.controlMu.Unlock()
	if cancel == nil {
		return false
	}
	cancel()
	return true
}

func (c *Controller) waitForPermission(ctx context.Context) error {
	c.controlMu.Lock()
	paused, resumeCh := c.paused, c.resumeCh
	c.controlMu.Unlock()
	if !paused {
		return nil
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-resumeCh:
		return nil
	}
}

func (c *Controller) registerCancel(chapter int, cancel context.CancelFunc) {
	c.controlMu.Lock()
	defer c.controlMu.Unlock()
	if c.cancels == nil {
		c.cancels = make(map[int]context.CancelFunc)
	}
	c.cancels[chapter] = cancel
}

func (c *Controller) clearCancel(chapter int) {
	c.controlMu.Lock()
	defer c.controlMu.Unlock()
	delete(c.cancels, chapter)
}

func (c *Controller) progress(update LiveProgress) {
	if c.Observe == nil {
		return
	}
	update.UpdatedAt = time.Now().UTC()
	c.Observe(update)
}

// Retry queues only failed or stale chapters selected by the local dashboard.
// It bypasses Coordinator policy because this is an explicit human recovery
// action, while still reusing the durable job, fingerprint, glossary and retry
// mechanics of the normal translation pipeline.
func (c *Controller) Retry(ctx context.Context, chapters []int) error {
	if c == nil || !c.Policy.Enabled {
		return fmt.Errorf("translation is not enabled")
	}
	chapters = normalizeChapters(chapters)
	policy := c.Policy.Normalize()
	if len(chapters) == 0 {
		return fmt.Errorf("retry requires at least one chapter")
	}
	if len(chapters) > policy.MaxBatchChapters {
		return fmt.Errorf("retry batch size %d exceeds max_batch_chapters %d", len(chapters), policy.MaxBatchChapters)
	}
	_, sources, err := c.collectSnapshot("web_retry")
	if err != nil {
		return err
	}
	status, err := c.Store.LoadStatus()
	if err != nil {
		return err
	}
	for _, chapter := range chapters {
		record, ok := status.Chapters[chapter]
		if !ok || (record.State != ChapterFailed && record.State != ChapterStale) {
			return fmt.Errorf("chapter %d is not failed or stale", chapter)
		}
		if _, ok := sources[chapter]; !ok {
			return fmt.Errorf("source chapter %d is unavailable", chapter)
		}
	}
	job, err := c.Store.QueueJob(Decision{
		Action:   DecisionRetranslate,
		Chapters: chapters,
		Reason:   "Người dùng yêu cầu thử lại từ dashboard",
	}, sources)
	if err != nil {
		return err
	}
	c.report("info", fmt.Sprintf("Đã xếp lại lô retry chương %s", formatChapters(job.Chapters)))
	return c.executeJob(ctx, job, sources)
}

// RunFullQueue persists every currently eligible, untranslated chapter as
// pending and then executes the queue in ascending chapter order with a bounded
// worker pool. Chinese source files remain read-only throughout the workflow.
func (c *Controller) RunFullQueue(ctx context.Context) error {
	if c == nil || !c.Policy.Enabled {
		return fmt.Errorf("translation is not enabled")
	}
	if c.Store == nil || c.Source == nil {
		return fmt.Errorf("translation controller requires store and source reader")
	}
	c.mu.Lock()
	if c.running {
		c.mu.Unlock()
		return fmt.Errorf("translation scheduler is already running")
	}
	c.running = true
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		c.running = false
		c.mu.Unlock()
	}()
	if recovered, err := c.Store.RecoverInterruptedJobs(); err != nil {
		return fmt.Errorf("recover interrupted translation queue: %w", err)
	} else if recovered > 0 {
		c.report("warn", fmt.Sprintf("Đã phục hồi %d trạng thái dịch bị gián đoạn sau khi engine khởi động lại", recovered))
	}

	snapshot, sources, err := c.collectSnapshot("dashboard_full_queue")
	if err != nil {
		return err
	}
	status, err := c.Store.LoadStatus()
	if err != nil {
		return err
	}
	policy := c.Policy.Normalize()
	blocked := make(map[int]bool, len(snapshot.PendingRewrites))
	for _, chapter := range snapshot.PendingRewrites {
		blocked[chapter] = true
	}
	chapters := make([]int, 0, len(snapshot.Completed))
	for _, source := range snapshot.Completed {
		chapter := source.Number
		if chapter <= 0 || blocked[chapter] {
			continue
		}
		record, exists := status.Chapters[chapter]
		if exists && record.State == ChapterCompleted && record.SourceSHA256 == sources[chapter].SHA256 {
			continue
		}
		if exists && record.Attempts >= policy.MaxRetries {
			continue
		}
		chapters = append(chapters, chapter)
	}
	sort.Ints(chapters)
	if len(chapters) == 0 {
		c.report("success", "Hàng đợi dịch đã hoàn tất; không còn chương đủ điều kiện cần dịch")
		return nil
	}
	job, err := c.Store.QueueJob(Decision{
		Action:   DecisionTranslate,
		Chapters: chapters,
		Reason:   "Dashboard yêu cầu dịch tuần tự toàn bộ chương chưa có bản Việt",
	}, sources)
	if err != nil {
		return err
	}
	c.report("info", fmt.Sprintf("Đã xếp hàng %d chương dịch; tối đa %d worker song song", len(chapters), policy.MaxConcurrentBatches))
	return c.executeJob(ctx, job, sources)
}

func (c *Controller) runOnce(ctx context.Context, trigger string) error {
	policy := c.Policy.Normalize()
	c.mu.Lock()
	wait := policy.Debounce - time.Since(c.lastDecision)
	c.mu.Unlock()
	if wait > 0 {
		timer := time.NewTimer(wait)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
		}
	}
	snapshot, sources, err := c.collectSnapshot(trigger)
	if err != nil {
		return err
	}
	decision, err := Decide(ctx, c.CoordinatorModel, c.CoordinatorPrompt, snapshot)
	c.mu.Lock()
	c.lastDecision = time.Now().UTC()
	c.mu.Unlock()
	audit := AuditDecision{
		SnapshotDigest: SnapshotDigest(snapshot),
		Trigger:        trigger,
		Decision:       decision,
		Valid:          err == nil,
		Provider:       c.CoordinatorMeta.Provider,
		Model:          c.CoordinatorMeta.Name,
	}
	if err != nil {
		audit.ValidationErr = err.Error()
		_ = c.Store.AppendDecision(audit)
		return err
	}
	if validationErr := decision.Validate(snapshot); validationErr != nil {
		audit.Valid = false
		audit.ValidationErr = validationErr.Error()
		_ = c.Store.AppendDecision(audit)
		return validationErr
	}
	if err := c.Store.AppendDecision(audit); err != nil {
		return fmt.Errorf("append translation decision: %w", err)
	}

	switch decision.Action {
	case DecisionWait:
		c.report("info", "Agent điều phối chưa mở lô dịch: "+decision.Reason)
		return nil
	case DecisionFinalize:
		c.report("success", "Agent điều phối xác nhận bản dịch đã sẵn sàng để xuất: "+decision.Reason)
		return nil
	case DecisionResume:
		job, err := c.resumeJob(decision.JobID)
		if err != nil {
			return err
		}
		return c.executeJob(ctx, job, sources)
	case DecisionTranslate, DecisionRetranslate:
		job, err := c.Store.QueueJob(decision, sources)
		if err != nil {
			return err
		}
		c.report("info", fmt.Sprintf("Agent điều phối mở lô dịch chương %s", formatChapters(job.Chapters)))
		return c.executeJob(ctx, job, sources)
	default:
		return fmt.Errorf("unsupported validated translation action %q", decision.Action)
	}
}

func (c *Controller) collectSnapshot(trigger string) (Snapshot, map[int]SourceChapter, error) {
	completed, err := c.Source.CompletedChapters()
	if err != nil {
		return Snapshot{}, nil, fmt.Errorf("load completed chapters: %w", err)
	}
	sort.Ints(completed)
	sources := make(map[int]SourceChapter, len(completed))
	chapters := make([]SourceChapter, 0, len(completed))
	for _, chapter := range completed {
		if chapter <= 0 {
			continue
		}
		text, err := c.Source.LoadCommittedChapter(chapter)
		if err != nil {
			return Snapshot{}, nil, fmt.Errorf("load committed chapter %d: %w", chapter, err)
		}
		if strings.TrimSpace(text) == "" {
			continue
		}
		source := NewSourceChapter(chapter, text)
		sources[chapter] = source
		chapters = append(chapters, source)
	}
	pending, err := c.Source.PendingRewrites()
	if err != nil {
		return Snapshot{}, nil, fmt.Errorf("load pending rewrites: %w", err)
	}
	completedBook, err := c.Source.IsBookCompleted()
	if err != nil {
		return Snapshot{}, nil, fmt.Errorf("load book phase: %w", err)
	}
	status, err := c.Store.LoadStatus()
	if err != nil {
		return Snapshot{}, nil, err
	}
	for chapter, record := range status.Chapters {
		source, exists := sources[chapter]
		if exists && record.SourceSHA256 != "" && record.SourceSHA256 != source.SHA256 && record.State != ChapterRunning {
			if err := c.Store.MarkStale(chapter, source.SHA256); err != nil {
				return Snapshot{}, nil, fmt.Errorf("mark chapter %d stale: %w", chapter, err)
			}
		}
	}
	status, err = c.Store.LoadStatus()
	if err != nil {
		return Snapshot{}, nil, err
	}
	glossary, err := c.Store.LoadGlossary()
	if err != nil {
		return Snapshot{}, nil, err
	}
	var running []Job
	for _, job := range status.Jobs {
		if job.State == JobQueued || job.State == JobRunning {
			running = append(running, job)
		}
	}
	sort.Slice(running, func(i, j int) bool { return running[i].CreatedAt.Before(running[j].CreatedAt) })
	return Snapshot{
		Completed:       chapters,
		PendingRewrites: append([]int(nil), pending...),
		Translated:      SortedRecords(status),
		RunningJobs:     running,
		GlossaryVersion: glossary.Version,
		BookCompleted:   completedBook,
		Trigger:         trigger,
		Policy:          c.Policy.Normalize(),
		CollectedAt:     time.Now().UTC(),
	}, sources, nil
}

func (c *Controller) resumeJob(id string) (Job, error) {
	status, err := c.Store.LoadStatus()
	if err != nil {
		return Job{}, err
	}
	job, ok := status.Jobs[id]
	if !ok {
		return Job{}, fmt.Errorf("translation job %s not found", id)
	}
	if job.State != JobQueued && job.State != JobRunning && job.State != JobFailed {
		return Job{}, fmt.Errorf("translation job %s cannot resume from %s", id, job.State)
	}
	return job, nil
}

func (c *Controller) executeJob(ctx context.Context, job Job, sources map[int]SourceChapter) error {
	job.State = JobRunning
	job.Attempts++
	if err := c.Store.UpdateJob(job); err != nil {
		return err
	}
	workerCount := min(c.Policy.Normalize().MaxConcurrentBatches, len(job.Chapters))
	chapters := make(chan int)
	failures := make(chan error, len(job.Chapters))
	var wg sync.WaitGroup
	for range workerCount {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for chapter := range chapters {
				if err := c.executeChapter(ctx, job, chapter, sources); err != nil {
					failures <- err
				}
			}
		}()
	}
	for _, chapter := range job.Chapters {
		chapters <- chapter
	}
	close(chapters)
	wg.Wait()
	close(failures)
	var errs []error
	for err := range failures {
		errs = append(errs, err)
	}
	if ctx.Err() != nil {
		job.State, job.LastError = JobCancelled, ctx.Err().Error()
		_ = c.Store.UpdateJob(job)
		return ctx.Err()
	}
	if len(errs) > 0 {
		job.State, job.LastError = JobFailed, errs[0].Error()
		_ = c.Store.UpdateJob(job)
		return errors.Join(errs...)
	}
	job.State = JobCompleted
	job.LastError = ""
	if err := c.Store.UpdateJob(job); err != nil {
		return err
	}
	c.report("success", fmt.Sprintf("Hoàn tất lô dịch chương %s", formatChapters(job.Chapters)))
	return nil
}

func (c *Controller) executeChapter(ctx context.Context, job Job, chapter int, sources map[int]SourceChapter) error {
	if err := c.waitForPermission(ctx); err != nil {
		return err
	}
	chapterCtx, cancel := context.WithCancel(ctx)
	c.registerCancel(chapter, cancel)
	defer func() {
		cancel()
		c.clearCancel(chapter)
	}()
	if err := chapterCtx.Err(); err != nil {
		return err
	}
	source, ok := sources[chapter]
	if !ok {
		return c.failChapter(job, chapter, fmt.Errorf("source chapter %d is unavailable", chapter))
	}
	status, err := c.Store.LoadStatus()
	if err != nil {
		return err
	}
	record := status.Chapters[chapter]
	if record.State == ChapterCompleted && record.SourceSHA256 == source.SHA256 {
		return nil
	}
	if record.Attempts >= c.Policy.Normalize().MaxRetries {
		return c.failChapter(job, chapter, fmt.Errorf("chapter %d exceeded translation retry limit", chapter))
	}
	c.progress(LiveProgress{Chapter: chapter, Stage: "preparing", SourcePreview: previewText(source.Text), Detail: "Đang đọc bản Trung và chuẩn bị glossary"})
	if err := c.Store.StartChapter(job.ID, source); err != nil {
		return c.failChapter(job, chapter, err)
	}
	glossary, err := c.Store.LoadGlossary()
	if err != nil {
		return c.failChapter(job, chapter, err)
	}
	previous, _ := c.previousTranslation(chapter)
	c.progress(LiveProgress{Chapter: chapter, Stage: "streaming", SourcePreview: previewText(source.Text), Detail: "Translation Agent đang sinh bản nháp Việt"})
	translated, err := TranslateWithPreview(chapterCtx, c.TranslatorModel, c.TranslatorPrompt, TranslationRequest{
		Chapter: chapter, ChineseText: source.Text, Glossary: glossary.Terms, PreviousVietnamese: previous, Instruction: record.Instruction,
	}, func(delta string) {
		c.progress(LiveProgress{Chapter: chapter, Stage: "streaming", Preview: delta})
	})
	if err != nil {
		return c.failChapter(job, chapter, err)
	}
	current, err := c.Source.LoadCommittedChapter(chapter)
	if err != nil {
		return c.failChapter(job, chapter, err)
	}
	if Digest(current) != source.SHA256 {
		_ = c.Store.MarkStale(chapter, Digest(current))
		return c.failChapter(job, chapter, fmt.Errorf("source chapter %d changed during translation", chapter))
	}
	updatedGlossary, err := c.Store.MergeGlossary(chapter, translated.Glossary)
	if err != nil {
		return c.failChapter(job, chapter, err)
	}
	title := ExtractTitleFromBody(translated.Text)
	if _, err := c.Store.CommitChapter(source, translated.Text, c.TranslatorMeta.Provider, c.TranslatorMeta.Name, job.ID, updatedGlossary.Version, title); err != nil {
		return c.failChapter(job, chapter, err)
	}
	c.progress(LiveProgress{Chapter: chapter, Stage: "completed", SourcePreview: previewText(source.Text), Preview: translated.Text, Detail: "Đã lưu bản dịch Việt bền vững"})
	c.report("info", fmt.Sprintf("Đã dịch xong chương %d (job %s)", chapter, job.ID))
	return nil
}

func (c *Controller) failChapter(job Job, chapter int, err error) error {
	_ = c.Store.FailChapter(job.ID, chapter, err)
	c.progress(LiveProgress{Chapter: chapter, Stage: "failed", Error: err.Error(), Detail: "Có thể xem lỗi và retry an toàn"})
	c.report("error", fmt.Sprintf("Dịch chương %d thất bại; có thể tiếp tục lại: %v", chapter, err))
	return err
}

func previewText(text string) string {
	const maxRunes = 900
	runes := []rune(strings.TrimSpace(text))
	if len(runes) <= maxRunes {
		return string(runes)
	}
	return string(runes[:maxRunes]) + "…"
}

func (c *Controller) previousTranslation(chapter int) (string, error) {
	if chapter <= 1 {
		return "", nil
	}
	text, record, err := c.Store.LoadChapter(chapter - 1)
	if err != nil || record.State != ChapterCompleted {
		return "", nil
	}
	return text, nil
}

func (c *Controller) report(level, text string) {
	if c.Report != nil {
		c.Report(level, text)
	}
}

func formatChapters(chapters []int) string {
	if len(chapters) == 0 {
		return ""
	}
	if len(chapters) == 1 {
		return fmt.Sprintf("%d", chapters[0])
	}
	return fmt.Sprintf("%d–%d", chapters[0], chapters[len(chapters)-1])
}

func normalizeChapters(chapters []int) []int {
	seen := make(map[int]struct{}, len(chapters))
	result := make([]int, 0, len(chapters))
	for _, chapter := range chapters {
		if chapter <= 0 {
			continue
		}
		if _, ok := seen[chapter]; ok {
			continue
		}
		seen[chapter] = struct{}{}
		result = append(result, chapter)
	}
	sort.Ints(result)
	return result
}
