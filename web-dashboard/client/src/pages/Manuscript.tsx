/** Chapter reader: outline rail + ZH/VI split pane. */
import { useEffect, useMemo, useState } from "react";
import { Link, useSearch } from "wouter";
import { ArrowLeft, BookOpenText, Columns2, Download, FileText, Languages, RefreshCw } from "lucide-react";
import { toast } from "sonner";
import ConnectionBanner from "@/components/ConnectionBanner";
import { useEngine } from "@/contexts/EngineContext";
import {
  fetchManuscriptChapter,
  fetchManuscriptOutline,
  requestExport,
  type ChapterManuscript,
  type ManuscriptChapterMeta,
} from "@/lib/engineApi";
import { formatNovelTitle } from "@/lib/novelTitle";

function stateTone(state?: string) {
  if (state === "completed") return "completed";
  if (state === "failed" || state === "stale") return "failed";
  if (state === "running") return "running";
  if (state === "pending") return "pending";
  return "none";
}

export default function Manuscript() {
  const search = useSearch();
  const params = useMemo(() => new URLSearchParams(search), [search]);
  const initialChapter = Number(params.get("ch") || "0");

  const { engine, connected, loading, error, transport, retryIn, refresh } = useEngine();
  const [outline, setOutline] = useState<ManuscriptChapterMeta[]>([]);
  const [selected, setSelected] = useState<number>(initialChapter > 0 ? initialChapter : 0);
  const [chapter, setChapter] = useState<ChapterManuscript | null>(null);
  const [loadingOutline, setLoadingOutline] = useState(false);
  const [loadingChapter, setLoadingChapter] = useState(false);
  const [onlyCompleted, setOnlyCompleted] = useState(false);
  const [view, setView] = useState<"split" | "zh" | "vi">("split");
  const [exporting, setExporting] = useState<string | null>(null);
  const novel = formatNovelTitle(engine?.snapshot?.novel_name);

  const runExport = async (language: "zh" | "vi", format: "epub" | "txt" = "epub") => {
    const key = `${language}-${format}`;
    setExporting(key);
    try {
      const res = await requestExport({ language, format, download: true });
      toast.success(`Đã tải ${format.toUpperCase()} ${language.toUpperCase()}`, {
        description: "downloaded" in res ? res.name : undefined,
      });
    } catch (cause) {
      toast.error("Export thất bại", {
        description: cause instanceof Error ? cause.message : "API lỗi.",
      });
    } finally {
      setExporting(null);
    }
  };

  const loadOutline = async () => {
    if (!connected) return;
    setLoadingOutline(true);
    try {
      const result = await fetchManuscriptOutline();
      setOutline(result.items || []);
      setSelected((current) => {
        if (current > 0 && result.items.some((item) => item.chapter === current)) return current;
        const preferred = [...result.items].reverse().find((item) => item.completed)?.chapter
          || result.items[0]?.chapter
          || 0;
        return preferred;
      });
    } catch (cause) {
      toast.error("Không tải được outline", { description: cause instanceof Error ? cause.message : "API lỗi." });
    } finally {
      setLoadingOutline(false);
    }
  };

  useEffect(() => {
    void loadOutline();
  }, [connected, engine?.snapshot?.completed_count, engine?.translation?.updated_at]);

  useEffect(() => {
    if (!connected || selected < 1) {
      setChapter(null);
      return;
    }
    let disposed = false;
    setLoadingChapter(true);
    void fetchManuscriptChapter(selected)
      .then((item) => {
        if (!disposed) setChapter(item);
      })
      .catch((cause) => {
        if (!disposed) {
          setChapter(null);
          toast.error(`Không mở được chương ${selected}`, {
            description: cause instanceof Error ? cause.message : "API lỗi.",
          });
        }
      })
      .finally(() => {
        if (!disposed) setLoadingChapter(false);
      });
    return () => {
      disposed = true;
    };
  }, [connected, selected]);

  const rows = useMemo(() => {
    const list = onlyCompleted ? outline.filter((item) => item.completed) : outline;
    return list;
  }, [onlyCompleted, outline]);

  const openChapter = (chapterNo: number) => {
    setSelected(chapterNo);
    const url = new URL(window.location.href);
    url.searchParams.set("ch", String(chapterNo));
    window.history.replaceState(null, "", `${url.pathname}?${url.searchParams.toString()}`);
  };

  return (
    <div className="content-wrap manuscript-page">
      <div className="mc-page-nav">
        <Link href="/" className="back-link"><ArrowLeft size={16} /> Điều khiển</Link>
        <div className="translation-brand">
          <span className="translation-brand-mark"><BookOpenText size={16} /></span>
          <span>Bản thảo ZH ↔ VI</span>
        </div>
        <div className="mc-page-actions toolbar-actions">
          <button className="btn" type="button" disabled={!!exporting} onClick={() => void runExport("zh", "epub")}>
            <Download size={14} /> {exporting === "zh-epub" ? "…" : "EPUB ZH"}
          </button>
          <button className="btn" type="button" disabled={!!exporting} onClick={() => void runExport("vi", "epub")}>
            <Download size={14} /> {exporting === "vi-epub" ? "…" : "EPUB VI"}
          </button>
          <button className="btn" type="button" disabled={!!exporting} onClick={() => void runExport("zh", "txt")}>
            <FileText size={14} /> TXT ZH
          </button>
          <button className="btn" type="button" disabled={!!exporting} onClick={() => void runExport("vi", "txt")}>
            <FileText size={14} /> TXT VI
          </button>
          <button className="refresh-button" type="button" onClick={() => { void refresh(); void loadOutline(); }} disabled={loadingOutline}>
            <RefreshCw size={14} className={loadingOutline ? "spin" : ""} /> Làm mới
          </button>
        </div>
      </div>

      <div className="translation-heading">
        <div>
          <p className="eyebrow">ĐỌC BẢN THẢO</p>
          <h1>{novel.vietnamese || "Chưa có truyện"}</h1>
          <p className="lede">Outline thật từ engine · bản Trung là SoT · bản Việt chỉ đọc artifact dịch.</p>
        </div>
        <div className="translation-book">
          <span>CHẾ ĐỘ XEM</span>
          <strong>{view === "split" ? "Song song" : view === "zh" ? "Chỉ Trung" : "Chỉ Việt"}</strong>
          <small>{chapter ? `Ch. ${chapter.chapter}` : "Chọn chương bên trái"}</small>
        </div>
      </div>

      <ConnectionBanner
        connected={connected}
        loading={loading}
        transport={transport}
        error={error}
        retryIn={retryIn}
        onRetry={() => { void refresh(); void loadOutline(); }}
      />

      <div className="ms-toolbar">
        <label className="ms-check">
          <input type="checkbox" checked={onlyCompleted} onChange={(e) => setOnlyCompleted(e.target.checked)} />
          Chỉ chương đã chốt
        </label>
        <div className="ms-view-toggle" role="group" aria-label="Chế độ xem">
          <button type="button" className={view === "split" ? "is-active" : ""} onClick={() => setView("split")}><Columns2 size={14} /> Song song</button>
          <button type="button" className={view === "zh" ? "is-active" : ""} onClick={() => setView("zh")}><FileText size={14} /> Trung</button>
          <button type="button" className={view === "vi" ? "is-active" : ""} onClick={() => setView("vi")}><Languages size={14} /> Việt</button>
        </div>
      </div>

      <div className="ms-layout">
        <aside className="ms-rail panel">
          <div className="panel-header">
            <div>
              <p className="panel-kicker">OUTLINE</p>
              <h3>{rows.length} chương</h3>
            </div>
          </div>
          <div className="ms-rail-list">
            {rows.map((item) => (
              <button
                key={item.chapter}
                type="button"
                className={`ms-rail-item ${selected === item.chapter ? "is-active" : ""} ${item.completed ? "is-done" : ""}`}
                onClick={() => openChapter(item.chapter)}
              >
                <strong>{String(item.chapter).padStart(2, "0")}</strong>
                <span className="ms-rail-title">{item.title || `Chương ${item.chapter}`}</span>
                <em className={`ms-vi-dot vi-${stateTone(item.translation_state)}`} title={item.translation_state || "chưa VI"} />
              </button>
            ))}
            {rows.length === 0 && <div className="activity-empty">{connected ? "Chưa có outline/chương." : "Chờ kết nối engine."}</div>}
          </div>
        </aside>

        <section className={`ms-reader panel ms-view-${view}`}>
          {loadingChapter ? (
            <div className="activity-empty">Đang tải chương {selected}…</div>
          ) : !chapter ? (
            <div className="activity-empty">Chọn một chương trên rail để đọc.</div>
          ) : (
            <>
              <header className="ms-reader-head">
                <div>
                  <p className="panel-kicker">CHƯƠNG {String(chapter.chapter).padStart(2, "0")}</p>
                  <h2>{chapter.title}</h2>
                  {chapter.core_event && <p>{chapter.core_event}</p>}
                </div>
                <div className="ms-reader-meta">
                  <span>ZH {chapter.chinese_chars.toLocaleString("vi-VN")} chữ</span>
                  <span>VI {chapter.vietnamese_chars.toLocaleString("vi-VN")} chữ</span>
                  <span className={`ms-state vi-${stateTone(chapter.translation_state)}`}>{chapter.translation_state || "no-vi"}</span>
                  {chapter.completed ? <span className="ms-state is-done">đã chốt</span> : <span className="ms-state">chưa chốt</span>}
                </div>
              </header>
              {chapter.translation_error && (
                <div className="ms-error">Lỗi dịch: {chapter.translation_error}</div>
              )}
              <div className="ms-panes">
                {(view === "split" || view === "zh") && (
                  <article className="ms-pane">
                    <div className="ms-pane-label">TRUNG · nguồn</div>
                    <pre>{chapter.chinese?.trim() || "Chưa có bản Trung đã commit."}</pre>
                  </article>
                )}
                {(view === "split" || view === "vi") && (
                  <article className="ms-pane ms-pane-vi">
                    <div className="ms-pane-label">VIỆT · artifact</div>
                    <pre>{chapter.vietnamese?.trim() || "Chưa có bản dịch (pending/failed/stale hoặc chưa bật translation)."}</pre>
                  </article>
                )}
              </div>
            </>
          )}
        </section>
      </div>
    </div>
  );
}
