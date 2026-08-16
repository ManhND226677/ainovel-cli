package webapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/voocel/ainovel-cli/internal/host/exp"
	"github.com/voocel/ainovel-cli/internal/library"
)

// contentDispositionAttachment builds a header that keeps Unicode filenames
// (VI/ZH titles) working in Chromium/Firefox/Safari.
//
// Chromium prefers filename*=UTF-8”… when present. The plain filename= value is
// only a legacy ASCII fallback — it must NEVER collapse to the generic
// "export.epub" when the real name is non-ASCII (that is what users were seeing).
func contentDispositionAttachment(filename string) string {
	filename = strings.TrimSpace(filename)
	filename = strings.ReplaceAll(filename, `"`, `'`)
	if filename == "" {
		filename = "ban-thao.epub"
	}
	ext := filepath.Ext(filename)
	base := strings.TrimSuffix(filename, ext)
	if ext == "" {
		ext = ".epub"
	}

	// Build a readable ASCII fallback from Latin letters/digits in the title.
	var ascii strings.Builder
	prevDash := false
	for _, r := range base {
		switch {
		case r >= 'A' && r <= 'Z', r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			ascii.WriteRune(r)
			prevDash = false
		case r == ' ' || r == '-' || r == '_' || r == '.':
			if !prevDash && ascii.Len() > 0 {
				ascii.WriteByte('-')
				prevDash = true
			}
		default:
			// strip CJK / accents from the legacy slot only
		}
	}
	fb := strings.Trim(ascii.String(), "-._")
	if fb == "" {
		// Pure CJK title: still avoid "export.epub" — use a descriptive stub + ext.
		if looksMostlyHanFilename(base) {
			fb = "ban-thao-trung"
		} else {
			fb = "ban-dich-viet"
		}
	}
	// RFC 5987: filename* uses percent-encoding of UTF-8 bytes.
	encoded := url.PathEscape(filename)
	// Some clients mishandle PathEscape '+' ; titles shouldn't have bare + often.
	return fmt.Sprintf(`attachment; filename="%s%s"; filename*=UTF-8''%s`, fb, ext, encoded)
}

func looksMostlyHanFilename(s string) bool {
	han, other := 0, 0
	for _, r := range s {
		if unicode.In(r, unicode.Han) {
			han++
		} else if unicode.IsLetter(r) {
			other++
		}
	}
	return han > 0 && han >= other
}

func (s *Server) libraryRoot() string {
	// Prefer project-local ./library next to cwd (same as CLI).
	return library.DefaultRoot
}

func (s *Server) openLibrary() (*library.Index, error) {
	return library.Open(s.libraryRoot())
}

func (s *Server) libraryHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.libraryList(w, r)
	case http.MethodPost:
		s.libraryCreate(w, r)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) libraryList(w http.ResponseWriter, r *http.Request) {
	idx, err := s.openLibrary()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	// Ensure current active book is registered.
	active := ""
	if s.runtime != nil {
		active = s.runtime.ActiveBookDir()
		if active != "" {
			if _, err := idx.EnsureRegistered(active, ""); err == nil {
				_ = idx.Save()
			}
		}
	}
	books := idx.List(active)
	writeJSON(w, http.StatusOK, map[string]any{
		"root":      idx.Root,
		"active_id": idx.ActiveID,
		"books":     books,
	})
}

type libraryCreateBody struct {
	Title       string `json:"title"`
	TitleVI     string `json:"title_vi"`
	Author      string `json:"author"`
	Description string `json:"description"`
	Slug        string `json:"slug"`
	Open        bool   `json:"open"` // switch immediately after create
}

func (s *Server) libraryCreate(w http.ResponseWriter, r *http.Request) {
	var body libraryCreateBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	in := library.CreateInput{
		Title: body.Title, TitleVI: body.TitleVI,
		Author: body.Author, Description: body.Description, Slug: body.Slug,
	}
	if body.Open {
		book, err := s.runtime.CreateBookAndOpen(s.libraryRoot(), in)
		if err != nil {
			// Distinguish create vs switch failures when possible.
			if strings.Contains(err.Error(), "but switch failed") {
				writeJSON(w, http.StatusCreated, map[string]any{
					"book":  book,
					"open":  false,
					"error": err.Error(),
				})
				return
			}
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{"book": book, "open": true})
		return
	}
	idx, err := s.openLibrary()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	book, err := idx.Create(in)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"book": book, "open": false})
}

func (s *Server) libraryOpen(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var body struct {
		ID   string `json:"id"`
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	idx, err := s.openLibrary()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	path := strings.TrimSpace(body.Path)
	id := strings.TrimSpace(body.ID)
	if path == "" && id != "" {
		b, ok := idx.Find(id)
		if !ok {
			writeError(w, http.StatusNotFound, "book not found")
			return
		}
		path = b.Path
		id = b.ID
	}
	if path == "" {
		writeError(w, http.StatusBadRequest, "id or path required")
		return
	}
	if err := s.runtime.SwitchBook(path); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	if id == "" {
		if b, err := idx.EnsureRegistered(path, ""); err == nil {
			id = b.ID
		}
	}
	if id != "" {
		_ = idx.SetActive(id)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":   true,
		"id":   id,
		"path": path,
		"dir":  s.runtime.ActiveBookDir(),
	})
}

