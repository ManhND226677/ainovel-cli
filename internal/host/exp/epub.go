package exp

import (
	"archive/zip"
	"bytes"
	"crypto/sha1"
	"fmt"
	"html"
	"path"
	"regexp"
	"strings"
	"time"
)

// epubMeta carries package-level metadata for EPUB 3 rendering.
type epubMeta struct {
	Title          string
	Author         string
	Description    string
	Language       Language // zh | vi
	CoverImage     []byte
	CoverMediaType string
}

func (m epubMeta) xmlLang() string {
	if m.Language == LanguageVietnamese {
		return "vi"
	}
	return "zh-CN"
}

func (m epubMeta) chapterLabel(ch int, title string) string {
	title = normalizeExportTitle(ch, m.Language, title)
	if m.Language == LanguageVietnamese {
		if title != "" {
			return fmt.Sprintf("Chương %d %s", ch, title)
		}
		return fmt.Sprintf("Chương %d", ch)
	}
	if title != "" {
		return fmt.Sprintf("第 %d 章 %s", ch, title)
	}
	return fmt.Sprintf("第 %d 章", ch)
}

// normalizeExportTitle strips chapter-number prefixes and rejects titles that are
// only "Chương N" / "第 N 章" so TOC never becomes "Chương 1 Chương 1".
func normalizeExportTitle(ch int, lang Language, title string) string {
	title = strings.TrimSpace(title)
	if title == "" {
		return ""
	}
	// Exact bare labels
	if lang == LanguageVietnamese {
		if strings.EqualFold(title, fmt.Sprintf("Chương %d", ch)) ||
			strings.EqualFold(title, fmt.Sprintf("Chuong %d", ch)) {
			return ""
		}
		// "Chương 12: Foo" / "Chương 12 Foo"
		lower := strings.ToLower(title)
		prefix := strings.ToLower(fmt.Sprintf("chương %d", ch))
		prefix2 := fmt.Sprintf("chuong %d", ch)
		if strings.HasPrefix(lower, prefix) || strings.HasPrefix(lower, prefix2) {
			rest := strings.TrimSpace(title)
			// drop first two fields
			fields := strings.Fields(rest)
			if len(fields) >= 3 {
				rest = strings.TrimSpace(strings.Join(fields[2:], " "))
				rest = strings.TrimLeft(rest, ":.-–— ")
				title = rest
			} else {
				return ""
			}
		}
	} else {
		bare := fmt.Sprintf("第 %d 章", ch)
		bare2 := fmt.Sprintf("第%d章", ch)
		if title == bare || title == bare2 {
			return ""
		}
		if strings.HasPrefix(title, bare) {
			title = strings.TrimSpace(strings.TrimPrefix(title, bare))
		} else if strings.HasPrefix(title, bare2) {
			title = strings.TrimSpace(strings.TrimPrefix(title, bare2))
		}
	}
	title = strings.TrimSpace(title)
	title = strings.Trim(title, ":.-–— ")
	if title == "" {
		return ""
	}
	// After stripping a "Chương N …" prefix, reject if what remains is still only a bare label.
	if lang == LanguageVietnamese {
		if strings.EqualFold(title, fmt.Sprintf("Chương %d", ch)) ||
			strings.EqualFold(title, fmt.Sprintf("Chuong %d", ch)) ||
			strings.EqualFold(title, "Chương") ||
			strings.EqualFold(title, "Chuong") {
			return ""
		}
		// "Chương 1" leftover from "Chương 1 Chương 1"
		if ok, _ := regexp.MatchString(`(?i)^chương\s+\d+$`, title); ok {
			return ""
		}
	}
	// Reject leftover Chinese for VI TOC
	if lang == LanguageVietnamese && looksMostlyHan(title) {
		return ""
	}
	return title
}

func (m epubMeta) volumeLabel(idx int, title string) string {
	title = strings.TrimSpace(title)
	if m.Language == LanguageVietnamese {
		if title != "" {
			return fmt.Sprintf("Tập %d %s", idx, title)
		}
		return fmt.Sprintf("Tập %d", idx)
	}
	if title != "" {
		return fmt.Sprintf("第 %d 卷 %s", idx, title)
	}
	return fmt.Sprintf("第 %d 卷", idx)
}

func (m epubMeta) coverExt() (name, mediaType string) {
	mt := strings.TrimSpace(m.CoverMediaType)
	if mt == "" {
		mt = sniffImageMediaType(m.CoverImage)
	}
	switch mt {
	case "image/png":
		return "cover.png", mt
	case "image/webp":
		return "cover.webp", mt
	case "image/gif":
		return "cover.gif", mt
	default:
		return "cover.jpg", "image/jpeg"
	}
}

