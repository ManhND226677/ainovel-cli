/** Ctrl/⌘+K command palette — jump + engine actions without leaving the page. */
import { useEffect, useMemo, useState } from "react";
import { useLocation } from "wouter";
import {
  ArchiveRestore,
  BookOpenText,
  Gauge,
  Languages,
  Library,
  Pause,
  Play,
  Settings2,
  Sparkles,
  Split,
  StepForward,
} from "lucide-react";
import { toast } from "sonner";
import {
  abortEngine,
  advanceNextChapter,
  continueEngine,
  pauseTranslation,
  requestTranslation,
  resumeEngine,
  resumeTranslation,
  setAdvanceMode,
  steerEngine,
} from "@/lib/engineApi";
import { useEngine } from "@/contexts/EngineContext";
import { primaryAction } from "@/lib/statusCopy";

type Command = {
  id: string;
  label: string;
  hint?: string;
  group: string;
  icon: typeof Gauge;
  run: () => void | Promise<void>;
};

export default function CommandPalette() {
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState("");
  const [active, setActive] = useState(0);
  const [, setLocation] = useLocation();
  const { connected, refresh, engine } = useEngine();
  const action = primaryAction(engine);

  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === "k") {
        event.preventDefault();
        setOpen((value) => !value);
        setQuery("");
        setActive(0);
      }
      if (event.key === "Escape") setOpen(false);
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);

  const commands = useMemo<Command[]>(() => {
    const go = (path: string) => () => {
      setLocation(path);
      setOpen(false);
    };
    const list: Command[] = [
      { id: "nav-home", label: "Mở Điều khiển", hint: "Mission Control", group: "Điều hướng", icon: Gauge, run: go("/") },
      { id: "nav-lib", label: "Mở Thư viện truyện", group: "Điều hướng", icon: Library, run: go("/library") },
      { id: "nav-co", label: "Đồng sáng tác (startup)", hint: "Chat + draft brief", group: "Sáng tác", icon: Sparkles, run: go("/cocreate") },
      { id: "nav-co-stage", label: "Đồng sáng tác giai đoạn", hint: "Pause + plan next arc", group: "Sáng tác", icon: Sparkles, run: go("/cocreate?mode=stage") },
      { id: "nav-ms", label: "Mở Bản thảo ZH↔VI", hint: "Đọc chương", group: "Điều hướng", icon: BookOpenText, run: go("/manuscript") },
      { id: "nav-tr", label: "Mở Bản dịch Việt", hint: "Batch / retry", group: "Điều hướng", icon: Split, run: go("/translation") },
      { id: "nav-snap", label: "Mở Snapshot", group: "Điều hướng", icon: ArchiveRestore, run: go("/snapshots") },
      { id: "nav-set", label: "Mở Thiết lập Model", group: "Điều hướng", icon: Settings2, run: go("/settings") },
      {
        id: "eng-primary",
        label: action.kind === "abort" ? "Tạm dừng engine" : "Tiếp tục / resume engine",
        hint: action.hint,
        group: "Engine",
        icon: action.kind === "abort" ? Pause : Play,
        run: async () => {
          if (!connected) return toast.error("Engine chưa kết nối");
          try {
            if (action.kind === "abort") await abortEngine();
            else await resumeEngine();
            toast.success(action.kind === "abort" ? "Đã tạm dừng" : "Đã resume");
            await refresh();
            setOpen(false);
          } catch (cause) {
            toast.error("Lệnh engine thất bại", { description: cause instanceof Error ? cause.message : "API lỗi" });
          }
        },
      },
      {
        id: "eng-steer",
        label: "Gửi chỉ dẫn nhanh…",
        hint: "Steer/Continue → Arbiter",
        group: "Engine",
        icon: Sparkles,
        run: async () => {
          if (!connected) return toast.error("Engine chưa kết nối");
          const text = window.prompt("Chỉ dẫn cho Arbiter");
          if (!text?.trim()) return;
          try {
            if (engine?.snapshot?.is_running) await steerEngine(text.trim());
            else await continueEngine(text.trim());
            toast.success("Đã gửi chỉ dẫn");
            await refresh();
            setOpen(false);
          } catch (cause) {
            toast.error("Gửi chỉ dẫn thất bại", { description: cause instanceof Error ? cause.message : "API lỗi" });
          }
        },
      },
      {
        id: "eng-review-on",
        label: "Bật duyệt từng chương",
        hint: "advance mode = review",
        group: "Sáng tác",
        icon: Sparkles,
        run: async () => {
          if (!connected) return toast.error("Engine chưa kết nối");
          try {
            await setAdvanceMode("review");
            toast.success("Đã bật chế độ duyệt");
            await refresh();
            setOpen(false);
          } catch (cause) {
            toast.error("Đổi mode thất bại", { description: cause instanceof Error ? cause.message : "API lỗi" });
          }
        },
      },
      {
        id: "eng-review-off",
        label: "Tắt duyệt — viết tự động",
        hint: "advance mode = auto",
        group: "Sáng tác",
        icon: Play,
        run: async () => {
          if (!connected) return toast.error("Engine chưa kết nối");
          try {
            await setAdvanceMode("auto");
            toast.success("Đã bật auto");
            await refresh();
            setOpen(false);
          } catch (cause) {
            toast.error("Đổi mode thất bại", { description: cause instanceof Error ? cause.message : "API lỗi" });
          }
        },
      },
      {
        id: "eng-next",
        label: "Cho phép +1 chương (/tiep-tuc)",
        hint: "AdvanceOneChapter",
        group: "Sáng tác",
        icon: StepForward,
        run: async () => {
          if (!connected) return toast.error("Engine chưa kết nối");
          try {
            await advanceNextChapter();
            toast.success("Đã cho phép chương tiếp");
            await refresh();
            setOpen(false);
          } catch (cause) {
            toast.error("Advance next thất bại", { description: cause instanceof Error ? cause.message : "API lỗi" });
          }
        },
      },
      {
        id: "tr-request",
        label: "Gọi Translation Coordinator",
        hint: "/dich",
        group: "Dịch",
        icon: Languages,
        run: async () => {
          if (!connected) return toast.error("Engine chưa kết nối");
          try {
            const result = await requestTranslation();
            toast.success("Đã gọi Coordinator", { description: result.message });
            await refresh();
            setOpen(false);
          } catch (cause) {
            toast.error("Coordinator lỗi", { description: cause instanceof Error ? cause.message : "API lỗi" });
          }
        },
      },
      {
        id: "tr-pause",
        label: "Tạm dừng hàng đợi dịch",
        group: "Dịch",
        icon: Pause,
        run: async () => {
          if (!connected) return toast.error("Engine chưa kết nối");
          try {
            const result = await pauseTranslation();
            toast.success(result.message || "Đã pause dịch");
            await refresh();
            setOpen(false);
          } catch (cause) {
            toast.error("Pause dịch lỗi", { description: cause instanceof Error ? cause.message : "API lỗi" });
          }
        },
      },
      {
        id: "tr-resume",
        label: "Tiếp tục hàng đợi dịch",
        group: "Dịch",
        icon: Play,
        run: async () => {
          if (!connected) return toast.error("Engine chưa kết nối");
          try {
            const result = await resumeTranslation();
            toast.success(result.message || "Đã resume dịch");
            await refresh();
            setOpen(false);
          } catch (cause) {
            toast.error("Resume dịch lỗi", { description: cause instanceof Error ? cause.message : "API lỗi" });
          }
        },
      },
    ];
    return list;
  }, [action.hint, action.kind, connected, engine?.snapshot?.is_running, refresh, setLocation]);

  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase();
    if (!q) return commands;
    return commands.filter((item) => `${item.label} ${item.hint || ""} ${item.group}`.toLowerCase().includes(q));
  }, [commands, query]);

  useEffect(() => {
    setActive(0);
  }, [query, open]);

  if (!open) return null;

  return (
    <div className="cmdk-root" role="dialog" aria-modal="true" aria-label="Command palette">
      <button type="button" className="cmdk-backdrop" aria-label="Đóng" onClick={() => setOpen(false)} />
      <div className="cmdk-panel">
        <input
          autoFocus
          className="cmdk-input"
          placeholder="Gõ lệnh hoặc trang… (Esc đóng)"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "ArrowDown") {
              e.preventDefault();
              setActive((i) => Math.min(filtered.length - 1, i + 1));
            } else if (e.key === "ArrowUp") {
              e.preventDefault();
              setActive((i) => Math.max(0, i - 1));
            } else if (e.key === "Enter") {
              e.preventDefault();
              const item = filtered[active];
              if (item) void item.run();
            }
          }}
        />
        <div className="cmdk-list">
          {filtered.map((item, index) => {
            const Icon = item.icon;
            return (
              <button
                key={item.id}
                type="button"
                className={`cmdk-item ${index === active ? "is-active" : ""}`}
                onMouseEnter={() => setActive(index)}
                onClick={() => void item.run()}
              >
                <Icon size={16} />
                <span className="cmdk-item-main">
                  <strong>{item.label}</strong>
                  {item.hint ? <small>{item.hint}</small> : null}
                </span>
                <em>{item.group}</em>
              </button>
            );
          })}
          {filtered.length === 0 && <div className="cmdk-empty">Không khớp lệnh</div>}
        </div>
      </div>
    </div>
  );
}
