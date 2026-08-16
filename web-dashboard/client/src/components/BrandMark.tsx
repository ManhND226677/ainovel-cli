/** Local brand mark — no external /manus-storage asset dependency. */
type Props = {
  compact?: boolean;
  className?: string;
};

export default function BrandMark({ compact = false, className = "" }: Props) {
  return (
    <div className={`brand-mark ${compact ? "brand-mark-compact" : ""} ${className}`.trim()}>
      <span className="brand-mark-badge" aria-hidden>
        <svg viewBox="0 0 32 32" width="30" height="30" fill="none">
          <rect x="2" y="2" width="28" height="28" rx="8" fill="#f0d8c7" />
          <path
            d="M9 8.5h9.5c2.4 0 4 1.4 4 3.6 0 1.7-1 2.9-2.6 3.4L23 23h-3.1l-2.7-6.7H12.2V23H9V8.5Zm3.2 2.5v4.3h5.1c1.2 0 1.9-.6 1.9-1.6s-.7-1.6-1.9-1.6h-5.1Z"
            fill="#b9563c"
          />
          <circle cx="24.2" cy="9.2" r="1.6" fill="#2f6b61" />
        </svg>
      </span>
      {!compact && (
        <span>
          ainovel<span className="brand-dot">.</span>
        </span>
      )}
    </div>
  );
}
