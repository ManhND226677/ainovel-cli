// Package translation implements the isolated Chinese-to-Vietnamese translation
// branch. It never writes the creative source of truth; all artifacts live under
// translations/vi in the book directory.
package translation

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"
)

const schemaVersion = 1

// ChapterState is the persisted state of a translated source chapter.
type ChapterState string

const (
	ChapterPending   ChapterState = "pending"
	ChapterRunning   ChapterState = "running"
	ChapterCompleted ChapterState = "completed"
	ChapterFailed    ChapterState = "failed"
	ChapterStale     ChapterState = "stale"
)

// JobState records a durable translation batch lifecycle.
type JobState string

const (
	JobQueued    JobState = "queued"
	JobRunning   JobState = "running"
	JobCompleted JobState = "completed"
	JobFailed    JobState = "failed"
	JobCancelled JobState = "cancelled"
)

// SourceChapter is an immutable snapshot of a committed Chinese chapter.
type SourceChapter struct {
	Number int    `json:"number"`
	Text   string `json:"-"`
	SHA256 string `json:"sha256"`
}

// NewSourceChapter builds a source snapshot and its content fingerprint.
func NewSourceChapter(number int, text string) SourceChapter {
	return SourceChapter{Number: number, Text: text, SHA256: Digest(text)}
}

// Digest returns a deterministic content fingerprint used to prevent stale commits.
func Digest(text string) string {
	sum := sha256.Sum256([]byte(text))
	return "sha256:" + hex.EncodeToString(sum[:])
}

