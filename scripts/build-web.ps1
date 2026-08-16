# Build frontend web-dashboard, sync the build into internal/webapi/static (go:embed)
# then compile the Go binary that already contains the UI.
# Usage: powershell -ExecutionPolicy Bypass -File scripts/build-web.ps1 [-SkipGo]
param(
    [switch]$SkipGo
)

$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot
$dash = Join-Path $root "web-dashboard"
$static = Join-Path $root "internal\webapi\static"

Write-Host "== 1/3 Build web-dashboard (pnpm build)" -ForegroundColor Cyan
Push-Location $dash
try {
    pnpm install --frozen-lockfile
    if ($LASTEXITCODE -ne 0) { throw "pnpm install failed" }
    pnpm run build
    if ($LASTEXITCODE -ne 0) { throw "pnpm build failed" }
} finally {
    Pop-Location
}

Write-Host "== 2/3 Sync dist/public -> internal/webapi/static" -ForegroundColor Cyan
$dist = Join-Path $dash "dist\public"
if (-not (Test-Path (Join-Path $dist "index.html"))) { throw "Missing $dist\index.html" }
if (Test-Path $static) { Remove-Item -Recurse -Force $static }
New-Item -ItemType Directory -Force -Path $static | Out-Null
Copy-Item -Recurse -Force (Join-Path $dist "*") $static
# Loại thư mục do plugin sinh ra nếu còn
$manus = Join-Path $static "__manus__"
if (Test-Path $manus) { Remove-Item -Recurse -Force $manus }

if ($SkipGo) {
    Write-Host "== 3/3 Skipped Go build (-SkipGo)" -ForegroundColor Yellow
    exit 0
}

Write-Host "== 3/3 Build Go binary (UI embedded)" -ForegroundColor Cyan
Push-Location $root
try {
    go build -o ainovel-cli.exe ./cmd/ainovel-cli
    if ($LASTEXITCODE -ne 0) { throw "go build failed" }
} finally {
    Pop-Location
}
Write-Host "Done: ainovel-cli.exe (web UI embedded)" -ForegroundColor Green
