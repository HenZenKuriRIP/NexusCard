#!/usr/bin/env bash
# =============================================================================
# NexusCard one-click installer (Linux)
#
# Downloads a pre-built binary from GitHub Releases (no Go toolchain required).
# Run as root (typical VPS login). Do NOT use sudo.
#
# Install (process substitution — preferred; keeps stdin free for prompts):
#   bash <(curl -fsSL https://raw.githubusercontent.com/HenZenKuriRIP/NexusCard/main/deploy/install.sh) \
#     pay.example.com
#
#   # Pin a release version
#   VERSION=v1.1.1 bash <(curl -fsSL https://raw.githubusercontent.com/HenZenKuriRIP/NexusCard/main/deploy/install.sh) \
#     pay.example.com
#
# Uninstall (same script, service only; keeps DB/certs):
#   bash <(curl -fsSL https://raw.githubusercontent.com/HenZenKuriRIP/NexusCard/main/deploy/install.sh) \
#     --uninstall
#
# From a local clone:
#   bash deploy/install.sh pay.example.com
#   bash deploy/install.sh --uninstall
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
  [[ "${EUID:-$(id -u)}" -eq 0 ]] || die "请使用 root 运行（不要加 sudo）:
  bash <(curl -fsSL https://raw.githubusercontent.com/HenZenKuriRIP/NexusCard/main/deploy/install.sh) <domain>"
}

