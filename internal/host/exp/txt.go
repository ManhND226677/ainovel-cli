package exp

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/voocel/ainovel-cli/internal/domain"
	"github.com/voocel/ainovel-cli/internal/translation"
)

// chapterTitleIndex 给定章号查标题，缺失返回空串。
type chapterTitleIndex map[int]string

func buildTitleIndex(outline []domain.OutlineEntry) chapterTitleIndex {
	idx := make(chapterTitleIndex, len(outline))
	for _, e := range outline {
		if e.Title != "" {
			idx[e.Chapter] = e.Title
		}
	}
	return idx
}

// chapterLocation 是某章在分层大纲中的归属。只保留导出版式需要的卷信息——
// 弧不进导出（读者视角下弧是过细的内部结构）。
type chapterLocation struct {
	VolumeIdx       int
	VolumeTitle     string
	IsFirstOfVolume bool
}

// buildLocations 按分层大纲的全局章节顺序构造 {chapter -> location}。
// 章号按 FlattenOutline 同样的规则重建（卷内弧内顺序累加），
// 以保持与 Progress.CompletedChapters 的章号一致。弧层仍要遍历（算全局章号必经），
// 但不落入 location——导出只在卷首插分隔。
func buildLocations(volumes []domain.VolumeOutline) map[int]chapterLocation {
	if len(volumes) == 0 {
		return nil
	}
	locs := make(map[int]chapterLocation)
	ch := 0
	for _, v := range volumes {
		firstOfVol := true
		for _, a := range v.Arcs {
			for range a.Chapters {
				ch++
				locs[ch] = chapterLocation{
					VolumeIdx:       v.Index,
					VolumeTitle:     v.Title,
					IsFirstOfVolume: firstOfVol,
				}
				firstOfVol = false
			}
		}
	}
	return locs
}

// chapterHeaderRe 匹配带章号的 Markdown 标题首行（# 第N章 / ## 第 12 章 ...）。
var chapterHeaderRe = regexp.MustCompile(`^#+\s+第.+?章`)

// atxTitleRe 提取 ATX 标题（# 标题）的文字部分。
var atxTitleRe = regexp.MustCompile(`^#{1,6}\s+(.+?)\s*$`)

// stripChapterTitleHeader 若首行是会与导出器统一标题重复的章节标题则剥掉。
// 两种情形：① "# 第N章 …"（带章号）；② markdown 标题且其文字恰是本章标题
// （writer 常把纯章节名当标题写进正文首行，如 "# 边村浮生"，与导出器生成的
// "第 N 章 边村浮生" 重复）。其它 h1（如 "# 序章"）视为正文一部分，保留。
// 调用方负责先 TrimSpace，因此前导空行不在考虑范围内。
func stripChapterTitleHeader(content, title string) string {
	first, rest, hasNewline := strings.Cut(content, "\n")
	if !isChapterTitleLine(first, title) {
		return content
	}
	if !hasNewline {
		return ""
	}
	return strings.TrimLeft(rest, "\n")
}

func isChapterTitleLine(line, title string) bool {
	if chapterHeaderRe.MatchString(line) {
		return true
	}
	if title = strings.TrimSpace(title); title == "" {
		return false
	}
	m := atxTitleRe.FindStringSubmatch(line)
	return len(m) == 2 && strings.TrimSpace(m[1]) == title
}

// renderTXT 拼接最终文本。
//
// 章节顺序由 chapters 决定（调用方已按章号升序去重）。bodies/titleIdx/locations
// 都按"缺失即降级"处理：标题缺失只输出 "第 N 章"；分层定位缺失就当扁平大纲。
func renderTXT(
	novelName string,
	chapters []int,
	titleIdx chapterTitleIndex,
	locations map[int]chapterLocation,
	bodies map[int]string,
) string {
	var b strings.Builder

	if name := strings.TrimSpace(novelName); name != "" {
		b.WriteString("《")
		b.WriteString(name)
		b.WriteString("》\n\n")
	}

	useLayered := len(locations) > 0

	for i, ch := range chapters {
		if useLayered {
			if loc, ok := locations[ch]; ok && loc.IsFirstOfVolume {
				b.WriteString("\n═══════════════════════════════════════════\n")
				fmt.Fprintf(&b, "           第 %d 卷  %s\n", loc.VolumeIdx, strings.TrimSpace(loc.VolumeTitle))
				b.WriteString("═══════════════════════════════════════════\n\n")
			}
		}

		title := strings.TrimSpace(titleIdx[ch])
		if title != "" {
			fmt.Fprintf(&b, "第 %d 章  %s\n\n", ch, title)
		} else {
			fmt.Fprintf(&b, "第 %d 章\n\n", ch)
		}

		body := stripChapterTitleHeader(strings.TrimSpace(bodies[ch]), title)
		b.WriteString(body)
		b.WriteString("\n")
		if i < len(chapters)-1 {
			b.WriteString("\n\n")
		}
	}
	return b.String()
}

