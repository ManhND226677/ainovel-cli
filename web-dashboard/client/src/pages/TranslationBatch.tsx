/** Translation workspace — consistent toolbar + continue missing chapters. */
import { useEffect, useMemo, useState } from "react";
import { Link } from "wouter";
import {
  ArrowLeft,
  BookOpenText,
  Check,
  CheckSquare,
  Clock3,
  FileDown,
  Languages,
  ListChecks,
  RefreshCw,
  Square,
  TriangleAlert,
} from "lucide-react";
import { toast } from "sonner";
import ConnectionBanner from "@/components/ConnectionBanner";
import LiveTranslationMonitor from "@/components/LiveTranslationMonitor";
import LoadingSkeleton from "@/components/LoadingSkeleton";
import { useEngine } from "@/contexts/EngineContext";
import {
  fetchGlossary,
  fetchManuscriptOutline,
  getTranslationReportUrl,
  requestTranslation,
  retryTranslation,
  updateGlossary,
  type ManuscriptChapterMeta,
  type TranslationChapter,
  type TranslationChapterState,
} from "@/lib/engineApi";

const stateCopy: Record<TranslationChapterState, { label: string; tone: string }> = {
  pending: { label: "Chưa dịch / hàng đợi", tone: "pending" },
  running: { label: "Đang dịch", tone: "running" },
  completed: { label: "Đã hoàn tất", tone: "completed" },
  failed: { label: "Lỗi cần thử lại", tone: "failed" },
  stale: { label: "Nguồn đã đổi", tone: "stale" },
};

function isGapState(state?: string) {
  return !state || state === "pending" || state === "failed" || state === "stale";
}

function chapterSort(left: TranslationChapter, right: TranslationChapter) {
  return left.chapter - right.chapter;
}

