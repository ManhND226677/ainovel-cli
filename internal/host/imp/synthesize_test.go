package imp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/voocel/agentcore"
	"github.com/voocel/ainovel-cli/internal/domain"
)

func factsN(n int) []ImportedChapterFacts {
	out := make([]ImportedChapterFacts, n)
	for i := 0; i < n; i++ {
		out[i] = ImportedChapterFacts{
			Chapter: i + 1, Title: "ç¬¬" + itoa(i+1) + "ç« ", CoreEvent: "äº‹ä»¶", Summary: "æ‘˜è¦",
			HookType: "mystery", DominantStrand: "quest",
		}
	}
	return out
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

func TestValidateStructure(t *testing.T) {
	ok := []ImportedVolumeRange{{Title: "å·ä¸€", Arcs: []ImportedArcRange{{StartChapter: 1, EndChapter: 3}}}}
	if err := validateStructure(ok, 3); err != nil {
		t.Fatalf("åˆæ³•ç»“æž„åº”é€šè¿‡ï¼š%v", err)
	}
	gap := []ImportedVolumeRange{{Arcs: []ImportedArcRange{{StartChapter: 1, EndChapter: 2}, {StartChapter: 4, EndChapter: 5}}}}
	if err := validateStructure(gap, 5); err == nil {
		t.Fatal("ç¼ºå£åº”æ‹’ç»")
	}
	short := []ImportedVolumeRange{{Arcs: []ImportedArcRange{{StartChapter: 1, EndChapter: 2}}}}
	if err := validateStructure(short, 3); err == nil {
		t.Fatal("æœªè¦†ç›– N åº”æ‹’ç»")
	}
}

func TestAssembleFoundationHappyClosed(t *testing.T) {
	facts := factsN(3)
	s := &BookSynthesis{
		Premise:      "# æµ‹è¯•ä¹¦\n\nå‰æ",
		Characters:   []domain.Character{{Name: "ç”²"}},
		PlanningTier: domain.PlanningTierShort,
		StoryStatus:  storyClosed,
		Compass:      domain.StoryCompass{EndingDirection: "æ”¶æŸ"},
		Structure:    []ImportedVolumeRange{{Title: "å·ä¸€", Arcs: []ImportedArcRange{{Title: "å¼§ä¸€", StartChapter: 1, EndChapter: 3}}}},
	}
	f, err := AssembleFoundation(s, facts, true, "book.txt")
	if err != nil {
		t.Fatalf("ç»„è£…åº”æˆåŠŸï¼š%v", err)
	}
	if len(domain.FlattenOutline(f.Volumes)) != 3 {
		t.Fatal("å±•å¼€ç« æ•°åº”ä¸º 3")
	}
	if !f.Volumes[len(f.Volumes)-1].Final {
		t.Fatal("closed æ—¶æœ«å·åº” Final")
	}
}

func TestAssembleFoundationTitleMismatch(t *testing.T) {
	facts := factsN(2)
	facts[1].Title = "" // ç ´åæ ‡é¢˜ä¸€è‡´æ€§ä¼šåœ¨ FlattenOutline æ ¡éªŒå¤±è´¥ï¼Ÿæ ‡é¢˜ç©ºä½†ç»“æž„å–è‡ª factsï¼Œæ•…ä¸€è‡´ã€‚
	// ç”¨ç»“æž„è¦†ç›–ä¸åˆ°çš„ç« åˆ¶é€ çœŸå®žä¸ä¸€è‡´ï¼šç« æ•°ä¸ç¬¦ã€‚
	s := &BookSynthesis{
		Premise: "# ä¹¦", Characters: []domain.Character{{Name: "ç”²"}},
		PlanningTier: domain.PlanningTierShort, StoryStatus: storyOpen,
		Compass:   domain.StoryCompass{EndingDirection: "x"},
		Structure: []ImportedVolumeRange{{Arcs: []ImportedArcRange{{StartChapter: 1, EndChapter: 1}}}},
	}
	if _, err := AssembleFoundation(s, facts, false, "b.txt"); err == nil {
		t.Fatal("ç»“æž„åªè¦†ç›– 1 ç« è€Œäº‹å®ž 2 ç« åº”æ‹’ç»")
	}
}

func TestEnsurePremiseTitle(t *testing.T) {
	if got := ensurePremiseTitle("æ­£æ–‡æ— æ ‡é¢˜", "æˆ‘çš„å°è¯´.txt"); got[0] != '#' {
		t.Fatalf("åº”è¡¥ä¹¦åæ ‡é¢˜ï¼š%q", got)
	}
	if got := ensurePremiseTitle("# å·²æœ‰ä¹¦å\næ­£æ–‡", "x.txt"); got != "# å·²æœ‰ä¹¦å\næ­£æ–‡" {
		t.Fatal("å·²æœ‰æ ‡é¢˜ä¸åº”æ”¹å†™")
	}
}

func TestPlanFactRangesSplits(t *testing.T) {
	facts := factsN(20)
	one := len(compactFact(facts[0]))
	ranges := planFactRanges(facts, one*3) // æ¯åŒºé—´çº¦ 3 ç« 
	if len(ranges) < 2 {
		t.Fatalf("åº”åˆ†å¤šåŒºé—´ï¼Œå¾— %d", len(ranges))
	}
	if ranges[0][0] != 0 || ranges[len(ranges)-1][1] != 20 {
		t.Fatal("åŒºé—´æœªå®Œæ•´è¦†ç›–")
	}
}

// TestToCompactCarriesEvidence å®ˆæŠ¤ #6ï¼šé€ç« åæŽ¨çš„ character/world evidence å¿…é¡»è¿›å…¥ç»¼åˆç´§å‡‘è§†å›¾ï¼Œ
// å¦åˆ™ç»¼åˆå™¨åªèƒ½ä»Žæ‘˜è¦è‡†é€ æ­£å¼è§’è‰²ä¸Žä¸–ç•Œè§„åˆ™ã€‚
func TestToCompactCarriesEvidence(t *testing.T) {
	f := ImportedChapterFacts{
		Chapter: 1, Title: "ç¬¬ä¸€ç« ", CoreEvent: "e", Summary: "s",
		CharacterEvidence: []ImportedCharacterFact{{Chapter: 1, Name: "ç”²", Note: "æ²‰ç¨³"}},
		WorldEvidence:     []ImportedWorldFact{{Chapter: 1, Category: "magic", Fact: "çµæ°”å……ç›ˆ"}},
	}
	cv := toCompact(f)
	if len(cv.CharacterEvidence) != 1 || cv.CharacterEvidence[0].Name != "ç”²" {
		t.Fatalf("character evidence æœªå¸¦å…¥ç´§å‡‘è§†å›¾ï¼š%+v", cv.CharacterEvidence)
	}
	if len(cv.WorldEvidence) != 1 || cv.WorldEvidence[0].Fact != "çµæ°”å……ç›ˆ" {
		t.Fatalf("world evidence æœªå¸¦å…¥ç´§å‡‘è§†å›¾ï¼š%+v", cv.WorldEvidence)
	}
}

// TestSynthesizeRejectsRangeMismatch å®ˆæŠ¤ #4ï¼šé•¿ä¹¦ Map é˜¶æ®µåŒºé—´æ‘˜è¦çš„èµ·æ­¢ç« å¿…é¡»ä¸Žè¯·æ±‚ä¸€è‡´ï¼Œ
// å¦åˆ™å½’å¹¶æ—¶ä¼šæŠŠé”™ä½åŒºé—´å½“ä½œæœ¬åŒºé—´æ‘˜è¦ã€‚
func TestSynthesizeRejectsRangeMismatch(t *testing.T) {
	err := validateRangeDigest(&RangeDigest{StartChapter: 1, EndChapter: 5, Plot: "é”™ä½åŒºé—´"}, 1, 2, "range digest")
	if err == nil {
		t.Fatal("åŒºé—´èµ·æ­¢ç« ä¸Žè¯·æ±‚ä¸ç¬¦åº”æ‹’ç»")
	}
	if !strings.Contains(err.Error(), "mismatches request") {
		t.Fatalf("é”™è¯¯åº”æŒ‡å‡ºåŒºé—´èŒƒå›´ä¸ç¬¦ï¼Œå¾—ï¼š%v", err)
	}
}

// TestGroupDigestsByBudget å®ˆæŠ¤ #3 å½’å¹¶åˆ†ç»„ï¼šè¿žç»­åŒºé—´æ‘˜è¦æŒ‰å­—èŠ‚é¢„ç®—åˆ†è¿žç»­ç»„ï¼Œå•æ‘˜è¦è¶…é¢„ç®—ä¹Ÿå•ç‹¬æˆç»„ã€‚
func TestGroupDigestsByBudget(t *testing.T) {
	ds := []RangeDigest{
		{StartChapter: 1, EndChapter: 5, Plot: strings.Repeat("x", 200)},
		{StartChapter: 6, EndChapter: 10, Plot: strings.Repeat("y", 200)},
		{StartChapter: 11, EndChapter: 15, Plot: strings.Repeat("z", 200)},
		{StartChapter: 16, EndChapter: 20, Plot: strings.Repeat("w", 200)},
	}
	per := len(mustJSON(t, ds[0]))
	groups := groupDigestsByBudget(ds, per*2+10) // æ¯ç»„çº¦å®¹çº³ 2 ä¸ª
	if len(groups) != 2 || len(groups[0]) != 2 || len(groups[1]) != 2 {
		t.Fatalf("åº”åˆ† 2 ç»„å„ 2 ä¸ªï¼Œå¾— %v", groups)
	}
	if groups[0][0].StartChapter != 1 || groups[1][1].EndChapter != 20 {
		t.Fatal("åˆ†ç»„æœªä¿æŒè¿žç»­è¦†ç›–")
	}
}

// TestReduceToFitMergesUntilBudget å®ˆæŠ¤ #3ï¼šåŒºé—´æ‘˜è¦æ€»é‡è¶…é¢„ç®—æ—¶é€å±‚å½’å¹¶åˆ°å¯å®¹çº³ï¼Œ
// è€Œéžæ— ç•Œè¿›å…¥æœ€ç»ˆç»¼åˆè°ƒç”¨ã€‚
func TestReduceToFitMergesUntilBudget(t *testing.T) {
	ds := []RangeDigest{
		{StartChapter: 1, EndChapter: 5, Plot: strings.Repeat("x", 200)},
		{StartChapter: 6, EndChapter: 10, Plot: strings.Repeat("y", 200)},
		{StartChapter: 11, EndChapter: 15, Plot: strings.Repeat("z", 200)},
		{StartChapter: 16, EndChapter: 20, Plot: strings.Repeat("w", 200)},
	}
	budget := len(mustJSON(t, ds[0]))*2 + 10
	// æ¯ç»„å½’å¹¶å‡ºä¸€ä¸ªå°æ‘˜è¦ï¼šç¬¬ 1-10 ç« ã€ç¬¬ 11-20 ç« ã€‚
	m := &mockModel{responses: []string{
		rangeDigestJSON(1, 10, "åˆå¹¶ä¸€"),
		rangeDigestJSON(11, 20, "åˆå¹¶äºŒ"),
	}}
	out, err := reduceToFit(context.Background(), m, "range", ds, budget, 4096, callProfile{})
	if err != nil {
		t.Fatalf("reduceToFit: %v", err)
	}
	if len(out) != 2 || out[0].StartChapter != 1 || out[0].EndChapter != 10 || out[1].StartChapter != 11 || out[1].EndChapter != 20 {
		t.Fatalf("åº”å½’å¹¶ä¸º 2 ä¸ªè¿žç»­åŒºé—´æ‘˜è¦ï¼Œå¾— %+v", out)
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestSynthesizeDirectWithMock(t *testing.T) {
	facts := factsN(3)
	resp := synthesisFixtureJSON(3, storyOpen)
	m := &mockModel{responses: []string{resp}}
	s, err := Synthesize(context.Background(), m, "sys", "range-sys", &Workspace{dir: t.TempDir()}, facts, 0, 4096, callProfile{})
	if err != nil {
		t.Fatalf("Synthesize: %v", err)
	}
	if s.StoryStatus != storyOpen || len(s.Structure) != 1 {
		t.Fatalf("ç»¼åˆç»“æžœä¸ç¬¦ï¼š%+v", s)
	}
	if _, err := AssembleFoundation(s, facts, false, "b.txt"); err != nil {
		t.Fatalf("ç»„è£…åº”æˆåŠŸï¼š%v", err)
	}
	_ = agentcore.StopReasonStop
}

