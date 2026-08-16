package translation

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	statusPath   = "status.json"
	glossaryPath = "glossary.json"
)

// Store owns only translations/vi. It has no reference to the creative Store,
// making it impossible for the translation branch to overwrite Chinese source
// artifacts through this API.
type Store struct {
	root string
	mu   sync.Mutex
}

// NewStore creates an isolated store rooted at <book>/translations/vi.
func NewStore(bookDir string) *Store {
	return &Store{root: filepath.Join(bookDir, "translations", "vi")}
}

func (s *Store) Root() string { return s.root }

// Init creates the isolated artifact layout. It is safe to call on resume.
func (s *Store) Init() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, path := range []string{
		s.root,
		filepath.Join(s.root, "chapters"),
		filepath.Join(s.root, "jobs"),
		filepath.Join(s.root, "audit"),
		filepath.Join(s.root, "exports"),
	} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			return err
		}
	}
	if _, err := os.Stat(filepath.Join(s.root, statusPath)); errors.Is(err, os.ErrNotExist) {
		if err := s.writeJSONUnlocked(statusPath, NewStatus()); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(s.root, glossaryPath)); errors.Is(err, os.ErrNotExist) {
		if err := s.writeJSONUnlocked(glossaryPath, Glossary{Terms: map[string]GlossaryEntry{}}); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	return nil
}

func (s *Store) LoadStatus() (Status, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadStatusUnlocked()
}

func (s *Store) loadStatusUnlocked() (Status, error) {
	var status Status
	if err := s.readJSONUnlocked(statusPath, &status); err != nil {
		return Status{}, err
	}
	if status.SchemaVersion == 0 {
		status.SchemaVersion = schemaVersion
	}
	if status.SchemaVersion != schemaVersion {
		return Status{}, fmt.Errorf("unsupported translation status schema %d", status.SchemaVersion)
	}
	if status.Chapters == nil {
		status.Chapters = make(map[int]ChapterRecord)
	}
	if status.Jobs == nil {
		status.Jobs = make(map[string]Job)
	}
	return status, nil
}

func (s *Store) SaveStatus(status Status) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	status.SchemaVersion = schemaVersion
	status.UpdatedAt = time.Now().UTC()
	return s.writeJSONUnlocked(statusPath, status)
}

// SetChapterInstruction persists a user intervention for the next processing
// attempt. This metadata never changes the Chinese source artifact.
func (s *Store) SetChapterInstruction(chapter int, instruction string) (ChapterRecord, error) {
	if chapter <= 0 {
		return ChapterRecord{}, fmt.Errorf("chapter must be positive")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	status, err := s.loadStatusUnlocked()
	if err != nil {
		return ChapterRecord{}, err
	}
	record, ok := status.Chapters[chapter]
	if !ok {
		return ChapterRecord{}, fmt.Errorf("chapter %d is not in translation queue", chapter)
	}
	record.Instruction = strings.TrimSpace(instruction)
	record.UpdatedAt = time.Now().UTC()
	status.Chapters[chapter] = record
	status.UpdatedAt = record.UpdatedAt
	if err := s.writeJSONUnlocked(statusPath, status); err != nil {
		return ChapterRecord{}, err
	}
	return record, nil
}

func (s *Store) LoadGlossary() (Glossary, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var glossary Glossary
	if err := s.readJSONUnlocked(glossaryPath, &glossary); err != nil {
		return Glossary{}, err
	}
	if glossary.Terms == nil {
		glossary.Terms = make(map[string]GlossaryEntry)
	}
	return glossary, nil
}

func (s *Store) SaveGlossary(glossary Glossary) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if glossary.Terms == nil {
		glossary.Terms = make(map[string]GlossaryEntry)
	}
	return s.writeJSONUnlocked(glossaryPath, glossary)
}

