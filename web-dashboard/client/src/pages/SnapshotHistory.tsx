import { useEffect, useState } from "react";
import { Link } from "wouter";
import { ArrowLeft, ArchiveRestore, Clock3, HardDriveDownload, Loader2, Plus, RotateCcw, ShieldCheck } from "lucide-react";
import { toast } from "sonner";
import ConnectionBanner from "@/components/ConnectionBanner";
import { createSnapshot, fetchSnapshots, restoreSnapshot } from "@/lib/engineApi";

type Snapshot = { id: string; title: string; created_at: string; size_kb: number; files: number };

export default function SnapshotHistory() {
  const [items, setItems] = useState<Snapshot[]>([]);
  const [loading, setLoading] = useState(true);
  const [working, setWorking] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  const load = async () => {
    setLoading(true);
    try {
      const response = await fetchSnapshots();
      setItems(response.items);
      setError(null);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Không thể tải lịch sử snapshot.");
    } finally { setLoading(false); }
  };
  useEffect(() => { void load(); }, []);

  const backupNow = async () => {
    const title = window.prompt("Tên ghi chú cho bản sao lưu", "Sao lưu trước khi chỉnh sửa")?.trim();
    if (title === undefined) return;
    setWorking("new");
    try {
      const result = await createSnapshot(title || "Sao lưu thủ công");
      toast.success("Đã tạo snapshot", { description: `${result.files} tệp · ${result.size_kb} KB` });
      await load();
    } catch (cause) {
      toast.error("Không thể tạo snapshot", { description: cause instanceof Error ? cause.message : "Engine trả về lỗi." });
    } finally { setWorking(null); }
  };

  const restore = async (snapshot: Snapshot) => {
    if (!window.confirm(`Khôi phục “${snapshot.title}”?\n\nEngine sẽ tự tạo một bản sao an toàn trước khi ghi đè các tệp trong snapshot.`)) return;
    setWorking(snapshot.id);
    try {
      const result = await restoreSnapshot(snapshot.id);
      toast.success("Đã khôi phục bản thảo", { description: result.message });
      await load();
    } catch (cause) {
      toast.error("Không thể khôi phục snapshot", { description: cause instanceof Error ? cause.message : "Engine trả về lỗi." });
    } finally { setWorking(null); }
  };

  return <div className="content-wrap snapshots-nested">
    <div className="mc-page-nav">
      <Link href="/" className="back-link"><ArrowLeft size={16} /> Điều khiển</Link>
      <div className="translation-brand"><span className="translation-brand-mark"><ArchiveRestore size={16} /></span><span>Lịch sử bản thảo</span></div>
      <button className="primary-button" style={{ marginTop: 0 }} onClick={() => void backupNow()} disabled={working !== null}>
        <Plus size={15} /> {working === "new" ? "Đang sao lưu…" : "Sao lưu ngay"}
      </button>
    </div>
    <div className="translation-heading">
      <div>
        <p className="eyebrow">KHO BẢN THẢO</p>
        <h1>Snapshot & khôi phục</h1>
        <p className="lede">Archive thật của output. Khôi phục luôn tạo bản sao an toàn trước khi ghi đè.</p>
      </div>
      <div className="translation-book">
        <span>AN TOÀN</span>
        <strong>Confirm trước khi ghi</strong>
        <small>Luôn có đường quay lui.</small>
      </div>
    </div>
      <ConnectionBanner connected={!error} loading={loading} transport={error ? "offline" : "live"} error={error} retryIn={0} onRetry={() => void load()} />
      <section className="snapshot-list">{!loading && items.length === 0 ? <div className="translation-empty"><HardDriveDownload size={26} /><strong>Chưa có snapshot nào.</strong><p>Nhấn “Sao lưu ngay” trước khi thử một hướng viết mới, chỉnh sửa lớn hoặc khôi phục dữ liệu.</p></div> : items.map((item) => <article className="snapshot-card" key={item.id}><div className="snapshot-icon"><ShieldCheck size={19} /></div><div className="snapshot-copy"><span>{new Date(item.created_at).toLocaleString("vi-VN", { dateStyle: "medium", timeStyle: "short" })}</span><h2>{item.title}</h2><p><code>{item.id}</code><i>•</i>{item.files} tệp<i>•</i>{item.size_kb} KB</p></div><button className="subtle-button" onClick={() => void restore(item)} disabled={working !== null}>{working === item.id ? <Loader2 className="spin" size={15} /> : <RotateCcw size={15} />} Khôi phục</button></article>)}</section>
      <section className="translation-footnote"><Clock3 size={14} /><span>Khôi phục snapshot sẽ ghi lại các tệp đã được archive. Engine giữ một snapshot “trước khôi phục” để bạn có đường quay lui ngay lập tức.</span></section>
  </div>;
}