// ChapterRecord is metadata for one Vietnamese artifact. The translated text is
// stored separately in translations/vi/chapters/{chapter}.md.
type ChapterRecord struct {
	Chapter          int          `json:"chapter"`
	State            ChapterState `json:"state"`
	SourceSHA256     string       `json:"source_sha256"`
	TranslatedSHA256 string       `json:"translated_sha256,omitempty"`
	// Title is the Vietnamese chapter title used in TOC/export. Optional for
	// legacy artifacts; export/backfill fills it so every completed chapter has one.
	Title           string    `json:"title,omitempty"`
	JobID           string    `json:"job_id,omitempty"`
	GlossaryVersion int       `json:"glossary_version,omitempty"`
	Provider        string    `json:"provider,omitempty"`
	Model           string    `json:"model,omitempty"`
	Attempts        int       `json:"attempts,omitempty"`
	Instruction     string    `json:"instruction,omitempty"`
	LastError       string    `json:"last_error,omitempty"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// GlossaryEntry locks a translation choice once committed. Source references
// retain auditability without exposing any creative control to the translator.
type GlossaryEntry struct {
	Source       string    `json:"source"`
	Vietnamese   string    `json:"vietnamese"`
	FirstChapter int       `json:"first_chapter"`
	LastChapter  int       `json:"last_chapter"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// Glossary is versioned so every chapter artifact can identify the terminology
// snapshot used during translation.
type Glossary struct {
	Version int                      `json:"version"`
	Terms   map[string]GlossaryEntry `json:"terms"`
}

// Job is a single ordered batch. A book intentionally has at most one running
// job to avoid concurrent glossary edits and inconsistent forms of address.
type Job struct {
	ID        string    `json:"id"`
	State     JobState  `json:"state"`
	Chapters  []int     `json:"chapters"`
	Reason    string    `json:"reason,omitempty"`
	Attempts  int       `json:"attempts,omitempty"`
	LastError string    `json:"last_error,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Status is the durable control-plane state for a book's Vietnamese artifacts.
type Status struct {
	SchemaVersion int                   `json:"schema_version"`
	Chapters      map[int]ChapterRecord `json:"chapters"`
	Jobs          map[string]Job        `json:"jobs"`
	UpdatedAt     time.Time             `json:"updated_at"`
}

func NewStatus() Status {
	return Status{
		SchemaVersion: schemaVersion,
		Chapters:      make(map[int]ChapterRecord),
		Jobs:          make(map[string]Job),
	}
}

// Policy is the deterministic boundary around a Coordinator decision.
type Policy struct {
	Enabled              bool
	MinStableChapters    int
	MaxBatchChapters     int
	MaxLagChapters       int
	MaxConcurrentBatches int
	MaxRetries           int
	Debounce             time.Duration
	AutoRetranslate      bool
}

func (p Policy) Normalize() Policy {
	if p.MinStableChapters <= 0 {
		p.MinStableChapters = 1
	}
	if p.MaxBatchChapters <= 0 {
		p.MaxBatchChapters = 8
	}
	if p.MaxLagChapters <= 0 {
		p.MaxLagChapters = 24
	}
	if p.MaxConcurrentBatches <= 0 {
		p.MaxConcurrentBatches = 1
	}
	if p.MaxRetries <= 0 {
		p.MaxRetries = 3
	}
	if p.Debounce <= 0 {
		p.Debounce = 15 * time.Second
	}
	return p
}

// Snapshot contains only durable facts the Translation Coordinator may reason
// about. It deliberately excludes mutable draft text and raw writer messages.
type Snapshot struct {
	Completed       []SourceChapter `json:"completed"`
	PendingRewrites []int           `json:"pending_rewrites"`
	Translated      []ChapterRecord `json:"translated"`
	RunningJobs     []Job           `json:"running_jobs"`
	GlossaryVersion int             `json:"glossary_version"`
	BookCompleted   bool            `json:"book_completed"`
	Trigger         string          `json:"trigger"`
	Policy          Policy          `json:"policy"`
	CollectedAt     time.Time       `json:"collected_at"`
}

// DecisionKind is the finite structured output surface available to the
// Translation Coordinator Agent.
type DecisionKind string

const (
	DecisionWait        DecisionKind = "wait"
	DecisionTranslate   DecisionKind = "translate_batch"
	DecisionResume      DecisionKind = "resume_batch"
	DecisionRetranslate DecisionKind = "retranslate_batch"
	DecisionFinalize    DecisionKind = "finalize_translation"
)

// Decision is returned by the coordinator and then mechanically validated by
// the Host/controller before any work is scheduled.
type Decision struct {
	Action   DecisionKind `json:"action"`
	Chapters []int        `json:"chapters,omitempty"`
	JobID    string       `json:"job_id,omitempty"`
	Reason   string       `json:"reason"`
}

// Validate rejects an untrusted coordinator decision against the snapshot.
func (d Decision) Validate(snapshot Snapshot) error {
	if strings.TrimSpace(d.Reason) == "" {
		return fmt.Errorf("translation decision requires a reason")
	}
	policy := snapshot.Policy.Normalize()
	switch d.Action {
	case DecisionWait:
		if len(d.Chapters) != 0 || d.JobID != "" {
			return fmt.Errorf("wait decision must not include a batch or job id")
		}
		return nil
	case DecisionFinalize:
		if !snapshot.BookCompleted {
			return fmt.Errorf("finalize_translation requires a completed book")
		}
		if len(d.Chapters) != 0 || d.JobID != "" {
			return fmt.Errorf("finalize_translation must not include a batch or job id")
		}
		return nil
	case DecisionResume:
		if d.JobID == "" {
			return fmt.Errorf("resume_batch requires job_id")
		}
		return nil
	case DecisionTranslate, DecisionRetranslate:
		if d.Action == DecisionRetranslate && !policy.AutoRetranslate {
			return fmt.Errorf("retranslate_batch requires translation.auto_retranslate_on_rewrite")
		}
		if len(d.Chapters) == 0 {
			return fmt.Errorf("%s requires at least one chapter", d.Action)
		}
		if len(d.Chapters) > policy.MaxBatchChapters {
			return fmt.Errorf("batch size %d exceeds max_batch_chapters %d", len(d.Chapters), policy.MaxBatchChapters)
		}
		if d.Action == DecisionTranslate && !snapshot.BookCompleted && len(d.Chapters) < policy.MinStableChapters {
			return fmt.Errorf("batch size %d is below min_stable_chapters %d", len(d.Chapters), policy.MinStableChapters)
		}
		if !sort.IntsAreSorted(d.Chapters) || slices.ContainsFunc(d.Chapters, func(ch int) bool { return ch <= 0 }) {
			return fmt.Errorf("batch chapters must be unique positive ascending values")
		}
		for i := 1; i < len(d.Chapters); i++ {
			if d.Chapters[i] == d.Chapters[i-1] {
				return fmt.Errorf("batch chapters must not contain duplicates")
			}
		}
		eligible := snapshot.eligible(d.Action == DecisionRetranslate)
		if d.Action == DecisionTranslate && len(eligible) >= policy.MaxLagChapters {
			oldest := 0
			for chapter := range eligible {
				if oldest == 0 || chapter < oldest {
					oldest = chapter
				}
			}
			if d.Chapters[0] != oldest {
				return fmt.Errorf("translation backlog requires the oldest eligible chapter %d first", oldest)
			}
		}
		for _, chapter := range d.Chapters {
			if !eligible[chapter] {
				return fmt.Errorf("chapter %d is not eligible for %s", chapter, d.Action)
			}
		}
		return nil
	default:
		return fmt.Errorf("unknown translation action %q", d.Action)
	}
}

func (s Snapshot) eligible(retranslate bool) map[int]bool {
	blocked := make(map[int]bool, len(s.PendingRewrites))
	for _, chapter := range s.PendingRewrites {
		blocked[chapter] = true
	}
	records := make(map[int]ChapterRecord, len(s.Translated))
	for _, record := range s.Translated {
		records[record.Chapter] = record
	}
	eligible := make(map[int]bool, len(s.Completed))
	for _, chapter := range s.Completed {
		if chapter.Number <= 0 || strings.TrimSpace(chapter.SHA256) == "" || blocked[chapter.Number] {
			continue
		}
		record, translated := records[chapter.Number]
		if !translated {
			eligible[chapter.Number] = true
			continue
		}
		if retranslate {
			eligible[chapter.Number] = record.State == ChapterStale || record.SourceSHA256 != chapter.SHA256
		}
	}
	return eligible
}
