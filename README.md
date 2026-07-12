# NexusCard

Digital goods storefront and payment cashier platform:

- Shop: US Apple ID, App Store gift cards, Google accounts, Netflix, streaming, data plans
- Auto-delivery of card secrets after payment (simulated credentials by default)
- Alipay official payment (page / wap)
- K2Board `giftcard` gateway integration (signed merchant API + notify)

**Module:** `github.com/HenZenKuriRIP/NexusCard`  
**Repository:** https://github.com/HenZenKuriRIP/NexusCard

---

## Features

| Area | Description |
|------|-------------|
| **Shop** `/shop/` | Category browse, checkout, cashier, secret delivery |
| **Admin** `/admin/` | Login, products, card pool, orders, merchants, **payment settings** |
| **Alipay** | Real pay + optional mock pay; credentials configurable in admin UI |
| **K2** | Merchant API compatible with K2Board `giftcard` gateway |
| **Security** | IP rate limits on shop / pay / login / merchant APIs |

---

## Quick start (local)

```bash
git clone https://github.com/HenZenKuriRIP/NexusCard.git
cd NexusCard
go test ./... -count=1
go run ./cmd/server -config configs/config.local.yaml
```

| URL | Notes |
|-----|--------|
| http://127.0.0.1:8088/shop/ | Customer storefront |
| http://127.0.0.1:8088/admin/ | Admin (default `admin` / `admin123`) |
| http://127.0.0.1:8088/healthz | Health check |

### Shop flow

1. Admin → **Payment** — enable mock pay for local tests, or enter Alipay keys
2. Shop → buy a product (e.g. US Apple ID)
3. Pay (mock or Alipay) → receive **SIM** credential text (auto-generated) or pool codes

Products default to `auto_generate=true` so the shop is sellable without importing a card pool.

### Config

Keep YAML minimal (server, DB, seed merchant, admin).  
**Configure Alipay in Admin → Payment** (stored in DB, hot-reloaded).

Bill subject defaults to neutral **`Digital Goods`** (no plan/VPN names).

---

## One-click install (Linux + domain)

On a Debian/Ubuntu (or RHEL-like) server, with DNS **A record** pointing to the host.

Pre-built **Linux amd64 / arm64** binaries are published on [GitHub Releases](https://github.com/HenZenKuriRIP/NexusCard/releases). The install script **does not** install Go or compile from source — it detects the host arch and downloads the matching binary.

```bash
# Recommended: pipe from main
curl -fsSL https://raw.githubusercontent.com/HenZenKuriRIP/NexusCard/main/deploy/install.sh \
  | sudo bash -s -- pay.example.com

# Pin a release tag
curl -fsSL https://raw.githubusercontent.com/HenZenKuriRIP/NexusCard/main/deploy/install.sh \
  | sudo VERSION=v1.0.0 bash -s -- pay.example.com

# From a local clone
sudo bash deploy/install.sh pay.example.com
```

The script walks through clear steps (with a short pause between each so the log is readable):

1. Preflight — root, OS, arch (`linux/amd64` or `linux/arm64`), DNS hint  
2. System packages — `curl`, `nginx`, optional `certbot` (**no Go / git / compiler**)  
3. Download — fetch binary + SHA256 from GitHub Releases  
4. Configuration — production YAML + random secrets  
5. systemd — service user and unit  
6. Nginx — reverse proxy to `127.0.0.1:8088`  
7. HTTPS — optional Let's Encrypt via certbot  
8. Final check — service + local healthz  

After install:

1. Open `https://YOUR_DOMAIN/admin/` (credentials in `/opt/giftcard-platform/README-DEPLOY.txt`)
2. **Payment** — paste Alipay keys, disable mock for production
3. Open `https://YOUR_DOMAIN/shop/` to verify

Uninstall:

```bash
sudo bash deploy/install.sh --uninstall
# or: curl -fsSL .../install.sh | sudo bash -s -- --uninstall
```

### Release binaries

| Asset | Platform |
|-------|----------|
| `nexuscard-linux-amd64` | x86_64 VPS |
| `nexuscard-linux-arm64` | aarch64 / ARM64 |
| `SHA256SUMS.txt` | checksums |

Tag a version to trigger CI (`.github/workflows/release.yml`):

```bash
git tag v1.0.0 && git push origin v1.0.0
# or build locally: VERSION=1.0.0 bash scripts/build-release.sh
```

---

## K2Board integration

Add payment method `giftcard` on the panel:

```json
{
  "base_url": "https://pay.example.com",
  "app_id": "k2-main",
  "api_secret": "<from README-DEPLOY.txt or seed_merchant>",
  "timeout_sec": 20,
  "product_name_template": "Digital Goods",
  "sign_version": "v1"
}
```

Panel `site_url` must be publicly reachable for platform → K2 notify.  
Alipay credentials stay **only on NexusCard**, not on K2.

Shop SKUs and K2 plans are **independent**. K2 checkout creates a dynamic payment order (`source=k2`); no 1:1 product mapping is required.

---

## Rate limits (default)

| Scope | Limit |
|-------|--------|
| Shop read | 120 / min / IP |
| Checkout | 20 / min / IP |
| Cashier / pay | 30 / min / IP |
| Admin login | 15 / min / IP |
| Merchant API (K2) | 120 / min / IP |
| Alipay notify | 300 / min / IP |

---

## Development

```bash
go test ./... -count=1
go build -o nexuscard ./cmd/server
```

### Layout

```
cmd/server/
deploy/install.sh
configs/
internal/
  alipay/ httpserver/ service/ models/ sign/ config/ database/
```

---

## License

See repository license file if present. Use in compliance with local laws and payment provider policies.
