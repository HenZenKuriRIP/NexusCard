#!/usr/bin/env bash
# =============================================================================
# NexusCard one-click installer (Linux)
#
# Downloads a pre-built binary from GitHub Releases (no Go toolchain required).
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/HenZenKuriRIP/NexusCard/main/deploy/install.sh \
#     | sudo bash -s -- pay.example.com
#
#   # Pin a release version
#   curl -fsSL ... | sudo VERSION=v1.0.0 bash -s -- pay.example.com
#
#   # From a local clone
#   sudo bash deploy/install.sh pay.example.com
#   sudo bash deploy/install.sh --uninstall
#
# Environment:
#   VERSION      Release tag (default: latest)
#   INSTALL_DIR  Install path (default: /opt/giftcard-platform)
#   LISTEN_PORT  App listen port (default: 8088)
#   REPO         owner/name (default: HenZenKuriRIP/NexusCard)
# =============================================================================
set -euo pipefail

# ── colours ─────────────────────────────────────────────────────────────────
if [[ -t 1 ]]; then
  RED=$'\033[0;31m'; GREEN=$'\033[0;32m'; YELLOW=$'\033[1;33m'
  CYAN=$'\033[0;36m'; BOLD=$'\033[1m'; DIM=$'\033[2m'; NC=$'\033[0m'
else
  RED=""; GREEN=""; YELLOW=""; CYAN=""; BOLD=""; DIM=""; NC=""
fi

# ── defaults ────────────────────────────────────────────────────────────────
INSTALL_DIR="${INSTALL_DIR:-/opt/giftcard-platform}"
SERVICE_NAME="giftcard-platform"
BIN_NAME="nexuscard"
LISTEN_PORT="${LISTEN_PORT:-8088}"
REPO="${REPO:-HenZenKuriRIP/NexusCard}"
VERSION="${VERSION:-latest}"
GITHUB_API="https://api.github.com"
GITHUB_DL="https://github.com/${REPO}/releases"
STEP_PAUSE="${STEP_PAUSE:-1.2}"   # seconds between major steps (readable pace)
TOTAL_STEPS=8

# ── helpers ─────────────────────────────────────────────────────────────────
ok()     { echo -e "  ${GREEN}✓${NC} $1"; }
info()   { echo -e "  ${DIM}→${NC} $1"; }
warn()   { echo -e "  ${YELLOW}!${NC} $1"; }
fail()   { echo -e "  ${RED}✗${NC} $1"; }
die()    { fail "$1"; exit 1; }
hr()     { echo -e "  ${DIM}────────────────────────────────────────${NC}"; }

pause() {
  # Keep install pace readable so operators can follow each step
  sleep "${STEP_PAUSE}" 2>/dev/null || sleep 1
}

step() {
  local n="$1" title="$2"
  echo ""
  hr
  echo -e "  ${CYAN}${BOLD}[${n}/${TOTAL_STEPS}]${NC} ${BOLD}${title}${NC}"
  hr
  pause
}

need_root() {
  [[ "${EUID:-$(id -u)}" -eq 0 ]] || die "请使用 root 运行: sudo bash deploy/install.sh <domain>"
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
  info "将从 GitHub Releases 拉取预编译二进制（无需安装 Go）"
  info "仓库: https://github.com/${REPO}"
  echo ""
}

# ── platform ────────────────────────────────────────────────────────────────
detect_platform() {
  local os arch
  os="$(uname -s | tr '[:upper:]' '[:lower:]')"
  arch="$(uname -m)"

  case "$os" in
    linux) ;;
    *) die "仅支持 Linux 服务器安装（当前: $(uname -s)）" ;;
  esac

  case "$arch" in
    x86_64|amd64) arch="amd64" ;;
    aarch64|arm64) arch="arm64" ;;
    *) die "不支持的 CPU 架构: ${arch}（支持 amd64 / arm64）" ;;
  esac

  PLATFORM_OS="linux"
  PLATFORM_ARCH="$arch"
  ASSET_NAME="${BIN_NAME}-${PLATFORM_OS}-${PLATFORM_ARCH}"
}

