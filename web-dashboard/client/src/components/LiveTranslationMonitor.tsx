import { useEffect, useMemo, useState } from "react";
import { CircleStop, Eye, Pause, Play, Radio, Send, Sparkles } from "lucide-react";
import { toast } from "sonner";
import {
  pauseTranslation,
  resumeTranslation,
  setTranslationInstruction,
  stopTranslationChapter,
  type LiveTranslationProgress,
  type TranslationChapter,
} from "@/lib/engineApi";

type Props = {
  progress: LiveTranslationProgress[];
	progressHistory: Record<number, LiveTranslationProgress[]>;
  chapters: TranslationChapter[];
  connected: boolean;
  paused: boolean;
  onAfterControl: () => void;
};

const stageLabel: Record<string, string> = {
  preparing: "Đang chuẩn bị",
  streaming: "Đang sinh bản nháp",
  completed: "Đã lưu bản dịch",
  failed: "Cần can thiệp",
};

function displayPreview(value?: string) {
  if (!value) return "Chưa nhận được delta bản nháp từ worker.";
  return value.length > 12000 ? `…${value.slice(-12000)}` : value;
}

export default function LiveTranslationMonitor({ progress, progressHistory, chapters, connected, paused, onAfterControl }: Props) {
  const active = useMemo(() => progress.filter((item) => item.stage === "preparing" || item.stage === "streaming"), [progress]);
  const [focusedChapter, setFocusedChapter] = useState<number | null>(null);
  const [instruction, setInstruction] = useState("");
  const [controlPending, setControlPending] = useState(false);
  const [instructionPending, setInstructionPending] = useState(false);

  useEffect(() => {
    if (focusedChapter !== null && progress.some((item) => item.chapter === focusedChapter)) return;
    setFocusedChapter(active[0]?.chapter ?? progress[0]?.chapter ?? null);
  }, [active, focusedChapter, progress]);

  const focused = useMemo(() => progress.find((item) => item.chapter === focusedChapter) || active[0] || progress[0], [active, focusedChapter, progress]);
  const focusedRecord = useMemo(() => chapters.find((item) => item.chapter === focused?.chapter), [chapters, focused?.chapter]);
	const timeline = focused ? progressHistory[focused.chapter] || [] : [];
  const stopAvailable = focused?.stage === "preparing" || focused?.stage === "streaming";

  const handlePauseResume = async () => {
    if (!connected) return toast.error("Engine local đang mất kết nối");
    setControlPending(true);
    try {
      const result = paused ? await resumeTranslation() : await pauseTranslation();
      toast.success(paused ? "Đã tiếp tục hàng đợi" : "Đã tạm dừng hàng đợi", { description: result.message });
      onAfterControl();
    } catch (error) {
      toast.error("Không thể đổi trạng thái hàng đợi", { description: error instanceof Error ? error.message : "Engine trả về lỗi." });
    } finally {
      setControlPending(false);
    }
  };

  const handleStop = async () => {
    if (!focused) return;
    setControlPending(true);
    try {
      const result = await stopTranslationChapter(focused.chapter);
      toast.success(`Đã gửi yêu cầu dừng chapter ${String(focused.chapter).padStart(2, "0")}`, { description: result.message });
      onAfterControl();
    } catch (error) {
      toast.error("Không thể dừng chapter", { description: error instanceof Error ? error.message : "Chapter có thể đã hoàn tất." });
    } finally {
      setControlPending(false);
    }
  };

  const handleInstruction = async () => {
    if (!focused) return;
    setInstructionPending(true);
    try {
      await setTranslationInstruction(focused.chapter, instruction);
      toast.success(`Đã lưu chỉ dẫn cho chapter ${String(focused.chapter).padStart(2, "0")}`, { description: "Chỉ dẫn được dùng ở lần gọi Translator tiếp theo, không sửa bản Trung." });
      setInstruction("");
      onAfterControl();
    } catch (error) {
      toast.error("Không thể lưu chỉ dẫn", { description: error instanceof Error ? error.message : "Engine trả về lỗi." });
    } finally {
      setInstructionPending(false);
    }
  };

  return <section className="live-translation-monitor" aria-label="Giám sát trực tiếp Translation Agent">
    <div className="live-monitor-header">
      <div>
        <p className="panel-kicker"><Radio size={13} /> LIVE TELEMETRY</p>
        <h2>Translation Agent đang làm gì?</h2>
        <p>Xem chapter được worker xử lý, nguồn Trung đã đọc và luồng bản nháp Việt trước khi artifact được lưu.</p>
      </div>
      <div className="live-monitor-actions">
        <span className={`queue-mode ${paused ? "is-paused" : ""}`}><span />{paused ? "Đang tạm dừng" : "Đang nhận chapter"}</span>
        <button className="subtle-button" type="button" onClick={() => void handlePauseResume()} disabled={!connected || controlPending}>
          {paused ? <Play size={14} /> : <Pause size={14} />}{paused ? "Tiếp tục" : "Tạm dừng"}
        </button>
      </div>
    </div>

    <div className="live-monitor-grid">
      <div className="live-worker-list">
        <div className="monitor-list-title"><Sparkles size={15} /><span>Worker hiện diện</span><strong>{active.length}</strong></div>
        {progress.length === 0 ? <div className="live-monitor-empty"><Eye size={20} /><p>Chưa có stream trong phiên này. Khi worker nhận chapter mới, dữ liệu live sẽ xuất hiện tại đây.</p></div> : progress.slice().sort((a, b) => new Date(b.updated_at).getTime() - new Date(a.updated_at).getTime()).slice(0, 6).map((item) => <button key={`${item.chapter}-${item.updated_at}`} type="button" className={`live-worker-card ${focused?.chapter === item.chapter ? "is-focused" : ""} ${item.stage}`} onClick={() => setFocusedChapter(item.chapter)}>
          <span className="worker-card-dot" />
          <span><small>CHAPTER {String(item.chapter).padStart(2, "0")}</small><strong>{stageLabel[item.stage] || item.stage}</strong><em>{item.detail || (item.error ? "Có lỗi cần xem lại" : "Đang chờ dữ liệu")}</em></span>
        </button>)}
      </div>

      <div className="live-preview-shell">
        {focused ? <>
          <div className="live-preview-title"><div><span>CHAPTER {String(focused.chapter).padStart(2, "0")}</span><strong>{stageLabel[focused.stage] || focused.stage}</strong></div><time>{new Date(focused.updated_at).toLocaleTimeString("vi-VN", { hour: "2-digit", minute: "2-digit", second: "2-digit" })}</time></div>
          {focused.error && <div className="live-error-note">{focused.error}</div>}
          <div className="live-preview-columns">
            <article><header>NGUỒN TRUNG · READ-ONLY</header><pre>{focused.source_preview || "Nguồn được gửi khi worker bắt đầu chapter. Hãy chọn chapter đang chạy để xem."}</pre></article>
            <article><header>BẢN NHÁP VIỆT · STREAM</header><pre className="draft-preview">{displayPreview(focused.preview)}</pre></article>
          </div>
		  <div className="live-timeline" aria-label={`Nhật ký live chapter ${focused.chapter}`}>
			<header>NHẬT KÝ THEO CHAPTER</header>
			{timeline.length === 0 ? <span>Chưa có mốc telemetry trong phiên này.</span> : <ol>{timeline.map((entry, index) => <li key={`${entry.stage}-${entry.updated_at}-${index}`}><time>{new Date(entry.updated_at).toLocaleTimeString("vi-VN", { hour: "2-digit", minute: "2-digit", second: "2-digit" })}</time><i className={entry.stage} /><strong>{stageLabel[entry.stage] || entry.stage}</strong><span>{entry.error || entry.detail || "Đã nhận cập nhật từ worker."}</span></li>)}</ol>}
		  </div>
          <div className="live-intervention">
            <div><label htmlFor="chapter-instruction">Chỉ dẫn cho lần xử lý kế tiếp</label><textarea id="chapter-instruction" value={instruction} onChange={(event) => setInstruction(event.target.value)} maxLength={4000} rows={2} placeholder="Ví dụ: Giữ cách xưng hô 'ta – ngươi', ưu tiên tên Hán–Việt trong glossary…" /><small>{focusedRecord?.instruction ? `Đang có chỉ dẫn đã lưu: ${focusedRecord.instruction}` : "Chỉ dẫn được lưu bền vững; bản tiếng Trung không bị thay đổi."}</small></div>
            <div className="intervention-actions"><button className="subtle-button" type="button" onClick={() => void handleInstruction()} disabled={!connected || instructionPending || !instruction.trim()}><Send size={14} />{instructionPending ? "Đang lưu…" : "Lưu chỉ dẫn"}</button><button className="danger-button" type="button" onClick={() => void handleStop()} disabled={!connected || controlPending || !stopAvailable}><CircleStop size={14} />Dừng chapter</button></div>
          </div>
        </> : <div className="live-monitor-empty large"><Eye size={22} /><strong>Đang chờ Translation Agent phát dữ liệu.</strong><p>Hàng đợi bền vững vẫn hoạt động bình thường; telemetry sẽ xuất hiện khi một worker bắt đầu chapter mới.</p></div>}
      </div>
    </div>
  </section>;
}
