package translation

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/voocel/agentcore"
)

func testSnapshot() Snapshot {
	one := NewSourceChapter(1, "第一章：风起")
	two := NewSourceChapter(2, "第二章：雨落")
	return Snapshot{
		Completed:       []SourceChapter{one, two},
		PendingRewrites: []int{2},
		Policy:          Policy{Enabled: true, MaxBatchChapters: 2},
	}
}

func TestDecisionValidateOnlyAllowsStableCommittedChapters(t *testing.T) {
	snapshot := testSnapshot()
	if err := (Decision{Action: DecisionTranslate, Chapters: []int{1}, Reason: "chương 1 đã ổn định"}).Validate(snapshot); err != nil {
		t.Fatalf("stable chapter should be eligible: %v", err)
	}
	if err := (Decision{Action: DecisionTranslate, Chapters: []int{2}, Reason: "bad"}).Validate(snapshot); err == nil {
		t.Fatal("pending rewrite chapter must be rejected")
	}
	if err := (Decision{Action: DecisionTranslate, Chapters: []int{2, 1}, Reason: "bad order"}).Validate(snapshot); err == nil {
		t.Fatal("unordered batch must be rejected")
	}
	if err := (Decision{Action: DecisionWait, Chapters: []int{1}, Reason: "bad wait"}).Validate(snapshot); err == nil {
		t.Fatal("wait with chapters must be rejected")
	}
}