# Refuse curl|bash / bash -s (script on stdin steals interactive prompts).
# Allow: regular file, or process substitution bash <(curl ...) → /dev/fd/N
refuse_stdin_script() {
  local src="${BASH_SOURCE[0]:-}"
  # bash -s / curl|bash: no script path; process sub has /dev/fd/* or /proc/self/fd/*
  if [[ -n "$src" && ( -f "$src" || "$src" == /dev/fd/* || "$src" == /proc/self/fd/* ) ]]; then
    return 0
  fi
  cat <<'EOF' >&2

  ✗ 请勿使用 curl | bash 管道安装（脚本占满 stdin，配置易损坏）。

  推荐方式（root，不要 sudo）:

    bash <(curl -fsSL https://raw.githubusercontent.com/HenZenKuriRIP/NexusCard/main/deploy/install.sh) \
      your.domain.com

  卸载:

    bash <(curl -fsSL https://raw.githubusercontent.com/HenZenKuriRIP/NexusCard/main/deploy/install.sh) \
      --uninstall

EOF
  exit 1
}

rand_hex() { head -c "${1:-16}" /dev/urandom | od -A n -t x1 | tr -d ' \n'; }

# Interactive terminal for prompts (script file + TTY).
has_tty() { [[ -t 0 ]] || [[ -r /dev/tty && -w /dev/tty ]]; }

# prompt VAR "message" ["default"]
# Sets VAR to user input, or default when empty / non-interactive.
prompt() {
  local __var="$1" __msg="$2" __def="${3:-}" __ans=""
  if [[ -t 0 ]]; then
    read -r -p "$__msg" __ans || __ans=""
  elif [[ -r /dev/tty && -w /dev/tty ]]; then
    read -r -p "$__msg" __ans </dev/tty || __ans=""
  fi
  if [[ -z "$__ans" ]]; then
    __ans="$__def"
  fi
  __ans="${__ans//$'\r'/}"
  __ans="${__ans//$'\n'/}"
  printf -v "$__var" '%s' "$__ans"
}

# YAML double-quoted scalar escaping
yaml_quote() {
  local s="$1"
  s="${s//\\/\\\\}"
  s="${s//\"/\\\"}"
  s="${s//$'\n'/\\n}"
  s="${s//$'\r'/}"
  printf '"%s"' "$s"
}

# Return 0 if config.yaml is parseable enough to start the service.
# Prefer the installed binary (-check-config); fall back to structural checks.
config_is_valid() {
  local f="$1"
  [[ -f "$f" && -s "$f" ]] || return 1
  grep -qE '^server:' "$f" || return 1
  grep -qE '^admin:' "$f" || return 1
  grep -qE '^db:' "$f" || return 1
  # Odd number of " on a line usually means a broken / polluted value
  if awk 'BEGIN{bad=0} {
      n=gsub(/"/,"&");
      if (n%2!=0) { bad=1; exit }
    } END{ exit bad }' "$f" 2>/dev/null; then
    :
  else
    return 1
  fi
  local bin="${INSTALL_DIR}/${BIN_NAME}"
  if [[ -x "$bin" ]]; then
    if "$bin" -check-config -config "$f" >/dev/null 2>&1; then
      return 0
    fi
    # Older binaries without -check-config: try load via short timeout run is too heavy
    # If binary supports the flag, failure means invalid YAML
    if "$bin" -h 2>&1 | grep -q 'check-config'; then
      return 1
    fi
  fi
  return 0
}

banner() {
  echo ""
  echo -e "${CYAN}${BOLD}"
  cat <<'EOF'
  ======================================================
   NexusCard  ·  One-Click Setup
   数字商品 · 支付宝 · K2 礼品卡 · 域名 + TLS
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

# ── TLS helpers ─────────────────────────────────────────────────────────────
# Return 0 if a non-expired Let's Encrypt cert for $1 already exists on disk.
has_valid_cert() {
  local domain="$1"
  local live="/etc/letsencrypt/live/${domain}/fullchain.pem"
  local key="/etc/letsencrypt/live/${domain}/privkey.pem"
  [[ -f "$live" && -f "$key" ]] || return 1
  if command -v openssl >/dev/null 2>&1; then
    # Valid if not expiring within 1 day
    openssl x509 -checkend 86400 -noout -in "$live" 2>/dev/null || return 1
  fi
  return 0
}

cert_expiry_info() {
  local live="/etc/letsencrypt/live/${1}/fullchain.pem"
  [[ -f "$live" ]] || return 0
  if command -v openssl >/dev/null 2>&1; then
    openssl x509 -enddate -noout -in "$live" 2>/dev/null | sed 's/notAfter=/到期: /' || true
  fi
}

# Write Nginx site: HTTP-only, or HTTPS with existing LE certs.
write_nginx_site() {
  local domain="$1"
  local with_ssl="${2:-0}"
  local site="$3"

  if [[ "$with_ssl" == "1" ]]; then
    local ssl_opts="" ssl_dh=""
    [[ -f /etc/letsencrypt/options-ssl-nginx.conf ]] \
      && ssl_opts="    include /etc/letsencrypt/options-ssl-nginx.conf;"
    [[ -f /etc/letsencrypt/ssl-dhparams.pem ]] \
      && ssl_dh="    ssl_dhparam /etc/letsencrypt/ssl-dhparams.pem;"

    cat > "$site" <<EOF
# Managed by NexusCard install.sh — HTTP → HTTPS
server {
    listen 80;
    listen [::]:80;
    server_name ${domain};

    location /.well-known/acme-challenge/ {
        root /var/www/html;
    }

    location / {
        return 301 https://\$host\$request_uri;
    }
}

server {
    listen 443 ssl http2;
    listen [::]:443 ssl http2;
    server_name ${domain};

    ssl_certificate     /etc/letsencrypt/live/${domain}/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/${domain}/privkey.pem;
${ssl_opts}
${ssl_dh}

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
  else
    cat > "$site" <<EOF
# Managed by NexusCard install.sh — HTTP only (TLS later via certbot)
server {
    listen 80;
    listen [::]:80;
    server_name ${domain};

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
  fi
}

# ── uninstall ───────────────────────────────────────────────────────────────
# Removes service + Nginx site only.
# PRESERVES: INSTALL_DIR (DB + config), Let's Encrypt certs under /etc/letsencrypt
do_uninstall() {
  banner
  echo -e "  ${YELLOW}${BOLD}卸载 ${SERVICE_NAME}${NC}"
  echo ""
  info "策略: 仅移除服务与站点配置；数据库与域名证书一律保留"
  pause

  info "停止并禁用 systemd 服务 …"
  systemctl stop "$SERVICE_NAME" 2>/dev/null || true
  systemctl disable "$SERVICE_NAME" 2>/dev/null || true
  rm -f "/etc/systemd/system/${SERVICE_NAME}.service"
  systemctl daemon-reload || true
  ok "服务已移除"

  info "清理 Nginx 站点配置（不触碰证书文件）…"
  if [[ -f /etc/nginx/sites-enabled/giftcard-platform ]]; then
    rm -f /etc/nginx/sites-enabled/giftcard-platform /etc/nginx/sites-available/giftcard-platform
  fi
  rm -f /etc/nginx/conf.d/giftcard-platform.conf 2>/dev/null || true
  if command -v nginx >/dev/null 2>&1; then
    nginx -t 2>/dev/null && systemctl reload nginx || true
  fi
  ok "Nginx 站点配置已清理"

  echo ""
  echo -e "  ${BOLD}已保留（重装可继续使用）:${NC}"
  if [[ -d "$INSTALL_DIR" ]]; then
    ok "数据目录: ${INSTALL_DIR}"
    [[ -f "${INSTALL_DIR}/data/giftcard.db" ]] && ok "数据库:   ${INSTALL_DIR}/data/giftcard.db"
    [[ -f "${INSTALL_DIR}/configs/config.yaml" ]] && ok "配置:     ${INSTALL_DIR}/configs/config.yaml"
  else
    warn "安装目录不存在: ${INSTALL_DIR}"
  fi
  if [[ -d /etc/letsencrypt/live ]]; then
    ok "Let's Encrypt 证书目录: /etc/letsencrypt（未删除）"
    # List lineages that look related / all live domains
    local d
    for d in /etc/letsencrypt/live/*/; do
      [[ -d "$d" ]] || continue
      local name; name="$(basename "$d")"
      [[ "$name" == "README" ]] && continue
      if has_valid_cert "$name"; then
        ok "  证书 ${name} 有效  $(cert_expiry_info "$name")"
      elif [[ -f "${d}fullchain.pem" ]]; then
        warn "  证书 ${name} 存在但可能已过期"
      fi
    done
  else
    info "本机暂无 /etc/letsencrypt/live 证书"
  fi

  echo ""
  warn "不会自动删除数据库与证书。彻底清除请手动执行:"
  info "  rm -rf ${INSTALL_DIR}"
  info "  certbot delete --cert-name <domain>   # 仅当确认不再需要证书时"
  echo ""
  ok "卸载完成（可随时用同一域名重新安装，将复用库与证书）"
  exit 0
}

# ── entry ───────────────────────────────────────────────────────────────────
refuse_stdin_script
[[ "${1:-}" == "--uninstall" ]] && { need_root; do_uninstall; }

need_root
banner

DOMAIN="${1:-}"
if [[ -z "$DOMAIN" ]]; then
  # Prefer domain remembered from previous install
  if [[ -f "${INSTALL_DIR}/configs/config.yaml" ]]; then
    PREV_DOMAIN="$(sed -n 's/.*public_base_url:[[:space:]]*"https\?:\/\/\([^"/]*\)".*/\1/p' \
      "${INSTALL_DIR}/configs/config.yaml" 2>/dev/null | head -1 || true)"
  fi
  if [[ -n "${PREV_DOMAIN:-}" ]]; then
    prompt DOMAIN "  请输入域名 [回车=沿用 ${PREV_DOMAIN}]: " "$PREV_DOMAIN"
  else
    if has_tty; then
      prompt DOMAIN "  请输入域名 (例如 pay.example.com): " ""
    else
      die "非交互安装必须提供域名: bash install.sh pay.example.com"
    fi
  fi
fi
[[ -n "$DOMAIN" ]] || die "域名不能为空"
DOMAIN="${DOMAIN#http://}"; DOMAIN="${DOMAIN#https://}"; DOMAIN="${DOMAIN%%/*}"

if ! has_tty; then
  info "非交互模式 → 使用默认选项（HTTPS=是，随机管理员密码）"
fi

# Detect reinstall / existing assets up front
REUSE_DB=0
REUSE_CFG=0
REUSE_CERT=0
[[ -f "${INSTALL_DIR}/data/giftcard.db" ]] && REUSE_DB=1
[[ -f "${INSTALL_DIR}/configs/config.yaml" ]] && REUSE_CFG=1
# Broken leftover config must not be reused (common after failed first install)
if [[ "$REUSE_CFG" -eq 1 ]]; then
  CFG_BROKEN=0
  if ! config_is_valid "${INSTALL_DIR}/configs/config.yaml"; then
    CFG_BROKEN=1
  elif systemctl is-failed --quiet "${SERVICE_NAME}" 2>/dev/null \
    || ! systemctl is-active --quiet "${SERVICE_NAME}" 2>/dev/null; then
    # Service never came up after previous install — check logs for YAML load errors
    if journalctl -u "${SERVICE_NAME}" -n 50 --no-pager 2>/dev/null \
      | grep -qE 'load config|did not find expected key|yaml:'; then
      CFG_BROKEN=1
    fi
  fi
  if [[ "$CFG_BROKEN" -eq 1 ]]; then
    warn "已有 config.yaml 无效或曾导致启动失败，将重新生成（原文件备份为 .broken）"
    mv -f "${INSTALL_DIR}/configs/config.yaml" \
      "${INSTALL_DIR}/configs/config.yaml.broken.$(date +%s)" 2>/dev/null || true
    REUSE_CFG=0
  fi
fi
has_valid_cert "$DOMAIN" && REUSE_CERT=1

echo ""
echo -e "  ${BOLD}检测到的已有数据${NC}"
if [[ "$REUSE_DB" -eq 1 ]]; then
  ok "数据库已存在 → 重装将保留订单/商品等数据"
else
  info "无历史数据库，将新建"
fi
if [[ "$REUSE_CFG" -eq 1 ]]; then
  ok "配置已存在 → 重装将保留密钥与管理员设置"
else
  info "无历史配置，将生成新配置"
fi
if [[ "$REUSE_CERT" -eq 1 ]]; then
  ok "域名 ${DOMAIN} 已有有效证书 → 不再重复申请  $(cert_expiry_info "$DOMAIN")"
else
  if [[ -f "/etc/letsencrypt/live/${DOMAIN}/fullchain.pem" ]]; then
    warn "证书文件存在但已过期/即将过期 → 将尝试续期/重签"
  else
    info "本机尚无 ${DOMAIN} 的 Let's Encrypt 证书"
  fi
fi
pause

echo ""
FORCE_RENEW=0
if [[ "$REUSE_CERT" -eq 1 ]]; then
  info "已有有效证书，默认跳过 certbot 申请"
  prompt FORCE_SSL "  是否强制重新申请证书？ [y/N]: " "N"
  if [[ "${FORCE_SSL,,}" == "y" || "${FORCE_SSL,,}" == "yes" ]]; then
    WANT_SSL="Y"
    REUSE_CERT=0
    FORCE_RENEW=1
    info "将强制重新申请证书"
  else
    WANT_SSL="reuse"
  fi
else
  prompt WANT_SSL "  是否申请 Let's Encrypt HTTPS？ [Y/n]: " "Y"
  WANT_SSL="${WANT_SSL:-Y}"
fi

# Admin credentials only when creating a brand-new config
ADMIN_USER="admin"
ADMIN_PASS=""
ADMIN_TOKEN=""
JWT_SECRET=""
API_SECRET=""
if [[ "$REUSE_CFG" -eq 0 ]]; then
  prompt ADMIN_USER "  管理员用户名 [admin]: " "admin"
  ADMIN_USER="${ADMIN_USER:-admin}"
  # Sanitize username: only allow simple identifiers for YAML safety
  if [[ ! "$ADMIN_USER" =~ ^[A-Za-z0-9_.@-]+$ ]]; then
    warn "管理员用户名含非法字符，回退为 admin"
    ADMIN_USER="admin"
  fi
  prompt ADMIN_PASS "  管理员密码 [回车=随机生成]: " ""
  if [[ -z "$ADMIN_PASS" ]]; then
    ADMIN_PASS="$(rand_hex 8)"
    info "已生成随机管理员密码"
  fi
  ADMIN_TOKEN="$(rand_hex 24)"
  JWT_SECRET="$(rand_hex 24)"
  API_SECRET="$(rand_hex 16)"
else
  info "沿用已有配置，不重置管理员密码与 API 密钥"
fi

echo ""
echo -e "  ${BOLD}安装摘要${NC}"
info "域名:       https://${DOMAIN}"
info "安装目录:   ${INSTALL_DIR}"
info "监听地址:   127.0.0.1:${LISTEN_PORT}"
info "Release:    ${VERSION}"
info "保留数据库: $([[ $REUSE_DB -eq 1 ]] && echo 是 || echo 否)"
info "保留配置:   $([[ $REUSE_CFG -eq 1 ]] && echo 是 || echo 否)"
info "证书策略:   $([[ $REUSE_CERT -eq 1 || "$WANT_SSL" == "reuse" ]] && echo "复用已有证书" || echo "$WANT_SSL")"
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
step 4 "写入 / 保留生产配置"

mkdir -p "${INSTALL_DIR}/data" "${INSTALL_DIR}/configs"
PUBLIC_URL="https://${DOMAIN}"
CFG="${INSTALL_DIR}/configs/config.yaml"

if [[ "$REUSE_CFG" -eq 1 && -f "$CFG" ]]; then
  info "检测到已有配置，保留密钥与业务设置 …"
  # Only refresh public_base_url / listen so domain re-bind works after reinstall
  if grep -q 'public_base_url:' "$CFG" 2>/dev/null; then
    sed -i "s|public_base_url:.*|public_base_url: \"${PUBLIC_URL}\"|" "$CFG" 2>/dev/null \
      || sed -i '' "s|public_base_url:.*|public_base_url: \"${PUBLIC_URL}\"|" "$CFG" 2>/dev/null || true
  fi
  if grep -q 'listen:' "$CFG" 2>/dev/null; then
    sed -i "s|listen:.*|listen: \"127.0.0.1:${LISTEN_PORT}\"|" "$CFG" 2>/dev/null \
      || sed -i '' "s|listen:.*|listen: \"127.0.0.1:${LISTEN_PORT}\"|" "$CFG" 2>/dev/null || true
  fi
  ok "已保留: ${CFG}"
  [[ "$REUSE_DB" -eq 1 ]] && ok "已保留数据库: ${INSTALL_DIR}/data/giftcard.db"

  cat > "${INSTALL_DIR}/README-DEPLOY.txt" <<EOF
NexusCard deployment info (reinstall — config/DB preserved)
===========================================================
Release:    ${RELEASE_TAG}
Platform:   ${PLATFORM_OS}/${PLATFORM_ARCH}
Domain:     ${PUBLIC_URL}
Shop:       ${PUBLIC_URL}/shop/
Admin:      ${PUBLIC_URL}/admin/
Health:     ${PUBLIC_URL}/healthz
Alipay notify: ${PUBLIC_URL}/alipay/notify

Config and admin credentials were kept from the previous install.
See previous notes or login with your existing admin account.

Binary:  ${INSTALL_DIR}/${BIN_NAME}
Config:  ${CFG}
DB:      ${INSTALL_DIR}/data/giftcard.db
Logs:    journalctl -u ${SERVICE_NAME} -f
EOF
  chmod 600 "$CFG" "${INSTALL_DIR}/README-DEPLOY.txt" 2>/dev/null || true
  ok "部署说明已更新（未重置密码）"
else
  info "生成全新 config.yaml 与部署说明 …"
  # Build YAML with properly quoted scalars
  {
    echo "# NexusCard production config (minimal)."
    echo "# Configure Alipay / Epay keys in Admin UI -> Payment (hot reload)."
    echo ""
    echo "server:"
    echo "  listen: $(yaml_quote "127.0.0.1:${LISTEN_PORT}")"
    echo "  public_base_url: $(yaml_quote "${PUBLIC_URL}")"
    echo "  admin_token: $(yaml_quote "${ADMIN_TOKEN}")"
    echo ""
    echo "db:"
    echo "  driver: sqlite"
    echo "  dsn: $(yaml_quote "${INSTALL_DIR}/data/giftcard.db")"
    echo ""
    echo "alipay:"
    echo "  mock_pay: true"
    echo "  bill_subject: $(yaml_quote "Digital Goods")"
    echo "  is_production: false"
    echo "  product: page"
    echo "  timeout_express: $(yaml_quote "30m")"
    echo ""
    echo "epay:"
    echo "  enabled: false"
    echo "  api_url: $(yaml_quote "")"
    echo "  pid: $(yaml_quote "")"
    echo "  key: $(yaml_quote "")"
    echo "  types: $(yaml_quote "alipay,wxpay")"
    echo "  name: $(yaml_quote "Digital Goods")"
    echo ""
    echo "notify_worker:"
    echo "  max_attempts: 12"
    echo "  base_backoff_sec: 5"
    echo "  poll_interval_sec: 2"
    echo ""
    echo "expire_worker:"
    echo "  interval_sec: 15"
    echo "  batch_size: 100"
    echo ""
    echo "security:"
    echo "  sign_skew_sec: 300"
    echo "  https_only: true"
    echo "  ssrf_block_private_ip: true"
    echo ""
    echo "seed_merchant:"
    echo "  app_id: $(yaml_quote "k2-main")"
    echo "  name: $(yaml_quote "K2Board")"
    echo "  api_secret: $(yaml_quote "${API_SECRET}")"
    echo ""
    echo "admin:"
    echo "  username: $(yaml_quote "${ADMIN_USER}")"
    echo "  password: $(yaml_quote "${ADMIN_PASS}")"
    echo "  jwt_secret: $(yaml_quote "${JWT_SECRET}")"
    echo "  site_name: $(yaml_quote "NexusCard")"
    echo ""
    echo "shop:"
    echo "  title: $(yaml_quote "NexusCard Store")"
    echo "  subtitle: $(yaml_quote "美区 Apple ID · 礼品卡 · Netflix / Google · 流量套餐 · 自动发货")"
    echo "  order_ttl_min: 30"
  } > "$CFG"

  # Prefer binary validation once the release binary is on disk (step 3 runs before this)
  if [[ -x "${INSTALL_DIR}/${BIN_NAME}" ]]; then
    if ! "${INSTALL_DIR}/${BIN_NAME}" -check-config -config "$CFG" >/dev/null 2>&1; then
      # v1.1.1+ supports -check-config; older binaries error on unknown flag — fall back
      if "${INSTALL_DIR}/${BIN_NAME}" -h 2>&1 | grep -q 'check-config'; then
        fail "生成的 config.yaml 校验失败，内容预览:"
        nl -ba "$CFG" | head -60 || true
        die "配置写入异常，请重试或到 GitHub 提 issue"
      fi
    fi
  fi
  if ! config_is_valid "$CFG"; then
    fail "生成的 config.yaml 校验失败，内容预览:"
    nl -ba "$CFG" | head -60 || true
    die "配置写入异常，请重试或到 GitHub 提 issue"
  fi

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
Epay notify:   ${PUBLIC_URL}/epay/notify
Epay return:   ${PUBLIC_URL}/epay/return

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
  2) Admin -> Payment: Alipay and/or 彩虹易支付 (epay), disable mock_pay for production
  3) Test shop purchase at /shop/ (cashier shows available pay methods)
  4) Point K2 payment method giftcard to base_url above

Binary:  ${INSTALL_DIR}/${BIN_NAME}
Config:  ${CFG}
Logs:    journalctl -u ${SERVICE_NAME} -f
EOF
  chmod 600 "$CFG" "${INSTALL_DIR}/README-DEPLOY.txt"
  ok "配置已写入 ${CFG}"
  ok "部署说明: ${INSTALL_DIR}/README-DEPLOY.txt"
fi
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

# If we already have a valid cert, write full HTTPS site now (skip later re-issue)
NGINX_SSL=0
if [[ "$REUSE_CERT" -eq 1 || "$WANT_SSL" == "reuse" ]] && has_valid_cert "$DOMAIN"; then
  NGINX_SSL=1
  info "使用已有证书写入 HTTPS 站点配置 …"
else
  info "先写入 HTTP 站点（证书步骤再升级为 HTTPS）…"
fi

write_nginx_site "$DOMAIN" "$NGINX_SSL" "$NGINX_SITE"

if [[ -d /etc/nginx/sites-enabled ]]; then
  ln -sfn "$NGINX_SITE" /etc/nginx/sites-enabled/giftcard-platform
  rm -f /etc/nginx/sites-enabled/default 2>/dev/null || true
fi

info "nginx -t && reload …"
nginx -t
systemctl enable nginx >/dev/null 2>&1 || true
systemctl reload nginx
if [[ "$NGINX_SSL" -eq 1 ]]; then
  ok "Nginx HTTPS: ${DOMAIN} → 127.0.0.1:${LISTEN_PORT}（复用证书）"
else
  ok "Nginx HTTP: ${DOMAIN} → 127.0.0.1:${LISTEN_PORT}"
fi
pause

# ═══════════════════════════════════════════════════════════════════════════
# [7] TLS
# ═══════════════════════════════════════════════════════════════════════════
step 7 "HTTPS / Let's Encrypt（检测后决定是否申请）"

apply_existing_cert() {
  info "检测域名证书: /etc/letsencrypt/live/${DOMAIN}/"
  if ! has_valid_cert "$DOMAIN"; then
    return 1
  fi
  ok "证书已存在且有效，跳过 certbot 申请"
  info "$(cert_expiry_info "$DOMAIN")"
  write_nginx_site "$DOMAIN" 1 "$NGINX_SITE"
  nginx -t && systemctl reload nginx
  sed -i "s|public_base_url:.*|public_base_url: \"https://${DOMAIN}\"|" \
    "${INSTALL_DIR}/configs/config.yaml" 2>/dev/null \
    || sed -i '' "s|public_base_url:.*|public_base_url: \"https://${DOMAIN}\"|" \
    "${INSTALL_DIR}/configs/config.yaml" 2>/dev/null || true
  systemctl restart "$SERVICE_NAME" || true
  ok "HTTPS 复用完成，未向 Let's Encrypt 发起新申请"
  return 0
}

issue_new_cert() {
  info "本机无有效证书（或用户强制重签），准备向 Let's Encrypt 申请 …"
  info "探测 HTTP 健康检查 http://${DOMAIN}/healthz …"
  local reachable=0 i
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

  if ! command -v certbot >/dev/null 2>&1; then
    warn "未安装 certbot；请手动配置 SSL"
    return 1
  fi

  info "运行 certbot --nginx -d ${DOMAIN} …"
  if certbot --nginx -d "$DOMAIN" --non-interactive --agree-tos \
      --register-unsafely-without-email --redirect; then
    ok "Let's Encrypt 证书已安装"
    sed -i "s|public_base_url:.*|public_base_url: \"https://${DOMAIN}\"|" \
      "${INSTALL_DIR}/configs/config.yaml" 2>/dev/null \
      || sed -i '' "s|public_base_url:.*|public_base_url: \"https://${DOMAIN}\"|" \
      "${INSTALL_DIR}/configs/config.yaml" 2>/dev/null || true
    systemctl restart "$SERVICE_NAME" || true
    return 0
  fi
  warn "certbot 失败。请确认 DNS A 记录指向本机，且 80/443 开放，然后执行:"
  warn "  certbot --nginx -d ${DOMAIN} --redirect"
  return 1
}

case "${WANT_SSL,,}" in
  n|no)
    warn "已跳过 TLS。请自行配置证书，并确认 public_base_url。"
    ;;
  reuse)
    apply_existing_cert || {
      warn "复用失败，证书无效或不存在，转为重新申请 …"
      issue_new_cert || true
    }
    ;;
  *)
    # Default Y: prefer reuse when cert already valid (avoids rate limits / duplicate issue)
    if [[ "$FORCE_RENEW" -eq 1 ]]; then
      info "用户要求强制重签，忽略已有证书 …"
      # certbot will renew/reinstall for this domain
      if command -v certbot >/dev/null 2>&1; then
        info "运行 certbot --nginx --force-renewal -d ${DOMAIN} …"
        if certbot --nginx -d "$DOMAIN" --non-interactive --agree-tos \
            --register-unsafely-without-email --redirect --force-renewal; then
          ok "证书已强制重新申请"
          write_nginx_site "$DOMAIN" 1 "$NGINX_SITE" 2>/dev/null || true
          nginx -t && systemctl reload nginx || true
          sed -i "s|public_base_url:.*|public_base_url: \"https://${DOMAIN}\"|" \
            "${INSTALL_DIR}/configs/config.yaml" 2>/dev/null || true
          systemctl restart "$SERVICE_NAME" || true
        else
          warn "强制重签失败；若证书仍有效可继续使用旧证书"
          apply_existing_cert || true
        fi
      else
        issue_new_cert || true
      fi
    elif has_valid_cert "$DOMAIN"; then
      ok "申请前检测: 域名证书已存在且有效，不再重复申请"
      apply_existing_cert || true
    else
      issue_new_cert || true
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
if [[ "$REUSE_CFG" -eq 1 ]]; then
  echo "  Admin:    沿用原配置中的账号（见历史 README-DEPLOY 或自行登录）"