// renderVietnameseTXT renders Vietnamese labels. Titles come from titleIdx
// (resolved before render so every chapter has a full name when available).
func renderVietnameseTXT(chapters []int, titleIdx chapterTitleIndex, bodies map[int]string) string {
	var b strings.Builder
	b.WriteString("BẢN DỊCH TIẾNG VIỆT\n\n")
	for i, ch := range chapters {
		body := strings.TrimSpace(bodies[ch])
		title := normalizeExportTitle(ch, LanguageVietnamese, titleIdx[ch])
		if title == "" {
			title = normalizeExportTitle(ch, LanguageVietnamese, extractVietnameseChapterTitle(body))
		}
		if title != "" {
			fmt.Fprintf(&b, "Chương %d  %s\n\n", ch, title)
			body = stripLeadingVietnameseTitle(body, title)
		} else {
			fmt.Fprintf(&b, "Chương %d\n\n", ch)
		}
		b.WriteString(body)
		b.WriteString("\n")
		if i < len(chapters)-1 {
			b.WriteString("\n\n")
		}
	}
	return b.String()
}

// resolveVietnameseTitles fills titleIdx for every chapter using, in order:
//  1. Stored ChapterRecord.Title
//  2. Explicit heading in the Vietnamese body
//  3. Glossary substitution of the Chinese summary/outline title
//  4. Optional TitleResolver (LLM batch) for leftovers
//  5. Bare "Chương N" (caller may still overwrite)
//
// Resolved titles are persisted back into the translation store when possible.
func resolveVietnameseTitles(
	deps Deps,
	chapters []int,
	bodies map[int]string,
	zhTitles chapterTitleIndex,
	records map[int]translation.ChapterRecord,
) (chapterTitleIndex, map[int]string) {
	titleIdx := make(chapterTitleIndex, len(chapters))
	persist := make(map[int]string)
	var glossary translation.Glossary
	if deps.Translation != nil {
		if g, err := deps.Translation.LoadGlossary(); err == nil {
			glossary = g
		}
	}

	needResolve := make(map[int]string) // ch → zh title
	for _, ch := range chapters {
		// 1) durable record (preferred — offline backfill writes here)
		if rec, ok := records[ch]; ok {
			if t := normalizeExportTitle(ch, LanguageVietnamese, rec.Title); t != "" {
				titleIdx[ch] = t
				bodies[ch] = stripLeadingVietnameseTitle(strings.TrimSpace(bodies[ch]), t)
				// Rewrite bad stored titles like "Chương 3"
				if strings.TrimSpace(rec.Title) != t {
					persist[ch] = t
				}
				continue
			}
		}
		// 2) body heading
		if t := normalizeExportTitle(ch, LanguageVietnamese, extractVietnameseChapterTitle(bodies[ch])); t != "" {
			titleIdx[ch] = t
			bodies[ch] = stripLeadingVietnameseTitle(strings.TrimSpace(bodies[ch]), t)
			persist[ch] = t
			continue
		}
		// 3) glossary on ZH title
		zh := strings.TrimSpace(zhTitles[ch])
		if zh != "" {
			if t := normalizeExportTitle(ch, LanguageVietnamese, translation.TranslateTitleWithGlossary(zh, glossary)); t != "" {
				titleIdx[ch] = t
				persist[ch] = t
				continue
			}
			needResolve[ch] = zh
		}
	}

	// 4) optional batch resolver (Host LLM) for every chapter still missing a real title.
	// Always merge whatever the resolver returned — even if it also returned an error
	// (partial batch success is common and must not be discarded).
	if len(needResolve) > 0 && deps.TitleResolver != nil {
		resolved, _ := deps.TitleResolver(needResolve)
		for ch, t := range resolved {
			t = normalizeExportTitle(ch, LanguageVietnamese, t)
			if t == "" {
				continue
			}
			titleIdx[ch] = t
			persist[ch] = t
			delete(needResolve, ch)
		}
	}

	// Persist newly discovered titles so the next export is instant.
	if deps.Translation != nil && len(persist) > 0 {
		_, _ = deps.Translation.SetChapterTitles(persist)
	}
	return titleIdx, bodies
}

