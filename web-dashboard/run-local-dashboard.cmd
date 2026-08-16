@echo off
cd /d D:\MyProject\ainovel-cli\web-dashboard
corepack pnpm dev -- --host 127.0.0.1 --port 5173 > dashboard-local.log 2>&1
