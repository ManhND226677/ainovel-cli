/**
 * Mission Control — one-screen density inspired by the TUI:
 * now-playing, agents with context bars, live log, sticky command bar.
 */
import { useEffect, useMemo, useRef, useState } from "react";
import { Link } from "wouter";
import {
  ArrowUpRight,
  BookOpen,
  Bot,
  Languages,
  Layers3,
  Pause,
  PenLine,
  Play,
  Sparkles,
  Split,
  WandSparkles,
} from "lucide-react";
import { toast } from "sonner";
import { Area, AreaChart, CartesianGrid, ResponsiveContainer, Tooltip, XAxis, YAxis } from "recharts";
import ConnectionBanner from "@/components/ConnectionBanner";
import LoadingSkeleton from "@/components/LoadingSkeleton";
import EngineControlPanel from "@/components/EngineControlPanel";
import NewBookPanel from "@/components/NewBookPanel";
import { useEngine } from "@/contexts/EngineContext";
import { abortEngine, continueEngine, resumeEngine, type LiveAgent, type LiveEvent } from "@/lib/engineApi";
import { formatNovelTitle } from "@/lib/novelTitle";
import {
  agentWorking,
  canonicalAgentName,
  labelFlow,
  labelPhase,
  labelRuntime,
  primaryAction,
} from "@/lib/statusCopy";

const agentIcons: Record<string, typeof Layers3> = {
  Architect: Layers3,
  Writer: PenLine,
  Editor: BookOpen,
  Arbiter: WandSparkles,
  Translation: Languages,
};
const agentColors: Record<string, string> = {
  Architect: "green",
  Writer: "terracotta",
  Editor: "blue",
  Arbiter: "gold",
  Translation: "translation",
};
const eventCategories = ["all", "DISPATCH", "TOOL", "DECISION", "SYSTEM", "REVIEW", "CHECK", "ERROR"];

function contextTone(percent: number) {
  if (percent >= 85) return "danger";
  if (percent >= 70) return "warn";
  return "ok";
}

function agentDetail(agent: LiveAgent) {
  if (agent.summary || agent.tool) return agent.summary || agent.tool || "";
  if (agentWorking(agent)) return "Đang thực thi tác vụ";
  if ((agent.state || "").toLowerCase() === "failed") return "Cần kiểm tra lỗi";
  return "Chờ tác vụ";
}

function matchesFilters(event: LiveEvent, agent: string, category: string) {
  const canonical = canonicalAgentName(event.agent || "").toLowerCase();
  const wantAgent = agent === "all" ? "" : agent.toLowerCase().replace(" agent", "");
  const agentOk = !wantAgent || canonical.toLowerCase().includes(wantAgent.replace("translation agent", "translation"));
  const categoryOk = category === "all" || (event.category || "").toUpperCase() === category.toUpperCase();
  return agentOk && categoryOk;
}