// renderEPUB 把章节集合打包成 EPUB 3 字节流。
func renderEPUB(
	meta epubMeta,
	chapters []int,
	titleIdx chapterTitleIndex,
	locations map[int]chapterLocation,
	bodies map[int]string,
) ([]byte, error) {
	if strings.TrimSpace(meta.Author) == "" {
		meta.Author = "ainovel-cli"
	}
	if meta.Language == "" {
		meta.Language = LanguageChinese
	}

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	mt, err := zw.CreateHeader(&zip.FileHeader{
		Name:   "mimetype",
		Method: zip.Store,
	})
	if err != nil {
		return nil, fmt.Errorf("create mimetype: %w", err)
	}
	if _, err := mt.Write([]byte("application/epub+zip")); err != nil {
		return nil, err
	}

	if err := zipDeflate(zw, "META-INF/container.xml", containerXML); err != nil {
		return nil, err
	}
	if err := zipDeflate(zw, "OEBPS/style.css", styleCSS); err != nil {
		return nil, err
	}

	hasTextCover := strings.TrimSpace(meta.Title) != ""
	hasImageCover := len(meta.CoverImage) > 0
	var coverImgName, coverImgType string
	if hasImageCover {
		coverImgName, coverImgType = meta.coverExt()
		if err := zipBytes(zw, "OEBPS/"+coverImgName, meta.CoverImage); err != nil {
			return nil, err
		}
	}
	if hasTextCover || hasImageCover {
		if err := zipDeflate(zw, "OEBPS/cover.xhtml", renderCoverXHTML(meta, hasImageCover, coverImgName)); err != nil {
			return nil, err
		}
	}

	// Group TOC by volume when locations exist.
	for _, ch := range chapters {
		loc, hasLoc := locations[ch]
		title := strings.TrimSpace(titleIdx[ch])
		body := stripChapterTitleHeader(strings.TrimSpace(bodies[ch]), title)
		xhtml := renderChapterXHTML(meta, ch, title, loc, hasLoc, body)
		if err := zipDeflate(zw, "OEBPS/"+chapterFileName(ch), xhtml); err != nil {
			return nil, err
		}
	}

	if err := zipDeflate(zw, "OEBPS/nav.xhtml", renderNavXHTML(meta, hasTextCover || hasImageCover, chapters, titleIdx, locations)); err != nil {
		return nil, err
	}

	if err := zipDeflate(zw, "OEBPS/content.opf", renderOPF(meta, hasTextCover || hasImageCover, hasImageCover, coverImgName, coverImgType, chapters)); err != nil {
		return nil, err
	}

	if err := zw.Close(); err != nil {
		return nil, fmt.Errorf("finalize zip: %w", err)
	}
	return buf.Bytes(), nil
}

func zipDeflate(zw *zip.Writer, name, content string) error {
	w, err := zw.Create(name)
	if err != nil {
		return fmt.Errorf("create %s: %w", name, err)
	}
	_, err = w.Write([]byte(content))
	return err
}

func zipBytes(zw *zip.Writer, name string, data []byte) error {
	w, err := zw.Create(name)
	if err != nil {
		return fmt.Errorf("create %s: %w", name, err)
	}
	_, err = w.Write(data)
	return err
}

func chapterFileName(ch int) string {
	return fmt.Sprintf("chapter%03d.xhtml", ch)
}

func chapterID(ch int) string {
	return fmt.Sprintf("ch%03d", ch)
}

const containerXML = `<?xml version="1.0" encoding="utf-8"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles>
    <rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/>
  </rootfiles>
</container>
`

const styleCSS = `body { font-family: "Noto Serif", "Source Han Serif", "Songti SC", "Times New Roman", serif; line-height: 1.75; margin: 1.2em 1em; color: #1a1a1a; }
h1.book-title { font-size: 2em; text-align: center; margin: 3.5em 0 0.6em; letter-spacing: 0.04em; }
p.book-author { text-align: center; color: #555; font-size: 1.05em; margin: 0.4em 0 2em; text-indent: 0; }
p.book-blurb { text-align: center; color: #666; font-size: 0.95em; margin: 1.5em 10%; text-indent: 0; line-height: 1.5; }
.cover-image { display: block; max-width: 90%; max-height: 85vh; margin: 1.5em auto; }
.volume-divider { font-size: 1.45em; text-align: center; margin: 3.2em 0 1em; font-weight: bold; }
h1.chapter-title { font-size: 1.35em; text-align: center; margin: 1.8em 0 1.4em; }
p { text-indent: 2em; margin: 0.45em 0; }
nav ol { list-style: none; padding-left: 0; }
nav li { margin: 0.35em 0; }
nav .vol { font-weight: bold; margin-top: 1em; }
`

