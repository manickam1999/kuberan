#!/usr/bin/env bash
#
# One-time VPS setup for Kuberan production deployment.
# Run this on a fresh Ubuntu 22.04+ / Debian 12+ machine.
#
# Usage: bash deploy/setup-vps.sh
#
set -euo pipefail

INSTALL_DIR="/opt/kuberan"

echo "========================================="
echo "  Kuberan Setup"
echo "========================================="

# --- Docker ---
if ! command -v docker &> /dev/null; then
    echo "==> Installing Docker..."
    curl -fsSL https://get.docker.com | sh
    systemctl enable docker
    systemctl start docker
    echo "==> Docker installed."
else
    echo "==> Docker already installed: $(docker --version)"
fi

# Ensure current user can run Docker (if not root)
if [ "$(id -u)" -ne 0 ]; then
    if ! groups | grep -q docker; then
        echo "==> Adding $(whoami) to docker group..."
        sudo usermod -aG docker "$(whoami)"
        echo "    NOTE: Log out and back in for group changes to take effect."
    fi
fi

# --- Clone or update repo ---
if [ -d "$INSTALL_DIR" ]; then
    echo "==> $INSTALL_DIR already exists. Pulling latest..."
    cd "$INSTALL_DIR"
    git pull origin main
else
    echo "==> Cloning repository to $INSTALL_DIR..."
    echo "    Enter your repository URL (e.g., https://github.com/user/kuberan.git):"
    read -r REPO_URL
    git clone "$REPO_URL" "$INSTALL_DIR"
    cd "$INSTALL_DIR"
fi

# --- Environment file ---
if [ ! -f "$INSTALL_DIR/.env.prod" ]; then
    echo "==> Creating .env.prod from template..."
    cp "$INSTALL_DIR/.env.prod.example" "$INSTALL_DIR/.env.prod"
    chmod 600 "$INSTALL_DIR/.env.prod"
    echo ""
    echo "    IMPORTANT: Edit $INSTALL_DIR/.env.prod with your production values:"
    echo "    - JWT_SECRET, PIPELINE_API_KEY, BOT_INTERNAL_SECRET (openssl rand -hex 32)"
    echo "    - TELEGRAM_BOT_TOKEN (from @BotFather), if you run the bot"
    echo "    - DATABASE: Choose OPTION A (self-hosted Postgres) or OPTION B (Supabase)"
    echo "      See docs/database-setup.md for full instructions for each option."
    echo "    - CORS_ORIGIN (exact web origin, e.g. https://app.kuberan.example.com)"
    echo "      and NEXT_PUBLIC_API_URL (your domain)"
    echo ""
    echo "    Run: nano $INSTALL_DIR/.env.prod"
    echo ""
else
    echo "==> .env.prod already exists."
fi

# --- Oracle cron job ---
echo "==> Setting up oracle cron job (every 30 minutes)..."
CRON_CMD="*/30 * * * * cd $INSTALL_DIR && docker compose -f docker-compose.prod.yml run --rm oracle >> /var/log/kuberan-oracle.log 2>&1"

# Add cron job to root's crontab if not already present
(sudo crontab -l 2>/dev/null || true) | grep -v "kuberan.*oracle" | { cat; echo "$CRON_CMD"; } | sudo crontab -
echo "    Oracle cron job installed (root crontab)."

# --- Log rotation ---
echo "==> Setting up log rotation..."
sudo tee /etc/logrotate.d/kuberan-oracle > /dev/null << 'EOF'
/var/log/kuberan-oracle.log {
    weekly
    rotate 4
    compress
    missingok
    notifempty
}
EOF

echo ""
echo "========================================="
echo "  Setup complete!"
echo "========================================="
echo ""
echo "Next steps:"
echo "  1. Create a Cloudflare Tunnel and run cloudflared ON THE HOST:"
echo "     - Go to Cloudflare Dashboard > Zero Trust > Networks > Tunnels"
echo "     - Create a tunnel and install its connector on this VPS"
echo "       (e.g. 'cloudflared service install <token>')"
echo "     - Route public hostnames to http://127.0.0.1:<port> (ports bind to"
echo "       loopback only; see the mapping in docker-compose.prod.yml)"
echo "  2. Edit .env.prod:  nano $INSTALL_DIR/.env.prod"
echo "  3. Deploy:          cd $INSTALL_DIR && ./deploy/deploy.sh"
echo ""