// MergeGlossary locks only new source terms. Once a term has been committed,
// later Translator output may update its LastChapter but cannot alter the
// selected Vietnamese expression without an explicit human glossary workflow.
func (s *Store) MergeGlossary(chapter int, candidates []GlossaryCandidate) (Glossary, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var glossary Glossary
	if err := s.readJSONUnlocked(glossaryPath, &glossary); err != nil {
		return Glossary{}, err
	}
	if glossary.Terms == nil {
		glossary.Terms = make(map[string]GlossaryEntry)
	}
	changed := false
	now := time.Now().UTC()
	for _, candidate := range candidates {
		source := strings.TrimSpace(candidate.Source)
		vietnamese := strings.TrimSpace(candidate.Vietnamese)
		if source == "" || vietnamese == "" {
			return Glossary{}, fmt.Errorf("glossary terms must be non-empty")
		}
		if existing, ok := glossary.Terms[source]; ok {
			if chapter > existing.LastChapter {
				existing.LastChapter = chapter
				existing.UpdatedAt = now
				glossary.Terms[source] = existing
				changed = true
			}
			continue
		}
		glossary.Terms[source] = GlossaryEntry{
			Source: source, Vietnamese: vietnamese, FirstChapter: chapter, LastChapter: chapter, UpdatedAt: now,
		}
		changed = true
	}
	if changed {
		glossary.Version++
		if err := s.writeJSONUnlocked(glossaryPath, glossary); err != nil {
			return Glossary{}, err
		}
	}
	return glossary, nil
}

