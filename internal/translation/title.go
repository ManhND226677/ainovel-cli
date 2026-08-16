package translation

import (
	"regexp"
	"sort"
	"strings"
	"unicode"
)

var (
	atxTitleLine = regexp.MustCompile(`^#{1,6}\s+(.+?)\s*$`)
	chuongLine   = regexp.MustCompile(`(?i)^chương\s+\d+\s*[:.\-–—]?\s*(.+)$`)
)

// ExtractTitleFromBody returns an explicit Vietnamese heading from chapter text,
// or empty if the body starts with ordinary prose.
func ExtractTitleFromBody(body string) string {
	body = strings.TrimSpace(body)
	if body == "" {
		return ""
	}
	first, _, _ := strings.Cut(body, "\n")
	first = strings.TrimSpace(first)
	if first == "" || looksMostlyHan(first) {
		return ""
	}
	if m := atxTitleLine.FindStringSubmatch(first); len(m) == 2 {
		t := strings.TrimSpace(m[1])
		// "# Chương 12: Foo" → strip chapter prefix
		if m2 := chuongLine.FindStringSubmatch(t); len(m2) == 2 {
			t = strings.TrimSpace(m2[1])
		}
		return cleanTitle(t)
	}
	if m := chuongLine.FindStringSubmatch(first); len(m) == 2 {
		return cleanTitle(m[1])
	}
	return ""
}

func cleanTitle(t string) string {
	t = strings.TrimSpace(t)
	t = strings.Trim(t, "#*-–—: ")
	runes := []rune(t)
	if len(runes) < 2 || len(runes) > 48 {
		return ""
	}
	if looksMostlyHan(t) {
		return ""
	}
	hasLetter := false
	for _, r := range runes {
		if unicode.IsLetter(r) {
			hasLetter = true
			break
		}
	}
	if !hasLetter {
		return ""
	}
	return t
}

func looksMostlyHan(s string) bool {
	han, other := 0, 0
	for _, r := range s {
		switch {
		case unicode.In(r, unicode.Han):
			han++
		case unicode.IsLetter(r):
			other++
		}
	}
	if han == 0 {
		return false
	}
	return han >= other
}

// TranslateTitleWithGlossary applies longest-match glossary substitution on a
// Chinese title. Returns empty if the result still looks mostly Chinese (i.e.
// glossary coverage was insufficient).
func TranslateTitleWithGlossary(zh string, glossary Glossary) string {
	zh = strings.TrimSpace(zh)
	if zh == "" {
		return ""
	}
	terms := glossary.Terms
	if len(terms) == 0 {
		return ""
	}
	// Longest source first.
	type pair struct {
		src, vi string
	}
	list := make([]pair, 0, len(terms))
	for src, e := range terms {
		src = strings.TrimSpace(src)
		vi := strings.TrimSpace(e.Vietnamese)
		if src == "" || vi == "" {
			continue
		}
		list = append(list, pair{src, vi})
	}
	sort.Slice(list, func(i, j int) bool {
		return len([]rune(list[i].src)) > len([]rune(list[j].src))
	})

	// Greedy left-to-right longest match over runes.
	runes := []rune(zh)
	var b strings.Builder
	i := 0
	matchedAny := false
	for i < len(runes) {
		matched := false
		rest := string(runes[i:])
		for _, p := range list {
			if strings.HasPrefix(rest, p.src) {
				b.WriteString(p.vi)
				i += len([]rune(p.src))
				matched = true
				matchedAny = true
				break
			}
		}
		if matched {
			continue
		}
		// Keep punctuation / spaces; drop bare Han chars that didn't match
		// so residual Chinese does not leak into TOC. Latin digits stay.
		r := runes[i]
		if unicode.In(r, unicode.Han) {
			// skip unmatched Han — marks incomplete translation
			i++
			continue
		}
		b.WriteRune(r)
		i++
	}
	out := strings.Join(strings.Fields(b.String()), " ")
	out = strings.Trim(out, " -–—:·、，。")
	if !matchedAny || out == "" || looksMostlyHan(out) {
		return ""
	}
	// Must have at least one letter
	hasLetter := false
	for _, r := range out {
		if unicode.IsLetter(r) {
			hasLetter = true
			break
		}
	}
	if !hasLetter {
		return ""
	}
	return out
}
