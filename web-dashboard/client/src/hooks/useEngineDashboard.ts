import { useCallback, useEffect, useRef, useState } from "react";
import {
  fetchEngineEvents,
  fetchEngineState,
  getEngineWebSocketURL,
  type EngineSocketMessage,
  type EngineState,
  type LiveEvent,
  type LiveTranslationProgress,
} from "@/lib/engineApi";

type ConnectionTransport = "connecting" | "live" | "polling" | "offline";

/** Stable identity for React keys + merge. Live WS events often have seq=0. */
function eventKey(event: LiveEvent) {
  if (event.seq && event.seq > 0) return `seq:${event.seq}`;
  return [
    "z",
    event.time || "",
    event.task_id || "",
    event.agent || "",
    event.category || "",
    event.kind || "",
    event.summary || "",
    event.level || "",
    event.failed ? "1" : "0",
  ].join("|");
}

function matchesFilters(event: LiveEvent, filters: { agent?: string; category?: string; kind?: string; level?: string }) {
  const canonicalAgent = (event.agent || "").toLowerCase().includes("translation") || event.agent === "translator"
    ? "translation agent"
    : (event.agent || "").toLowerCase();
  return (!filters.agent || canonicalAgent === filters.agent.toLowerCase())
    && (!filters.category || (event.category || "").toLowerCase() === filters.category.toLowerCase())
    && (!filters.kind || (event.kind || "").toLowerCase() === filters.kind.toLowerCase())
    && (!filters.level || (event.level || "").toLowerCase() === filters.level.toLowerCase());
}

function mergeEvents(current: LiveEvent[], incoming: LiveEvent[], filters: { agent?: string; category?: string; kind?: string; level?: string }) {
  if (incoming.length === 0) return current;
  const merged = new Map<string, LiveEvent>();
  for (const event of current) merged.set(eventKey(event), event);
  let changed = false;
  for (const event of incoming) {
    if (!matchesFilters(event, filters)) continue;
    const key = eventKey(event);
    const prev = merged.get(key);
    if (!prev) {
      merged.set(key, event);
      changed = true;
      continue;
    }
    // Prefer richer payload / higher seq without reshuffling identical rows.
    if ((event.seq || 0) > (prev.seq || 0) || (event.detail && event.detail !== prev.detail)) {
      merged.set(key, { ...prev, ...event });
      changed = true;
    }
  }
  if (!changed && merged.size === current.length) return current;
  return Array.from(merged.values()).sort((left, right) => {
    if (left.seq && right.seq && left.seq !== right.seq) return left.seq - right.seq;
    const lt = new Date(left.time).getTime();
    const rt = new Date(right.time).getTime();
    if (lt !== rt) return lt - rt;
    return eventKey(left).localeCompare(eventKey(right));
  }).slice(-120);
}

function sameEventList(a: LiveEvent[], b: LiveEvent[]) {
  if (a === b) return true;
  if (a.length !== b.length) return false;
  for (let i = 0; i < a.length; i += 1) {
    if (eventKey(a[i]) !== eventKey(b[i])) return false;
    if ((a[i].summary || "") !== (b[i].summary || "")) return false;
  }
  return true;
}

/** Ignore pure clock ticks on updated_at so idle engine doesn't thrash React. */
function meaningfulEngineChanged(prev: EngineState | null, next: EngineState) {
  if (!prev) return true;
  if (prev.online !== next.online) return true;
  if (prev.translation_paused !== next.translation_paused) return true;
  if (JSON.stringify(prev.snapshot) !== JSON.stringify({ ...next.snapshot })) return true;
  if (JSON.stringify(prev.agents) !== JSON.stringify(next.agents)) return true;
  if (JSON.stringify(prev.translation) !== JSON.stringify(next.translation)) return true;
  return false;
}

function readableConnectionError(cause: unknown) {
  if (cause instanceof TypeError) return "Không thể mở kết nối tới engine Go local; hãy kiểm tra tiến trình ainovel-cli web.";
  if (cause instanceof Error && cause.message) return cause.message;
  return "Không thể kết nối engine Go local.";
}