else
  echo "  User:     ${ADMIN_USER} / ${ADMIN_PASS}"
fi
echo "  Secrets:  ${INSTALL_DIR}/README-DEPLOY.txt"
echo "  Notify:   https://${DOMAIN}/alipay/notify"
echo "  Config:   ${INSTALL_DIR}/configs/config.yaml"
echo "  DB:       ${INSTALL_DIR}/data/giftcard.db$([[ $REUSE_DB -eq 1 ]] && echo ' (preserved)')"
echo "  Cert:     $(has_valid_cert "$DOMAIN" && echo "reused/valid $(cert_expiry_info "$DOMAIN")" || echo "see certbot / skip")"
echo "  Logs:     journalctl -u ${SERVICE_NAME} -f"
echo "  =============================================="
echo ""
echo -e "  ${BOLD}接下来请完成:${NC}"
if [[ "$REUSE_CFG" -eq 0 ]]; then
  echo "    1) 登录后台并修改密码"
else
  echo "    1) 使用原管理员账号登录后台"
fi
echo "    2) Admin → Payment: 填写支付宝密钥，生产环境关闭 mock_pay"
echo "    3) 在 /shop/ 试下一单"
echo "    4) K2 giftcard 的 base_url 设为 https://${DOMAIN}"
echo ""
echo -e "  ${DIM}卸载仅停服务，保留数据库与 /etc/letsencrypt 证书${NC}"
echo -e "  ${DIM}重装会复用 DB/配置/有效证书，避免重复申请域名证书${NC}"
echo -e "  ${DIM}指定版本: VERSION=v1.1.1 bash <(curl -fsSL https://raw.githubusercontent.com/${REPO}/main/deploy/install.sh) ${DOMAIN}${NC}"
echo -e "  ${DIM}卸载:     bash <(curl -fsSL https://raw.githubusercontent.com/${REPO}/main/deploy/install.sh) --uninstall${NC}"
echo ""