func TestTranslationStoreCommitIsIsolatedAndMarksStale(t *testing.T) {
	bookDir := t.TempDir()
	store := NewStore(bookDir)
	if err := store.Init(); err != nil {
		t.Fatal(err)
	}
	source := NewSourceChapter(1, "第一章：风起")
	decision := Decision{Action: DecisionTranslate, Chapters: []int{1}, Reason: "scene boundary"}
	job, err := store.QueueJob(decision, map[int]SourceChapter{1: source})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.StartChapter(job.ID, source); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CommitChapter(source, "Chương 1: Gió nổi", "provider", "model", job.ID, 1); err != nil {
		t.Fatal(err)
	}
	text, record, err := store.LoadChapter(1)
	if err != nil {
		t.Fatal(err)
	}
	if text != "Chương 1: Gió nổi" || record.State != ChapterCompleted || record.SourceSHA256 != source.SHA256 {
		t.Fatalf("unexpected completed artifact: text=%q record=%+v", text, record)
	}
	if err := store.MarkStale(1, Digest("第一章：风起（重写）")); err != nil {
		t.Fatal(err)
	}
	status, err := store.LoadStatus()
	if err != nil {
		t.Fatal(err)
	}
	if got := status.Chapters[1].State; got != ChapterStale {
		t.Fatalf("state after source rewrite = %s, want stale", got)
	}
	if _, err := os.Stat(filepath.Join(bookDir, "chapters", "01.md")); !os.IsNotExist(err) {
		t.Fatalf("translation must not write source chapters: err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(bookDir, "translations", "vi", "chapters", "01.md")); err != nil {
		t.Fatalf("translation artifact missing: %v", err)
	}
}

func TestQueueJobMarksWholeBacklogPending(t *testing.T) {
	store := NewStore(t.TempDir())
	if err := store.Init(); err != nil {
		t.Fatal(err)
	}
	sources := map[int]SourceChapter{
		1: NewSourceChapter(1, "第一章"),
		2: NewSourceChapter(2, "第二章"),
		3: NewSourceChapter(3, "第三章"),
	}
	job, err := store.QueueJob(Decision{Action: DecisionTranslate, Chapters: []int{1, 2, 3}, Reason: "toàn bộ backlog"}, sources)
	if err != nil {
		t.Fatal(err)
	}
	status, err := store.LoadStatus()
	if err != nil {
		t.Fatal(err)
	}
	for _, chapter := range job.Chapters {
		record := status.Chapters[chapter]
		if record.State != ChapterPending || record.JobID != job.ID || record.SourceSHA256 != sources[chapter].SHA256 {
			t.Fatalf("chapter %d was not queued as durable pending: %+v", chapter, record)
		}
	}
}

type retryPoolSource struct{ texts map[int]string }

func (s retryPoolSource) CompletedChapters() ([]int, error) { return []int{1, 2, 3}, nil }
func (s retryPoolSource) PendingRewrites() ([]int, error)   { return nil, nil }
func (s retryPoolSource) IsBookCompleted() (bool, error)    { return true, nil }
func (s retryPoolSource) LoadCommittedChapter(chapter int) (string, error) {
	return s.texts[chapter], nil
}

type retryPoolModel struct {
	active atomic.Int32
	max    atomic.Int32
}

func (m *retryPoolModel) Generate(_ context.Context, _ []agentcore.Message, _ []agentcore.ToolSpec, _ ...agentcore.CallOption) (*agentcore.LLMResponse, error) {
	active := m.active.Add(1)
	for {
		current := m.max.Load()
		if active <= current || m.max.CompareAndSwap(current, active) {
			break
		}
	}
	time.Sleep(20 * time.Millisecond)
	m.active.Add(-1)
	return &agentcore.LLMResponse{Message: agentcore.Message{
		Role:       agentcore.RoleAssistant,
		Content:    []agentcore.ContentBlock{agentcore.TextBlock(`{"text":"Bản dịch thử","glossary":[]}`)},
		StopReason: agentcore.StopReasonStop,
	}}, nil
}

func (m *retryPoolModel) GenerateStream(ctx context.Context, msgs []agentcore.Message, tools []agentcore.ToolSpec, options ...agentcore.CallOption) (<-chan agentcore.StreamEvent, error) {
	response, err := m.Generate(ctx, msgs, tools, options...)
	if err != nil {
		return nil, err
	}
	stream := make(chan agentcore.StreamEvent, 1)
	stream <- agentcore.StreamEvent{Type: agentcore.StreamEventDone, Message: response.Message, StopReason: response.Message.StopReason}
	close(stream)
	return stream, nil
}

func (m *retryPoolModel) SupportsTools() bool { return true }

func TestRetryRunsWithWorkerPoolAndPreservesSource(t *testing.T) {
	store := NewStore(t.TempDir())
	if err := store.Init(); err != nil {
		t.Fatal(err)
	}
	sourceTexts := map[int]string{1: "第一章：风起", 2: "第二章：雨落", 3: "第三章：云开"}
	sources := make(map[int]SourceChapter, len(sourceTexts))
	for chapter, text := range sourceTexts {
		sources[chapter] = NewSourceChapter(chapter, text)
	}
	initial, err := store.QueueJob(Decision{Action: DecisionTranslate, Chapters: []int{1, 2, 3}, Reason: "initial failure"}, sources)
	if err != nil {
		t.Fatal(err)
	}
	for chapter, source := range sources {
		if err := store.StartChapter(initial.ID, source); err != nil {
			t.Fatal(err)
		}
		if err := store.FailChapter(initial.ID, chapter, errors.New("temporary provider error")); err != nil {
			t.Fatal(err)
		}
	}
	initial.State = JobFailed
	if err := store.UpdateJob(initial); err != nil {
		t.Fatal(err)
	}
	if err := store.MarkStale(2, Digest("第二章：旧版本")); err != nil {
		t.Fatal(err)
	}
	beforeRetry, err := store.LoadStatus()
	if err != nil {
		t.Fatal(err)
	}
	if beforeRetry.Chapters[2].State != ChapterStale {
		t.Fatalf("chapter 2 state = %s, want stale before retry", beforeRetry.Chapters[2].State)
	}
	model := &retryPoolModel{}
	controller := &Controller{
		Store: store, Source: retryPoolSource{texts: sourceTexts}, TranslatorModel: model,
		Policy: Policy{Enabled: true, MaxBatchChapters: 3, MaxConcurrentBatches: 3, MaxRetries: 3},
	}
	if err := controller.Retry(context.Background(), []int{1, 2, 3}); err != nil {
		t.Fatal(err)
	}
	if model.max.Load() < 2 {
		t.Fatalf("retry worker pool did not run concurrently; max active = %d", model.max.Load())
	}
	status, err := store.LoadStatus()
	if err != nil {
		t.Fatal(err)
	}
	for chapter, original := range sourceTexts {
		record := status.Chapters[chapter]
		if record.State != ChapterCompleted || record.SourceSHA256 != Digest(original) {
			t.Fatalf("retry chapter %d = %+v, want completed and original fingerprint", chapter, record)
		}
		if got, err := controller.Source.LoadCommittedChapter(chapter); err != nil || got != original {
			t.Fatalf("retry changed Chinese source chapter %d: got=%q err=%v", chapter, got, err)
		}
	}
}

func TestSnapshotDigestDoesNotDependOnChapterBody(t *testing.T) {
	one := testSnapshot()
	two := testSnapshot()
	two.Completed[0].Text = "nội dung khác nhưng cùng fingerprint fact"
	if SnapshotDigest(one) != SnapshotDigest(two) {
		t.Fatal("audit digest must not include chapter body")
	}
}

func TestTranslationStoreMergeGlossaryLocksExistingTerms(t *testing.T) {
	store := NewStore(t.TempDir())
	if err := store.Init(); err != nil {
		t.Fatal(err)
	}
	first, err := store.MergeGlossary(1, []GlossaryCandidate{{Source: "师父", Vietnamese: "sư phụ"}})
	if err != nil {
		t.Fatal(err)
	}
	if first.Version != 1 || first.Terms["师父"].Vietnamese != "sư phụ" {
		t.Fatalf("unexpected first glossary: %+v", first)
	}
	second, err := store.MergeGlossary(4, []GlossaryCandidate{{Source: "师父", Vietnamese: "thầy"}})
	if err != nil {
		t.Fatal(err)
	}
	term := second.Terms["师父"]
	if term.Vietnamese != "sư phụ" || term.LastChapter != 4 {
		t.Fatalf("existing term must remain locked while last chapter updates: %+v", term)
	}
}

func TestDecisionValidateRespectsRetranslateAndBacklogPolicy(t *testing.T) {
	snapshot := Snapshot{
		Completed: []SourceChapter{
			NewSourceChapter(1, "一"), NewSourceChapter(2, "二"), NewSourceChapter(3, "三"),
		},
		Policy: Policy{Enabled: true, MaxBatchChapters: 2, MaxLagChapters: 2, AutoRetranslate: false},
	}
	if err := (Decision{Action: DecisionTranslate, Chapters: []int{2}, Reason: "bad backlog order"}).Validate(snapshot); err == nil {
		t.Fatal("backlog batch must start with oldest eligible chapter")
	}
	snapshot.Translated = []ChapterRecord{{Chapter: 1, State: ChapterStale, SourceSHA256: "old"}}
	if err := (Decision{Action: DecisionRetranslate, Chapters: []int{1}, Reason: "source changed"}).Validate(snapshot); err == nil {
		t.Fatal("retranslate must require explicit opt-in")
	}
}
