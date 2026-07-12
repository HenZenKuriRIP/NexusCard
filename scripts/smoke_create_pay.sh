#!/usr/bin/env bash
# Smoke: create a signed merchant order and print cashier URL.
set -euo pipefail
BASE="${BASE:-http://127.0.0.1:8088}"
APP_ID="${APP_ID:-k2-main}"
SECRET="${SECRET:-test_secret}"
OUT="SMOKE$(date +%s)"
NOTIFY="${NOTIFY_URL:-http://127.0.0.1:9999/api/v1/payment/notify/giftcard}"
RETURN="${RETURN_URL:-http://127.0.0.1:3000/#/user/order-result?trade_no=$OUT}"
EXPIRE=$(($(date +%s) + 1800))

BODY=$(printf '%s' "{\"out_trade_no\":\"$OUT\",\"amount\":100,\"currency\":\"CNY\",\"subject\":\"Digital Goods\",\"notify_url\":\"$NOTIFY\",\"return_url\":\"$RETURN\",\"expire_at\":$EXPIRE}")
TS=$(date +%s)
NONCE=$(openssl rand -hex 8)
BODY_SHA=$(printf '%s' "$BODY" | openssl dgst -sha256 -hex | awk '{print $2}')
RAW="app_id=${APP_ID}&body_sha256=${BODY_SHA}&nonce=${NONCE}&timestamp=${TS}${SECRET}"
if command -v md5 >/dev/null 2>&1; then
  SIG=$(printf '%s' "$RAW" | md5 -q)
else
  SIG=$(printf '%s' "$RAW" | md5sum | awk '{print $1}')
fi

echo "POST $BASE/api/v1/orders out_trade_no=$OUT"
curl -sS -X POST "$BASE/api/v1/orders" \
  -H "Content-Type: application/json" \
  -H "X-App-Id: $APP_ID" \
  -H "X-Timestamp: $TS" \
  -H "X-Nonce: $NONCE" \
  -H "X-Signature: $SIG" \
  -d "$BODY"
echo
echo "Open cashier_url from JSON, or mock-pay with cashier_token."