export function useEngineDashboard(filters: { agent?: string; category?: string; kind?: string; level?: string }) {
  const [engine, setEngine] = useState<EngineState | null>(null);
  const [events, setEvents] = useState<LiveEvent[]>([]);
  const [liveProgress, setLiveProgress] = useState<Record<number, LiveTranslationProgress>>({});
  const [liveProgressHistory, setLiveProgressHistory] = useState<Record<number, LiveTranslationProgress[]>>({});
  const [connected, setConnected] = useState(false);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [transport, setTransport] = useState<ConnectionTransport>("connecting");
  const [retryIn, setRetryIn] = useState(0);
  const transportRef = useRef<ConnectionTransport>("connecting");

  useEffect(() => {
    transportRef.current = transport;
  }, [transport]);

  const applyEngine = useCallback((next: EngineState) => {
    setEngine((prev) => (meaningfulEngineChanged(prev, next) ? next : prev));
  }, []);

  const refresh = useCallback(async () => {
    try {
      const [nextEngine, nextEvents] = await Promise.all([fetchEngineState(), fetchEngineEvents(filters)]);
      applyEngine(nextEngine);
      setEvents((current) => {
        // Manual refresh always accepts server list, but keep referential equality when identical.
        const next = nextEvents.items || [];
        return sameEventList(current, next) ? current : next.slice(-120);
      });
      setConnected(true);
      setTransport((current) => (current === "live" ? current : "polling"));
      setError(null);
    } catch (cause) {
      setConnected(false);
      setTransport("offline");
      setError(readableConnectionError(cause));
    } finally {
      setLoading(false);
    }
  }, [applyEngine, filters.agent, filters.category, filters.kind, filters.level]);

  useEffect(() => {
    let disposed = false;
    let socket: WebSocket | null = null;
    let reconnectTimer: number | undefined;
    let pollingTimer: number | undefined;
    let countdownTimer: number | undefined;
    let retryAt = 0;
    let attempts = 0;
    let socketGeneration = 0;

    const clearPolling = () => {
      if (pollingTimer !== undefined) window.clearInterval(pollingTimer);
      pollingTimer = undefined;
    };

    const poll = async () => {
      // When WebSocket is healthy, skip polling entirely — it was rewriting the
      // event list every 5s and making the activity feed "jump" while idle.
      const mode = () => transportRef.current;
      if (mode() === "live") return;
      try {
        const [nextEngine, nextEvents] = await Promise.all([fetchEngineState(), fetchEngineEvents(filters)]);
        if (disposed) return;
        if (mode() === "live") return;
        applyEngine(nextEngine);
        setEvents((current) => {
          if (mode() === "live") return current;
          const next = nextEvents.items || [];
          return sameEventList(current, next) ? current : next.slice(-120);
        });
        setConnected(true);
        setTransport((current) => (current === "live" ? current : "polling"));
        setError(null);
      } catch (cause) {
        if (disposed) return;
        if (mode() === "live") return;
        setConnected(false);
        setTransport("offline");
        setError(readableConnectionError(cause));
      } finally {
        if (!disposed) setLoading(false);
      }
    };

    const startPolling = () => {
      if (pollingTimer === undefined) pollingTimer = window.setInterval(() => void poll(), 8000);
    };

    const scheduleReconnect = () => {
      if (disposed) return;
      attempts += 1;
      const delay = Math.min(10000, 800 * (2 ** Math.min(attempts - 1, 4)));
      retryAt = Date.now() + delay;
      setRetryIn(Math.ceil(delay / 1000));
      if (countdownTimer !== undefined) window.clearInterval(countdownTimer);
      countdownTimer = window.setInterval(() => {
        const remaining = Math.max(0, Math.ceil((retryAt - Date.now()) / 1000));
        setRetryIn(remaining);
        if (remaining === 0 && countdownTimer !== undefined) {
          window.clearInterval(countdownTimer);
          countdownTimer = undefined;
        }
      }, 250);
      reconnectTimer = window.setTimeout(connect, delay);
    };

    function maybeNotify(event: LiveEvent) {
      // Desktop notifications: errors + batch-level only. Never spam per-chapter
      // "Đã dịch xong chương N" on every page open / WS snapshot.
      if (typeof window === "undefined" || !("Notification" in window)) return;
      if (Notification.permission !== "granted") return;
      const summary = (event.summary || "").toLowerCase();
      const category = (event.category || "").toUpperCase();
      const agent = (event.agent || "").toLowerCase();
      const isError = event.level === "error" || event.failed || summary.includes("lỗi") || summary.includes("failed");
      const isBatch =
        summary.includes("lô")
        || summary.includes("batch")
        || summary.includes("hàng đợi")
        || summary.includes("xếp hàng");
      const isTranslation = agent.includes("translation") || category === "TRANSLATION" || category === "REVIEW";
      if (!isTranslation) return;
      if (!isError && !isBatch) return;
      // Ignore stale history (>2 min) that can ride along non-snapshot messages.
      const ageMs = Date.now() - new Date(event.time).getTime();
      if (Number.isFinite(ageMs) && ageMs > 120_000) return;
      new Notification(isError ? "Cảnh báo dịch thuật ainovel" : "Bản dịch Việt cập nhật", {
        body: event.summary || "Trạng thái batch dịch đã thay đổi.",
      });
    }

    function applySocketMessage(message: EngineSocketMessage) {
      if (message.type === "error") {
        setError(message.error || "WebSocket engine trả về lỗi không xác định.");
        return;
      }
      if (message.state) applyEngine(message.state);
      if (message.type === "translation_progress" && message.live_progress) {
        const incomingProgress = message.live_progress;
        setLiveProgress((current) => {
          const previous = current[incomingProgress.chapter];
          const isDelta = incomingProgress.stage === "streaming" && Boolean(incomingProgress.preview);
          const preview = isDelta
            ? `${previous?.preview || ""}${incomingProgress.preview || ""}`.slice(-24000)
            : incomingProgress.preview ?? previous?.preview;
          const next = { ...previous, ...incomingProgress, preview };
          if (
            previous
            && previous.stage === next.stage
            && previous.preview === next.preview
            && previous.detail === next.detail
            && previous.error === next.error
          ) {
            return current;
          }
          return { ...current, [incomingProgress.chapter]: next };
        });
        setLiveProgressHistory((current) => {
          const previous = current[incomingProgress.chapter] || [];
          const last = previous[previous.length - 1];
          if (incomingProgress.stage === "streaming" && last?.stage === "streaming") {
            const mergedPreview = `${last.preview || ""}${incomingProgress.preview || ""}`.slice(-6000);
            return {
              ...current,
              [incomingProgress.chapter]: [...previous.slice(0, -1), { ...last, ...incomingProgress, preview: mergedPreview }],
            };
          }
          return {
            ...current,
            [incomingProgress.chapter]: [...previous, incomingProgress].slice(-24),
          };
        });
      }
      const incoming = [...(message.events || []), ...(message.event ? [message.event] : [])];
      if (incoming.length > 0) {
        setEvents((current) => {
          // Initial snapshot: replace once if empty, otherwise merge (avoid full reshuffle).
          if (message.type === "snapshot" && current.length === 0) {
            return mergeEvents([], incoming, filters);
          }
          return mergeEvents(current, incoming, filters);
        });
        const latest = incoming[incoming.length - 1];
        if (latest && message.type !== "snapshot") maybeNotify(latest);
      }
      setLoading(false);
      setConnected(true);
      setTransport("live");
      setRetryIn(0);
      setError(null);
    }

    function connect() {
      if (disposed) return;
      const generation = ++socketGeneration;
      setTransport(attempts > 0 ? "polling" : "connecting");
      try {
        socket = new WebSocket(getEngineWebSocketURL());
      } catch (cause) {
        setError(readableConnectionError(cause));
        startPolling();
        scheduleReconnect();
        return;
      }
      socket.onopen = () => {
        if (disposed || generation !== socketGeneration) return;
        attempts = 0;
        clearPolling();
        setConnected(true);
        setTransport("live");
        setRetryIn(0);
        setError(null);
      };
      socket.onmessage = (message) => {
        if (disposed || generation !== socketGeneration) return;
        try {
          applySocketMessage(JSON.parse(message.data) as EngineSocketMessage);
        } catch {
          setError("WebSocket gửi dữ liệu không hợp lệ; đang chờ lần kết nối kế tiếp.");
        }
      };
      socket.onerror = () => {
        if (!disposed && generation === socketGeneration) {
          setError("WebSocket không phản hồi; dashboard sẽ tự chuyển sang polling tạm thời.");
        }
      };
      socket.onclose = () => {
        if (disposed || generation !== socketGeneration) return;
        setConnected(false);
        setTransport("polling");
        startPolling();
        scheduleReconnect();
      };
    }

    setLoading(true);
    // One bootstrap fetch, then prefer WebSocket. No interval while live.
    void poll();
    connect();
    return () => {
      disposed = true;
      socketGeneration += 1;
      clearPolling();
      if (reconnectTimer !== undefined) window.clearTimeout(reconnectTimer);
      if (countdownTimer !== undefined) window.clearInterval(countdownTimer);
      socket?.close();
    };
  }, [applyEngine, filters.agent, filters.category, filters.kind, filters.level]);

  return {
    engine,
    events,
    liveProgress: Object.values(liveProgress),
    liveProgressHistory,
    connected,
    loading,
    error,
    transport,
    retryIn,
    refresh,
  };
}
