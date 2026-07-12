#!/usr/bin/env bash
# Build common Linux release binaries for NexusCard.
# Output: dist/nexuscard-linux-{amd64,arm64} + SHA256SUMS.txt
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

VERSION="${VERSION:-dev}"
# Strip leading v if present
VERSION="${VERSION#v}"
BIN_BASE="nexuscard"
OUT_DIR="${OUT_DIR:-${ROOT}/dist}"
LDFLAGS="-s -w -X main.version=${VERSION}"

mkdir -p "$OUT_DIR"
rm -f "${OUT_DIR}/${BIN_BASE}-linux-"* "${OUT_DIR}/SHA256SUMS.txt" 2>/dev/null || true

# Common Linux targets used by cloud VPS / ARM boards
TARGETS=(
  "linux/amd64"
  "linux/arm64"
)

echo "==> Building NexusCard v${VERSION}"
echo "    output: ${OUT_DIR}"
echo ""

for pair in "${TARGETS[@]}"; do
  GOOS="${pair%/*}"
  GOARCH="${pair#*/}"
  name="${BIN_BASE}-${GOOS}-${GOARCH}"
  out="${OUT_DIR}/${name}"
  echo "  [build] ${name} ..."
  CGO_ENABLED=0 GOOS="$GOOS" GOARCH="$GOARCH" \
    go build -trimpath -ldflags="$LDFLAGS" -o "$out" ./cmd/server
  chmod +x "$out"
  size="$(wc -c <"$out" | tr -d ' ')"
  echo "          $(du -h "$out" | awk '{print $1}') (${size} bytes)"
done

echo ""
echo "==> Checksums"
(
  cd "$OUT_DIR"
  # Prefer sha256sum (Linux CI); fall back to shasum (macOS)
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum ${BIN_BASE}-linux-* > SHA256SUMS.txt
  else
    shasum -a 256 ${BIN_BASE}-linux-* > SHA256SUMS.txt
  fi
  cat SHA256SUMS.txt
)

echo ""
echo "==> Done. Artifacts in ${OUT_DIR}:"
ls -lh "$OUT_DIR"
