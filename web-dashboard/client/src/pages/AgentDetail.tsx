/** Agent detail nested in AppShell — live data only, no fake sample state. */
import { useEffect, useMemo, useState } from "react";
import { Link, useRoute } from "wouter";
import { ArrowLeft, Bot, CheckCircle2, CircleAlert, Clock3, Gauge, Layers3, Languages, PenLine, Sparkles } from "lucide-react";
import ConnectionBanner from "@/components/ConnectionBanner";
import { useEngine } from "@/contexts/EngineContext";
import { fetchAgentDetail, type AgentDetail as AgentDetailData, type AgentRole, type LiveAgent } from "@/lib/engineApi";
import { canonicalAgentName } from "@/lib/statusCopy";

const roles: AgentRole[] = ["Architect", "Writer", "Editor", "Arbiter", "Translation Agent"];
const icons = { Architect: Layers3, Writer: PenLine, Editor: CheckCircle2, Arbiter: Sparkles, "Translation Agent": Languages };
const colors = { Architect: "green", Writer: "terracotta", Editor: "blue", Arbiter: "gold", "Translation Agent": "translation" };

function emptyAgent(role: AgentRole): LiveAgent {
  return {
    name: role,
    role,
    state: "idle",
    turn: 0,
    updated_at: new Date(0).toISOString(),
    context: {
      tokens: 0,
      window: 0,
      percent: 0,
      scope: "",
      strategy: "",
      active_messages: 0,
      summary_messages: 0,
      compacted_count: 0,
      kept_count: 0,
    },
  };
}

export default function AgentDetail() {
  const [, params] = useRoute("/agents/:role");
  const role = useMemo(() => {
    const slug = decodeURIComponent(params?.role || "");
    return roles.find((item) => item.toLowerCase().replaceAll(" ", "-") === slug) || "Arbiter";
  }, [params?.role]);

  const [data, setData] = useState<AgentDetailData | null>(null);
  const { engine, events, connected, loading, error, transport, retryIn, refresh } = useEngine();

  useEffect(() => {
    let disposed = false;
    void fetchAgentDetail(role).then((result) => {
      if (!disposed) setData(result);
    }).catch(() => undefined);
    return () => { disposed = true; };
  }, [role]);

  const liveAgent = engine?.agents.find((candidate) => {
    const name = canonicalAgentName(candidate.name || candidate.role);
    if (role === "Translation Agent") return name === "Translation";
    return name === role || candidate.role === role || candidate.name === role;
  });
  const Icon = icons[role];
  const tone = colors[role];
  const agent = liveAgent || data?.agent || emptyAgent(role);
  const history = (events.length > 0
    ? events.filter((item) => {
      const name = canonicalAgentName(item.agent || "");
      if (role === "Translation Agent") return name === "Translation";
      return !item.agent || name === role || (item.agent || "").toLowerCase().includes(role.toLowerCase());
    })
    : (data?.history || []));
  const isActive = agent.state === "working" || agent.state === "running";

  return (
    <div className="content-wrap agent-detail-nested">
      <div className="mc-page-nav">
        <Link href="/" className="back-link"><ArrowLeft size={16} /> Điều khiển</Link>
        <div className="mc-role-tabs">
          {roles.map((item) => (
            <Link
              key={item}
              href={`/agents/${item.toLowerCase().replaceAll(" ", "-")}`}
              className={`mc-role-tab ${item === role ? "is-active" : ""}`}
            >
              {item}
            </Link>
          ))}
        </div>
      </div>

      <div className="detail-kicker">
        <span className={`agent-icon agent-${tone}`}><Icon size={18} /></span>
        <span>HỒ SƠ AGENT</span>
      </div>
      <div className="detail-heading">
        <div>
          <span className="detail-editorial-tag">
            {role === "Translation Agent" ? "BẢN DỊCH VIỆT" : "BAN BIÊN TẬP"}
          </span>
          <h1>{role}</h1>
          <p>{connected ? "Snapshot + event live từ engine Go." : "Chưa kết nối — không hiển thị dữ liệu mẫu."}</p>
        </div>
        <span className={`detail-status ${isActive ? "detail-status-active" : ""}`}>
          <span />{isActive ? "Đang thực thi" : agent.state || "idle"}
        </span>
      </div>

      <ConnectionBanner connected={connected} loading={loading} transport={transport} error={error} retryIn={retryIn} onRetry={() => void refresh()} />

      <section className="detail-grid">
        <div className="detail-card detail-overview">
          <div className="detail-card-label">TÁC VỤ HIỆN TẠI</div>
          <h2>{agent.summary || "Chưa có tác vụ mới"}</h2>
          <p>{agent.tool ? `Tool: ${agent.tool}` : "Engine chưa ghi nhận tool đang chạy."}</p>
          <div className="detail-meta">
            <span><Clock3 size={14} /> Lượt {agent.turn}</span>
            <span><Bot size={14} /> {agent.role}</span>
          </div>
        </div>
        <div className="detail-card">
          <div className="detail-card-label">NGỮ CẢNH</div>
          <div className="context-number">{Math.round(agent.context?.percent || 0)}<small>%</small></div>
          <div className="context-track"><div style={{ width: `${Math.min(100, agent.context?.percent || 0)}%` }} /></div>
          <div className="context-meta">
            <span>{(agent.context?.tokens || 0).toLocaleString("vi-VN")} token</span>
            <span>{(agent.context?.window || 0).toLocaleString("vi-VN")} cửa sổ</span>
          </div>
          <p className="context-note"><Gauge size={14} /> {agent.context?.strategy || "—"}</p>
        </div>
      </section>

      <section className="detail-card history-card">
        <div className="detail-card-head">
          <div>
            <div className="detail-card-label">DÒNG THỜI GIAN</div>
            <h2>Lịch sử hoạt động</h2>
          </div>
          <span className="history-count">{history.length} sự kiện</span>
        </div>
        <div className="detail-history">
          {history.length > 0 ? history.slice().reverse().map((item) => (
            <div className="detail-event" key={`${item.seq}-${item.time}-${item.summary}`}>
              <span className="detail-event-dot" />
              <time>{new Date(item.time).toLocaleString("vi-VN", { hour: "2-digit", minute: "2-digit", day: "2-digit", month: "2-digit" })}</time>
              <div>
                <strong>{item.category || "SYSTEM"}</strong>
                <p>{item.summary || "Không có mô tả"}</p>
                <small>{item.kind || item.priority || "info"}{item.detail ? ` · ${item.detail}` : ""}</small>
              </div>
            </div>
          )) : (
            <div className="detail-empty">
              <CircleAlert size={18} />
              <span>Chưa có lịch sử live. Chạy engine web để xem event thật.</span>
            </div>
          )}
        </div>
      </section>
    </div>
  );
}
