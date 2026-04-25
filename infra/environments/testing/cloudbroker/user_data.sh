#!/bin/bash
set -euo pipefail
exec > /var/log/user-data.log 2>&1

echo "=== Installing Caddy ==="
dnf install -y yum-utils
cat > /etc/yum.repos.d/caddy.repo << 'REPO'
[caddy-stable]
name=Caddy Stable
baseurl=https://dl.cloudsmith.io/public/caddy/stable/rpm/el/9/$basearch
enabled=1
gpgcheck=0
REPO
dnf install -y caddy || {
  curl -fsSL "https://github.com/caddyserver/caddy/releases/download/v2.9.1/caddy_2.9.1_linux_arm64.tar.gz" \
    | tar -xz -C /usr/local/bin caddy
}

echo "=== Setting up broker binary placeholder ==="
# The broker binary will be uploaded via SCP after terraform apply.
# Create the systemd service so it's ready to start.
mkdir -p /opt/raikada

cat > /etc/systemd/system/cloudbroker.service << 'SERVICE'
[Unit]
Description=Raikada Cloud Broker
After=network.target

[Service]
ExecStart=/opt/raikada/cloudbroker -addr :8080 -token ${broker_token} -tenant raikada
Restart=always
RestartSec=5
WorkingDirectory=/opt/raikada

[Install]
WantedBy=multi-user.target
SERVICE

echo "=== Configuring Caddy ==="
cat > /etc/caddy/Caddyfile << 'CADDY'
${domain} {
    reverse_proxy localhost:8080
}
CADDY

systemctl enable caddy
systemctl daemon-reload

echo "=== Setup complete ==="
echo "Upload the broker binary to /opt/raikada/cloudbroker then run:"
echo "  systemctl start cloudbroker && systemctl start caddy"
