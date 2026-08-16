import { useCallback, useEffect, useMemo, useState } from "react";
import { Link, useSearch } from "wouter";
import {
  ArrowLeft,
  Loader2,
  MessageSquarePlus,
  Play,
  Send,
  Sparkles,
  Square,
  Users,
} from "lucide-react";
import { toast } from "sonner";
import {
  cancelCoCreate,
  commitCoCreate,
  fetchCoCreate,
  messageCoCreate,
  startCoCreate,
  type CoCreateSession,
} from "@/lib/engineApi";
import { useEngine } from "@/contexts/EngineContext";
import ConnectionBanner from "@/components/ConnectionBanner";

function useQueryMode(): "startup" | "stage" {
  const search = useSearch();
  const q = new URLSearchParams(search.startsWith("?") ? search.slice(1) : search);
  return q.get("mode") === "stage" ? "stage" : "startup";
}

export default function CoCreatePage() {
  const preferredMode = useQueryMode();
  const { connected, loading, error, transport, retryIn, refresh } = useEngine();
  const [session, setSession] = useState<CoCreateSession | null>(null);
  const [title, setTitle] = useState("");
  const [input, setInput] = useState("");
  const [draftEdit, setDraftEdit] = useState("");
  const [busy, setBusy] = useState(false);
  const [booting, setBooting] = useState(true);

  const load = useCallback(async () => {
    try {
      const s = await fetchCoCreate();
      setSession(s);
      if (s.draft) setDraftEdit(s.draft);
      if (s.title) setTitle(s.title);
    } catch (e) {
      // ignore offline
    } finally {
      setBooting(false);
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load, connected]);

  const active = !!session?.active;
  const mode = (session?.mode as "startup" | "stage" | undefined) || preferredMode;
  const canCommit = !!(session?.can_commit || draftEdit.trim());

  const history = useMemo(() => session?.history || [], [session?.history]);

  async function handleStart() {
    if (!connected) return toast.error("Engine chưa kết nối");
    if (!input.trim()) return toast("Nhập ý tưởng / câu hỏi đầu tiên");
    setBusy(true);
    try {
      const res = await startCoCreate({
        mode: preferredMode,
        message: input.trim(),
        title: title.trim() || undefined,
      });
      if (!res.ok) toast.error(res.error || "Co-create lỗi");
      else toast.success(preferredMode === "stage" ? "Đã vào đồng sáng tác giai đoạn" : "Đã bắt đầu đồng sáng tác");
      if (res.session) {
        setSession(res.session);
        if (res.session.draft) setDraftEdit(res.session.draft);
      } else await load();
      setInput("");
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "Không start được co-create");
    } finally {
      setBusy(false);
    }
  }

  async function handleSend() {
    if (!connected) return toast.error("Engine chưa kết nối");
    if (!input.trim()) return;
    setBusy(true);
    try {
      const res = await messageCoCreate(input.trim());
      if (!res.ok) toast.error(res.error || "Gửi thất bại");
      if (res.session) {
        setSession(res.session);
        if (res.session.draft) setDraftEdit(res.session.draft);
      } else await load();
      setInput("");
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "Gửi thất bại");
    } finally {
      setBusy(false);
    }
  }

  async function handleCommit() {
    if (!connected) return toast.error("Engine chưa kết nối");
    if (!draftEdit.trim()) return toast.error("Draft còn trống");
    setBusy(true);
    try {
      const res = await commitCoCreate({ title: title.trim() || undefined, draft: draftEdit.trim() });
      toast.success(res.message || "Đã chốt và chạy engine");
      setSession({ active: false });
      await refresh();
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "Commit thất bại");
    } finally {
      setBusy(false);
    }
  }

  async function handleCancel() {
    setBusy(true);
    try {
      await cancelCoCreate();
      setSession({ active: false });
      setDraftEdit("");
      toast.message("Đã hủy đồng sáng tác");
      await refresh();
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "Hủy thất bại");
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="mc-page">
      <div className="mc-page-nav">
        <Link href="/" className="btn btn-ghost">
          <ArrowLeft size={16} /> Điều khiển
        </Link>
        <div className="translation-brand">
          <span className="translation-brand-mark">
            <Users size={16} />
          </span>
          <span>Đồng sáng tác</span>
        </div>
      </div>

      <ConnectionBanner
        connected={connected}
        loading={loading || booting}
        transport={transport}
        error={error}
        retryIn={retryIn}
        onRetry={() => void refresh()}
      />

      <header className="page-hero">
        <p className="panel-kicker">CO-CREATE</p>
        <h1>{mode === "stage" ? "Đồng sáng tác giai đoạn" : "Đồng sáng tác mở truyện"}</h1>
        <p className="lede">
          Chat với trợ lý để làm rõ ý tưởng, draft brief tích lũy bên phải. Khi đủ, bấm <strong>Chốt & chạy</strong>.
          {mode === "startup"
            ? " Startup sẽ tạo thư mục truyện mới (không đè sách cũ) rồi StartPrepared."
            : " Stage sẽ Pause engine, chốt hướng rồi Continue."}
        </p>
      </header>

      {!active ? (
        <section className="translation-panel">
          <div className="batch-toolbar">
            <div>
              <p className="panel-kicker">BẮT ĐẦU</p>
              <h2>Phiên {preferredMode === "stage" ? "stage" : "startup"}</h2>
            </div>
          </div>
          {preferredMode === "startup" && (
            <label>
              Tên truyện (tuỳ chọn)
              <input value={title} onChange={(e) => setTitle(e.target.value)} placeholder="Để trống = lấy từ draft" />
            </label>
          )}
          <label className="span-2" style={{ display: "block", marginTop: 10 }}>
            Ý tưởng / câu mở đầu
            <textarea
              rows={4}
              value={input}
              onChange={(e) => setInput(e.target.value)}
              placeholder={
                preferredMode === "stage"
                  ? "VD: 3 cung tiếp theo muốn tăng xung đột nội bộ môn phái..."
                  : "VD: Huyền huyễn, nhân vật chính bị hệ thống khóa lý trí, 200 chương..."
              }
            />
          </label>
          <div className="toolbar-actions" style={{ marginTop: 12 }}>
            <button type="button" className="btn btn-primary" disabled={!connected || busy} onClick={() => void handleStart()}>
              {busy ? <Loader2 className="spin" size={16} /> : <MessageSquarePlus size={16} />}
              Bắt đầu đồng sáng tác
            </button>
            <Link href="/" className="btn">
              Về Home (quick start 1 prompt)
            </Link>
          </div>
        </section>
      ) : (
        <div className="mc-grid-main" style={{ gridTemplateColumns: "1.1fr 0.9fr", gap: 14 }}>
          <section className="panel">
            <div className="panel-header">
              <div>
                <p className="panel-kicker">HỘI THOẠI</p>
                <h3>{mode} {session?.busy || busy ? "· đang nghĩ…" : session?.ready ? "· sẵn sàng chốt" : ""}</h3>
              </div>
              <button type="button" className="btn" disabled={busy} onClick={() => void handleCancel()}>
                <Square size={14} /> Hủy
              </button>
            </div>
            <div className="mc-activity-list" style={{ maxHeight: 420, overflow: "auto" }}>
              {history.map((m, idx) => (
                <div key={idx} className="activity-row" style={{ alignItems: "flex-start" }}>
                  <span className={`activity-line line-${m.role === "user" ? "terracotta" : "green"}`} />
                  <div>
                    <strong>{m.role === "user" ? "Bạn" : "Trợ lý"}</strong>
                    <span style={{ whiteSpace: "pre-wrap" }}>{m.content}</span>
                  </div>
                </div>
              ))}
              {history.length === 0 && <div className="activity-empty">Chưa có tin nhắn</div>}
            </div>
            {(session?.suggestions || []).length > 0 && (
              <div className="toolbar-actions" style={{ flexWrap: "wrap", marginTop: 8 }}>
                {(session?.suggestions || []).map((s) => (
                  <button key={s} type="button" className="btn" onClick={() => setInput((v) => (v ? v + " " + s : s))}>
                    <Sparkles size={12} /> {s}
                  </button>
                ))}
              </div>
            )}
            <div className="prompt-box mc-prompt-box" style={{ marginTop: 10 }}>
              <textarea
                rows={3}
                value={input}
                disabled={busy || !!session?.busy}
                onChange={(e) => setInput(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) {
                    e.preventDefault();
                    void handleSend();
                  }
                }}
                placeholder="Trả lời trợ lý… (Ctrl/⌘+Enter gửi)"
              />
              <button className="send-button" disabled={busy || !!session?.busy || !input.trim()} onClick={() => void handleSend()} aria-label="Gửi">
                {busy || session?.busy ? <Loader2 className="spin" size={16} /> : <Send size={16} />}
              </button>
            </div>
          </section>

          <section className="panel">
            <div className="panel-header">
              <div>
                <p className="panel-kicker">DRAFT BRIEF</p>
                <h3>Chỉnh trước khi chốt</h3>
              </div>
            </div>
            {mode === "startup" && (
              <label style={{ display: "block", marginBottom: 8 }}>
                Tên truyện
                <input value={title} onChange={(e) => setTitle(e.target.value)} placeholder="Tên folder / metadata" />
              </label>
            )}
            <textarea
              rows={18}
              value={draftEdit}
              onChange={(e) => setDraftEdit(e.target.value)}
              placeholder="Draft sẽ tích lũy sau mỗi lượt chat…"
              style={{ width: "100%", fontFamily: "ui-monospace, SFMono-Regular, Menlo, monospace", fontSize: 12 }}
            />
            <div className="toolbar-actions" style={{ marginTop: 10 }}>
              <button type="button" className="btn btn-primary" disabled={!connected || busy || !canCommit} onClick={() => void handleCommit()}>
                {busy ? <Loader2 className="spin" size={16} /> : <Play size={16} />}
                Chốt & chạy
              </button>
            </div>
            <p className="muted" style={{ marginTop: 8 }}>
              {mode === "startup"
                ? "Commit = tạo sách mới trong library + StartPrepared(draft)."
                : "Commit = ResumeFromCoCreate(draft) để Arbiter định tuyến tiếp."}
            </p>
          </section>
        </div>
      )}
    </div>
  );
}