# Resolve release tag + download URL for the binary asset
resolve_release() {
  local tag api_json asset_url sums_url

  info "解析 Release 版本 …"
  if [[ "$VERSION" == "latest" ]]; then
    api_json="$(curl -fsSL "${GITHUB_API}/repos/${REPO}/releases/latest" 2>/dev/null)" \
      || die "无法访问 GitHub API（${GITHUB_API}/repos/${REPO}/releases/latest）。请检查网络或设置 VERSION=vX.Y.Z"
    tag="$(printf '%s' "$api_json" | sed -n 's/.*"tag_name":[[:space:]]*"\([^"]*\)".*/\1/p' | head -1)"
    [[ -n "$tag" ]] || die "未找到 latest Release，请先发布版本或指定 VERSION=vX.Y.Z"
  else
    tag="$VERSION"
    [[ "$tag" == v* ]] || tag="v${tag}"
    # Verify tag exists
    if ! curl -fsSL -o /dev/null -w "%{http_code}" "${GITHUB_API}/repos/${REPO}/releases/tags/${tag}" 2>/dev/null | grep -qE '200'; then
      # soft check — still try download URL
      warn "无法通过 API 校验 tag ${tag}，将直接尝试下载"
    fi
  fi

  RELEASE_TAG="$tag"
  BINARY_URL="${GITHUB_DL}/download/${RELEASE_TAG}/${ASSET_NAME}"
  SUMS_URL="${GITHUB_DL}/download/${RELEASE_TAG}/SHA256SUMS.txt"

  info "版本: ${RELEASE_TAG}"
  info "平台: ${PLATFORM_OS}/${PLATFORM_ARCH}"
  info "资产: ${ASSET_NAME}"
  info "地址: ${BINARY_URL}"
  pause
}

download_binary() {
  local tmp_dir tmp_bin tmp_sums expected actual

  tmp_dir="$(mktemp -d)"
  tmp_bin="${tmp_dir}/${ASSET_NAME}"
  tmp_sums="${tmp_dir}/SHA256SUMS.txt"
  # shellcheck disable=SC2064
  trap "rm -rf '${tmp_dir}'" RETURN

  info "下载二进制（可能需要数秒，请稍候）…"
  if ! curl -fL --progress-bar -o "$tmp_bin" "$BINARY_URL"; then
    die "下载失败: ${BINARY_URL}
  请确认 Release ${RELEASE_TAG} 已包含 ${ASSET_NAME}
  查看: https://github.com/${REPO}/releases"
  fi

  [[ -s "$tmp_bin" ]] || die "下载的文件为空"
  chmod +x "$tmp_bin"

  # Optional checksum verification
  info "校验 SHA256（如有校验文件）…"
  if curl -fsSL -o "$tmp_sums" "$SUMS_URL" 2>/dev/null && [[ -s "$tmp_sums" ]]; then
    expected="$(grep -E "[[:space:]]${ASSET_NAME}\$" "$tmp_sums" | awk '{print $1}' | head -1)"
    if [[ -n "$expected" ]]; then
      if command -v sha256sum >/dev/null 2>&1; then
        actual="$(sha256sum "$tmp_bin" | awk '{print $1}')"
      elif command -v shasum >/dev/null 2>&1; then
        actual="$(shasum -a 256 "$tmp_bin" | awk '{print $1}')"
      else
        actual=""
        warn "系统无 sha256sum/shasum，跳过校验"
      fi
      if [[ -n "$actual" ]]; then
        if [[ "$actual" != "$expected" ]]; then
          die "校验和不匹配
  期望: ${expected}
  实际: ${actual}"
        fi
        ok "SHA256 校验通过"
      fi
    else
      warn "校验文件中未找到 ${ASSET_NAME}，跳过校验"
    fi
  else
    warn "未获取到 SHA256SUMS.txt，跳过校验（建议使用官方 Release）"
  fi

  # Quick binary smoke: ELF header
  if command -v file >/dev/null 2>&1; then
    info "文件类型: $(file -b "$tmp_bin" | head -c 80)"
  fi
  # Read first 4 bytes as hex; ELF magic is 7f 45 4c 46
  magic="$(od -An -tx1 -N4 "$tmp_bin" | tr -d ' \n')"
  if [[ "$magic" != "7f454c46" ]]; then
    die "下载的文件不是 ELF 可执行文件，请检查 Release 资产"
  fi

  mkdir -p "$INSTALL_DIR"
  install -m 0755 "$tmp_bin" "${INSTALL_DIR}/${BIN_NAME}"
  ok "已安装: ${INSTALL_DIR}/${BIN_NAME}"
  pause
}

# ── uninstall ───────────────────────────────────────────────────────────────
do_uninstall() {
  banner
  echo -e "  ${YELLOW}${BOLD}卸载 ${SERVICE_NAME}${NC}"
  echo ""
  info "停止并禁用 systemd 服务 …"
  systemctl stop "$SERVICE_NAME" 2>/dev/null || true
  systemctl disable "$SERVICE_NAME" 2>/dev/null || true
  rm -f "/etc/systemd/system/${SERVICE_NAME}.service"
  systemctl daemon-reload || true
  ok "服务已移除"

  info "清理 Nginx 站点配置 …"
  if [[ -f /etc/nginx/sites-enabled/giftcard-platform ]]; then
    rm -f /etc/nginx/sites-enabled/giftcard-platform /etc/nginx/sites-available/giftcard-platform
    nginx -t && systemctl reload nginx || true
  fi
  rm -f /etc/nginx/conf.d/giftcard-platform.conf 2>/dev/null || true
  ok "Nginx 配置已清理"

  echo ""
  warn "数据目录 ${INSTALL_DIR} 已保留（含数据库与配置）"
  info "如需彻底删除:  rm -rf ${INSTALL_DIR}"
  echo ""
  ok "卸载完成"
  exit 0
}

# ── entry ───────────────────────────────────────────────────────────────────
[[ "${1:-}" == "--uninstall" ]] && { need_root; do_uninstall; }

need_root
banner

DOMAIN="${1:-}"
if [[ -z "$DOMAIN" ]]; then
  read -r -p "  请输入域名 (例如 pay.example.com): " DOMAIN
fi
[[ -n "$DOMAIN" ]] || die "域名不能为空"
DOMAIN="${DOMAIN#http://}"; DOMAIN="${DOMAIN#https://}"; DOMAIN="${DOMAIN%%/*}"

echo ""
read -r -p "  是否申请 Let's Encrypt HTTPS？ [Y/n]: " WANT_SSL
WANT_SSL="${WANT_SSL:-Y}"

read -r -p "  管理员用户名 [admin]: " ADMIN_USER
ADMIN_USER="${ADMIN_USER:-admin}"
read -r -p "  管理员密码 [回车=随机生成]: " ADMIN_PASS
if [[ -z "$ADMIN_PASS" ]]; then
  ADMIN_PASS="$(rand_hex 8)"
  info "已生成随机管理员密码"
fi

ADMIN_TOKEN="$(rand_hex 24)"
JWT_SECRET="$(rand_hex 24)"
API_SECRET="$(rand_hex 16)"

echo ""
echo -e "  ${BOLD}安装摘要${NC}"
info "域名:       https://${DOMAIN}"
info "安装目录:   ${INSTALL_DIR}"
info "监听地址:   127.0.0.1:${LISTEN_PORT}"
info "Release:    ${VERSION}"
info "管理员:     ${ADMIN_USER}"
pause

# ═══════════════════════════════════════════════════════════════════════════
# [1] Preflight
# ═══════════════════════════════════════════════════════════════════════════
step 1 "环境预检"

detect_platform
ok "操作系统: Linux / $(uname -r)"
ok "CPU 架构: ${PLATFORM_ARCH} ($(uname -m))"

if ! command -v curl >/dev/null 2>&1; then
  info "未检测到 curl，将在下一步随系统包一并安装"
else
  ok "curl 可用"
fi

# DNS hint (non-fatal)
if command -v getent >/dev/null 2>&1; then
  RESOLVED="$(getent hosts "$DOMAIN" 2>/dev/null | awk '{print $1}' | head -1 || true)"
  if [[ -n "$RESOLVED" ]]; then
    ok "域名解析: ${DOMAIN} → ${RESOLVED}"
  else
    warn "当前无法解析 ${DOMAIN}（若尚未配置 DNS，HTTPS 申请可能失败）"
  fi
fi
pause

# ═══════════════════════════════════════════════════════════════════════════
# [2] System packages
# ═══════════════════════════════════════════════════════════════════════════
step 2 "安装系统依赖"

info "需要: curl、ca-certificates、nginx；可选 certbot（HTTPS）"
info "不需要: Go 编译环境 / git / build-essential"
export DEBIAN_FRONTEND=noninteractive

if command -v apt-get >/dev/null 2>&1; then
  info "使用 apt 更新软件源 …"
  apt-get update -qq
  info "安装 curl ca-certificates nginx …"
  apt-get install -y -qq curl ca-certificates nginx >/dev/null
  info "安装 certbot（可选）…"
  apt-get install -y -qq certbot python3-certbot-nginx >/dev/null 2>&1 \
    || warn "certbot 未安装；可稍后手动配置 TLS"
elif command -v dnf >/dev/null 2>&1; then
  info "使用 dnf 安装依赖 …"
  dnf install -y curl ca-certificates nginx >/dev/null
  dnf install -y certbot python3-certbot-nginx >/dev/null 2>&1 || warn "certbot 未安装"
elif command -v yum >/dev/null 2>&1; then
  info "使用 yum 安装依赖 …"
  yum install -y curl ca-certificates nginx >/dev/null
  yum install -y certbot python3-certbot-nginx >/dev/null 2>&1 || warn "certbot 未安装"
else
  die "不支持的包管理器（需要 apt / dnf / yum）"
fi
ok "系统依赖就绪"
pause

# ═══════════════════════════════════════════════════════════════════════════
# [3] Download binary from Releases
# ═══════════════════════════════════════════════════════════════════════════
step 3 "从 GitHub Releases 拉取二进制"

resolve_release
download_binary

if ver_out="$("${INSTALL_DIR}/${BIN_NAME}" -version 2>/dev/null)"; then
  ok "二进制可执行（version: ${ver_out}）"
else
  ok "二进制已部署"
fi
pause

# ═══════════════════════════════════════════════════════════════════════════
# [4] Configuration
# ═══════════════════════════════════════════════════════════════════════════
step 4 "写入生产配置"

mkdir -p "${INSTALL_DIR}/data" "${INSTALL_DIR}/configs"
PUBLIC_URL="https://${DOMAIN}"

info "生成 config.yaml 与部署说明 …"
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
Release:    ${RELEASE_TAG}
Platform:   ${PLATFORM_OS}/${PLATFORM_ARCH}
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

Binary:  ${INSTALL_DIR}/${BIN_NAME}
Config:  ${INSTALL_DIR}/configs/config.yaml
Logs:    journalctl -u ${SERVICE_NAME} -f
EOF
chmod 600 "${INSTALL_DIR}/configs/config.yaml" "${INSTALL_DIR}/README-DEPLOY.txt"
ok "配置已写入 ${INSTALL_DIR}/configs/config.yaml"
ok "部署说明: ${INSTALL_DIR}/README-DEPLOY.txt"
pause

# ═══════════════════════════════════════════════════════════════════════════
# [5] systemd
# ═══════════════════════════════════════════════════════════════════════════
step 5 "注册 systemd 服务"

info "创建系统用户 giftcard（若不存在）…"
id -u giftcard >/dev/null 2>&1 || useradd --system --home "$INSTALL_DIR" --shell /usr/sbin/nologin giftcard
chown -R giftcard:giftcard "$INSTALL_DIR"
ok "权限: giftcard:giftcard → ${INSTALL_DIR}"

info "写入 unit: /etc/systemd/system/${SERVICE_NAME}.service"
cat > "/etc/systemd/system/${SERVICE_NAME}.service" <<EOF
[Unit]
Description=NexusCard payment and digital goods platform
After=network.target
Documentation=https://github.com/${REPO}

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

info "daemon-reload → enable → restart …"
systemctl daemon-reload
systemctl enable "$SERVICE_NAME" >/dev/null
systemctl restart "$SERVICE_NAME"
sleep 2
if systemctl is-active --quiet "$SERVICE_NAME"; then
  ok "服务运行中: ${SERVICE_NAME}"
else
  journalctl -u "$SERVICE_NAME" -n 40 --no-pager || true
  die "服务启动失败，请查看上方日志"
fi
pause

# ═══════════════════════════════════════════════════════════════════════════
# [6] Nginx
# ═══════════════════════════════════════════════════════════════════════════
step 6 "配置 Nginx 反向代理"

NGINX_SITE="/etc/nginx/sites-available/giftcard-platform"
if [[ ! -d /etc/nginx/sites-available ]]; then
  mkdir -p /etc/nginx/conf.d
  NGINX_SITE="/etc/nginx/conf.d/giftcard-platform.conf"
fi

info "写入站点: ${NGINX_SITE}"
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

info "nginx -t && reload …"
nginx -t
systemctl enable nginx >/dev/null 2>&1 || true
systemctl reload nginx
ok "Nginx: ${DOMAIN} → 127.0.0.1:${LISTEN_PORT}"
pause

# ═══════════════════════════════════════════════════════════════════════════
# [7] TLS
# ═══════════════════════════════════════════════════════════════════════════
step 7 "HTTPS / Let's Encrypt"

case "${WANT_SSL,,}" in
  n|no)
    warn "已跳过 TLS。请自行配置证书，并确认 public_base_url。"
    ;;
  *)
    info "探测本机 HTTP 健康检查 http://${DOMAIN}/healthz …"
    reachable=0
    for i in 1 2 3 4 5 6 7 8; do
      if curl -fsS -m 5 "http://${DOMAIN}/healthz" >/dev/null 2>&1; then
        ok "HTTP 可达（第 ${i} 次探测）"
        reachable=1
        break
      fi
      info "等待服务就绪… (${i}/8)"
      sleep 2
    done
    if [[ "$reachable" -ne 1 ]]; then
      warn "暂时无法通过域名访问 /healthz（DNS 或防火墙可能未就绪）"
      warn "仍将尝试申请证书；失败时可稍后手动执行 certbot"
    fi

    if command -v certbot >/dev/null 2>&1; then
      info "运行 certbot --nginx …"
      if certbot --nginx -d "$DOMAIN" --non-interactive --agree-tos \
          --register-unsafely-without-email --redirect; then
        ok "Let's Encrypt 证书已安装"
        sed -i "s|public_base_url:.*|public_base_url: \"https://${DOMAIN}\"|" \
          "${INSTALL_DIR}/configs/config.yaml" 2>/dev/null \
          || sed -i '' "s|public_base_url:.*|public_base_url: \"https://${DOMAIN}\"|" \
          "${INSTALL_DIR}/configs/config.yaml" 2>/dev/null || true
        systemctl restart "$SERVICE_NAME" || true
      else
        warn "certbot 失败。请确认 DNS A 记录指向本机，且 80/443 开放，然后执行:"
        warn "  certbot --nginx -d ${DOMAIN} --redirect"
      fi
    else
      warn "未安装 certbot；请手动配置 SSL"
    fi
    ;;
