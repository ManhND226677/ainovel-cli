/** Sổ Tay Gốm Men: loading giữ nhịp bố cục giấy/ngà, chuyển động nhẹ và không gây giật layout. */
type LoadingSkeletonProps = {
  variant?: "dashboard" | "batch";
};

export default function LoadingSkeleton({ variant = "dashboard" }: LoadingSkeletonProps) {
  if (variant === "batch") {
    return <div className="loading-skeleton-batch" aria-label="Đang tải trạng thái dịch" role="status">
      <div className="skeleton-line skeleton-line-wide" />
      <div className="skeleton-line skeleton-line-medium" />
      <div className="skeleton-table"><span /><span /><span /><span /><span /><span /><span /><span /></div>
    </div>;
  }

  return <div className="loading-skeleton-dashboard" aria-label="Đang tải dashboard" role="status">
    <div className="skeleton-line skeleton-line-short" />
    <div className="skeleton-title" />
    <div className="skeleton-line skeleton-line-medium" />
    <div className="skeleton-hero" />
    <div className="skeleton-metrics"><span /><span /><span /><span /></div>
    <div className="skeleton-columns"><span /><span /></div>
  </div>;
}
