#!/usr/bin/env bash
# Build frontend web-dashboard, sync the build into internal/webapi/static (go:embed)
# then compile the Go binary that already contains the UI.
# Usage: scripts/build-web.sh [--skip-go]
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DASH="$ROOT/web-dashboard"
STATIC="$ROOT/internal/webapi/static"
SKIP_GO=0
[ "${1:-}" = "--skip-go" ] && SKIP_GO=1

echo "== 1/3 Build web-dashboard (pnpm build)"
(
  cd "$DASH"
  pnpm install --frozen-lockfile
  pnpm run build
)

echo "== 2/3 Sync dist/public -> internal/webapi/static"
DIST="$DASH/dist/public"
[ -f "$DIST/index.html" ] || { echo "Missing $DIST/index.html" >&2; exit 1; }
rm -rf "$STATIC"
mkdir -p "$STATIC"
cp -a "$DIST/." "$STATIC/"
rm -rf "$STATIC/__manus__"

if [ "$SKIP_GO" = "1" ]; then
  echo "== 3/3 Skipped Go build (--skip-go)"
  exit 0
fi

echo "== 3/3 Build Go binary (UI embedded)"
cd "$ROOT"
go build -o ainovel-cli ./cmd/ainovel-cli
echo "Done: ainovel-cli (web UI embedded)"
