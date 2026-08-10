package imp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/voocel/ainovel-cli/internal/domain"
	"github.com/voocel/ainovel-cli/internal/store"
)

func mustLoadState(t *testing.T, w *Workspace) Facts {
	t.Helper()
	f, err := LoadState(w)
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	return f
}

func TestNextActionChain(t *testing.T) {
	cases := []struct {
		name string
		f    Facts
		want Action
	}{
		{"ç©º", Facts{}, ActionIngest},
		{"å·²å»ºåŒºå¾…åˆ‡åˆ†", Facts{WorkspaceReady: true}, ActionSegment},
		{"å·²åˆ‡åˆ†å¾…ç¡®è®¤", Facts{WorkspaceReady: true, Segmented: true}, ActionAwaitConfirmation},
		{"å·²ç¡®è®¤å¾…åˆ†æž", Facts{WorkspaceReady: true, Segmented: true, Confirmed: true, ExpectedChapters: 3}, ActionAnalyze},
		{"åˆ†æžæœªæ»¡", Facts{WorkspaceReady: true, Segmented: true, Confirmed: true, ExpectedChapters: 3, AnalyzedChapters: 2}, ActionAnalyze},
		{"åˆ†æžé½å¾…ç»¼åˆ", Facts{WorkspaceReady: true, Segmented: true, Confirmed: true, ExpectedChapters: 3, AnalyzedChapters: 3}, ActionSynthesize},
		{"ç»¼åˆåŽ uncertain å¾…è£å®š", Facts{WorkspaceReady: true, Segmented: true, Confirmed: true, ExpectedChapters: 3, AnalyzedChapters: 3, Synthesized: true, StoryUncertain: true}, ActionAwaitStoryResolution},
		{"uncertain å·²è£å®šå¾…å‘å¸ƒ", Facts{WorkspaceReady: true, Segmented: true, Confirmed: true, ExpectedChapters: 3, AnalyzedChapters: 3, Synthesized: true, StoryUncertain: true, StoryResolved: true}, ActionPublish},
		{"æ˜Žç¡®çŠ¶æ€å¾…å‘å¸ƒ", Facts{WorkspaceReady: true, Segmented: true, Confirmed: true, ExpectedChapters: 3, AnalyzedChapters: 3, Synthesized: true}, ActionPublish},
		{"å…¨éƒ¨ä¸€è‡´", Facts{WorkspaceReady: true, Segmented: true, Confirmed: true, ExpectedChapters: 3, AnalyzedChapters: 3, Synthesized: true, Published: true}, ActionDone},
		{"å‘å¸ƒç»ˆæ€çŸ­è·¯ä¸Šæ¸¸å¤±é²œ", Facts{Published: true}, ActionDone},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := NextAction(c.f)
			if got != c.want {
				t.Fatalf("NextAction=%s want=%s", got, c.want)
			}
			// å¯¹åŒä¸€äº‹å®žå¿«ç…§æ’å®šã€‚
			if NextAction(c.f) != got {
				t.Fatal("NextAction å¯¹åŒä¸€ Facts ä¸æ’å®š")
			}
		})
	}
}

