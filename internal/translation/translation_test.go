package translation

import (
	"os"
	"path/filepath"
	"testing"
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
