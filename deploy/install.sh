#!/usr/bin/env bash
# =============================================================================
# NexusCard one-click installer (Linux)
#
# Usage:
#   sudo bash deploy/install.sh pay.example.com
#   sudo bash deploy/install.sh --uninstall
#
# Installs binary, config, systemd, Nginx reverse proxy, optional Let's Encrypt.
# =============================================================================
set -euo pipefail

if [[ -t 1 ]]; then
  RED=$'\033[0;31m'; GREEN=$'\033[0;32m'; YELLOW=$'\033[1;33m'
  CYAN=$'\033[0;36m'; BOLD=$'\033[1m'; DIM=$'\033[2m'; NC=$'\033[0m'
else
  RED=""; GREEN=""; YELLOW=""; CYAN=""; BOLD=""; DIM=""; NC=""
fi

INSTALL_DIR="${INSTALL_DIR:-/opt/giftcard-platform}"
SERVICE_NAME="giftcard-platform"
BIN_NAME="nexuscard"
LISTEN_PORT="${LISTEN_PORT:-8088}"
REPO_URL="${REPO_URL:-https://github.com/HenZenKuriRIP/NexusCard.git}"
SRC_DIR=""

ok()   { echo -e "  ${GREEN}OK${NC} $1"; }
info() { echo -e "  ${DIM}->${NC} $1"; }
warn() { echo -e "  ${YELLOW}!${NC} $1"; }
fail() { echo -e "  ${RED}X${NC} $1"; }
die()  { fail "$1"; exit 1; }

need_root() {
  [[ "${EUID:-$(id -u)}" -eq 0 ]] || die "Run as root: sudo bash deploy/install.sh"
}

rand_hex() { head -c "${1:-16}" /dev/urandom | od -A n -t x1 | tr -d ' \n'; }

banner() {
  echo ""
  echo -e "${CYAN}${BOLD}"
  cat <<'EOF'
  ======================================================
   NexusCard  ·  One-Click Setup
   Digital goods · Alipay · K2 giftcard · Domain + TLS
  ======================================================
EOF
  echo -e "${NC}"
}

do_uninstall() {
  banner
  echo -e "${YELLOW}Uninstall ${SERVICE_NAME}${NC}"
  systemctl stop "$SERVICE_NAME" 2>/dev/null || true
  systemctl disable "$SERVICE_NAME" 2>/dev/null || true
  rm -f "/etc/systemd/system/${SERVICE_NAME}.service"
  systemctl daemon-reload || true
  if [[ -f /etc/nginx/sites-enabled/giftcard-platform ]]; then
    rm -f /etc/nginx/sites-enabled/giftcard-platform /etc/nginx/sites-available/giftcard-platform
    nginx -t && systemctl reload nginx || true
  fi
  rm -f /etc/nginx/conf.d/giftcard-platform.conf 2>/dev/null || true
  warn "Data directory ${INSTALL_DIR} is kept. Remove manually: rm -rf ${INSTALL_DIR}"
  ok "Service removed"
  exit 0
}

[[ "${1:-}" == "--uninstall" ]] && { need_root; do_uninstall; }

need_root
banner

DOMAIN="${1:-}"
if [[ -z "$DOMAIN" ]]; then
  read -r -p "  Domain (e.g. pay.example.com): " DOMAIN
fi
[[ -n "$DOMAIN" ]] || die "Domain is required"
DOMAIN="${DOMAIN#http://}"; DOMAIN="${DOMAIN#https://}"; DOMAIN="${DOMAIN%%/*}"

read -r -p "  Obtain Let's Encrypt HTTPS? [Y/n]: " WANT_SSL
WANT_SSL="${WANT_SSL:-Y}"

read -r -p "  Admin username [admin]: " ADMIN_USER
ADMIN_USER="${ADMIN_USER:-admin}"
read -r -p "  Admin password [random]: " ADMIN_PASS
if [[ -z "$ADMIN_PASS" ]]; then
  ADMIN_PASS="$(rand_hex 8)"
fi
ADMIN_TOKEN="$(rand_hex 24)"
JWT_SECRET="$(rand_hex 24)"
API_SECRET="$(rand_hex 16)"

echo ""
info "Domain: https://${DOMAIN}"
info "Install dir: ${INSTALL_DIR}"
info "Listen: 127.0.0.1:${LISTEN_PORT}"

# ── packages ────────────────────────────────────────────────────────────────
echo ""
echo -e "${BOLD}[1/7] System packages${NC}"
export DEBIAN_FRONTEND=noninteractive
if command -v apt-get >/dev/null 2>&1; then
  apt-get update -qq
  apt-get install -y -qq curl ca-certificates nginx git build-essential >/dev/null
  apt-get install -y -qq certbot python3-certbot-nginx >/dev/null 2>&1 || warn "certbot not installed; TLS can be added later"
elif command -v yum >/dev/null 2>&1; then
  yum install -y curl nginx git gcc make >/dev/null