func TestLoadStateReflectsWorkspace(t *testing.T) {
	book := t.TempDir()
	// æœªå»ºåŒºï¼šéžæ´»åŠ¨ â†’ ingestã€‚
	w := OpenWorkspace(book)
	if NextAction(mustLoadState(t, w)) != ActionIngest {
		t.Fatal("ç©ºä¹¦åº”å…ˆ ingest")
	}
	// å»ºåŒºåŽï¼šworkspace readyã€æœªåˆ‡åˆ† â†’ segmentã€‚
	src := filepath.Join(book, "book.txt")
	if err := os.WriteFile(src, []byte("ç¬¬ä¸€ç« \næ­£æ–‡\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ws, _, err := Ingest(book, src, Intent{})
	if err != nil {
		t.Fatalf("Ingest: %v", err)
	}
	f := mustLoadState(t, ws)
	if !f.WorkspaceReady || f.Segmented {
		t.Fatalf("å»ºåŒºåŽäº‹å®žä¸ç¬¦ï¼š%+v", f)
	}
	if NextAction(f) != ActionSegment {
		t.Fatal("å»ºåŒºåŽåº” segment")
	}
}

func TestLoadStateReportsCorruptArtifact(t *testing.T) {
	book := t.TempDir()
	src := filepath.Join(book, "book.txt")
	if err := os.WriteFile(src, []byte("ç¬¬ä¸€ç« \næ­£æ–‡\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ws, _, err := Ingest(book, src, Intent{})
	if err != nil {
		t.Fatal(err)
	}
	if err := ws.writeAtomic(fileSegmentation, []byte("{")); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadState(ws); err == nil || !strings.Contains(err.Error(), "unexpected end of JSON input") {
		t.Fatalf("æŸåå·¥ä»¶ä¸å¾—ä¼ªè£…æˆå°šæœªåˆ‡åˆ†: %v", err)
	}
}

func TestIngestSnapshotConsistent(t *testing.T) {
	book := t.TempDir()
	src := filepath.Join(book, "book.txt")
	content := "ç¬¬ä¸€ç« \r\næ­£æ–‡ä¸€\r\n\r\nç¬¬äºŒç« \r\næ­£æ–‡äºŒ"
	if err := os.WriteFile(src, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	ws, m, err := Ingest(book, src, Intent{})
	if err != nil {
		t.Fatalf("Ingest: %v", err)
	}
	if m.Encoding != encodingUTF8 || m.SourceName != "book.txt" {
		t.Fatalf("manifest ä¸ç¬¦ï¼š%+v", m)
	}
	snap, err := ws.LoadSource()
	if err != nil {
		t.Fatal(err)
	}
	// æºå¿«ç…§å¿…é¡»å·²å½’ä¸€åŒ–ï¼Œä¸”æ‘˜è¦ä¸Ž manifest ä¸€è‡´ã€‚
	if string(snap) != "ç¬¬ä¸€ç« \næ­£æ–‡ä¸€\n\nç¬¬äºŒç« \næ­£æ–‡äºŒ" {
		t.Fatalf("æºå¿«ç…§æœªå½’ä¸€åŒ–ï¼š%q", snap)
	}
	if Digest(snap) != m.NormalizedSHA256 {
		t.Fatal("æºå¿«ç…§æ‘˜è¦ä¸Ž manifest ä¸ä¸€è‡´")
	}
}

// TestGuidanceChangeInvalidatesSegmentation å®ˆæŠ¤ Â§18.3ï¼šåˆ‡åˆ†æŒ‡å¯¼æ˜¯ segmentation çš„è¯­ä¹‰è¾“å…¥ï¼Œ
// æŒ‡å¯¼å˜åŒ–ä½¿æ—§åˆ‡åˆ†ï¼ˆåŠå…¶å…¨éƒ¨ä¸‹æ¸¸ï¼‰è‡ªç„¶å¤±é…é‡åšï¼Œä¸éœ€è¦æ‰‹å·¥å¤±æ•ˆè§„åˆ™ã€‚
func TestGuidanceChangeInvalidatesSegmentation(t *testing.T) {
	book := t.TempDir()
	src := filepath.Join(book, "book.txt")
	if err := os.WriteFile(src, []byte("ç¬¬ä¸€ç« \næ­£æ–‡\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ws, _, err := Ingest(book, src, Intent{})
	if err != nil {
		t.Fatalf("Ingest: %v", err)
	}
	norm, err := ws.LoadSource()
	if err != nil {
		t.Fatal(err)
	}
	seg := Segmentation{Chapters: []ChapterSpan{{Number: 1, Title: "ç¬¬ä¸€ç« ", Start: 0, End: len(norm)}}}
	if err := writeArtifact(ws, fileSegmentation, segmentInputDigest(Digest(norm), "", segmentPromptVersion), seg); err != nil {
		t.Fatal(err)
	}
	if !mustLoadState(t, ws).Segmented {
		t.Fatal("æ— æŒ‡å¯¼æ—¶åˆ‡åˆ†åº”æœ‰æ•ˆ")
	}
	if err := ws.writeAtomic(fileGuidance, []byte("å¹•é—´ä¹Ÿæ˜¯ç‹¬ç«‹ç« èŠ‚")); err != nil {
		t.Fatal(err)
	}
	if mustLoadState(t, ws).Segmented {
		t.Fatal("æŒ‡å¯¼å˜åŒ–åŽæ—§åˆ‡åˆ†åº”å¤±æ•ˆï¼ˆéœ€é‡è¯†åˆ«ï¼‰")
	}
}

// TestResumeSummary å®ˆæŠ¤ Â§18.2 å¯åŠ¨æç¤ºï¼šæ— å·¥ä½œåŒºè¿”å›žç©ºä¸²ï¼›åœåœ¨åŠè·¯æ—¶ç»™å‡ºé˜¶æ®µåŒ–æè¿°ï¼Œ
// ä½¿ç”¨æˆ·ä¸å¿…ç­‰åˆ°åˆ›ä½œè¢«é—¨ç¦æ‹’ç»æ‰å‘çŽ°è¿™æœ¬ä¹¦åœåœ¨å¯¼å…¥åŠè·¯ã€‚
func TestResumeSummary(t *testing.T) {
	dir := t.TempDir()
	st := store.NewStore(dir)
	if err := st.Init(); err != nil {
		t.Fatal(err)
	}
	if got := ResumeSummary(st); got != "" {
		t.Fatalf("æ— å¯¼å…¥å·¥ä½œåŒºåº”è¿”å›žç©ºä¸²ï¼Œå¾— %q", got)
	}
	src := filepath.Join(dir, "book.txt")
	if err := os.WriteFile(src, []byte("ç¬¬ä¸€ç« \næ­£æ–‡\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ws, _, err := Ingest(dir, src, Intent{})
	if err != nil {
		t.Fatalf("Ingest: %v", err)
	}
	if got := ResumeSummary(st); !strings.Contains(got, "发现未完成的导入") && !strings.Contains(got, "phát hiện một bản nhập chưa hoàn thành") {
		t.Fatalf("åˆšå»ºåŒºåº”æç¤ºæœªå®Œæˆåˆ‡åˆ†ï¼Œå¾— %q", got)
	}
	// åˆ‡åˆ†+ç¡®è®¤å°±ç»ªã€åˆ†æž 0/1 â†’ æç¤ºåˆ†æžè¿›åº¦ã€‚
	norm, _ := ws.LoadSource()
	seg := Segmentation{Chapters: []ChapterSpan{{Number: 1, Title: "ç¬¬ä¸€ç« ", Start: 0, End: len(norm)}}}
	if err := writeArtifact(ws, fileSegmentation, segmentInputDigest(Digest(norm), "", segmentPromptVersion), seg); err != nil {
		t.Fatal(err)
	}
	raw, _ := ws.readBytes(fileSegmentation)
	if err := writeArtifact(ws, fileConfirmation, Digest(raw), Confirmation{Method: confirmMethodAuto, Chapters: 1}); err != nil {
		t.Fatal(err)
	}
	if got := ResumeSummary(st); !strings.Contains(got, "已分析") && !strings.Contains(got, "đã phân tích") {
		t.Fatalf("åº”æç¤ºåˆ†æžè¿›åº¦ï¼Œå¾— %q", got)
	}
}

// TestResumeStatusPublishedIsTerminal å®ˆæŠ¤å‘å¸ƒç»ˆæ€ï¼ˆå®žæµ‹äº‹æ•…ï¼‰ï¼šä¹¦å·²å…¨é‡å‘å¸ƒåŽï¼Œ
// segmentPromptVersion å‡çº§ä½¿å·¥ä½œåŒºåˆ‡åˆ†å·¥ä»¶å¤±é²œï¼ŒResumeStatus ä¸å¾—æ®æ­¤æŠŠä¹¦åˆ¤å›ž
// "å¯¼å…¥åŠè·¯"â€”â€”å¦åˆ™ startEngine è·¨é‡å¯é—¨ç¦ä¼šæ°¸ä¹…æ‹’å¯å·²å‘å¸ƒä¹¦çš„ç»­å†™ã€‚
func TestResumeStatusPublishedIsTerminal(t *testing.T) {
	dir := t.TempDir()
	st := store.NewStore(dir)
	if err := st.Init(); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(dir, "book.txt")
	if err := os.WriteFile(src, []byte("ç¬¬ä¸€ç« \næ­£æ–‡\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ws, _, err := Ingest(dir, src, Intent{})
	if err != nil {
		t.Fatalf("Ingest: %v", err)
	}
	norm, _ := ws.LoadSource()
	// ç”¨æ—§ç‰ˆæœ¬å·å†™åˆ‡åˆ†ï¼šæ¨¡æ‹Ÿå‘å¸ƒåŽ prompt å‡çº§å¯¼è‡´çš„ digest å¤±é…ã€‚
	seg := Segmentation{Chapters: []ChapterSpan{{Number: 1, Title: "ç¬¬ä¸€ç« ", Start: 0, End: len(norm)}}}
	if err := writeArtifact(ws, fileSegmentation, segmentInputDigest(Digest(norm), "", "seg-v0"), seg); err != nil {
		t.Fatal(err)
	}
	// æœªå‘å¸ƒ + åˆ‡åˆ†å¤±é²œï¼šä»æ˜¯åŠè·¯å¯¼å…¥ï¼Œé—¨ç¦åº”æ‹¦ã€‚
	if active, done, err := ResumeStatus(st); err != nil || !active || done {
		t.Fatalf("æœªå‘å¸ƒçš„å¤±é²œå·¥ä½œåŒºåº”åˆ¤æœªå®Œæˆï¼ˆactive=%v done=%vï¼‰", active, done)
	}
	// æ­£å¼åº“å·²æŒ‰è¯¥åˆ‡åˆ†å…¨é‡è½åº“ â†’ å‘å¸ƒå¯¹è´¦é€šè¿‡ï¼Œç»ˆæ€ä¸å—ä¸Šæ¸¸å¤±é²œå½±å“ã€‚
	if err := st.Outline.SavePremise("å‰æ"); err != nil {
		t.Fatal(err)
	}
	if err := st.Outline.SaveOutline([]domain.OutlineEntry{{Chapter: 1, Title: "ç¬¬ä¸€ç« "}}); err != nil {
		t.Fatal(err)
	}
	if err := st.Progress.Save(&domain.Progress{NovelName: "ä¹¦", CompletedChapters: []int{1}}); err != nil {
		t.Fatal(err)
	}
	if active, done, err := ResumeStatus(st); err != nil || !active || !done {
		t.Fatalf("å·²å‘å¸ƒä¹¦åº”åˆ¤å¯¼å…¥å®Œæˆï¼ˆactive=%v done=%vï¼‰", active, done)
	}
	if got := ResumeSummary(st); got != "" {
		t.Fatalf("å·²å‘å¸ƒä¹¦ä¸åº”æç¤ºæœªå®Œæˆå¯¼å…¥ï¼Œå¾— %q", got)
	}
}

func TestImportPreconditions(t *testing.T) {
	// ç©ºä¹¦é€šè¿‡ã€‚
	empty := store.NewStore(t.TempDir())
	if err := checkImportPreconditions(empty); err != nil {
		t.Fatalf("ç©ºä¹¦åº”é€šè¿‡å‰ç½®æ ¡éªŒï¼š%v", err)
	}
	// æœ‰å®Œæˆç« èŠ‚è¢«æ‹’ã€‚
	nonEmpty := store.NewStore(t.TempDir())
	if err := nonEmpty.Progress.Save(&domain.Progress{CompletedChapters: []int{1, 2}}); err != nil {
		t.Fatal(err)
	}
	if err := checkImportPreconditions(nonEmpty); err == nil {
		t.Fatal("éžç©ºä¹¦åº”è¢«æ‹’ç»å¯¼å…¥")
	}
}

