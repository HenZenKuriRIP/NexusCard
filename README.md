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

On a Debian/Ubuntu (or similar) server, with DNS **A record** pointing to the host:

```bash
# From the repository root
sudo bash deploy/install.sh pay.example.com
```

The script will:

1. Install dependencies (nginx, git, go toolchain if missing)
2. Build the binary into `/opt/giftcard-platform` (NexusCard)
3. Write production config + random secrets
4. Install **systemd** service
5. Configure **Nginx** reverse proxy
6. Optionally obtain **Let's Encrypt** certificate via certbot

After install:

1. Open `https://YOUR_DOMAIN/admin/` (credentials in `/opt/giftcard-platform/README-DEPLOY.txt`)
2. **Payment** — paste Alipay keys, disable mock for production
3. Open `https://YOUR_DOMAIN/shop/` to verify

Uninstall:

```bash
sudo bash deploy/install.sh --uninstall
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
