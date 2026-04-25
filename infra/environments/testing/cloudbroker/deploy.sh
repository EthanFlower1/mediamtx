#!/bin/bash
# deploy.sh — Build, upload, and start the cloud broker + frp server on EC2.
#
# Prerequisites:
#   1. terraform apply (creates the EC2 instance)
#   2. SSH key pair configured (ssh_key_name in terraform.tfvars)
#   3. DNS: connect.raikada.com → EC2 elastic IP
#   4. DNS: *.raikada.com → CNAME to connect.raikada.com
#
# Usage:
#   cd infra/environments/testing/cloudbroker
#   ./deploy.sh
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../../../.." && pwd)"

# Read broker token from tfvars.
BROKER_TOKEN=$(grep broker_token "$SCRIPT_DIR/terraform.tfvars" | sed 's/.*= *"\(.*\)"/\1/')

# Get the EC2 IP from Terraform output.
PUBLIC_IP=$(terraform -chdir="$SCRIPT_DIR" output -raw public_ip 2>/dev/null)
if [ -z "$PUBLIC_IP" ]; then
    echo "ERROR: No public_ip output. Run 'terraform apply' first."
    exit 1
fi

SSH_KEY="${SSH_KEY:-$HOME/.ssh/raikada-broker.pem}"
SSH="ssh -i $SSH_KEY -o StrictHostKeyChecking=no ec2-user@$PUBLIC_IP"
SCP="scp -i $SSH_KEY -o StrictHostKeyChecking=no"

echo "=== Cross-compiling broker for Linux ARM64 ==="
cd "$PROJECT_ROOT"
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o /tmp/cloudbroker-linux-arm64 ./cmd/cloudbroker
echo "Built: $(ls -lh /tmp/cloudbroker-linux-arm64 | awk '{print $5}')"

echo ""
echo "=== Uploading to $PUBLIC_IP ==="
$SCP /tmp/cloudbroker-linux-arm64 ec2-user@"$PUBLIC_IP":/tmp/cloudbroker

echo ""
echo "=== Deploying ==="
$SSH << REMOTE
sudo mv /tmp/cloudbroker /opt/raikada/cloudbroker
sudo chmod +x /opt/raikada/cloudbroker

# Update systemd service with frp flags.
sudo tee /etc/systemd/system/cloudbroker.service > /dev/null << 'SERVICE'
[Unit]
Description=Raikada Cloud Broker + FRP Server
After=network.target

[Service]
ExecStart=/opt/raikada/cloudbroker -addr :8080 -token ${BROKER_TOKEN} -tenant raikada -frp-port 7000 -frp-http-port 7080 -subdomain-host raikada.com
Restart=always
RestartSec=5
WorkingDirectory=/opt/raikada

[Install]
WantedBy=multi-user.target
SERVICE

# Update Caddy for wildcard subdomain routing.
sudo tee /etc/caddy/Caddyfile > /dev/null << 'CADDY'
connect.raikada.com {
    reverse_proxy localhost:8080
}

*.raikada.com {
    tls {
        dns cloudflare {env.CF_API_TOKEN}
    }
    reverse_proxy localhost:7080
}
CADDY

# Reload and restart.
sudo systemctl daemon-reload
sudo systemctl restart cloudbroker
sudo systemctl restart caddy
sleep 3

echo "=== Broker status ==="
sudo systemctl is-active cloudbroker
echo "=== Caddy status ==="
sudo systemctl is-active caddy
echo "=== Health check ==="
curl -s http://localhost:8080/healthz && echo ""
echo "=== FRP check ==="
netstat -tlnp 2>/dev/null | grep 7000 || ss -tlnp | grep 7000 || echo "port 7000 check skipped"
REMOTE

echo ""
echo "=== Done ==="
echo ""
echo "Cloud broker:  https://connect.raikada.com"
echo "FRP server:    connect.raikada.com:7000"
echo "Subdomain:     https://{alias}.raikada.com"
echo ""
echo "NOTE: Wildcard subdomains (*.raikada.com) require either:"
echo "  1. Cloudflare proxy enabled (orange cloud) on the * CNAME record, OR"
echo "  2. Caddy with cloudflare DNS plugin + CF_API_TOKEN env var for DNS-01 ACME"
echo ""
echo "For now, add individual A records per site alias in Cloudflare:"
echo "  ethans-home → $PUBLIC_IP (DNS only)"