esac
pause

# ═══════════════════════════════════════════════════════════════════════════
# [8] Final check
# ═══════════════════════════════════════════════════════════════════════════
step 8 "最终检查与汇总"

info "检查本机进程与端口 …"
if systemctl is-active --quiet "$SERVICE_NAME"; then
  ok "systemd: active"
else
  fail "systemd: inactive"
fi

if curl -fsS -m 3 "http://127.0.0.1:${LISTEN_PORT}/healthz" >/dev/null 2>&1; then
  ok "本地 healthz: OK"
else
  warn "本地 healthz 暂无响应，请查看: journalctl -u ${SERVICE_NAME} -n 50"
fi

echo ""
echo -e "${GREEN}${BOLD}  安装完成${NC}"
echo "  =============================================="
echo "  Release:  ${RELEASE_TAG}  (${PLATFORM_OS}/${PLATFORM_ARCH})"
echo "  Binary:   ${INSTALL_DIR}/${BIN_NAME}"
echo "  Shop:     https://${DOMAIN}/shop/"
echo "  Admin:    https://${DOMAIN}/admin/"
echo "  User:     ${ADMIN_USER} / ${ADMIN_PASS}"
echo "  Secrets:  ${INSTALL_DIR}/README-DEPLOY.txt"
echo "  Notify:   https://${DOMAIN}/alipay/notify"
echo "  Config:   ${INSTALL_DIR}/configs/config.yaml"
echo "  Logs:     journalctl -u ${SERVICE_NAME} -f"
echo "  =============================================="
echo ""
echo -e "  ${BOLD}接下来请完成:${NC}"
echo "    1) 登录后台并修改密码"
echo "    2) Admin → Payment: 填写支付宝密钥，生产环境关闭 mock_pay"
echo "    3) 在 /shop/ 试下一单"
echo "    4) K2 giftcard 的 base_url 设为 https://${DOMAIN}"
echo ""
echo -e "  ${DIM}重新安装/升级同一版本: 再次运行本脚本即可覆盖二进制${NC}"
echo -e "  ${DIM}指定版本: VERSION=v1.0.0 sudo -E bash deploy/install.sh ${DOMAIN}${NC}"
echo ""