func (s *Server) libraryArchive(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var body struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.ID) == "" {
		writeError(w, http.StatusBadRequest, "id required")
		return
	}
	idx, err := s.openLibrary()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	b, ok := idx.Find(body.ID)
	if !ok {
		writeError(w, http.StatusNotFound, "book not found")
		return
	}
	// Refuse archiving the currently open book without switching first.
	if samePathAPI(b.Path, s.runtime.ActiveBookDir()) {
		writeError(w, http.StatusConflict, "cannot archive the active novel — open another first")
		return
	}
	if err := idx.Archive(body.ID); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) libraryActive(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	sum := s.runtime.InspectActiveBook()
	writeJSON(w, http.StatusOK, map[string]any{
		"active": sum,
		"dir":    s.runtime.ActiveBookDir(),
	})
}

func (s *Server) libraryCreateAndStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var body struct {
		Title       string `json:"title"`
		TitleVI     string `json:"title_vi"`
		Author      string `json:"author"`
		Description string `json:"description"`
		Slug        string `json:"slug"`
		Prompt      string `json:"prompt"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	prompt := strings.TrimSpace(body.Prompt)
	if prompt == "" {
		writeError(w, http.StatusBadRequest, "prompt is required")
		return
	}
	title := strings.TrimSpace(body.Title)
	if title == "" {
		runes := []rune(prompt)
		if len(runes) > 40 {
			title = string(runes[:40])
		} else {
			title = prompt
		}
	}
	book, err := s.runtime.CreateBookAndOpen(s.libraryRoot(), library.CreateInput{
		Title: title, TitleVI: body.TitleVI, Author: body.Author, Description: body.Description, Slug: body.Slug,
	})
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	if err := s.runtime.PrepareUserRules(prompt); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	if err := s.runtime.StartPrepared(prompt); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{
		"ok":   true,
		"book": book,
		"dir":  s.runtime.ActiveBookDir(),
	})
}

func samePathAPI(a, b string) bool {
	aa, e1 := filepath.Abs(a)
	bb, e2 := filepath.Abs(b)
	if e1 != nil || e2 != nil {
		return filepath.Clean(a) == filepath.Clean(b)
	}
	return filepath.Clean(aa) == filepath.Clean(bb)
}

// ── Export ──────────────────────────────────────────────

func (s *Server) exportHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var body struct {
		Format    string `json:"format"`   // txt | epub
		Language  string `json:"language"` // zh | vi
		From      int    `json:"from"`
		To        int    `json:"to"`
		Author    string `json:"author"`
		Title     string `json:"title"`
		Download  bool   `json:"download"` // stream file to browser
		Overwrite bool   `json:"overwrite"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	opts := exp.Options{
		From: body.From, To: body.To,
		Author: body.Author, Title: body.Title,
		Overwrite: true, // dashboard writes into book dir exports/
	}
	switch strings.ToLower(strings.TrimSpace(body.Format)) {
	case "", "epub":
		opts.Format = exp.FormatEPUB
	case "txt":
		opts.Format = exp.FormatTXT
	default:
		writeError(w, http.StatusBadRequest, "format must be txt or epub")
		return
	}
	switch strings.ToLower(strings.TrimSpace(body.Language)) {
	case "", "zh", "trung":
		opts.Language = exp.LanguageChinese
	case "vi", "viet":
		opts.Language = exp.LanguageVietnamese
	default:
		writeError(w, http.StatusBadRequest, "language must be zh or vi")
		return
	}

	// Write under {book}/exports/ using the language-appropriate novel title
	// (ZH name for Chinese export, VI name for Vietnamese export).
	dir := s.runtime.ActiveBookDir()
	exportDir := filepath.Join(dir, "exports")
	_ = os.MkdirAll(exportDir, 0o755)
	ext := string(opts.Format)
	if strings.TrimSpace(opts.Title) == "" {
		opts.Title = s.runtime.ExportDisplayTitle(opts.Language)
	}
	base := exp.SanitizeFileName(opts.Title)
	if base == "" || base == "novel" {
		if opts.Language == exp.LanguageVietnamese {
			base = "ban-dich-viet"
		} else {
			base = "ban-thao-trung"
		}
	}
	opts.OutPath = filepath.Join(exportDir, base+"."+ext)

	res, err := s.runtime.Export(r.Context(), opts)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if body.Download {
		ct := "text/plain; charset=utf-8"
		if opts.Format == exp.FormatEPUB {
			ct = "application/epub+zip"
		}
		fname := filepath.Base(res.Path)
		w.Header().Set("Content-Type", ct)
		// ASCII-safe percent-encoded UTF-8 (HTTP headers are not reliably UTF-8).
		// Dashboard decodes with decodeURIComponent. Also set RFC 5987 filename*.
		w.Header().Set("X-Export-Filename", url.PathEscape(fname))
		w.Header().Set("Content-Disposition", contentDispositionAttachment(fname))
		http.ServeFile(w, r, res.Path)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"path":     res.Path,
		"chapters": res.Chapters,
		"bytes":    res.Bytes,
		"skipped":  res.Skipped,
		"format":   opts.Format,
		"language": opts.Language,
	})
}