export default function Home() {
  const [prompt, setPrompt] = useState("");
  const [filterAgent, setFilterAgent] = useState("all");
  const [filterCategory, setFilterCategory] = useState("all");
  const [busy, setBusy] = useState(false);
  const [showAllActivity, setShowAllActivity] = useState(true);
  const promptRef = useRef<HTMLTextAreaElement>(null);

  const { engine, events, connected, loading, error, transport, retryIn, refresh, playing } = useEngine();
  const snapshot = engine?.snapshot;
  const novelTitle = formatNovelTitle(snapshot?.novel_name);
  const action = useMemo(() => primaryAction(engine), [engine]);

  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      const tag = (event.target as HTMLElement | null)?.tagName;
      if (event.key === "/" && tag !== "INPUT" && tag !== "TEXTAREA") {
        event.preventDefault();
        promptRef.current?.focus();
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);

  const liveAgents = engine?.agents ?? [];
  const displayedAgents = liveAgents.map((agent) => {
    const name = canonicalAgentName(agent.name || agent.role);
    return {
      raw: agent,
      name,
      detail: agentDetail(agent),
      color: agentColors[name] || "gold",
      icon: agentIcons[name] || Bot,
      working: agentWorking(agent),
      href: `/agents/${encodeURIComponent((name === "Translation" ? "Translation Agent" : name).toLowerCase().replaceAll(" ", "-"))}`,
      percent: agent.context?.percent ?? 0,
      tokens: agent.context?.tokens ?? 0,
      window: agent.context?.window ?? 0,
      strategy: agent.context?.strategy || "",
    };
  });

  const completedCount = snapshot?.completed_count ?? 0;
  const totalChapters = snapshot?.total_chapters ?? 0;
  const progressPercent = Math.min(100, Math.round((completedCount / Math.max(totalChapters, 1)) * 100));
  const translationChapters = Object.values(engine?.translation?.chapters || {});
  const translationCompleted = translationChapters.filter((c) => c.state === "completed").length;
  const translationIssues = translationChapters.filter((c) => c.state === "failed" || c.state === "stale").length;
  const translationRunning = translationChapters.filter((c) => c.state === "running").length;
  const activeCount = displayedAgents.filter((a) => a.working).length;

  const activityRows = useMemo(() => {
    if (!connected) return [];
    return events
      .filter((event) => matchesFilters(event, filterAgent, filterCategory))
      .slice()
      .reverse()
      .map((event, index) => {
        const stable = event.seq && event.seq > 0
          ? `seq-${event.seq}`
          : `z-${event.time}-${event.task_id || ""}-${event.agent || ""}-${event.category || ""}-${event.kind || ""}-${event.summary || ""}`;
        return {
          time: new Date(event.time).toLocaleTimeString("vi-VN", { hour: "2-digit", minute: "2-digit", second: "2-digit" }),
          title: event.summary || event.category || "Sự kiện",
          detail: `${canonicalAgentName(event.agent || "")} · ${event.category || "SYSTEM"}`,
          tone: event.failed || event.level === "error" ? "gold" : event.priority === "control" ? "terracotta" : "green",
          // Index suffix only as last-resort uniqueness; primary key must be content-stable.
          key: `${stable}#${index}`,
        };
      });
  }, [connected, events, filterAgent, filterCategory]);

  const translationChart = useMemo(() => {
    const records = translationChapters.slice().sort((a, b) => new Date(a.updated_at).getTime() - new Date(b.updated_at).getTime());
    if (records.length === 0) return [];
    let completed = 0;
    let recovery = 0;
    return records.map((record) => {
      if (record.state === "completed") completed += 1;
      if (record.state === "failed" || record.state === "stale") recovery += 1;
      return {
        time: new Date(record.updated_at).toLocaleTimeString("vi-VN", { hour: "2-digit", minute: "2-digit" }),
        completed,
        recovery,
      };
    });
  }, [engine?.translation?.chapters]);

  const recentChapters = useMemo(() => {
    const n = Math.min(8, completedCount);
    return Array.from({ length: n }, (_, index) => {
      const chapter = Math.max(1, completedCount - n + 1 + index);
      const vi = engine?.translation?.chapters?.[String(chapter)];
      return { no: chapter, vi: vi?.state || "none" };
    });
  }, [completedCount, engine?.translation?.chapters]);

  // Prefer real outline titles when snapshot later exposes them via manuscript API on reader.

  const handlePrimary = async () => {
    if (!connected || action.kind === "disabled" || busy) return;
    setBusy(true);
    try {
      if (action.kind === "abort") {
        await abortEngine();
        toast.success("Đã gửi lệnh tạm dừng", { description: "Engine giữ checkpoint hiện tại." });
      } else {
        await resumeEngine();
        toast.success("Đã yêu cầu tiếp tục", { description: "Snapshot sẽ cập nhật khi worker chạy." });
      }
      await refresh();
    } catch (cause) {
      toast.error("Không điều khiển được engine", { description: cause instanceof Error ? cause.message : "API lỗi." });
    } finally {
      setBusy(false);
    }
  };

  const handlePrompt = async () => {
    if (!prompt.trim()) {
      toast("Nhập chỉ dẫn trước", { description: "Ví dụ: Giảm nhịp chiến đấu chương này, thêm đối thoại." });
      return;
    }
    if (!connected) {
      toast.error("Engine chưa kết nối");
      return;
    }
    setBusy(true);
    try {
      await continueEngine(prompt.trim());
      toast.success("Đã gửi chỉ dẫn", { description: "Arbiter sẽ triage phạm vi ảnh hưởng." });
      setPrompt("");
      await refresh();
    } catch (cause) {
      toast.error("Gửi chỉ dẫn thất bại", { description: cause instanceof Error ? cause.message : "API lỗi." });
    } finally {
      setBusy(false);
    }
  };

  if (loading && !engine) {
    return (
      <div className="content-wrap">
        <LoadingSkeleton />
      </div>
    );
  }

  return (
    <div className="content-wrap mc-home">
      <ConnectionBanner
        connected={connected}
        loading={loading}
        transport={transport}
        error={error}
        retryIn={retryIn}
        onRetry={() => void refresh()}
      />

      <NewBookPanel
        connected={connected}
        isRunning={!!snapshot?.is_running}
        prompt={prompt}
        onStarted={async () => {
          await refresh();
        }}
      />

      <EngineControlPanel
        engine={engine}
        connected={connected}
        prompt={prompt}
        onPromptClear={() => setPrompt("")}
        onDone={async () => {
          await refresh();
        }}
      />

      <section className={`mc-now mc-now-${playing.tone}`} aria-live="polite">
        <div className="mc-now-main">
          <p className="panel-kicker">ĐANG XẢY RA</p>
          <h1>{playing.title}</h1>
          <p>{playing.detail}</p>
          <div className="mc-now-meta">
            <span>{labelRuntime(snapshot?.runtime_state)}</span>
            <span>{labelPhase(snapshot?.phase)}</span>
            <span>{labelFlow(snapshot?.flow)}</span>
            {snapshot?.current_chapter ? <span>Ch. {snapshot.current_chapter}</span> : null}
            {snapshot?.current_volume_arc ? <span>{snapshot.current_volume_arc}</span> : null}
          </div>
        </div>
        <div className="mc-now-side">
          <div className="mc-now-title-block">
            <strong>{novelTitle.vietnamese || "Chưa có tên truyện"}</strong>
            {novelTitle.chineseSource && <small>{novelTitle.chineseSource}</small>}
          </div>
          <div className="mc-progress-block">
            <div className="progress-label">
              <span>Tiến độ ZH</span>
              <strong>{connected ? `${progressPercent}%` : "—"}</strong>
            </div>
            <div className="progress-track">
              <div className="progress-fill" style={{ width: `${connected ? progressPercent : 0}%` }} />
            </div>
            <div className="progress-meta">
              <span>{completedCount} chốt</span>
              <span>{totalChapters > 0 ? `mục tiêu ~${totalChapters}` : "dynamic"}</span>
            </div>
          </div>
          <button className="primary-button mc-primary" onClick={() => void handlePrimary()} disabled={!connected || busy || action.kind === "disabled"}>
            {action.kind === "abort" ? <Pause size={16} /> : <Play size={16} fill="currentColor" />}
            {busy ? "Đang gửi…" : action.label}
            <ArrowUpRight size={14} />
          </button>
          <small className="mc-action-hint">{action.hint}</small>
        </div>
      </section>

      <section className="mc-kpi" aria-label="Chỉ số nhanh">
        <div className="mc-kpi-item">
          <span>Chương ZH</span>
          <strong>{completedCount}<small>/{totalChapters || "—"}</small></strong>
        </div>
        <div className="mc-kpi-item">
          <span>Dịch VI</span>
          <strong>{translationCompleted}<small>/{totalChapters || completedCount || "—"}</small></strong>
          {(translationIssues > 0 || translationRunning > 0) && (
            <em>{translationRunning > 0 ? `${translationRunning} đang dịch` : ""}{translationIssues > 0 ? ` · ${translationIssues} cần xử lý` : ""}</em>
          )}
        </div>
        <div className="mc-kpi-item">
          <span>Agent live</span>
          <strong>{activeCount}<small>/{liveAgents.length || 0}</small></strong>
        </div>
        <div className="mc-kpi-item mc-kpi-warm">
          <span>Chi phí</span>
          <strong>${(snapshot?.total_cost_usd ?? 0).toFixed(2)}<small>{(snapshot?.budget_limit_usd ?? 0) > 0 ? ` / $${snapshot?.budget_limit_usd}` : ""}</small></strong>
        </div>
        <div className="mc-kpi-item">
          <span>Chữ</span>
          <strong>{(snapshot?.total_word_count ?? 0).toLocaleString("vi-VN")}</strong>
        </div>
      </section>

      <section className="mc-grid-main">
        <div className="panel mc-panel-agents">
          <div className="panel-header">
            <div>
              <p className="panel-kicker">BAN BIÊN TẬP</p>
              <h3>Ai đang làm gì</h3>
            </div>
            <span className="mc-soft-badge">{activeCount} active</span>
          </div>
          <div className="mc-agent-list">
            {displayedAgents.map((agent) => {
              const Icon = agent.icon;
              const tone = contextTone(agent.percent);
              return (
                <Link key={agent.name} href={agent.href} className={`mc-agent-row ${agent.working ? "is-working" : ""}`}>
                  <div className={`agent-icon agent-${agent.color}`}><Icon size={15} /></div>
                  <div className="mc-agent-body">
                    <div className="mc-agent-top">
                      <strong>{agent.name}</strong>
                      <span className={`mc-agent-state ${agent.working ? "on" : ""}`}>{agent.raw.state || "idle"}</span>
                    </div>
                    <span className="mc-agent-detail">{agent.detail}</span>
                    {agent.window > 0 && (
                      <div className="mc-ctx">
                        <div className={`mc-ctx-track mc-ctx-${tone}`}>
                          <div style={{ width: `${Math.min(100, agent.percent)}%` }} />
                        </div>
                        <small>{Math.round(agent.percent)}% · {(agent.tokens / 1000).toFixed(0)}k/{(agent.window / 1000).toFixed(0)}k{agent.strategy ? ` · ${agent.strategy}` : ""}</small>
                      </div>
                    )}
                  </div>
                  {agent.working && <span className={`agent-pulse pulse-${agent.color}`} />}
                </Link>
              );
            })}
            {displayedAgents.length === 0 && <div className="activity-empty">Chưa có agent trong snapshot.</div>}
          </div>
        </div>

        <div className="panel mc-panel-activity">
          <div className="panel-header">
            <div>
              <p className="panel-kicker">NHẬT KÝ LIVE</p>
              <h3>Sự kiện gần đây</h3>
            </div>
            <button type="button" className="text-button" onClick={() => setShowAllActivity((v) => !v)}>
              {showAllActivity ? "Thu gọn" : "Mở rộng"}
            </button>
          </div>
          <div className="filter-controls">
            <select className="log-select" value={filterAgent} onChange={(e) => setFilterAgent(e.target.value)} aria-label="Lọc agent">
              <option value="all">Tất cả agent</option>
              {["Architect", "Writer", "Editor", "Arbiter", "Translation Agent"].map((name) => (
                <option key={name} value={name}>{name}</option>
              ))}
            </select>
            <select className="log-select" value={filterCategory} onChange={(e) => setFilterCategory(e.target.value)} aria-label="Lọc sự kiện">
              {eventCategories.map((category) => (
                <option key={category} value={category}>{category === "all" ? "Mọi loại" : category}</option>
              ))}
            </select>
          </div>
          <div className="mc-activity-list">
            {activityRows.slice(0, showAllActivity ? 40 : 8).map((item) => (
              <div className="activity-row" key={item.key}>
                <span className={`activity-line line-${item.tone}`} />
                <time>{item.time}</time>
                <div>
                  <strong>{item.title}</strong>
                  <span>{item.detail}</span>
                </div>
              </div>
            ))}
            {activityRows.length === 0 && <div className="activity-empty">Chưa có event — engine im hoặc filter đang hẹp.</div>}
          </div>
        </div>
      </section>

      <section className="mc-grid-secondary">
        <div className="panel">
          <div className="panel-header">
            <div>
              <p className="panel-kicker">CHƯƠNG GẦN ĐÂY</p>
              <h3>ZH ↔ VI</h3>
            </div>
            <Link href="/manuscript" className="text-button">Đọc bản thảo <ArrowUpRight size={14} /></Link>
          </div>
          <div className="mc-chapter-rail">
            {recentChapters.map((ch) => (
              <Link key={ch.no} href={`/manuscript?ch=${ch.no}`} className={`mc-ch-chip vi-${ch.vi}`}>
                <strong>{String(ch.no).padStart(2, "0")}</strong>
                <span>{ch.vi === "none" ? "chưa VI" : ch.vi}</span>
              </Link>
            ))}
            {recentChapters.length === 0 && <div className="activity-empty">Chưa có chương chốt.</div>}
          </div>
          {(snapshot?.last_commit_summary || snapshot?.last_review_summary) && (
            <div className="mc-last-notes">
              {snapshot?.last_commit_summary && <p><em>Commit</em> {snapshot.last_commit_summary}</p>}
              {snapshot?.last_review_summary && <p><em>Review</em> {snapshot.last_review_summary}</p>}
            </div>
          )}
        </div>

        <div className="panel translation-chart-panel">
          <div className="panel-header">
            <div>
              <p className="panel-kicker">DỊCH VI · XU HƯỚNG</p>
              <h3>Hoàn tất / cần phục hồi</h3>
            </div>
            <Link href="/translation" className="text-button"><Split size={14} /> Workspace</Link>
          </div>
          {translationChart.length > 0 ? (
            <div className="translation-chart">
              <ResponsiveContainer width="100%" height={160}>
                <AreaChart data={translationChart} margin={{ top: 8, right: 8, left: -20, bottom: 0 }}>
                  <defs>
                    <linearGradient id="completedGradient" x1="0" x2="0" y1="0" y2="1">
                      <stop offset="0%" stopColor="#2f6b61" stopOpacity={0.32} />
                      <stop offset="100%" stopColor="#2f6b61" stopOpacity={0.02} />
                    </linearGradient>
                    <linearGradient id="recoveryGradient" x1="0" x2="0" y1="0" y2="1">
                      <stop offset="0%" stopColor="#b9563c" stopOpacity={0.2} />
                      <stop offset="100%" stopColor="#b9563c" stopOpacity={0} />
                    </linearGradient>
                  </defs>
                  <CartesianGrid vertical={false} stroke="#e9dfd1" strokeDasharray="2 4" />
                  <XAxis dataKey="time" tickLine={false} axisLine={false} tick={{ fill: "#8a7f73", fontSize: 10 }} />
                  <YAxis allowDecimals={false} tickLine={false} axisLine={false} tick={{ fill: "#8a7f73", fontSize: 10 }} />
                  <Tooltip contentStyle={{ borderRadius: 8, border: "1px solid #e4d9cb", background: "#fffdf9", fontSize: 11 }} />
                  <Area type="monotone" dataKey="completed" name="Hoàn tất" stroke="#2f6b61" strokeWidth={2} fill="url(#completedGradient)" />
                  <Area type="monotone" dataKey="recovery" name="Phục hồi" stroke="#b9563c" strokeWidth={1.5} fill="url(#recoveryGradient)" />
                </AreaChart>
              </ResponsiveContainer>
            </div>
          ) : (
            <div className="chart-empty">Bật translation và chạy batch để thấy nhịp độ tại đây.</div>
          )}
        </div>
      </section>

      <section className="mc-command" aria-label="Thanh chỉ dẫn">
        <div className="mc-command-head">
          <Sparkles size={14} />
          <strong>Chỉ dẫn nhanh</strong>
          <span>như ô nhập TUI · phím <kbd>/</kbd> · Ctrl/⌘+Enter gửi</span>
        </div>
        <div className="prompt-box mc-prompt-box">
          <textarea
            ref={promptRef}
            value={prompt}
            onChange={(e) => setPrompt(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) {
                e.preventDefault();
                void handlePrompt();
              }
            }}
            placeholder="Gõ chỉ dẫn cho Arbiter… (Ctrl/⌘+Enter để gửi)"
            rows={2}
          />
          <button className="send-button" onClick={() => void handlePrompt()} disabled={busy || !connected} aria-label="Gửi chỉ dẫn">
            <ArrowUpRight size={17} />
          </button>
        </div>
      </section>
    </div>
  );
}
