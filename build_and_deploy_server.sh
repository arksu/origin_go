#!/usr/bin/env bash
set -euo pipefail

PROJECT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

SSH_HOST="${SSH_HOST:-ssh.ark.su}"
REMOTE_APP_DIR="${REMOTE_APP_DIR:-/opt/origin_go/current}"
REMOTE_BINARY="${REMOTE_BINARY:-$REMOTE_APP_DIR/gameserver}"
REMOTE_SERVICE="${REMOTE_SERVICE:-origin-gameserver}"

GOOS_TARGET="${GOOS_TARGET:-linux}"
GOARCH_TARGET="${GOARCH_TARGET:-amd64}"
GOCACHE_DIR="${GOCACHE_DIR:-/tmp/go-build}"

LOCAL_BINARY="${LOCAL_BINARY:-$PROJECT_DIR/build/gameserver-${GOOS_TARGET}-${GOARCH_TARGET}}"

echo "[1/4] Building server binary"
mkdir -p "$(dirname "$LOCAL_BINARY")"
(
  cd "$PROJECT_DIR"
  GOCACHE="$GOCACHE_DIR" GOOS="$GOOS_TARGET" GOARCH="$GOARCH_TARGET" go build -o "$LOCAL_BINARY" ./cmd/gameserver
)

echo "[2/4] Uploading binary to $SSH_HOST:$REMOTE_BINARY"
cat "$LOCAL_BINARY" | ssh "$SSH_HOST" "set -euo pipefail; \
  mkdir -p '$REMOTE_APP_DIR'; \
  if [ -f '$REMOTE_BINARY' ]; then cp '$REMOTE_BINARY' '$REMOTE_BINARY.bak.'\"\$(date +%Y%m%d%H%M%S)\"; fi; \
  cat > '$REMOTE_BINARY.new'; \
  install -m 755 '$REMOTE_BINARY.new' '$REMOTE_BINARY'; \
  rm -f '$REMOTE_BINARY.new'"

echo "[3/4] Restarting service $REMOTE_SERVICE"
ssh "$SSH_HOST" "set -euo pipefail; \
  (sudo systemctl restart '$REMOTE_SERVICE' || systemctl restart '$REMOTE_SERVICE'); \
  (sudo systemctl is-active '$REMOTE_SERVICE' || systemctl is-active '$REMOTE_SERVICE')"

echo "[4/4] Tail recent service logs"
ssh "$SSH_HOST" "journalctl -u '$REMOTE_SERVICE' --since '2 min ago' --no-pager | tail -n 80 || true"

echo "Done."