else
  die "Unsupported package manager (need apt or yum)"
fi
ok "Dependencies ready"

# ── Go ──────────────────────────────────────────────────────────────────────
echo -e "${BOLD}[2/7] Go toolchain${NC}"
if ! command -v go >/dev/null 2>&1; then
  info "Installing Go 1.22+"
  ARCH="$(uname -m)"
  case "$ARCH" in
    x86_64|amd64) GOARCH=amd64 ;;
    aarch64|arm64) GOARCH=arm64 ;;
    *) GOARCH=amd64 ;;
  esac
  curl -fsSL "https://go.dev/dl/go1.22.10.linux-${GOARCH}.tar.gz" -o /tmp/go.tgz
  rm -rf /usr/local/go
  tar -C /usr/local -xzf /tmp/go.tgz
  export PATH="/usr/local/go/bin:$PATH"
  echo 'export PATH=/usr/local/go/bin:$PATH' >/etc/profile.d/golang.sh
fi
export PATH="/usr/local/go/bin:${PATH:-}"
ok "go $(go version | awk '{print $3}')"

# ── source ──────────────────────────────────────────────────────────────────
echo -e "${BOLD}[3/7] Build${NC}"
mkdir -p "$INSTALL_DIR"
if [[ -f "./cmd/server/main.go" && -f "./go.mod" ]]; then
  SRC_DIR="$(pwd)"
  info "Using current source tree: $SRC_DIR"
elif [[ -n "${REPO_URL}" ]]; then
  SRC_DIR="/tmp/nexuscard-src"
  rm -rf "$SRC_DIR"
  git clone --depth 1 "$REPO_URL" "$SRC_DIR"
else
  SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
  if [[ -f "$SCRIPT_DIR/cmd/server/main.go" ]]; then
    SRC_DIR="$SCRIPT_DIR"
  else
    die "Source not found. Run from repo root: sudo bash deploy/install.sh $DOMAIN"
  fi
fi

cd "$SRC_DIR"
CGO_ENABLED=0 go build -ldflags="-s -w" -o "${INSTALL_DIR}/${BIN_NAME}" ./cmd/server
ok "Binary: ${INSTALL_DIR}/${BIN_NAME}"

# ── config ──────────────────────────────────────────────────────────────────
echo -e "${BOLD}[4/7] Configuration${NC}"
mkdir -p "${INSTALL_DIR}/data" "${INSTALL_DIR}/configs"
PUBLIC_URL="https://${DOMAIN}"

cat > "${INSTALL_DIR}/configs/config.yaml" <<EOF
# NexusCard production config (minimal).
# Configure Alipay keys in Admin UI -> Payment (hot reload).

server:
  listen: "127.0.0.1:${LISTEN_PORT}"
  public_base_url: "${PUBLIC_URL}"
  admin_token: "${ADMIN_TOKEN}"

db:
  driver: sqlite
  dsn: "${INSTALL_DIR}/data/giftcard.db"

alipay:
  mock_pay: true
  bill_subject: "Digital Goods"
  is_production: false
  product: "page"
  timeout_express: "30m"

notify_worker:
  max_attempts: 12
  base_backoff_sec: 5
  poll_interval_sec: 2

expire_worker:
  interval_sec: 15
  batch_size: 100

security:
  sign_skew_sec: 300
  https_only: true
  ssrf_block_private_ip: true

seed_merchant:
  app_id: "k2-main"
  name: "K2Board"
  api_secret: "${API_SECRET}"

admin:
  username: "${ADMIN_USER}"
  password: "${ADMIN_PASS}"
  jwt_secret: "${JWT_SECRET}"
  site_name: "NexusCard"

shop:
  title: "NexusCard Store"
  subtitle: "US Apple ID · App Store cards · Google / Netflix · Data plans · Instant delivery"
  order_ttl_min: 30
EOF

cat > "${INSTALL_DIR}/README-DEPLOY.txt" <<EOF
NexusCard deployment info
=========================
Domain:     ${PUBLIC_URL}
Shop:       ${PUBLIC_URL}/shop/
Admin:      ${PUBLIC_URL}/admin/
Health:     ${PUBLIC_URL}/healthz
Alipay notify: ${PUBLIC_URL}/alipay/notify

Admin user: ${ADMIN_USER}
Admin pass: ${ADMIN_PASS}
API token (Bearer): ${ADMIN_TOKEN}

K2 giftcard:
  base_url:   ${PUBLIC_URL}
  app_id:     k2-main
  api_secret: ${API_SECRET}
  product_name_template: Digital Goods

Next steps:
  1) Login admin, change password
  2) Admin -> Payment: enter Alipay keys, disable mock_pay for production
  3) Test shop purchase at /shop/
  4) Point K2 payment method giftcard to base_url above

Config file: ${INSTALL_DIR}/configs/config.yaml
Logs: journalctl -u ${SERVICE_NAME} -f
EOF
chmod 600 "${INSTALL_DIR}/configs/config.yaml" "${INSTALL_DIR}/README-DEPLOY.txt"
ok "Config written"