export default function TranslationBatch() {
  const [selected, setSelected] = useState<number[]>([]);
  const [retrying, setRetrying] = useState(false);
  const [coordinating, setCoordinating] = useState(false);
  const [statusFilter, setStatusFilter] = useState<string>("all");
  const [jobFilter, setJobFilter] = useState<string>("all");
  const [searchQuery, setSearchQuery] = useState("");
  const [timeFilter, setTimeFilter] = useState("all");
  const [glossaryDraft, setGlossaryDraft] = useState("");
  const [showGlossary, setShowGlossary] = useState(false);
  const [savingGlossary, setSavingGlossary] = useState(false);
  const [outline, setOutline] = useState<ManuscriptChapterMeta[]>([]);

  const {
    engine,
    connected,
    liveProgress,
    liveProgressHistory,
    loading,
    error,
    transport,
    retryIn,
    refresh,
  } = useEngine();

  const translationMap = engine?.translation?.chapters || {};
  const jobsMap = useMemo(() => engine?.translation?.jobs || {}, [engine?.translation?.jobs]);
  const jobIds = useMemo(() => Object.keys(jobsMap), [jobsMap]);

  // Merge ZH-completed outline with translation status so never-queued gaps appear.
  const allChapters = useMemo(() => {
    const byChapter = new Map<number, TranslationChapter>();
    Object.values(translationMap).forEach((item) => byChapter.set(item.chapter, item));
    for (const row of outline) {
      if (!row.completed || byChapter.has(row.chapter)) continue;
      byChapter.set(row.chapter, {
        chapter: row.chapter,
        state: "pending",
        source_sha256: "",
        updated_at: new Date(0).toISOString(),
        last_error: "Chưa có bản Việt — cần xếp vào hàng đợi dịch",
      });
    }
    return Array.from(byChapter.values()).sort(chapterSort);
  }, [translationMap, outline]);

  useEffect(() => {
    if (!connected) return;
    void fetchGlossary()
      .then((glossary) => {
        setGlossaryDraft(
          Object.values(glossary.terms)
            .sort((a, b) => a.source.localeCompare(b.source))
            .map((item) => `${item.source} => ${item.vietnamese}`)
            .join("\n"),
        );
      })
      .catch(() => {});
    void fetchManuscriptOutline()
      .then((result) => setOutline(result.items || []))
      .catch(() => setOutline([]));
  }, [connected, engine?.snapshot?.completed_count, engine?.translation?.updated_at]);

  const failedCount = allChapters.filter((c) => c.state === "failed" || c.state === "stale").length;
  const completedCount = allChapters.filter((c) => c.state === "completed").length;
  const runningCount = allChapters.filter((c) => c.state === "running").length;
  const pendingCount = allChapters.filter((c) => c.state === "pending").length;
  const gapChapters = allChapters.filter((c) => isGapState(c.state));
  const gapCount = gapChapters.length;
  const zhCompleted = outline.filter((item) => item.completed).length || engine?.snapshot?.completed_count || 0;

  const chapters = useMemo(() => {
    const now = Date.now();
    return allChapters.filter((ch) => {
      if (statusFilter !== "all") {
        if (statusFilter === "issue" && ch.state !== "failed" && ch.state !== "stale") return false;
        if (statusFilter === "gap" && !isGapState(ch.state)) return false;
        if (statusFilter !== "issue" && statusFilter !== "gap" && ch.state !== statusFilter) return false;
      }
      if (jobFilter !== "all" && ch.job_id !== jobFilter) return false;
      if (searchQuery.trim()) {
        const q = searchQuery.toLowerCase();
        const hit =
          String(ch.chapter).includes(q)
          || (ch.source_sha256 || "").toLowerCase().includes(q)
          || (ch.last_error || "").toLowerCase().includes(q);
        if (!hit) return false;
      }
      if (timeFilter !== "all" && ch.updated_at) {
        const updatedTime = new Date(ch.updated_at).getTime();
        if (updatedTime > 0) {
          const diffHours = (now - updatedTime) / (1000 * 60 * 60);
          if (timeFilter === "today" && diffHours > 24) return false;
          if (timeFilter === "week" && diffHours > 24 * 7) return false;
        }
      }
      return true;
    });
  }, [allChapters, statusFilter, jobFilter, searchQuery, timeFilter]);

  const selectedSet = useMemo(() => new Set(selected), [selected]);
  const retryableSelected = allChapters.filter(
    (c) => selectedSet.has(c.chapter) && (c.state === "failed" || c.state === "stale"),
  );

  const toggleChapter = (chapter: number) => {
    setSelected((current) =>
      current.includes(chapter) ? current.filter((item) => item !== chapter) : [...current, chapter],
    );
  };

  const selectByPredicate = (predicate: (chapter: TranslationChapter) => boolean) => {
    const targets = chapters.filter(predicate).map((c) => c.chapter);
    const same =
      targets.length > 0
      && targets.length === selected.length
      && targets.every((chapter) => selectedSet.has(chapter));
    setSelected(same ? [] : targets);
  };

  const toggleFailed = () => selectByPredicate((c) => c.state === "failed" || c.state === "stale");
  const toggleGaps = () => selectByPredicate((c) => isGapState(c.state));

  const handleRetry = async () => {
    if (!connected) {
      toast.error("Chưa thể retry khi mất kết nối", { description: error || "Hãy mở ainovel-cli web." });
      return;
    }
    if (retryableSelected.length === 0) {
      toast("Chọn chương lỗi/stale trước", {
        description: "Retry chỉ nhận failed hoặc stale. Chương chưa dịch dùng «Dịch nốt chương sót».",
      });
      return;
    }
    setRetrying(true);
    try {
      const entries = Object.fromEntries(
        glossaryDraft
          .split("\n")
          .map((line) => line.trim())
          .filter(Boolean)
          .map((line) => {
            const [source, ...target] = line.split("=>");
            return [source?.trim() || "", target.join("=>").trim()];
          })
          .filter(([source, target]) => source && target),
      );
      if (Object.keys(entries).length > 0) await updateGlossary(entries);
      const result = await retryTranslation(retryableSelected.map((c) => c.chapter));
      toast.success("Đã xếp batch retry", { description: result.message });
      setSelected([]);
      await refresh();
    } catch (cause) {
      toast.error("Không thể xếp batch retry", {
        description: cause instanceof Error ? cause.message : "API lỗi.",
      });
    } finally {
      setRetrying(false);
    }
  };

  const handleContinueMissing = async () => {
    if (!connected) {
      toast.error("Chưa thể dịch khi mất kết nối", { description: error || "Hãy mở ainovel-cli web." });
      return;
    }
    if (gapCount === 0 && runningCount === 0) {
      toast.success("Không còn chương sót", {
        description: "Mọi chương ZH đã chốt đều có bản Việt completed (hoặc đang chạy).",
      });
      return;
    }
    setCoordinating(true);
    try {
      // Backend RunFullQueue: every eligible ZH-completed chapter not already
      // a fresh completed VI artifact, in ascending order.
      const result = await requestTranslation();
      toast.success("Đã xếp hàng dịch nốt chương còn sót", {
        description: result.message || `${gapCount} chương gap sẽ được xử lý theo thứ tự.`,
      });
      setStatusFilter("gap");
      await refresh();
      const outlineNext = await fetchManuscriptOutline().catch(() => null);
      if (outlineNext) setOutline(outlineNext.items || []);
    } catch (cause) {
      toast.error("Không thể bắt đầu dịch nốt", {
        description: cause instanceof Error ? cause.message : "Engine trả về lỗi.",
      });
    } finally {
      setCoordinating(false);
    }
  };

  const handleSaveGlossary = async () => {
    const entries = Object.fromEntries(
      glossaryDraft
        .split("\n")
        .map((line) => line.trim())
        .filter(Boolean)
        .map((line) => {
          const [source, ...target] = line.split("=>");
          return [source?.trim() || "", target.join("=>").trim()];
        })
        .filter(([source, target]) => source && target),
    );
    if (Object.keys(entries).length === 0) {
      toast("Chưa có thuật ngữ hợp lệ", { description: "Cú pháp: thuật ngữ Trung => bản dịch Việt." });
      return;
    }
    setSavingGlossary(true);
    try {
      const glossary = await updateGlossary(entries);
      setGlossaryDraft(
        Object.values(glossary.terms)
          .sort((a, b) => a.source.localeCompare(b.source))
          .map((item) => `${item.source} => ${item.vietnamese}`)
          .join("\n"),
      );
      toast.success("Đã lưu glossary", { description: `Version ${glossary.version}.` });
    } catch (cause) {
      toast.error("Không thể lưu glossary", {
        description: cause instanceof Error ? cause.message : "Engine lỗi.",
      });
    } finally {
      setSavingGlossary(false);
    }
  };

  const novelName = engine?.snapshot.novel_name || "Tác phẩm hiện tại";
  const busy = coordinating || retrying || loading || !connected;

  return (
    <div className="content-wrap translation-nested">
      <div className="mc-page-nav">
        <Link href="/" className="back-link"><ArrowLeft size={16} /> Điều khiển</Link>
        <div className="translation-brand">
          <span className="translation-brand-mark"><Languages size={16} /></span>
          <span>Bản dịch Việt</span>
        </div>
        <div className="mc-page-actions toolbar-actions">
          <Link href="/manuscript" className="btn btn-ghost">
            <BookOpenText size={14} /> Đọc ZH↔VI
          </Link>
          <button
            className="btn btn-ghost"
            type="button"
            onClick={() => {
              if (typeof window !== "undefined" && "Notification" in window) {
                Notification.requestPermission().then((perm) => {
                  if (perm === "granted") toast.success("Đã bật thông báo trình duyệt");
                  else toast("Quyền thông báo chưa được cấp");
                });
              } else toast("Trình duyệt không hỗ trợ thông báo");
            }}
          >
            Thông báo
          </button>
          <button className="btn btn-ghost" type="button" onClick={() => void refresh()} disabled={loading}>
            <RefreshCw size={14} className={loading ? "spin" : ""} /> Làm mới
          </button>
        </div>
      </div>

      <div className="translation-heading">
        <div>
          <p className="eyebrow">BÀN DỊCH VIỆT</p>
          <h1>Quản lý lô dịch</h1>
          <p className="lede">
            ZH chốt {zhCompleted} · VI xong {completedCount} · còn sót {gapCount}
            {failedCount > 0 ? ` (trong đó ${failedCount} lỗi/stale)` : ""}.
          </p>
        </div>
        <div className="translation-book">
          <span>ĐANG MỞ</span>
          <strong>{novelName}</strong>
          <small>{engine?.translation ? "Store dịch bền vững" : "Chưa có status dịch"}</small>
        </div>
      </div>

      <ConnectionBanner
        connected={connected}
        loading={loading}
        transport={transport}
        error={error}
        retryIn={retryIn}
        onRetry={() => void refresh()}
      />

      {loading && !engine ? (
        <LoadingSkeleton variant="batch" />
      ) : (
        <>
          <section className="translation-metrics">
            <div><span>Đã hoàn tất</span><strong>{completedCount}</strong><small>/ {zhCompleted || "—"} ZH</small></div>
            <div><span>Đang dịch</span><strong>{runningCount}</strong><small>worker</small></div>
            <div><span>Chưa dịch</span><strong>{pendingCount}</strong><small>pending</small></div>
            <div className={gapCount > 0 ? "metric-alert" : ""}>
              <span>Còn sót</span><strong>{gapCount}</strong><small>pending+failed+stale</small>
            </div>
            <div className={failedCount > 0 ? "metric-alert" : ""}>
              <span>Lỗi / stale</span><strong>{failedCount}</strong><small>cần retry</small>
            </div>
            <div><span>Đã chọn</span><strong>{selected.length}</strong><small>chapter</small></div>
          </section>

          <LiveTranslationMonitor
            progress={liveProgress}
            progressHistory={liveProgressHistory}
            chapters={allChapters}
            connected={connected}
            paused={engine?.translation_paused || false}
            onAfterControl={() => void refresh()}
          />

          <section className="translation-panel">
            <div className="batch-toolbar" role="toolbar" aria-label="Thao tác lô dịch">
              <div className="batch-toolbar-copy">
                <p className="panel-kicker">ĐIỀU PHỐI LÔ DỊCH</p>
                <h2>Danh sách chương</h2>
                <span>
                  {chapters.length > 0
                    ? `${chapters.length} / ${allChapters.length} đang hiển thị`
                    : "Chưa có dữ liệu chương"}
                </span>
              </div>
              <div className="toolbar-actions batch-toolbar-actions">
                <button
                  type="button"
                  className="btn btn-primary"
                  onClick={() => void handleContinueMissing()}
                  disabled={busy || (gapCount === 0 && runningCount === 0)}
                  title="Xếp hàng mọi chương ZH đã chốt chưa có bản VI completed"
                >
                  <Languages size={14} />
                  {coordinating
                    ? "Đang xếp hàng…"
                    : gapCount > 0
                      ? `Dịch nốt ${gapCount} chương sót`
                      : "Đã dịch đủ"}
                </button>
                <button
                  type="button"
                  className="btn btn-secondary"
                  onClick={toggleGaps}
                  disabled={gapCount === 0}
                >
                  <ListChecks size={14} /> Chọn còn sót
                </button>
                <button
                  type="button"
                  className="btn btn-secondary"
                  onClick={toggleFailed}
                  disabled={failedCount === 0}
                >
                  <CheckSquare size={14} /> Chọn lỗi
                </button>
                <button
                  type="button"
                  className="btn btn-secondary"
                  onClick={() => void handleRetry()}
                  disabled={retrying || retryableSelected.length === 0 || !connected}
                >
                  <RefreshCw size={14} className={retrying ? "spin" : ""} />
                  {retrying
                    ? "Đang xếp retry…"
                    : retryableSelected.length > 0
                      ? `Retry ${retryableSelected.length}`
                      : "Retry đã chọn"}
                </button>
                <button
                  type="button"
                  className="btn btn-ghost"
                  onClick={() => setShowGlossary((value) => !value)}
                >
                  Thuật ngữ
                </button>
              </div>
            </div>

            {showGlossary && (
              <div className="glossary-editor">
                <div>
                  <p className="panel-kicker">GLOSSARY · APPEND-ONLY</p>
                  <strong>Khóa cách gọi trước khi dịch</strong>
                  <span>Mỗi dòng: <code>tên gốc =&gt; bản Việt</code>. Term đã chốt không bị ghi đè.</span>
                </div>
                <textarea
                  value={glossaryDraft}
                  onChange={(event) => setGlossaryDraft(event.target.value)}
                  placeholder={"云城 => Thành phố Mây\n沈砚 => Thẩm Nghiễn"}
                  rows={6}
                />
                <button
                  type="button"
                  className="btn btn-secondary"
                  onClick={() => void handleSaveGlossary()}
                  disabled={savingGlossary}
                >
                  {savingGlossary ? "Đang lưu…" : "Lưu glossary"}
                </button>
              </div>
            )}

            <div className="batch-filters toolbar-filters">
              <input
                type="search"
                className="filter-input"
                placeholder="Tìm số chương, SHA, lỗi…"
                value={searchQuery}
                onChange={(e) => setSearchQuery(e.target.value)}
              />
              <select className="filter-select" value={statusFilter} onChange={(e) => setStatusFilter(e.target.value)}>
                <option value="all">Trạng thái: Tất cả ({allChapters.length})</option>
                <option value="gap">Còn sót — chưa VI/lỗi ({gapCount})</option>
                <option value="completed">Đã hoàn tất ({completedCount})</option>
                <option value="running">Đang dịch ({runningCount})</option>
                <option value="issue">Lỗi / stale ({failedCount})</option>
                <option value="pending">Chưa dịch ({pendingCount})</option>
              </select>
              <select className="filter-select" value={jobFilter} onChange={(e) => setJobFilter(e.target.value)}>
                <option value="all">Job: Tất cả ({jobIds.length})</option>
                {jobIds.map((jid) => (
                  <option key={jid} value={jid}>Job #{jid.slice(0, 8)}</option>
                ))}
              </select>
              <select className="filter-select" value={timeFilter} onChange={(e) => setTimeFilter(e.target.value)}>
                <option value="all">Thời gian: Tất cả</option>
                <option value="today">Hôm nay (24h)</option>
                <option value="week">7 ngày qua</option>
              </select>
              {(statusFilter !== "all" || jobFilter !== "all" || searchQuery || timeFilter !== "all") && (
                <button
                  type="button"
                  className="btn btn-ghost btn-compact"
                  onClick={() => {
                    setStatusFilter("all");
                    setJobFilter("all");
                    setSearchQuery("");
                    setTimeFilter("all");
                  }}
                >
                  Xóa lọc
                </button>
              )}
              <div className="toolbar-actions filter-export">
                <a
                  href={getTranslationReportUrl("md", jobFilter !== "all" ? jobFilter : undefined)}
                  target="_blank"
                  rel="noreferrer"
                  className="btn btn-ghost btn-compact"
                >
                  <FileDown size={13} /> MD
                </a>
                <a
                  href={getTranslationReportUrl("txt", jobFilter !== "all" ? jobFilter : undefined)}
                  target="_blank"
                  rel="noreferrer"
                  className="btn btn-ghost btn-compact"
                >
                  <FileDown size={13} /> TXT
                </a>
              </div>
            </div>

            {chapters.length === 0 ? (
              <div className="translation-empty">
                <span className="empty-seal">BẢN VIỆT</span>
                <Languages size={24} />
                <strong>{connected ? "Không có chương khớp bộ lọc" : "Chờ kết nối engine"}</strong>
                <p>
                  {connected
                    ? "Thử «Còn sót» hoặc «Dịch nốt chương sót» để xếp hàng các chương ZH chưa có VI."
                    : "Kết nối ainovel-cli web để mở sổ batch thật."}
                </p>
              </div>
            ) : (
              <div className="translation-table-wrap">
                <table className="translation-table">
                  <thead>
                    <tr>
                      <th>
                        <button
                          className="select-all-button"
                          type="button"
                          onClick={toggleGaps}
                          aria-label="Chọn các chương còn sót"
                        >
                          {gapCount > 0 && selected.length === gapCount && gapChapters.every((c) => selectedSet.has(c.chapter))
                            ? <CheckSquare size={15} />
                            : <Square size={15} />}
                        </button>
                      </th>
                      <th>Chương</th>
                      <th>Trạng thái</th>
                      <th>Lần thử</th>
                      <th>Dấu vân tay nguồn</th>
                      <th>Cập nhật</th>
                      <th>Lỗi / ghi chú</th>
                    </tr>
                  </thead>
                  <tbody>
                    {chapters.map((chapter) => {
                      const copy = stateCopy[chapter.state] || stateCopy.pending;
                      const isSelected = selectedSet.has(chapter.chapter);
                      const canRetry = chapter.state === "failed" || chapter.state === "stale";
                      const isGap = isGapState(chapter.state);
                      return (
                        <tr
                          key={chapter.chapter}
                          className={`${isSelected ? "row-selected" : ""} ${isGap ? "row-gap" : ""}`}
                        >
                          <td>
                            <button
                              type="button"
                              className={`row-checkbox ${isSelected ? "is-selected" : ""}`}
                              onClick={() => toggleChapter(chapter.chapter)}
                              aria-label={`Chọn chương ${chapter.chapter}`}
                            >
                              {isSelected ? <Check size={14} /> : <Square size={15} />}
                            </button>
                          </td>
                          <td><strong className="chapter-id">{String(chapter.chapter).padStart(2, "0")}</strong></td>
                          <td>
                            <span className={`translation-state state-${copy.tone}`}>
                              <span />{copy.label}
                            </span>
                          </td>
                          <td>{chapter.attempts || 0}</td>
                          <td>
                            <code title={chapter.source_sha256}>
                              {chapter.source_sha256 ? `${chapter.source_sha256.slice(0, 19)}…` : "—"}
                            </code>
                          </td>
                          <td>
                            <span className="updated-cell">
                              <Clock3 size={12} />
                              {chapter.updated_at && new Date(chapter.updated_at).getTime() > 0
                                ? new Date(chapter.updated_at).toLocaleString("vi-VN", {
                                  day: "2-digit",
                                  month: "2-digit",
                                  hour: "2-digit",
                                  minute: "2-digit",
                                })
                                : "chưa xếp"}
                            </span>
                          </td>
                          <td>
                            <span className={canRetry || isGap ? "error-cell" : "muted-cell"}>
                              {chapter.last_error || (canRetry ? "Có thể thử lại" : isGap ? "Chưa có VI" : "Không có lỗi")}
                            </span>
                          </td>
                        </tr>
                      );
                    })}
                  </tbody>
                </table>
              </div>
            )}
          </section>

          <div className="translation-footnote">
            <TriangleAlert size={14} />
            <span>
              <strong>Dịch nốt chương sót</strong> xếp mọi chương ZH đã chốt chưa có VI completed (kể cả chưa từng vào queue).
              <strong> Retry</strong> chỉ nhận failed/stale đã chọn. Bản Trung không bị ghi đè.
            </span>
          </div>
        </>
      )}
    </div>
  );
}
