package imp

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/voocel/ainovel-cli/internal/store"
	"github.com/voocel/ainovel-cli/internal/tools"
)

// testDeps æž„é€ ä¸‰ä¸ªè¯­ä¹‰å‡½æ•°åŒç”¨ä¸€ä¸ª mock æ¡£ä½çš„æœ€å° Depsã€‚
func testDeps(st *store.Store, m callModel) Deps {
	c := Caller{Model: m}
	return Deps{
		Store:         st,
		CommitChapter: tools.NewCommitChapterTool(st, tools.NewStyleStatsIndex(st)),
		Segment:       c,
		Analyze:       c,
		Synthesize:    c,
		Prompts:       Prompts{Segment: "seg", Analyze: "ana", Synthesize: "syn", Range: "range"},
	}
}

// TestRunEndToEnd ç”¨ mock æ¨¡åž‹é©±åŠ¨å®Œæ•´ç®¡çº¿ ingestâ†’segmentâ†’analyzeâ†’synthesizeâ†’publishï¼Œ
// ç»çœŸå®ž commit_chapter è½ç›˜ï¼ŒéªŒè¯æ­£å¼ Foundation ä¸Žå…¨éƒ¨ç« èŠ‚å°±ç»ªã€‚
func TestRunEndToEnd(t *testing.T) {
	dir := t.TempDir()
	st := store.NewStore(dir)
	if err := st.Init(); err != nil {
		t.Fatalf("store init: %v", err)
	}
	src := filepath.Join(dir, "book.txt")
	if err := os.WriteFile(src, []byte("ç¬¬ä¸€ç« \næ­£æ–‡ä¸€\nç¬¬äºŒç« \næ­£æ–‡äºŒ\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	seg := boundariesJSON(
		boundaryFixture("L1", "", kindChapter, "ç¬¬ä¸€ç« "),
		boundaryFixture("L3", "", kindChapter, "ç¬¬äºŒç« "),
	)
	ana := `{"chapters":[` + factsJSON(1, "ç¬¬ä¸€ç« ") + `,` + factsJSON(2, "ç¬¬äºŒç« ") + `]}`
	syn := synthesisFixtureJSON(2, storyClosed)
	m := &mockModel{responses: []string{seg, ana, syn}}

	ch, err := Run(context.Background(), testDeps(st, m), Options{SourcePath: src, AutoConfirm: true, ContinueAfter: true})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	var runErr error
	var doneSeen bool
	for ev := range ch {
		if ev.Stage == StageError {
			runErr = ev.Err
		}
		if ev.Stage == StageDone {
			doneSeen = true
		}
	}
	if runErr != nil {
		t.Fatalf("ç®¡çº¿å¤±è´¥ï¼š%v", runErr)
	}
	if !doneSeen {
		t.Fatal("æœªæ”¶åˆ° StageDone")
	}
	// æ­£å¼çŠ¶æ€å°±ç»ªï¼špremise ä¸Žè¦†ç›–å…¨ç« çš„æ‰å¹³å¤§çº²å·²è½ç›˜ï¼ˆworld_rules åˆæ³•ä¸ºç©ºï¼Œä¸åšè¦æ±‚ï¼‰ã€‚
	if p, _ := st.Outline.LoadPremise(); p == "" {
		t.Fatal("premise æœªè½ç›˜")
	}
	if o, _ := st.Outline.LoadOutline(); len(o) != 2 {
		t.Fatalf("æ‰å¹³å¤§çº²åº”è¦†ç›– 2 ç« ï¼Œå¾— %d", len(o))
	}
	prog, _ := st.Progress.Load()
	if prog == nil || len(prog.CompletedChapters) != 2 {
		t.Fatalf("åº”å®Œæˆ 2 ç« ï¼š%+v", prog)
	}
	if active, done, err := ResumeStatus(st); err != nil || !active || !done {
		t.Fatalf("ResumeStatus åº”ä¸º active&doneï¼Œå¾— active=%v done=%v", active, done)
	}
	// --continueï¼šä¸è®¾å¯¼å…¥å®Œæˆ Holdï¼ˆäº¤ç”± host è‡ªåŠ¨æŽ¥åŠ›ï¼‰ã€‚
	if meta, _ := st.RunMeta.Load(); meta != nil && meta.AdvanceHold != nil {
		t.Fatalf("--continue ä¸åº”ç•™ä¸‹å¯¼å…¥å®Œæˆ Holdï¼š%+v", meta.AdvanceHold)
	}
}

// TestRunSetsCompletionHold éªŒè¯éž --continue å¯¼å…¥å®ŒæˆåŽè®¾ç½® boundary Holdï¼ˆRFC Â§12.4ï¼‰ã€‚
// Hold æ˜¯"å¯¼å…¥åŽä¸è¯¯ç»­å†™"çš„å”¯ä¸€ä¿éšœï¼Œå¿…é¡»åœ¨å‘å¸ƒè·¯å¾„æŒä¹…åŒ–ã€‚
func TestRunSetsCompletionHold(t *testing.T) {
	dir := t.TempDir()
	st := store.NewStore(dir)
	if err := st.Init(); err != nil {
		t.Fatalf("store init: %v", err)
	}
	src := filepath.Join(dir, "book.txt")
	if err := os.WriteFile(src, []byte("ç¬¬ä¸€ç« \næ­£æ–‡ä¸€\nç¬¬äºŒç« \næ­£æ–‡äºŒ\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	seg := boundariesJSON(
		boundaryFixture("L1", "", kindChapter, "ç¬¬ä¸€ç« "),
		boundaryFixture("L3", "", kindChapter, "ç¬¬äºŒç« "),
	)
	ana := `{"chapters":[` + factsJSON(1, "ç¬¬ä¸€ç« ") + `,` + factsJSON(2, "ç¬¬äºŒç« ") + `]}`
	syn := synthesisFixtureJSON(2, storyClosed)
	m := &mockModel{responses: []string{seg, ana, syn}}

	ch, err := Run(context.Background(), testDeps(st, m), Options{SourcePath: src, AutoConfirm: true}) // æ—  --continue
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	for ev := range ch {
		if ev.Stage == StageError {
			t.Fatalf("ç®¡çº¿å¤±è´¥ï¼š%v", ev.Err)
		}
	}
	meta, err := st.RunMeta.Load()
	if err != nil {
		t.Fatalf("load run meta: %v", err)
	}
	if meta == nil || meta.AdvanceHold == nil {
		t.Fatalf("å¯¼å…¥å®Œæˆåº”è®¾ç½® boundary Holdï¼Œå¾— %+v", meta)
	}
}

// TestRunRejectsDifferentSource å®ˆæŠ¤æ¢æºæ‹¦æˆªï¼ˆRFC Â§12.1/Â§18.2ï¼‰ï¼šå·¥ä½œåŒºè¿›è¡Œä¸­ä¼ å…¥ä¸åŒ
// å†…å®¹çš„æºæ–‡ä»¶å¿…é¡»æ˜Žç¡®æŠ¥é”™â€”â€”ingest åªåœ¨æ— å·¥ä½œåŒºæ—¶æ‰§è¡Œï¼Œä¸æ¯”å¯¹ä¼šé™é»˜ä»Žæ—§ä¹¦æ–­ç‚¹ç»§ç»­ã€
// æŠŠæ—§ä¹¦å‘å¸ƒå®Œæ¯•è€Œæ–°æ–‡ä»¶ä¸€ä¸ªå­—èŠ‚éƒ½æ²¡è¯»ã€‚åŒä¸€æ–‡ä»¶é‡å¤ä¼ è·¯å¾„æ˜¯å¸¸è§æ¢å¤ä¹ æƒ¯ï¼ŒæŒ‰å†…å®¹æ‘˜è¦æ¯”å¯¹æ”¾è¡Œã€‚
func TestRunRejectsDifferentSource(t *testing.T) {
	dir := t.TempDir()
	st := store.NewStore(dir)
	if err := st.Init(); err != nil {
		t.Fatal(err)
	}
	a := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(a, []byte("ç¬¬ä¸€ç« \næ­£æ–‡ä¸€\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Ingest(dir, a, Options{}.intent()); err != nil {
		t.Fatalf("å»ºç«‹å·¥ä½œåŒºï¼š%v", err)
	}
	b := filepath.Join(dir, "b.txt")
	if err := os.WriteFile(b, []byte("å®Œå…¨ä¸åŒçš„å¦ä¸€æœ¬ä¹¦\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ch, err := Run(context.Background(), testDeps(st, &mockModel{responses: []string{"{}"}}), Options{SourcePath: b})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	var runErr error
	for ev := range ch {
		if ev.Stage == StageError {
			runErr = ev.Err
		}
	}
	if runErr == nil || (!strings.Contains(runErr.Error(), "content differs") && !strings.Contains(runErr.Error(), "nội dung khác nhau")) {
		t.Fatalf("ä¸åŒæºæ–‡ä»¶åº”è¢«æ˜Žç¡®æ‹’ç»ï¼Œå¾— %v", runErr)
	}
}

// TestConfirmNotesGate å®ˆæŠ¤ --yes çš„å®¹é”™é—¨æ§›ï¼šè¯­ä¹‰å®¹é”™ï¼ˆNotes éžç©ºï¼‰å‘ç”Ÿè¿‡çš„åˆ‡åˆ†ç»“æž„
// è¢«ç¡®å®šæ€§æ”¹å†™è¿‡ï¼Œä¸ç”±æœªçœ‹é¢„è§ˆçš„ --yes ç›²æ”¾è¡Œï¼›TUI é¢„è§ˆåŽæŒ‰ yï¼ˆAcceptSegmentationï¼‰æ”¾è¡Œï¼Œ
// ç¡®è®¤æ–¹æ³•è®° user_confirmed æº¯æºã€‚
func TestConfirmNotesGate(t *testing.T) {
	newRunner := func(opts Options, notes []string) *runner {
		ws := &Workspace{dir: t.TempDir()}
		if err := ws.writeJSON(fileIntent, Intent{}); err != nil {
			t.Fatal(err)
		}
		seg := Segmentation{Chapters: []ChapterSpan{{Number: 1, Title: "ç¬¬ä¸€ç« ", End: 10}}, Notes: notes}
		if err := writeArtifact(ws, fileSegmentation, "d", seg); err != nil {
			t.Fatal(err)
		}
		return &runner{opts: opts, events: make(chan Event, 8), ws: ws}
	}
	r := newRunner(Options{AutoConfirm: true}, []string{"ç©ºæ­£æ–‡å ä½å¹¶å…¥å‰æ®µ"})
	if r.confirm() {
		t.Fatal("--yes ä¸åº”æ”¾è¡Œå¸¦å®¹é”™è¯´æ˜Žçš„åˆ‡åˆ†")
	}
	if ev := <-r.events; !strings.Contains(ev.Message, "未自动放行，请人工核对") && !strings.Contains(ev.Message, "vui lòng kiểm tra thủ công") {
		t.Fatalf("é¢„è§ˆåº”è¯´æ˜Žæœªæ”¾è¡ŒåŽŸå› ï¼š%q", ev.Message)
	}
	if !newRunner(Options{AutoConfirm: true}, nil).confirm() {
		t.Fatal("--yes åº”æ”¾è¡Œæ— å®¹é”™è¯´æ˜Žçš„åˆ‡åˆ†")
	}
	r = newRunner(Options{AcceptSegmentation: true}, []string{"ç©ºæ­£æ–‡å ä½å¹¶å…¥å‰æ®µ"})
	if !r.confirm() {
		t.Fatal("é¢„è§ˆåŽçš„äººå·¥ y åº”æ”¾è¡Œå¸¦å®¹é”™è¯´æ˜Žçš„åˆ‡åˆ†")
	}
	conf, err := readArtifact[Confirmation](r.ws, fileConfirmation)
	if err != nil {
		t.Fatal(err)
	}
	if conf.Payload.Method != confirmMethodUser {
		t.Fatalf("äººå·¥ç¡®è®¤åº”è®° user_confirmedï¼Œå¾— %q", conf.Payload.Method)
	}
}

// TestStoryChoiceIgnoresStaleResolution å®ˆæŠ¤ #5ï¼šé‡æ–°ç»¼åˆåŽæ—§æ•…äº‹è£å®šå¤±æ•ˆï¼Œ
// storyChoice ä¸å¾—æŠŠæ—§ open/closed é™é»˜å¥—åˆ°æ–° synthesis ä¸Šï¼ˆå¦åˆ™ç”¨æˆ·ä¸ä¼šè¢«é‡æ–°å¾è¯¢ï¼‰ã€‚
func TestStoryChoiceIgnoresStaleResolution(t *testing.T) {
	ws := OpenWorkspace(t.TempDir())
	if err := ws.writeJSON(fileIntent, Intent{}); err != nil {
		t.Fatal(err)
	}
	if err := writeArtifact(ws, fileSynthesis, "d", BookSynthesis{Premise: "p1", StoryStatus: storyUncertain}); err != nil {
		t.Fatal(err)
	}
	raw, _ := ws.readBytes(fileSynthesis)
	if err := writeArtifact(ws, fileStoryResolve, Digest(raw), StoryResolution{Choice: storyClosed}); err != nil {
		t.Fatal(err)
	}
	r := &runner{ws: ws}
	if got, err := r.storyChoice(); err != nil || got != storyClosed {
		t.Fatalf("ç»‘å®šå½“å‰ synthesis çš„è£å®šåº”è¿”å›ž closedï¼Œå¾— %q", got)
	}
	// é‡æ–°ç»¼åˆï¼šæ”¹å†™ synthesis â†’ æ—§è£å®š InputDigest å¤±é…ï¼Œåº”è¢«å¿½ç•¥ï¼Œå›žåˆ°"éœ€é‡æ–°å¾è¯¢"ï¼ˆè¿”å›žç©ºï¼‰ã€‚
	if err := writeArtifact(ws, fileSynthesis, "d", BookSynthesis{Premise: "p2", StoryStatus: storyUncertain}); err != nil {
		t.Fatal(err)
	}
	if got, err := r.storyChoice(); err != nil || got != "" {
		t.Fatalf("é‡æ–°ç»¼åˆåŽæ—§è£å®šåº”å¤±æ•ˆè¿”å›žç©ºï¼Œå¾— %q", got)
	}
}

// TestBudgetsFromDepsPerTier å®ˆæŠ¤æ¡£ä½æ—‹é’®ï¼ˆRFC Â§13.1ï¼‰ï¼šå„è¯­ä¹‰å‡½æ•°é¢„ç®—æŒ‰å„è‡ªæ¡£ä½æ´¾ç”Ÿï¼Œ
// å»‰ä»·æ¡£ä½çš„å°çª—å£åªçº¦æŸå®ƒè‡ªå·±çš„å‡½æ•°ï¼Œä¸æ‹–ç´¯å…¶å®ƒé˜¶æ®µã€‚
func TestBudgetsFromDepsPerTier(t *testing.T) {
	small := ModelRuntime{ContextTokens: 32000, MaxOutputTokens: 4000}
	big := ModelRuntime{ContextTokens: 200000, MaxOutputTokens: 16000}
	b := budgetsFromDeps(Deps{
		Segment:    Caller{Runtime: small},
		Analyze:    Caller{Runtime: big},
		Synthesize: Caller{Runtime: big},
	})
	if b.SegmentChunkBytes >= b.Analyze.ContextBytes {
		t.Fatalf("segment å°æ¡£ä½çª—å£åº”åªçº¦æŸè‡ªèº«ï¼šseg=%d analyze=%d", b.SegmentChunkBytes, b.Analyze.ContextBytes)
	}
	if b.Analyze.MaxOutputTokens != 16000 || b.SegmentMaxTokens != 4000 {
		t.Fatalf("è¾“å‡ºé¢„ç®—åº”å„å–è‡ªèº«æ¡£ä½ä¸Šé™ï¼šanalyze=%d segment=%d", b.Analyze.MaxOutputTokens, b.SegmentMaxTokens)
	}
}

// TestRunSavesFailureOnContractViolation å®ˆæŠ¤ Â§14.2ï¼šåŽŸç”Ÿ Schema å¥‘çº¦è¿çº¦
// å¿…é¡»ç«‹å³æš´éœ²ï¼Œå¹¶æŠŠåŽŸå§‹å“åº”ä¸Žå…ƒæ•°æ®è½ failures/ã€‚
func TestRunSavesFailureOnContractViolation(t *testing.T) {
	dir := t.TempDir()
	st := store.NewStore(dir)
	if err := st.Init(); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(dir, "book.txt")
	if err := os.WriteFile(src, []byte("ç¬¬ä¸€ç« \næ­£æ–‡\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := &nativeImportModel{mockModel: &mockModel{responses: []string{"è¿™ä¸æ˜¯ JSON"}}}
	ch, err := Run(context.Background(), testDeps(st, m), Options{SourcePath: src, AutoConfirm: true})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	var failed bool
	for ev := range ch {
		if ev.Stage == StageError {
			failed = true
		}
	}
	if !failed {
		t.Fatal("éžæ³•è¾“å‡ºåº”ä»¥ StageError ç»“æŸ")
	}
	ws := OpenWorkspace(dir)
	if !ws.has("failures/last-response.txt") {
		t.Fatal("åº”ä¿å­˜æœ€åŽä¸€æ¬¡åŽŸå§‹æ¨¡åž‹å“åº”")
	}
	var meta FailureMeta
	if err := ws.readJSON("failures/last.json", &meta); err != nil {
		t.Fatalf("è¯»å¤±è´¥å…ƒæ•°æ®ï¼š%v", err)
	}
	if meta.Stage != string(ActionSegment) {
		t.Fatalf("å¤±è´¥å…ƒæ•°æ®åº”æ ‡æ³¨ segment é˜¶æ®µï¼Œå¾— %q", meta.Stage)
	}
}

// TestRunGuidanceResegments å®ˆæŠ¤ Â§18.3ï¼šæ¢å¤æ—¶æºå¸¦ --guide ä½¿æ—§åˆ‡åˆ†è‡ªç„¶å¤±é…ï¼Œ
// æŒ‰æ–°æŒ‡å¯¼é‡æ–°è¯†åˆ«å¹¶å†æ¬¡åœåœ¨ç¡®è®¤å¤„ï¼›æ–°åˆ‡åˆ† InputDigest ç»‘å®šæŒ‡å¯¼æ–‡æœ¬ã€‚
func TestRunGuidanceResegments(t *testing.T) {
	dir := t.TempDir()
	st := store.NewStore(dir)
	if err := st.Init(); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(dir, "book.txt")
	if err := os.WriteFile(src, []byte("ç¬¬ä¸€ç« \næ­£æ–‡ä¸€\nç¬¬äºŒç« \næ­£æ–‡äºŒ\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	drain := func(ch <-chan Event) (awaiting bool) {
		for ev := range ch {
			if ev.Stage == StageError {
				t.Fatalf("ç®¡çº¿å¤±è´¥ï¼š%v", ev.Err)
			}
			if ev.Stage == StageAwaitingConfirmation {
				awaiting = true
			}
		}
		return awaiting
	}
	// é¦–æ¬¡äº¤äº’å¯¼å…¥ï¼šæ¨¡åž‹æŠŠå…¨ä¹¦åˆ‡æˆ 1 ç« ï¼Œåœåœ¨ç¡®è®¤ã€‚
	one := boundariesJSON(boundaryFixture("L1", "", kindChapter, "ç¬¬ä¸€ç« "))
	ch, err := Run(context.Background(), testDeps(st, &mockModel{responses: []string{one}}), Options{SourcePath: src})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !drain(ch) {
		t.Fatal("é¦–æ¬¡å¯¼å…¥åº”åœåœ¨åˆ‡åˆ†ç¡®è®¤")
	}
	// å¸¦æŒ‡å¯¼æ¢å¤ï¼šæ—§åˆ‡åˆ†å¤±é… â†’ é‡è¯†åˆ«ä¸º 2 ç« ï¼Œå†æ¬¡åœåœ¨ç¡®è®¤ã€‚
	two := boundariesJSON(
		boundaryFixture("L1", "", kindChapter, "ç¬¬ä¸€ç« "),
		boundaryFixture("L3", "", kindChapter, "ç¬¬äºŒç« "),
	)
	guidance := "ç¬¬äºŒç« ä¹Ÿæ˜¯ç‹¬ç«‹ç« èŠ‚"
	ch2, err := Run(context.Background(), testDeps(st, &mockModel{responses: []string{two}}), Options{Guidance: guidance})
	if err != nil {
		t.Fatalf("æ¢å¤ Run: %v", err)
	}
	if !drain(ch2) {
		t.Fatal("é‡è¯†åˆ«åŽåº”å†æ¬¡åœåœ¨åˆ‡åˆ†ç¡®è®¤")
	}
	ws := OpenWorkspace(dir)
	art, err := readArtifact[Segmentation](ws, fileSegmentation)
	if err != nil {
		t.Fatalf("è¯»åˆ‡åˆ†å·¥ä»¶ï¼š%v", err)
	}
	if len(art.Payload.Chapters) != 2 {
		t.Fatalf("åº”æŒ‰æŒ‡å¯¼åˆ‡æˆ 2 ç« ï¼Œå¾— %d", len(art.Payload.Chapters))
	}
	norm, _ := ws.LoadSource()
	if art.InputDigest != segmentInputDigest(Digest(norm), guidance, segmentPromptVersion) {
		t.Fatal("æ–°åˆ‡åˆ† InputDigest åº”ç»‘å®šæŒ‡å¯¼æ–‡æœ¬")
	}
}

// TestBudgetsFromRuntime éªŒè¯åŒé¢„ç®—éšæ¨¡åž‹çœŸå®žå®¹é‡æ”¾å¤§ï¼Œèƒ½åŠ›æœªçŸ¥æ—¶å›žé€€ä¿å®ˆé»˜è®¤ï¼ˆRFC Â§9.2/Â§21ï¼‰ã€‚
func TestBudgetsFromRuntime(t *testing.T) {
	if got := budgetsFromRuntime(ModelRuntime{}); got != DefaultRunBudgets() {
		t.Fatal("èƒ½åŠ›æœªçŸ¥åº”å›žé€€ä¿å®ˆé»˜è®¤")
	}
	small := budgetsFromRuntime(ModelRuntime{ContextTokens: 32000, MaxOutputTokens: 4000})
	big := budgetsFromRuntime(ModelRuntime{ContextTokens: 200000, MaxOutputTokens: 16000})
	if big.Analyze.ContextBytes <= small.Analyze.ContextBytes {
		t.Fatalf("æ›´å¤§ context åº”æ”¾å¤§ analyze è¾“å…¥é¢„ç®—ï¼šsmall=%d big=%d", small.Analyze.ContextBytes, big.Analyze.ContextBytes)
	}
	if big.Analyze.MaxOutputTokens != 16000 {
		t.Fatalf("è¾“å‡ºé¢„ç®—åº”å–æ¨¡åž‹ completion ä¸Šé™ï¼Œå¾— %d", big.Analyze.MaxOutputTokens)
	}
}

// TestProfileForKeyPolicy å®ˆæŠ¤äº‹ä»¶åˆå¹¶èŒƒå›´ï¼šè¯·æ±‚é€€é¿ï¼ˆå¸¦æˆªæ­¢æ—¶åˆ»ï¼‰åŒ Key åŽŸåœ°è·³åŠ¨ï¼›
// æ ¡éªŒé‡é—®æ˜¯è·¨è°ƒç”¨çš„è¯­ä¹‰äº‹ä»¶ï¼Œä¸å¸¦ Key å„è‡ªæˆè¡Œâ€”â€”åˆ‡åˆ†é€å—è°ƒç”¨ï¼Œå…±ç”¨ Key ä¼šè®©åŽå—è¦†ç›–å‰å—ï¼Œ
// é¢æ¿åªå‰©ä¸€æ¡ unit_id ä¸æ–­å˜åŒ–çš„è¡Œï¼ŒæŽ’æŸ¥çº¿ç´¢å…¨ä¸¢ï¼›step æ˜¯æ™®é€šè¿›åº¦äº‹ä»¶ï¼ˆæ— è­¦ç¤ºçº§åˆ«ï¼‰ã€‚
func TestProfileForKeyPolicy(t *testing.T) {
	r := &runner{events: make(chan Event, 3)}
	prof := r.profileFor(Caller{}, StageSegmenting)
	prof.notify("é€€é¿", time.Now().Add(time.Second))
	prof.notify("é‡é—®", time.Time{})
	prof.step(2, 12, "åˆ‡åˆ†ç¬¬ %d/%d å—...", 2, 12)
	backoff, reask, step := <-r.events, <-r.events, <-r.events
	if backoff.Key == "" || backoff.Level != "warn" || backoff.RetryAt.IsZero() {
		t.Fatalf("è¯·æ±‚é€€é¿åº”ä¸ºå¸¦ Key ä¸Žæˆªæ­¢æ—¶åˆ»çš„ warn äº‹ä»¶ï¼š%+v", backoff)
	}
	if reask.Key != "" || reask.Level != "warn" {
		t.Fatalf("æ ¡éªŒé‡é—®åº”ä¸ºä¸å¸¦ Key çš„ warn äº‹ä»¶ï¼ˆç‹¬ç«‹æˆè¡Œï¼‰ï¼š%+v", reask)
	}
	if step.Level != "" || step.Current != 2 || step.Total != 12 {
		t.Fatalf("step åº”ä¸ºæ™®é€šè¿›åº¦äº‹ä»¶ï¼š%+v", step)
	}
}

// TestCallProfileOptions éªŒè¯ callProfile åªè´Ÿè´£è¾“å‡ºé¢„ç®—ä¸Ž thinkingï¼›response_format
// ç”± callStructured æ ¹æ®æ¨¡åž‹äº‹å®žå’Œ Contract é€‰æ‹©ï¼Œä¸èƒ½åœ¨ Profile ä¸­é‡å¤ç»„è£…ã€‚
func TestCallProfileOptions(t *testing.T) {
	if got := (callProfile{}).callOptions(100); len(got) != 1 {
		t.Fatalf("é›¶å€¼åªåº”å¸¦ maxTokensï¼Œå¾— %d ä¸ª option", len(got))
	}
	if got := (callProfile{thinking: "high"}).callOptions(100); len(got) != 2 {
		t.Fatalf("thinking åº”å¸¦ 2 ä¸ª optionï¼Œå¾— %d", len(got))
	}
}