# ── systemd ─────────────────────────────────────────────────────────────────
echo -e "${BOLD}[5/7] systemd service${NC}"
id -u giftcard >/dev/null 2>&1 || useradd --system --home "$INSTALL_DIR" --shell /usr/sbin/nologin giftcard
chown -R giftcard:giftcard "$INSTALL_DIR"

cat > "/etc/systemd/system/${SERVICE_NAME}.service" <<EOF
[Unit]
Description=NexusCard payment and digital goods platform
After=network.target

[Service]
Type=simple
User=giftcard
Group=giftcard
WorkingDirectory=${INSTALL_DIR}
ExecStart=${INSTALL_DIR}/${BIN_NAME} -config ${INSTALL_DIR}/configs/config.yaml
Restart=on-failure
RestartSec=3
LimitNOFILE=65535
Environment=GIN_MODE=release

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable "$SERVICE_NAME"
systemctl restart "$SERVICE_NAME"
sleep 1
systemctl is-active --quiet "$SERVICE_NAME" && ok "Service running" || { journalctl -u "$SERVICE_NAME" -n 40 --no-pager; die "Service failed to start"; }

# ── nginx ───────────────────────────────────────────────────────────────────
echo -e "${BOLD}[6/7] Nginx reverse proxy${NC}"
NGINX_SITE="/etc/nginx/sites-available/giftcard-platform"
if [[ ! -d /etc/nginx/sites-available ]]; then
  mkdir -p /etc/nginx/conf.d
  NGINX_SITE="/etc/nginx/conf.d/giftcard-platform.conf"
fi

cat > "$NGINX_SITE" <<EOF
server {
    listen 80;
    listen [::]:80;
    server_name ${DOMAIN};

    client_max_body_size 8m;

    location / {
        proxy_pass http://127.0.0.1:${LISTEN_PORT};
        proxy_http_version 1.1;
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto \$scheme;
        proxy_read_timeout 60s;
    }
}
EOF

if [[ -d /etc/nginx/sites-enabled ]]; then
  ln -sfn "$NGINX_SITE" /etc/nginx/sites-enabled/giftcard-platform
  rm -f /etc/nginx/sites-enabled/default 2>/dev/null || true
fi
nginx -t
systemctl enable nginx >/dev/null 2>&1 || true
systemctl reload nginx
ok "Nginx: ${DOMAIN} -> 127.0.0.1:${LISTEN_PORT}"

# ── TLS ─────────────────────────────────────────────────────────────────────
echo -e "${BOLD}[7/7] HTTPS${NC}"
case "${WANT_SSL,,}" in
  n|no)
    warn "Skipped TLS. Set public_base_url and terminate SSL yourself."
    ;;
  *)
    info "Probing http://${DOMAIN}/healthz ..."
    for _ in 1 2 3 4 5 6 7 8; do
      if curl -fsS -m 5 "http://${DOMAIN}/healthz" >/dev/null 2>&1; then
        ok "HTTP reachable"
        break
      fi
      sleep 2
    done
    if command -v certbot >/dev/null 2>&1; then
      if certbot --nginx -d "$DOMAIN" --non-interactive --agree-tos --register-unsafely-without-email --redirect; then
        ok "Let's Encrypt certificate installed"
        sed -i "s|public_base_url:.*|public_base_url: \"https://${DOMAIN}\"|" "${INSTALL_DIR}/configs/config.yaml" 2>/dev/null \
          || sed -i '' "s|public_base_url:.*|public_base_url: \"https://${DOMAIN}\"|" "${INSTALL_DIR}/configs/config.yaml" 2>/dev/null || true
        systemctl restart "$SERVICE_NAME" || true
      else
        warn "certbot failed. Ensure DNS A record points here and ports 80/443 are open, then run:"
        warn "  certbot --nginx -d ${DOMAIN} --redirect"
      fi
    else
      warn "certbot missing; configure SSL manually"
    fi
    ;;
esac

echo ""
echo -e "${GREEN}${BOLD}Install complete${NC}"
echo "----------------------------------------------"
echo "  Shop:    https://${DOMAIN}/shop/"
echo "  Admin:   https://${DOMAIN}/admin/"
echo "  User:    ${ADMIN_USER} / ${ADMIN_PASS}"
echo "  Secrets: ${INSTALL_DIR}/README-DEPLOY.txt"
echo "  Notify:  https://${DOMAIN}/alipay/notify"
echo "  Config:  ${INSTALL_DIR}/configs/config.yaml"
echo "  Logs:    journalctl -u ${SERVICE_NAME} -f"
echo "----------------------------------------------"
echo "Next:"
echo "  1) Admin -> Payment: set Alipay keys, disable mock for production"
echo "  2) Test shop purchase"
echo "  3) Configure K2 giftcard base_url=https://${DOMAIN}"
echo ""
