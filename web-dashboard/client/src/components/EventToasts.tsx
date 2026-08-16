/** Toast only for events that arrive AFTER the initial history hydrate. */
import { useEffect, useRef, useState } from "react";
import { toast } from "sonner";
import { useEngine } from "@/contexts/EngineContext";
import type { LiveEvent } from "@/lib/engineApi";

function eventFingerprint(event: LiveEvent) {
  if (event.seq && event.seq > 0) return `seq:${event.seq}`;
  return [
    "z",
    event.time || "",
    event.task_id || "",
    event.agent || "",
    event.category || "",
    event.kind || "",
    event.summary || "",
    event.failed ? "1" : "0",
  ].join("|");
}

function shouldNotify(event: LiveEvent) {
  const summary = (event.summary || "").toLowerCase();
  const category = (event.category || "").toUpperCase();
  const level = (event.level || "").toLowerCase();
  const agent = (event.agent || "").toLowerCase();

  if (event.failed || level === "error" || category === "ERROR") return "error" as const;

  // Per-chapter "Đã dịch xong chương N (job …)" is already in the activity feed.
  if (agent.includes("translation") || category === "TRANSLATION") {
    if (summary.includes("thất bại") || summary.includes("failed") || summary.includes("lỗi")) {
      return "error" as const;
    }
    if (
      summary.includes("lô")
      || summary.includes("batch")
      || summary.includes("hàng đợi")
      || summary.includes("xếp")
      || summary.includes("hoàn tất hàng")
      || summary.includes("không còn chương")
    ) {
      return "info" as const;
    }
    return null;
  }

  if (category === "REVIEW" || summary.includes("review") || summary.includes("biên tập")) {
    return "review" as const;
  }
  if (
    summary.includes("commit")
    || summary.includes("chốt chương")
    || summary.includes("đã lưu")
    || (summary.includes("hoàn tất") && !summary.includes("chương"))
    || category === "CHECK"
  ) {
    return "success" as const;
  }
  return null;
}

export default function EventToasts() {
  const { events, connected, transport } = useEngine();
  const seen = useRef<Set<string>>(new Set());
  const [liveArmed, setLiveArmed] = useState(false);
  const armTimer = useRef<number | undefined>(undefined);

  // Phase 1: while connecting / first second of live, mark everything seen and stay silent.
  useEffect(() => {
    if (!connected) {
      setLiveArmed(false);
      if (armTimer.current !== undefined) {
        window.clearTimeout(armTimer.current);
        armTimer.current = undefined;
      }
      return;
    }

    for (const event of events) {
      seen.current.add(eventFingerprint(event));
    }

    if (!liveArmed && (transport === "live" || transport === "polling") && armTimer.current === undefined) {
      armTimer.current = window.setTimeout(() => {
        // Final swallow of anything that landed during the grace window.
        setLiveArmed(true);
        armTimer.current = undefined;
      }, 1500);
    }
  }, [connected, transport, events, liveArmed]);

  // When arming flips on, mark the current buffer once more so the first
  // post-arm render does not toast the historical tail.
  useEffect(() => {
    if (!liveArmed) return;
    for (const event of events) {
      seen.current.add(eventFingerprint(event));
    }
  }, [liveArmed]); // eslint-disable-line react-hooks/exhaustive-deps -- only on arm edge

  // Phase 2: only brand-new events after arm.
  useEffect(() => {
    if (!connected || !liveArmed) return;

    for (const event of events) {
      const key = eventFingerprint(event);
      if (seen.current.has(key)) continue;
      seen.current.add(key);

      const ageMs = Date.now() - new Date(event.time).getTime();
      if (Number.isFinite(ageMs) && ageMs > 90_000) continue;

      const kind = shouldNotify(event);
      if (!kind) continue;

      const title = event.summary || event.category || "Sự kiện engine";
      const description = [event.agent, event.category].filter(Boolean).join(" · ");
      if (kind === "error") toast.error(title, { description });
      else if (kind === "success") toast.success(title, { description });
      else if (kind === "review") toast(title, { description });
      else toast.message(title, { description });
    }

    if (seen.current.size > 500) {
      seen.current = new Set(Array.from(seen.current).slice(-250));
    }
  }, [connected, liveArmed, events]);

  useEffect(() => () => {
    if (armTimer.current !== undefined) window.clearTimeout(armTimer.current);
  }, []);

  return null;
}
