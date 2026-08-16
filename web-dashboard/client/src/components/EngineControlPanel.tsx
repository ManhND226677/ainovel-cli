import { useMemo, useState } from "react";
import {
  FastForward,
  Pause,
  Play,
  ShieldCheck,
  Sparkles,
  StepForward,
} from "lucide-react";
import { toast } from "sonner";
import {
  abortEngine,
  advanceNextChapter,
  continueEngine,
  resumeEngine,
  setAdvanceMode,
  steerEngine,
  type EngineState,
} from "@/lib/engineApi";
import { primaryAction } from "@/lib/statusCopy";

type Props = {
  engine: EngineState | null | undefined;
  connected: boolean;
  prompt: string;
  onPromptClear?: () => void;
  onDone?: () => void | Promise<void>;
};

export default function EngineControlPanel({ engine, connected, prompt, onPromptClear, onDone }: Props) {
  const [busy, setBusy] = useState<string | null>(null);
  const snap = engine?.snapshot;
  const action = useMemo(() => primaryAction(engine), [engine]);
  const mode = (snap?.advance_mode || "auto").toLowerCase();
  const isReview = mode === "review";
  const waitingGate = isReview && !snap?.is_running && (snap?.phase === "writing" || !!snap?.has_advance_hold);

  async function run(id: string, fn: () => Promise<unknown>, ok: string) {
    if (!connected) {
      toast.error("Engine chưa kết nối");
      return;
    }
    setBusy(id);
    try {
      await fn();
      toast.success(ok);
      await onDone?.();
    } catch (cause) {
      toast.error("Lệnh thất bại", {
        description: cause instanceof Error ? cause.message : "API lỗi",
      });
    } finally {
      setBusy(null);
    }
  }

  return (
    <section className="translation-panel mc-engine-control" style={{ marginBottom: 12 }}>
      <div className="batch-toolbar">
        <div>
          <p className="panel-kicker">BẢNG ĐIỀU KHIỂN</p>
          <h2 style={{ margin: 0, fontSize: 18 }}>Điều khiển sáng tác từ Web</h2>
          <p className="muted" style={{ margin: "6px 0 0" }}>
            Resume / tạm dừng / chỉ dẫn / chế độ duyệt chương — không cần TUI cho vòng đời thường ngày.
            {snap?.book_dir ? (
              <>
                {" "}
                <span className="mono">{snap.book_dir}</span>
              </>
            ) : null}
          </p>
          {snap?.has_advance_hold && snap.advance_hold_reason ? (
            <p className="muted" style={{ marginTop: 6 }}>
              Đang giữ nhịp: {snap.advance_hold_reason}
            </p>
          ) : null}
          {snap?.pending_steer ? (
            <p className="muted" style={{ marginTop: 6 }}>
              Chỉ dẫn chờ xử lý: {snap.pending_steer}
            </p>
          ) : null}
        </div>
        <div className="toolbar-actions" style={{ flexWrap: "wrap" }}>
          <button
            type="button"
            className="btn btn-primary"
            disabled={!connected || !!busy || action.kind === "disabled"}
            onClick={() =>
              void run(
                "primary",
                () => (action.kind === "abort" ? abortEngine() : resumeEngine()),
                action.kind === "abort" ? "Đã tạm dừng engine" : "Đã resume engine",
              )
            }
          >
            {action.kind === "abort" ? <Pause size={16} /> : <Play size={16} />}
            {busy === "primary" ? "Đang gửi…" : action.label}
          </button>

          <button
            type="button"
            className="btn"
            disabled={!connected || !!busy}
            onClick={() =>
              void run(
                "mode",
                () => setAdvanceMode(isReview ? "auto" : "review"),
                isReview ? "Đã bật viết tự động" : "Đã bật duyệt từng chương",
              )
            }
          >
            <ShieldCheck size={16} />
            {isReview ? "Duyệt: BẬT → tắt" : "Duyệt: TẮT → bật"}
          </button>

          <button
            type="button"
            className="btn"
            disabled={!connected || !!busy || !!snap?.is_running}
            title="Cho phép viết thêm 1 chương khi đang ở chế độ duyệt"
            onClick={() =>
              void run("next", () => advanceNextChapter(), "Đã cho phép chương tiếp theo")
            }
          >
            <StepForward size={16} />
            Cho phép +1 chương
          </button>

          <button
            type="button"
            className="btn"
            disabled={!connected || !!busy || !prompt.trim()}
            onClick={() =>
              void run(
                "steer",
                async () => {
                  // Prefer Steer while running; Continue works for idle injection paths.
                  if (snap?.is_running) await steerEngine(prompt.trim());
                  else await continueEngine(prompt.trim());
                  onPromptClear?.();
                },
                snap?.is_running ? "Đã gửi chỉ dẫn (steer)" : "Đã gửi chỉ dẫn (continue)",
              )
            }
          >
            <Sparkles size={16} />
            Gửi chỉ dẫn
          </button>

          <button
            type="button"
            className="btn"
            disabled={!connected || !!busy || !!snap?.is_running}
            onClick={() =>
              void run("resume2", () => resumeEngine(), "Đã resume")
            }
          >
            <FastForward size={16} />
            Resume
          </button>
        </div>
      </div>
      <div className="library-card-stats" style={{ marginTop: 10 }}>
        <span>Mode: <strong>{isReview ? "review (duyệt)" : "auto"}</strong></span>
        {typeof snap?.advance_permit_chapter === "number" && snap.advance_permit_chapter > 0 ? (
          <span>Permit ch. {snap.advance_permit_chapter}</span>
        ) : null}
        <span>{snap?.is_running ? "Engine: chạy" : "Engine: rảnh"}</span>
        {waitingGate ? <span>Chờ bạn cho phép chương mới</span> : null}
      </div>
    </section>
  );
}
