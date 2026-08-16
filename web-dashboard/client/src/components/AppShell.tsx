/** Shared SPA shell: sidebar + top strip stay mounted across routes (no full reload). */
import { useMemo, useState, type ReactNode } from "react";
import { Link, useLocation } from "wouter";
import {
  BookOpen,
  BookOpenText,
  FileArchive,
  Gauge,
  Languages,
  Library,
  Menu,
  PanelLeftClose,
  Settings2,
  Split,
  Users,
} from "lucide-react";
import { formatNovelTitle } from "@/lib/novelTitle";
import type { EngineState } from "@/lib/engineApi";
import BrandMark from "@/components/BrandMark";

type NavKey = "home" | "manuscript" | "translation" | "library" | "cocreate" | "snapshots" | "settings";

const navItems: { key: NavKey; href: string; label: string; icon: typeof Gauge; match: (path: string) => boolean }[] = [
  { key: "home", href: "/", label: "Điều khiển", icon: Gauge, match: (p) => p === "/" || p.startsWith("/agents/") },
  { key: "library", href: "/library", label: "Thư viện", icon: Library, match: (p) => p.startsWith("/library") },
  { key: "cocreate", href: "/cocreate", label: "Đồng sáng tác", icon: Users, match: (p) => p.startsWith("/cocreate") },
  { key: "manuscript", href: "/manuscript", label: "Đọc bản thảo", icon: BookOpenText, match: (p) => p.startsWith("/manuscript") },
  { key: "translation", href: "/translation", label: "Bản dịch Việt", icon: Split, match: (p) => p.startsWith("/translation") },
  { key: "snapshots", href: "/snapshots", label: "Snapshot", icon: FileArchive, match: (p) => p.startsWith("/snapshots") },
  { key: "settings", href: "/settings", label: "Thiết lập", icon: Settings2, match: (p) => p.startsWith("/settings") },
];

type AppShellProps = {
  children: ReactNode;
  engine?: EngineState | null;
  connected?: boolean;
  transport?: string;
  /** Optional footer strip (TUI-like status bar). */
  footer?: ReactNode;
  /** Hide dense chrome on simple pages if needed. */
  dense?: boolean;
};

export default function AppShell({ children, engine, connected = false, transport = "offline", footer, dense = true }: AppShellProps) {
  const [location] = useLocation();
  const [sidebarOpen, setSidebarOpen] = useState(false);
  const snapshot = engine?.snapshot;
  const novelTitle = formatNovelTitle(snapshot?.novel_name);
  const novelName = novelTitle.vietnamese || "Chưa mở truyện";
  const completed = snapshot?.completed_count ?? 0;
  const total = snapshot?.total_chapters ?? 0;
  const translationDone = useMemo(
    () => Object.values(engine?.translation?.chapters || {}).filter((c) => c.state === "completed").length,
    [engine?.translation?.chapters],
  );
  const liveLabel = !connected ? "Ngoại tuyến" : transport === "live" ? "Live" : transport === "polling" ? "Polling" : "Kết nối…";
  const liveTone = !connected ? "off" : transport === "live" ? "live" : "warn";

  return (
    <div className={`app-shell mc-shell ${dense ? "mc-dense" : ""}`}>
      <aside className={`sidebar mc-sidebar ${sidebarOpen ? "sidebar-open" : ""}`}>
        <div className="sidebar-top">
          <Link href="/" className="brand-link" onClick={() => setSidebarOpen(false)} aria-label="ainovel home">
            <BrandMark />
          </Link>
          <button className="icon-button mobile-close" onClick={() => setSidebarOpen(false)} aria-label="Đóng menu">
            <PanelLeftClose size={18} />
          </button>
        </div>

        <Link href="/library" className="workspace-switcher mc-workspace" onClick={() => setSidebarOpen(false)} title="Đổi / tạo truyện">
          <div className="workspace-cover"><BookOpen size={16} /></div>
          <div className="workspace-copy">
            <span>Đang mở · bấm để đổi</span>
            <strong title={novelName}>{novelName}</strong>
            {novelTitle.chineseSource && <small title={novelTitle.chineseSource}>ZH: {novelTitle.chineseSource}</small>}
          </div>
        </Link>

        <div className="mc-mini-stats" aria-label="Tiến độ nhanh">
          <div><span>ZH</span><strong>{completed}{total > 0 ? `/${total}` : ""}</strong></div>
          <div><span>VI</span><strong>{translationDone}{total > 0 ? `/${total}` : ""}</strong></div>
          <div className={`mc-live-chip mc-live-${liveTone}`}><Languages size={11} /> {liveLabel}</div>
        </div>

        <div className="nav-section-label">BÀN ĐIỀU KHIỂN</div>
        <nav className="side-nav" aria-label="Điều hướng chính">
          {navItems.map((item) => {
            const Icon = item.icon;
            const active = item.match(location);
            return (
              <Link
                key={item.key}
                href={item.href}
                className={`nav-item ${active ? "nav-item-active" : ""}`}
                onClick={() => setSidebarOpen(false)}
              >
                <Icon size={17} strokeWidth={active ? 2.3 : 1.8} />
                <span>{item.label}</span>
              </Link>
            );
          })}
        </nav>

        <div className="mc-sidebar-hint">
          <strong>Phím tắt</strong>
          <p><kbd>Ctrl/⌘ K</kbd> palette · <kbd>/</kbd> chỉ dẫn · sidebar giữ ngữ cảnh truyện</p>
        </div>
      </aside>

      <div className="mc-main-column">
        <header className="topbar mc-topbar">
          <button className="icon-button mobile-menu" onClick={() => setSidebarOpen(true)} aria-label="Mở menu">
            <Menu size={20} />
          </button>
          <div className="breadcrumb mc-breadcrumb">
            <span>ainovel</span>
            <span className="slash">/</span>
            <strong>{novelName}</strong>
            {snapshot?.phase && <em className="mc-phase-chip">{snapshot.phase}</em>}
            {snapshot?.flow && <em className="mc-flow-chip">{snapshot.flow}</em>}
          </div>
          <div className={`mc-conn-pill mc-live-${liveTone}`} title={connected ? "Engine local" : "Chưa kết nối"}>
            <span className="status-dot" />
            {liveLabel}
          </div>
        </header>

        <main className="main-area mc-main-area">{children}</main>
        {footer}
      </div>

      {sidebarOpen && <button className="mc-backdrop" aria-label="Đóng menu" onClick={() => setSidebarOpen(false)} />}
    </div>
  );
}
