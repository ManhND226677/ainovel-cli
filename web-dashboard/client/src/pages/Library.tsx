import { useCallback, useEffect, useState } from "react";
import { Link } from "wouter";
import { Archive, ArrowLeft, BookPlus, FolderOpen, Library as LibraryIcon, Loader2, PenLine } from "lucide-react";
import { toast } from "sonner";
import {
  archiveLibraryBook,
  createLibraryBook,
  createLibraryBookAndStart,
  fetchLibrary,
  openLibraryBook,
  type LibraryBook,
  type LibraryResponse,
} from "@/lib/engineApi";
import { useEngine } from "@/contexts/EngineContext";

export default function LibraryPage() {
  const { refresh } = useEngine();
  const [data, setData] = useState<LibraryResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [busyId, setBusyId] = useState<string | null>(null);
  const [creating, setCreating] = useState<"open" | "start" | null>(null);
  const [form, setForm] = useState({ title: "", title_vi: "", author: "", description: "", prompt: "" });

  const load = useCallback(async () => {
    setLoading(true);
    try {
      setData(await fetchLibrary());
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "Không tải được thư viện");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  async function handleOpen(book: LibraryBook) {
    setBusyId(book.id);
    try {
      await openLibraryBook({ id: book.id });
      toast.success(`Đã mở: ${book.title}`);
      await refresh();
      await load();
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "Không mở được truyện");
    } finally {
      setBusyId(null);
    }
  }

  async function handleArchive(book: LibraryBook) {
    if (!window.confirm(`Cất "${book.title}" vào archive? Thư mục truyện được giữ nguyên, chỉ ẩn khỏi danh sách.`)) {
      return;
    }
    setBusyId(book.id);
    try {
      await archiveLibraryBook({ id: book.id });
      toast.success(`Đã cất vào archive: ${book.title}`);
      await load();
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "Không cất được truyện");
    } finally {
      setBusyId(null);
    }
  }

  async function handleCreate(e: React.FormEvent, mode: "open" | "start") {
    e.preventDefault();
    if (!form.title.trim() && !(mode === "start" && form.prompt.trim())) {
      toast.error("Cần tên truyện (hoặc nhập yêu cầu truyện để tự đặt tên)");
      return;
    }
    if (mode === "start" && !form.prompt.trim()) {
      toast.error("Chế độ 'Tạo & viết ngay' cần nội dung yêu cầu truyện (prompt)");
      return;
    }
    setCreating(mode);
    try {
      if (mode === "start") {
        await createLibraryBookAndStart({ ...form, prompt: form.prompt });
        toast.success(`Đã tạo và bắt đầu viết: ${form.title || "truyện mới"}`);
      } else {
        const res = await createLibraryBook({ ...form, open: true });
        toast.success(`Đã tạo: ${res.book.title}`);
      }
      setForm({ title: "", title_vi: "", author: "", description: "", prompt: "" });
      await refresh();
      await load();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Tạo truyện thất bại");
    } finally {
      setCreating(null);
    }
  }

  return (
    <div className="mc-page library-page">
      <div className="mc-page-nav">
        <Link href="/" className="btn btn-ghost">
          <ArrowLeft size={16} /> Điều khiển
        </Link>
        <div className="translation-brand">
          <span className="translation-brand-mark">
            <LibraryIcon size={16} />
          </span>
          <span>Thư viện truyện</span>
        </div>
      </div>

      <header className="page-hero">
        <p className="panel-kicker">MULTI-NOVEL</p>
        <h1>Quản lý truyện</h1>
        <p className="lede">
          Mỗi truyện một thư mục riêng dưới library/. Dashboard chỉ mở <strong>một truyện active</strong> tại một thời điểm.
          Engine đang chạy thì phải tạm dừng trước khi chuyển. Tạo và mở không xóa truyện cũ.
        </p>
      </header>

      <section className="translation-panel">
        <div className="batch-toolbar">
          <div>
            <p className="panel-kicker">TẠO TRUYỆN MỚI</p>
            <h2>Thêm vào thư viện</h2>
          </div>
        </div>
        <form className="library-create-form" onSubmit={(e) => void handleCreate(e, "open")}>
          <label>
            Tên truyện (bắt buộc khi không có yêu cầu)
            <input
              value={form.title}
              onChange={(e) => setForm((f) => ({ ...f, title: e.target.value }))}
              placeholder="Tiên Đạo Dị Biến…"
            />
          </label>
          <label>
            Tên VI (tuỳ chọn)
            <input
              value={form.title_vi}
              onChange={(e) => setForm((f) => ({ ...f, title_vi: e.target.value }))}
              placeholder="Hiển thị / EPUB VI"
            />
          </label>
          <label>
            Tác giả
            <input
              value={form.author}
              onChange={(e) => setForm((f) => ({ ...f, author: e.target.value }))}
              placeholder="Bút danh"
            />
          </label>
          <label className="span-2">
            Mô tả
            <textarea
              value={form.description}
              onChange={(e) => setForm((f) => ({ ...f, description: e.target.value }))}
              rows={2}
              placeholder="Logline ngắn cho EPUB metadata"
            />
          </label>
          <label className="span-2">
            Yêu cầu truyện (prompt — cần khi bấm "Tạo & viết ngay")
            <textarea
              value={form.prompt}
              onChange={(e) => setForm((f) => ({ ...f, prompt: e.target.value }))}
              rows={3}
              placeholder="Thể loại, bối cảnh, nhân vật chính, giọng văn… Engine sẽ viết chương tiếng Trung theo yêu cầu này."
            />
          </label>
          <div className="toolbar-actions span-2">
            <button type="submit" className="btn" disabled={creating !== null}>
              {creating === "open" ? <Loader2 className="spin" size={16} /> : <BookPlus size={16} />}
              Tạo &amp; mở (chưa viết)
            </button>
            <button
              type="button"
              className="btn btn-primary"
              disabled={creating !== null}
              onClick={(e) => void handleCreate(e, "start")}
            >
              {creating === "start" ? <Loader2 className="spin" size={16} /> : <PenLine size={16} />}
              Tạo &amp; viết ngay
            </button>
          </div>
        </form>
      </section>

      <section className="translation-panel" style={{ marginTop: 16 }}>
        <div className="batch-toolbar">
          <div>
            <p className="panel-kicker">DANH SÁCH</p>
            <h2>{loading ? "Đang tải…" : `${data?.books?.length ?? 0} truyện`}</h2>
            {data?.root && <span className="muted mono">{data.root}</span>}
          </div>
          <button type="button" className="btn" onClick={() => void load()} disabled={loading}>
            Làm mới
          </button>
        </div>

        <div className="library-grid">
          {(data?.books ?? []).map((book) => (
            <article key={book.id} className={`library-card ${book.active ? "is-active" : ""}`}>
              <div className="library-card-top">
                <strong title={book.title}>{book.title}</strong>
                {book.active && <span className="mc-flow-chip">Đang mở</span>}
              </div>
              {book.title_vi && <p className="muted">{book.title_vi}</p>}
              {book.author && <p className="muted">Tác giả: {book.author}</p>}
              <div className="library-card-stats">
                <span>ZH {book.completed_zh ?? 0}{book.total_chapters ? `/${book.total_chapters}` : ""}</span>
                <span>VI {book.completed_vi ?? 0}</span>
                {book.phase && <span>{book.phase}</span>}
              </div>
              <div className="toolbar-actions">
                <button
                  type="button"
                  className="btn btn-primary"
                  disabled={!!book.active || busyId === book.id}
                  onClick={() => void handleOpen(book)}
                >
                  {busyId === book.id ? <Loader2 className="spin" size={16} /> : <FolderOpen size={16} />}
                  {book.active ? "Đang mở" : "Mở truyện"}
                </button>
                <button
                  type="button"
                  className="btn"
                  title={book.active ? "Phải mở truyện khác trước khi cất truyện đang mở" : "Cất vào archive"}
                  disabled={!!book.active || busyId === book.id}
                  onClick={() => void handleArchive(book)}
                >
                  <Archive size={16} />
                  Lưu trữ
                </button>
              </div>
            </article>
          ))}
          {!loading && (data?.books?.length ?? 0) === 0 && (
            <p className="muted">Chưa có truyện trong thư viện. Tạo mới ở form trên, hoặc mở engine trên một output/novel sẵn có.</p>
          )}
        </div>
      </section>
    </div>
  );
}
