package host

import (
	"path/filepath"
	"testing"

	"github.com/voocel/ainovel-cli/internal/domain"
	storepkg "github.com/voocel/ainovel-cli/internal/store"
)

func TestLoadChapterManuscript_ChineseOnly(t *testing.T) {
	dir := t.TempDir()
	st := storepkg.NewStore(filepath.Join(dir, "book"))

	if err := st.Init(); err != nil {
		t.Fatal(err)
	}
	if err := st.Progress.Init("Thử nghiệm", 3); err != nil {
		t.Fatal(err)
	}
	if err := st.Outline.SaveOutline([]domain.OutlineEntry{{
		Chapter: 1, Title: "Mở đầu", CoreEvent: "Gió nổi",
	}}); err != nil {
		t.Fatal(err)
	}
	if err := st.Drafts.SaveFinalChapter(1, "第一章正文。"); err != nil {
		t.Fatal(err)
	}
	if err := st.Progress.MarkChapterComplete(1, 6, "", ""); err != nil {
		t.Fatal(err)
	}

	h := &Host{store: st}
	got, err := h.LoadChapterManuscript(1)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Completed || got.Title != "Mở đầu" || got.Chinese != "第一章正文。" || got.ChineseChars != 6 {
		t.Fatalf("unexpected manuscript: %+v", got)
	}
	if got.Vietnamese != "" {
		t.Fatalf("expected empty vietnamese, got %q", got.Vietnamese)
	}

	outline, err := h.ManuscriptOutline()
	if err != nil {
		t.Fatal(err)
	}
	if len(outline) == 0 || outline[0].Chapter != 1 || !outline[0].Completed {
		t.Fatalf("outline = %+v", outline)
	}
}
