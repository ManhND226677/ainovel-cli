package translation

import (
	"context"
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

	mu           sync.Mutex
	running      bool
	pending      bool
	trigger      string
	lastDecision time.Time
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
	for _, chapter := range job.Chapters {
		if err := ctx.Err(); err != nil {
			job.State, job.LastError = JobCancelled, err.Error()
			_ = c.Store.UpdateJob(job)
			return err
		}
		source, ok := sources[chapter]
		if !ok {
			job.State, job.LastError = JobFailed, fmt.Sprintf("source chapter %d is unavailable", chapter)
			_ = c.Store.UpdateJob(job)
			return fmt.Errorf("%s", job.LastError)
		}
		status, err := c.Store.LoadStatus()
		if err != nil {
			return err
		}
		record := status.Chapters[chapter]
		if record.State == ChapterCompleted && record.SourceSHA256 == source.SHA256 {
			continue // resume is idempotent for chapters that committed before interruption
		}
		if record.Attempts >= c.Policy.Normalize().MaxRetries {
			return c.fail(job, chapter, fmt.Errorf("chapter %d exceeded translation retry limit", chapter))
		}
		if err := c.Store.StartChapter(job.ID, source); err != nil {
			job.State, job.LastError = JobFailed, err.Error()
			_ = c.Store.UpdateJob(job)
			return err
		}
		glossary, err := c.Store.LoadGlossary()
		if err != nil {
			return c.fail(job, chapter, err)
		}
		previous, _ := c.previousTranslation(chapter)
		translated, err := Translate(ctx, c.TranslatorModel, c.TranslatorPrompt, TranslationRequest{
			Chapter:            chapter,
			ChineseText:        source.Text,
			Glossary:           glossary.Terms,
			PreviousVietnamese: previous,
		})
		if err != nil {
			return c.fail(job, chapter, err)
		}
		current, err := c.Source.LoadCommittedChapter(chapter)
		if err != nil {
			return c.fail(job, chapter, err)
		}
		if Digest(current) != source.SHA256 {
			_ = c.Store.MarkStale(chapter, Digest(current))
			return c.fail(job, chapter, fmt.Errorf("source chapter %d changed during translation", chapter))
		}
		updatedGlossary, err := c.Store.MergeGlossary(chapter, translated.Glossary)
		if err != nil {
			return c.fail(job, chapter, err)
		}
		if _, err := c.Store.CommitChapter(source, translated.Text, c.TranslatorMeta.Provider, c.TranslatorMeta.Name, job.ID, updatedGlossary.Version); err != nil {
			return c.fail(job, chapter, err)
		}
		c.report("info", fmt.Sprintf("Đã dịch xong chương %d (job %s)", chapter, job.ID))
	}
	job.State = JobCompleted
	job.LastError = ""
	if err := c.Store.UpdateJob(job); err != nil {
		return err
	}
	c.report("success", fmt.Sprintf("Hoàn tất lô dịch chương %s", formatChapters(job.Chapters)))
	return nil
}

func (c *Controller) fail(job Job, chapter int, err error) error {
	_ = c.Store.FailChapter(job.ID, chapter, err)
	job.State = JobFailed
	job.LastError = err.Error()
	_ = c.Store.UpdateJob(job)
	c.report("error", fmt.Sprintf("Dịch chương %d thất bại; có thể tiếp tục lại: %v", chapter, err))
	return err
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
