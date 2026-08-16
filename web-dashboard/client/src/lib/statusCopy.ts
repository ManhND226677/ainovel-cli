/** Human labels for engine phase/flow/runtime — keep UI scannable in Vietnamese. */
import type { EngineState, LiveAgent, LiveEvent } from "@/lib/engineApi";

const phaseLabel: Record<string, string> = {
  init: "Khởi tạo",
  premise: "Tiền đề",
  outline: "Đề cương",
  writing: "Đang viết",
  complete: "Hoàn tất",
};

const flowLabel: Record<string, string> = {
  writing: "Viết chương",
  reviewing: "Biên tập",
  rewriting: "Viết lại",
  polishing: "Mài giũa",
  steering: "Xử lý chỉ dẫn",
};

const runtimeLabel: Record<string, string> = {
  running: "Đang chạy",
  paused: "Tạm dừng",
  idle: "Chờ",
  complete: "Xong",
  failed: "Lỗi",
  stopped: "Dừng",
};

export function labelPhase(v?: string) {
  if (!v) return "—";
  return phaseLabel[v] || v;
}

export function labelFlow(v?: string) {
  if (!v) return "—";
  return flowLabel[v] || v;
}

export function labelRuntime(v?: string) {
  if (!v) return "—";
  return runtimeLabel[v] || v;
}

export function canonicalAgentName(value: string) {
  if (!value) return "Engine";
  const lower = value.toLowerCase();
  if (value === "translator" || lower.includes("translation")) return "Translation";
  if (lower.includes("architect")) return "Architect";
  if (lower.includes("writer")) return "Writer";
  if (lower.includes("editor")) return "Editor";
  if (lower.includes("arbiter")) return "Arbiter";
  return value;
}

export function agentRouteSlug(name: string) {
  const canonical = canonicalAgentName(name);
  const label = canonical === "Translation" ? "Translation Agent" : canonical;
  return label.toLowerCase().replaceAll(" ", "-");
}

export function agentWorking(agent: LiveAgent) {
  const s = (agent.state || "").toLowerCase();
  return s === "working" || s === "running" || s === "active";
}

/** One-line “what is happening now” for mission control. */
export function nowPlaying(engine: EngineState | null | undefined, events: LiveEvent[]): {
  title: string;
  detail: string;
  tone: "work" | "wait" | "done" | "error" | "off";
  agent?: string;
} {
  if (!engine?.online && !engine?.snapshot) {
    return { title: "Chưa kết nối engine", detail: "Chạy ainovel-cli web rồi mở lại dashboard.", tone: "off" };
  }
  const snap = engine.snapshot;
  if (!snap?.is_running) {
    if (snap?.phase === "complete") {
      return { title: "Sách đã hoàn tất", detail: snap.last_commit_summary || "Có thể xuất bản hoặc dịch nốt chương còn lại.", tone: "done" };
    }
    return {
      title: snap?.status_label || "Engine đang chờ",
      detail: snap?.last_commit_summary || snap?.last_review_summary || "Bấm Tiếp tục để resume từ checkpoint.",
      tone: "wait",
    };
  }
  const active = (engine.agents || []).find(agentWorking);
  if (active) {
    const name = canonicalAgentName(active.name || active.role);
    const tool = active.tool ? ` · ${active.tool}` : "";
    return {
      title: `${name} đang làm việc`,
      detail: active.summary || active.task_kind || `Lượt ${active.turn || 0}${tool}`,
      tone: "work",
      agent: name,
    };
  }
  const latest = [...events].reverse().find((e) => e.summary);
  if (latest) {
    return {
      title: latest.summary || "Engine đang chạy",
      detail: `${canonicalAgentName(latest.agent || "")} · ${latest.category || "SYSTEM"}`,
      tone: latest.failed || latest.level === "error" ? "error" : "work",
      agent: canonicalAgentName(latest.agent || ""),
    };
  }
  return {
    title: labelFlow(snap.flow),
    detail: snap.current_chapter > 0 ? `Chương ${snap.current_chapter}` : labelPhase(snap.phase),
    tone: "work",
  };
}

export function primaryAction(engine: EngineState | null | undefined): {
  kind: "resume" | "abort" | "disabled";
  label: string;
  hint: string;
} {
  const snap = engine?.snapshot;
  if (!engine || !snap) {
    return { kind: "disabled", label: "Chờ kết nối", hint: "API local chưa online" };
  }
  if (snap.is_running) {
    return { kind: "abort", label: "Tạm dừng", hint: "Giữ checkpoint, dừng vòng engine" };
  }
  if (snap.phase === "complete") {
    return { kind: "resume", label: "Mở lại / tiếp", hint: "Resume lifecycle nếu còn việc phụ" };
  }
  return { kind: "resume", label: "Tiếp tục sáng tác", hint: "Resume từ checkpoint gần nhất" };
}
