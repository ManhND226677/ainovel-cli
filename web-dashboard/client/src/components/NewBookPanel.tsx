import { useEffect, useState } from "react";
import { Link } from "wouter";
import { BookPlus, Library, Loader2 } from "lucide-react";
import { toast } from "sonner";
import {
  createLibraryBookAndStart,
  fetchActiveBook,
  type ActiveBookSummary,
} from "@/lib/engineApi";

type Props = {
  connected: boolean;
  isRunning?: boolean;
  prompt: string;
  onStarted?: () => void | Promise<void>;
};

export default function NewBookPanel({ connected, isRunning, prompt, onStarted }: Props) {
  const [title, setTitle] = useState("");
  const [busy, setBusy] = useState(false);
  const [active, setActive] = useState<ActiveBookSummary | null>(null);

  useEffect(() => {
    if (!connected) {
      setActive(null);
      return;
    }
    let cancelled = false;
    void fetchActiveBook()
      .then((res) => {
        if (!cancelled) setActive(res.active || null);
      })
      .catch(() => {
        if (!cancelled) setActive(null);
      });
    return () => {
      cancelled = true;
    };
  }, [connected, isRunning]);

  async function handleCreate() {
    if (!connected) {
      toast.error("Engine chưa kết nối");
      return;
    }
    if (!prompt.trim()) {
      toast("Cần prompt sáng tác", {
        description: "Nhập ý tưởng vào ô chỉ dẫn phía dưới, rồi bấm Tạo truyện mới.",
      });
      return;
    }
    if (isRunning) {
      toast.error("Engine đang chạy", { description: "Tạm dừng truyện hiện tại trước." });
      return;
    }
    setBusy(true);
    try {
      const res = await createLibraryBookAndStart({
        title: title.trim() || undefined,
        prompt: prompt.trim(),
      });
      toast.success("Đã tạo thư mục mới và bắt đầu viết", {
        description: res.book?.title
          ? "«" + res.book.title + "» — truyện cũ vẫn trong Thư viện."
          : "Truyện cũ không bị ghi đè.",
      });
      setTitle("");
      const next = await fetchActiveBook();
      setActive(next.active || null);
      await onStarted?.();
    } catch (cause) {
      toast.error("Không tạo được truyện mới", {
        description: cause instanceof Error ? cause.message : "API lỗi.",
      });
    } finally {
      setBusy(false);
    }
  }

  if (!connected) return null;

  return (
    <section className="translation-panel" style={{ marginBottom: 12 }}>
      <div className="batch-toolbar">
        <div>
          <p className="panel-kicker">MULTI-BOOK</p>
          <h2 style={{ margin: 0, fontSize: 18 }}>Truyện đang mở và truyện mới</h2>
          <p className="muted" style={{ margin: "6px 0 0" }}>
            Mỗi truyện một thư mục. Tạo mới không ghi đè truyện cũ.
            {active?.dir ? (
              <>
                {" "}
                Đang mở: <span className="mono">{active.dir}</span>
                {active.completed ? " · " + active.completed + " chương ZH" : ""}
                {active.phase ? " · " + active.phase : ""}
              </>
            ) : null}
          </p>
          {active?.blocks_new_start && active.message ? (
            <p className="muted" style={{ marginTop: 6 }}>
              {active.message}
            </p>
          ) : null}
        </div>
        <div className="toolbar-actions">
          <Link href="/library" className="btn">
            <Library size={16} /> Thư viện
          </Link>
          <Link href="/cocreate" className="btn">
            Đồng sáng tác
          </Link>
        </div>
      </div>
      <div className="library-create-form" style={{ marginTop: 12 }}>
        <label>
          Tên truyện mới (tuỳ chọn)
          <input
            value={title}
            onChange={(e) => setTitle(e.target.value)}
            placeholder="Để trống = lấy từ prompt"
          />
        </label>
        <div className="toolbar-actions">
          <button
            type="button"
            className="btn btn-primary"
            disabled={!connected || busy || !!isRunning}
            onClick={() => void handleCreate()}
          >
            {busy ? <Loader2 className="spin" size={16} /> : <BookPlus size={16} />}
            {busy ? "Đang tạo…" : "Quick start: tạo & viết"}
          </button>
          <Link href="/cocreate" className="btn btn-primary">
            Đồng sáng tác (chat)
          </Link>
          <Link href="/cocreate?mode=stage" className="btn">
            Co-create giai đoạn
          </Link>
          <small className="muted">Quick start dùng ô chỉ dẫn bên dưới. Muốn hỏi–đáp làm brief → Đồng sáng tác.</small>
        </div>
      </div>
    </section>
  );
}