func renderChapterXHTML(meta epubMeta, ch int, title string, loc chapterLocation, hasLoc bool, body string) string {
	var b strings.Builder
	displayTitle := meta.chapterLabel(ch, title)
	lang := meta.xmlLang()

	fmt.Fprintf(&b, `<?xml version="1.0" encoding="utf-8"?>
<!DOCTYPE html>
<html xmlns="http://www.w3.org/1999/xhtml" xml:lang="%s" lang="%s">
<head>
  <title>%s</title>
  <link rel="stylesheet" type="text/css" href="style.css"/>
</head>
<body>
`, lang, lang, html.EscapeString(displayTitle))

	if hasLoc && loc.IsFirstOfVolume {
		fmt.Fprintf(&b, "  <div class=\"volume-divider\">%s</div>\n",
			html.EscapeString(meta.volumeLabel(loc.VolumeIdx, loc.VolumeTitle)))
	}

	fmt.Fprintf(&b, "  <h1 class=\"chapter-title\">%s</h1>\n", html.EscapeString(displayTitle))
	for _, para := range splitParagraphs(body) {
		fmt.Fprintf(&b, "  <p>%s</p>\n", html.EscapeString(para))
	}
	b.WriteString("</body>\n</html>\n")
	return b.String()
}

func splitParagraphs(body string) []string {
	body = strings.ReplaceAll(body, "\r\n", "\n")
	parts := strings.Split(body, "\n\n")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		p = strings.ReplaceAll(p, "\n", " ")
		out = append(out, p)
	}
	return out
}

func renderCoverXHTML(meta epubMeta, hasImage bool, imgName string) string {
	lang := meta.xmlLang()
	title := strings.TrimSpace(meta.Title)
	coverTitle := "封面"
	if meta.Language == LanguageVietnamese {
		coverTitle = "Bìa"
	}
	var b strings.Builder
	fmt.Fprintf(&b, `<?xml version="1.0" encoding="utf-8"?>
<!DOCTYPE html>
<html xmlns="http://www.w3.org/1999/xhtml" xml:lang="%s" lang="%s">
<head>
  <title>%s</title>
  <link rel="stylesheet" type="text/css" href="style.css"/>
</head>
<body>
`, lang, lang, html.EscapeString(coverTitle))
	if hasImage && imgName != "" {
		fmt.Fprintf(&b, "  <img class=\"cover-image\" src=\"%s\" alt=\"%s\"/>\n",
			html.EscapeString(path.Base(imgName)), html.EscapeString(title))
	}
	if title != "" {
		fmt.Fprintf(&b, "  <h1 class=\"book-title\">%s</h1>\n", html.EscapeString(title))
	}
	if a := strings.TrimSpace(meta.Author); a != "" && a != "ainovel-cli" {
		fmt.Fprintf(&b, "  <p class=\"book-author\">%s</p>\n", html.EscapeString(a))
	}
	if d := strings.TrimSpace(meta.Description); d != "" {
		fmt.Fprintf(&b, "  <p class=\"book-blurb\">%s</p>\n", html.EscapeString(d))
	}
	b.WriteString("</body>\n</html>\n")
	return b.String()
}

func renderNavXHTML(meta epubMeta, hasCover bool, chapters []int, titleIdx chapterTitleIndex, locations map[int]chapterLocation) string {
	lang := meta.xmlLang()
	tocTitle := "目录"
	coverLabel := "封面"
	if meta.Language == LanguageVietnamese {
		tocTitle = "Mục lục"
		coverLabel = "Bìa"
	}
	var b strings.Builder
	fmt.Fprintf(&b, `<?xml version="1.0" encoding="utf-8"?>
<!DOCTYPE html>
<html xmlns="http://www.w3.org/1999/xhtml" xmlns:epub="http://www.idpf.org/2007/ops" xml:lang="%s" lang="%s">
<head>
  <title>%s</title>
  <link rel="stylesheet" type="text/css" href="style.css"/>
</head>
<body>
  <nav epub:type="toc">
    <h1>%s</h1>
    <ol>
`, lang, lang, html.EscapeString(tocTitle), html.EscapeString(tocTitle))
	if hasCover {
		fmt.Fprintf(&b, "      <li><a href=\"cover.xhtml\">%s</a></li>\n", html.EscapeString(coverLabel))
	}

	var lastVol int
	volOpen := false
	for _, ch := range chapters {
		if loc, ok := locations[ch]; ok && loc.IsFirstOfVolume {
			if volOpen {
				b.WriteString("      </ol></li>\n")
			}
			fmt.Fprintf(&b, "      <li class=\"vol\"><span>%s</span>\n      <ol>\n",
				html.EscapeString(meta.volumeLabel(loc.VolumeIdx, loc.VolumeTitle)))
			volOpen = true
			lastVol = loc.VolumeIdx
			_ = lastVol
		}
		display := meta.chapterLabel(ch, titleIdx[ch])
		fmt.Fprintf(&b, "      <li><a href=\"%s\">%s</a></li>\n",
			chapterFileName(ch), html.EscapeString(display))
	}
	if volOpen {
		b.WriteString("      </ol></li>\n")
	}

	b.WriteString(`    </ol>
  </nav>
</body>
</html>
`)
	return b.String()
}