// extractVietnameseChapterTitle returns a short non-CJK first-line title only when
// the translator put an *explicit* heading. Ordinary prose first lines must not
// be promoted (they are story text). Empty → TOC shows bare "Chương N".
//
// Accepted forms:
//   - Markdown ATX: "# Đêm mưa" / "## Chương 12: Đêm mưa"
//   - Plain "Chương 12: Đêm mưa" / "Chương 12 - Đêm mưa"
//
// Rejected: Chinese outline titles, bare "Chương 12", plain prose paragraphs.
func extractVietnameseChapterTitle(body string) string {
	body = strings.TrimSpace(body)
	if body == "" {
		return ""
	}
	first, _, _ := strings.Cut(body, "\n")
	first = strings.TrimSpace(first)
	if first == "" || looksMostlyHan(first) {
		return ""
	}

	explicitMarkdown := false
	if m := atxTitleRe.FindStringSubmatch(first); len(m) == 2 {
		first = strings.TrimSpace(m[1])
		explicitMarkdown = true
	}

	lower := strings.ToLower(first)
	if strings.HasPrefix(lower, "chương") {
		fields := strings.Fields(first)
		if len(fields) < 3 {
			return "" // "Chương 12" alone
		}
		// Chương <n> [sep] <title...>
		rest := strings.TrimSpace(strings.Join(fields[2:], " "))
		rest = strings.TrimLeft(rest, ":.-–— ")
		first = rest
		if first == "" {
			return ""
		}
		// "Chương N …" counts as explicit even without markdown.
		explicitMarkdown = true
	}

	// Without markdown or "Chương N …" prefix, do not invent titles from prose.
	if !explicitMarkdown {
		return ""
	}

	first = strings.TrimSpace(first)
	if looksMostlyHan(first) {
		return ""
	}
	runes := []rune(first)
	if len(runes) < 2 || len(runes) > 48 {
		return ""
	}
	hasLetter := false
	for _, r := range runes {
		if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= 'À' && r <= 'ỹ') {
			hasLetter = true
			break
		}
	}
	if !hasLetter {
		return ""
	}
	return first
}

func stripLeadingVietnameseTitle(body, title string) string {
	body = strings.TrimSpace(body)
	title = strings.TrimSpace(title)
	if body == "" || title == "" {
		return body
	}
	first, rest, hasNL := strings.Cut(body, "\n")
	firstTrim := strings.TrimSpace(first)
	if m := atxTitleRe.FindStringSubmatch(firstTrim); len(m) == 2 {
		firstTrim = strings.TrimSpace(m[1])
	}
	// Exact title, or "Chương N Title"
	if firstTrim == title || strings.HasSuffix(firstTrim, title) && strings.Contains(strings.ToLower(firstTrim), "chương") {
		if !hasNL {
			return ""
		}
		return strings.TrimLeft(rest, "\n")
	}
	return body
}

func looksMostlyHan(s string) bool {
	han, other := 0, 0
	for _, r := range s {
		switch {
		case r >= 0x4E00 && r <= 0x9FFF:
			han++
		case r == ' ' || r == '\t' || r == '#' || r == ':' || r == '-' || r == '–' || r == '—':
			// ignore
		case (r >= '0' && r <= '9'):
			// ignore digits in "Chương 12"
		default:
			other++
		}
	}
	if han == 0 {
		return false
	}
	return han >= other
}