// CommitChapter atomically writes the Vietnamese text and updates its status
// metadata. The caller must re-check the source fingerprint before invoking it.
// title may be empty; when empty, a heading is extracted from the body if present.
func (s *Store) CommitChapter(source SourceChapter, text, provider, model, jobID string, glossaryVersion int, title string) (ChapterRecord, error) {
	if source.Number <= 0 || source.SHA256 == "" {
		return ChapterRecord{}, fmt.Errorf("invalid source chapter")
	}
	if text == "" {
		return ChapterRecord{}, fmt.Errorf("translated text is empty")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	status, err := s.loadStatusUnlocked()
	if err != nil {
		return ChapterRecord{}, err
	}
	title = strings.TrimSpace(title)
	if title == "" {
		title = ExtractTitleFromBody(text)
	}
	// Preserve previous title if new commit did not yield one.
	if title == "" {
		if prev, ok := status.Chapters[source.Number]; ok {
			title = strings.TrimSpace(prev.Title)
		}
	}
	record := ChapterRecord{
		Chapter:          source.Number,
		State:            ChapterCompleted,
		SourceSHA256:     source.SHA256,
		TranslatedSHA256: Digest(text),
		Title:            title,
		JobID:            jobID,
		GlossaryVersion:  glossaryVersion,
		Provider:         provider,
		Model:            model,
		UpdatedAt:        time.Now().UTC(),
	}
	if err := s.writeTextUnlocked(filepath.Join("chapters", chapterFileName(source.Number)), text); err != nil {
		return ChapterRecord{}, err
	}
	status.Chapters[source.Number] = record
	status.SchemaVersion = schemaVersion
	status.UpdatedAt = record.UpdatedAt
	if err := s.writeJSONUnlocked(statusPath, status); err != nil {
		return ChapterRecord{}, err
	}
	return record, nil
}

// SetChapterTitles merges Vietnamese titles into completed chapter records.
// Empty values are ignored. Returns how many records were updated.
func (s *Store) SetChapterTitles(titles map[int]string) (int, error) {
	if s == nil || len(titles) == 0 {
		return 0, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	status, err := s.loadStatusUnlocked()
	if err != nil {
		return 0, err
	}
	n := 0
	now := time.Now().UTC()
	for ch, title := range titles {
		title = strings.TrimSpace(title)
		if ch <= 0 || title == "" {
			continue
		}
		rec, ok := status.Chapters[ch]
		if !ok {
			continue
		}
		if strings.TrimSpace(rec.Title) == title {
			continue
		}
		rec.Title = title
		rec.UpdatedAt = now
		status.Chapters[ch] = rec
		n++
	}
	if n == 0 {
		return 0, nil
	}
	status.SchemaVersion = schemaVersion
	status.UpdatedAt = now
	if err := s.writeJSONUnlocked(statusPath, status); err != nil {
		return 0, err
	}
	return n, nil
}

func (s *Store) LoadChapter(chapter int) (string, ChapterRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	status, err := s.loadStatusUnlocked()
	if err != nil {
		return "", ChapterRecord{}, err
	}
	record, ok := status.Chapters[chapter]
	if !ok {
		return "", ChapterRecord{}, os.ErrNotExist
	}
	text, err := os.ReadFile(filepath.Join(s.root, "chapters", chapterFileName(chapter)))
	if err != nil {
		return "", ChapterRecord{}, err
	}
	return string(text), record, nil
}

// QueueJob persists a valid batch before any agent call starts. It refuses a
// second in-flight job, which protects the per-book glossary from concurrent edits.
func (s *Store) QueueJob(decision Decision, sources map[int]SourceChapter) (Job, error) {
	if decision.Action != DecisionTranslate && decision.Action != DecisionRetranslate {
		return Job{}, fmt.Errorf("only translation decisions can be queued")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	status, err := s.loadStatusUnlocked()
	if err != nil {
		return Job{}, err
	}
	for _, existing := range status.Jobs {
		if existing.State == JobQueued || existing.State == JobRunning {
			return Job{}, fmt.Errorf("translation job %s is already %s", existing.ID, existing.State)
		}
	}
	id, err := randomID()
	if err != nil {
		return Job{}, err
	}
	now := time.Now().UTC()
	job := Job{ID: id, State: JobQueued, Chapters: append([]int(nil), decision.Chapters...), Reason: decision.Reason, CreatedAt: now, UpdatedAt: now}
	status.Jobs[id] = job
	for _, chapter := range job.Chapters {
		record := status.Chapters[chapter]
		record.Chapter = chapter
		record.State = ChapterPending
		record.SourceSHA256 = sources[chapter].SHA256
		record.JobID = id
		record.LastError = ""
		record.UpdatedAt = now
		status.Chapters[chapter] = record
	}
	status.UpdatedAt = now
	if err := s.writeJSONUnlocked(statusPath, status); err != nil {
		return Job{}, err
	}
	return job, nil
}

// StartChapter marks one source as in-flight before the LLM call. This makes a
// crash resumable without claiming that an artifact was already committed.
func (s *Store) StartChapter(jobID string, source SourceChapter) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	status, err := s.loadStatusUnlocked()
	if err != nil {
		return err
	}
	record := status.Chapters[source.Number]
	if record.JobID != jobID {
		return fmt.Errorf("chapter %d is not assigned to job %s", source.Number, jobID)
	}
	record.State = ChapterRunning
	record.SourceSHA256 = source.SHA256
	record.Attempts++
	record.LastError = ""
	record.UpdatedAt = time.Now().UTC()
	status.Chapters[source.Number] = record
	status.UpdatedAt = record.UpdatedAt
	return s.writeJSONUnlocked(statusPath, status)
}

// FailChapter retains a failed source checkpoint for retry/resume.
func (s *Store) FailChapter(jobID string, chapter int, err error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	status, loadErr := s.loadStatusUnlocked()
	if loadErr != nil {
		return loadErr
	}
	record := status.Chapters[chapter]
	if record.JobID != jobID {
		return fmt.Errorf("chapter %d is not assigned to job %s", chapter, jobID)
	}
	record.State = ChapterFailed
	if err != nil {
		record.LastError = err.Error()
	}
	record.UpdatedAt = time.Now().UTC()
	status.Chapters[chapter] = record
	status.UpdatedAt = record.UpdatedAt
	return s.writeJSONUnlocked(statusPath, status)
}

func (s *Store) UpdateJob(job Job) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	status, err := s.loadStatusUnlocked()
	if err != nil {
		return err
	}
	if _, ok := status.Jobs[job.ID]; !ok {
		return os.ErrNotExist
	}
	job.UpdatedAt = time.Now().UTC()
	status.Jobs[job.ID] = job
	status.UpdatedAt = job.UpdatedAt
	return s.writeJSONUnlocked(statusPath, status)
}

// RecoverInterruptedJobs converts only orphaned in-flight state left by a
// process restart into retryable failures. It never changes Chinese source or
// completed Vietnamese artifacts. A subsequent queue pass creates a fresh job
// and preserves the original source fingerprint for every recovered chapter.
func (s *Store) RecoverInterruptedJobs() (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	status, err := s.loadStatusUnlocked()
	if err != nil {
		return 0, err
	}
	now := time.Now().UTC()
	const reason = "translation engine restarted while this chapter was running; safe retry required"
	recovered := 0
	for id, job := range status.Jobs {
		if job.State != JobQueued && job.State != JobRunning {
			continue
		}
		job.State = JobFailed
		job.LastError = reason
		job.UpdatedAt = now
		status.Jobs[id] = job
		recovered++
	}
	for chapter, record := range status.Chapters {
		if record.State != ChapterRunning {
			continue
		}
		record.State = ChapterFailed
		record.LastError = reason
		record.UpdatedAt = now
		status.Chapters[chapter] = record
		recovered++
	}
	if recovered == 0 {
		return 0, nil
	}
	status.UpdatedAt = now
	if err := s.writeJSONUnlocked(statusPath, status); err != nil {
		return 0, err
	}
	return recovered, nil
}

// MarkStale marks a committed translation invalid whenever the corresponding
// Chinese source fingerprint changes. It never deletes the historical artifact.
func (s *Store) MarkStale(chapter int, currentSourceSHA string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	status, err := s.loadStatusUnlocked()
	if err != nil {
		return err
	}
	record, ok := status.Chapters[chapter]
	if !ok {
		return nil
	}
	if record.SourceSHA256 == currentSourceSHA {
		return nil
	}
	record.State = ChapterStale
	record.SourceSHA256 = currentSourceSHA
	record.LastError = "source_changed"
	record.UpdatedAt = time.Now().UTC()
	status.Chapters[chapter] = record
	status.UpdatedAt = record.UpdatedAt
	return s.writeJSONUnlocked(statusPath, status)
}

func (s *Store) AppendDecision(record any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	path := filepath.Join(s.root, "audit", "translation-decisions.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()
	if _, err := file.Write(append(data, '\n')); err != nil {
		return err
	}
	return file.Sync()
}

func (s *Store) ExportPath(name string) string {
	return filepath.Join(s.root, "exports", name)
}

func (s *Store) writeJSONUnlocked(rel string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return s.writeBytesUnlocked(rel, append(data, '\n'))
}

func (s *Store) readJSONUnlocked(rel string, value any) error {
	data, err := os.ReadFile(filepath.Join(s.root, rel))
	if err != nil {
		return err
	}
	return json.Unmarshal(data, value)
}

func (s *Store) writeTextUnlocked(rel, text string) error {
	return s.writeBytesUnlocked(rel, []byte(text))
}

func (s *Store) writeBytesUnlocked(rel string, data []byte) error {
	path := filepath.Join(s.root, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

func chapterFileName(chapter int) string { return fmt.Sprintf("%02d.md", chapter) }

func randomID() (string, error) {
	var bytes [8]byte
	if _, err := io.ReadFull(rand.Reader, bytes[:]); err != nil {
		return "", err
	}
	return "tr-" + hex.EncodeToString(bytes[:]), nil
}

// SortedRecords returns a stable copy suited for a Coordinator snapshot.
func SortedRecords(status Status) []ChapterRecord {
	records := make([]ChapterRecord, 0, len(status.Chapters))
	for _, record := range status.Chapters {
		records = append(records, record)
	}
	sort.Slice(records, func(i, j int) bool { return records[i].Chapter < records[j].Chapter })
	return records
}