func renderOPF(meta epubMeta, hasCover, hasImage bool, imgName, imgType string, chapters []int) string {
	bookID := bookIdentifier(meta.Title + "|" + string(meta.Language))
	modified := time.Now().UTC().Format("2006-01-02T15:04:05Z")
	title := strings.TrimSpace(meta.Title)
	if title == "" {
		title = "Untitled"
	}
	lang := meta.xmlLang()
	author := strings.TrimSpace(meta.Author)
	if author == "" {
		author = "ainovel-cli"
	}

	var b strings.Builder
	fmt.Fprintf(&b, `<?xml version="1.0" encoding="utf-8"?>
<package xmlns="http://www.idpf.org/2007/opf" version="3.0" unique-identifier="bookid" xml:lang="%s">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
    <dc:identifier id="bookid">%s</dc:identifier>
    <dc:title>%s</dc:title>
    <dc:language>%s</dc:language>
    <dc:creator id="creator">%s</dc:creator>
    <meta refines="#creator" property="role" scheme="marc:relators">aut</meta>
`, lang, html.EscapeString(bookID), html.EscapeString(title), html.EscapeString(lang), html.EscapeString(author))
	if d := strings.TrimSpace(meta.Description); d != "" {
		fmt.Fprintf(&b, "    <dc:description>%s</dc:description>\n", html.EscapeString(d))
	}
	fmt.Fprintf(&b, "    <meta property=\"dcterms:modified\">%s</meta>\n", modified)
	if hasImage {
		b.WriteString("    <meta name=\"cover\" content=\"cover-image\"/>\n")
	}
	b.WriteString(`  </metadata>
  <manifest>
    <item id="nav" href="nav.xhtml" media-type="application/xhtml+xml" properties="nav"/>
    <item id="css" href="style.css" media-type="text/css"/>
`)
	if hasCover {
		b.WriteString(`    <item id="cover" href="cover.xhtml" media-type="application/xhtml+xml"/>` + "\n")
	}
	if hasImage {
		fmt.Fprintf(&b, `    <item id="cover-image" href="%s" media-type="%s" properties="cover-image"/>`+"\n",
			html.EscapeString(imgName), html.EscapeString(imgType))
	}
	for _, ch := range chapters {
		fmt.Fprintf(&b, `    <item id="%s" href="%s" media-type="application/xhtml+xml"/>`+"\n",
			chapterID(ch), chapterFileName(ch))
	}

	b.WriteString("  </manifest>\n  <spine>\n")
	if hasCover {
		b.WriteString(`    <itemref idref="cover"/>` + "\n")
	}
	b.WriteString(`    <itemref idref="nav"/>` + "\n")
	for _, ch := range chapters {
		fmt.Fprintf(&b, `    <itemref idref="%s"/>`+"\n", chapterID(ch))
	}
	b.WriteString("  </spine>\n</package>\n")
	return b.String()
}

func bookIdentifier(seed string) string {
	h := sha1.New()
	h.Write([]byte(seed))
	sum := h.Sum(nil)
	return fmt.Sprintf("urn:uuid:%x-%x-%x-%x-%x",
		sum[0:4], sum[4:6], sum[6:8], sum[8:10], sum[10:16])
}

func sniffImageMediaType(data []byte) string {
	if len(data) >= 3 && data[0] == 0xff && data[1] == 0xd8 && data[2] == 0xff {
		return "image/jpeg"
	}
	if len(data) >= 8 && bytes.Equal(data[:8], []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a}) {
		return "image/png"
	}
	if len(data) >= 12 && string(data[:4]) == "RIFF" && string(data[8:12]) == "WEBP" {
		return "image/webp"
	}
	if len(data) >= 6 && (string(data[:6]) == "GIF87a" || string(data[:6]) == "GIF89a") {
		return "image/gif"
	}
	return "image/jpeg"
}
