/** Sổ Tay Gốm Men: cảnh báo kết nối dùng màu trà/đất nung, ưu tiên giải thích và hành động hồi phục. */
import { AlertTriangle, LoaderCircle, RefreshCw, Wifi, WifiOff } from "lucide-react";

type ConnectionBannerProps = {
  connected: boolean;
  loading: boolean;
  transport: "connecting" | "live" | "polling" | "offline";
  error?: string | null;
  retryIn?: number;
  onRetry?: () => void;
};

export default function ConnectionBanner({ connected, loading, transport, error, retryIn = 0, onRetry }: ConnectionBannerProps) {
  if (connected && transport === "live") {
    return <div className="connection-banner connection-banner-live" role="status">
      <Wifi size={14} /><strong>WebSocket đang cập nhật trực tiếp</strong><span>Snapshot và event mới được nhận từ engine Go.</span>
    </div>;
  }

  if (loading || transport === "connecting") {
    return <div className="connection-banner connection-banner-loading" role="status">
      <LoaderCircle size={14} className="spin" /><strong>Đang kết nối engine Go local…</strong><span>Dashboard đang lấy snapshot đầu tiên.</span>
    </div>;
  }

  const fallback = transport === "polling" && connected;
  return <div className={`connection-banner ${fallback ? "connection-banner-polling" : "connection-banner-error"}`} role="alert">
    {fallback ? <RefreshCw size={14} /> : <WifiOff size={14} />}
    <div className="connection-copy"><strong>{fallback ? "WebSocket tạm gián đoạn" : "Mất kết nối engine Go local"}</strong><span>{error || "Không nhận được phản hồi từ API local."}</span></div>
    <div className="connection-actions">{retryIn > 0 && <small>Tự nối lại sau {retryIn}s</small>}{onRetry && <button type="button" onClick={onRetry}><AlertTriangle size={12} /> Thử ngay</button>}</div>
  </div>;
}
