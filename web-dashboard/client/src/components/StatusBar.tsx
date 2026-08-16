/** TUI-parity bottom strip: one glance cost / phase / progress. */
import type { EngineState } from "@/lib/engineApi";

function money(n: number) {
  if (!n || n <= 0) return "$0";
  if (n < 0.01) return `$${n.toFixed(4)}`;
  return `$${n.toFixed(2)}`;
}

type Props = {
  engine?: EngineState | null;
  connected: boolean;
  transport: string;
  workingLabel?: string;
};

export default function StatusBar({ engine, connected, transport, workingLabel }: Props) {
  const snap = engine?.snapshot;
  const cost = snap?.total_cost_usd ?? 0;
  const budget = snap?.budget_limit_usd ?? 0;
  const ratio = budget > 0 ? cost / budget : 0;
  const budgetTone = ratio >= 1 ? "danger" : ratio >= 0.8 ? "warn" : "ok";
  const completed = snap?.completed_count ?? 0;
  const total = snap?.total_chapters ?? 0;
  const words = snap?.total_word_count ?? 0;
  const viDone = Object.values(engine?.translation?.chapters || {}).filter((c) => c.state === "completed").length;
  const link = !connected ? "OFF" : transport === "live" ? "WS" : transport === "polling" ? "POLL" : "…";

  return (
    <footer className="mc-statusbar" role="contentinfo">
      <div className="mc-statusbar-left">
        <span className={`mc-sb-link mc-sb-link-${connected ? (transport === "live" ? "live" : "poll") : "off"}`}>{link}</span>
        {snap?.runtime_state && <span className="mc-sb-seg"><em>run</em> {snap.runtime_state}</span>}
        {snap?.phase && <span className="mc-sb-seg"><em>phase</em> {snap.phase}</span>}
        {snap?.flow && <span className="mc-sb-seg"><em>flow</em> {snap.flow}</span>}
        {workingLabel && <span className="mc-sb-seg mc-sb-work"><em>now</em> {workingLabel}</span>}
      </div>
      <div className="mc-statusbar-right">
        <span className="mc-sb-seg"><em>ZH</em> {completed}{total > 0 ? `/${total}` : ""}</span>
        <span className="mc-sb-seg"><em>VI</em> {viDone}</span>
        {words > 0 && <span className="mc-sb-seg"><em>字</em> {words.toLocaleString("vi-VN")}</span>}
        <span className={`mc-sb-seg mc-sb-cost mc-sb-cost-${budgetTone}`}>
          <em>$</em> {money(cost)}{budget > 0 ? `/${money(budget)}` : ""}
        </span>
      </div>
    </footer>
  );
}
