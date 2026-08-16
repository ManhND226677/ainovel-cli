#!/bin/bash
set -euo pipefail
APP_DIR=/opt/ainovel
BIN=$APP_DIR/ainovel-cli
TOKEN_FILE=$APP_DIR/web.token
SERVICE=ainovel-web.service

mkdir -p "$APP_DIR" /var/lib/ainovel/output /var/lib/ainovel/library
if [ ! -f "$TOKEN_FILE" ]; then
  TOKEN=$(openssl rand -hex 24 2>/dev/null || head -c 48 /dev/urandom | od -An -tx1 | tr -d ' \n' | head -c 48)
  echo -n "$TOKEN" > "$TOKEN_FILE"
  chmod 600 "$TOKEN_FILE"
fi
TOKEN=$(cat "$TOKEN_FILE")

# Giao diện web đã được nhúng sẵn trong binary, không cần upload thư mục UI riêng.
install -m 755 /tmp/ainovel-cli-linux-amd64 "$BIN"

# Keep existing user config if present
if [ -f /root/.ainovel/config.json ] && [ ! -f $APP_DIR/config.json ]; then
  cp /root/.ainovel/config.json $APP_DIR/config.json || true
fi

cat >/etc/systemd/system/$SERVICE <<EOF
[Unit]
Description=ainovel-cli web dashboard + API
After=network.target

[Service]
Type=simple
User=root
WorkingDirectory=$APP_DIR
Environment=HOME=/root
ExecStart=$BIN web --addr 0.0.0.0:10001 --token $TOKEN --output-dir /var/lib/ainovel/output/novel
Restart=on-failure
RestartSec=3
LimitNOFILE=65535

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable $SERVICE
systemctl restart $SERVICE
sleep 1
systemctl --no-pager --full status $SERVICE | sed -n '1,25p'
echo TOKEN=$TOKEN
curl -fsS http://127.0.0.1:10001/api/health || true
echo
curl -fsSI http://127.0.0.1:10001/ | head -n 15 || true
ss -lntup | grep 10001 || true
