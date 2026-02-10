#!/usr/bin/env bash
set -euo pipefail

PROJECT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

SSH_HOST="${SSH_HOST:-ssh.ark.su}"
REMOTE_WEB_DIR="${REMOTE_WEB_DIR:-/var/www/origin.ark.su}"

CLIENT_DIR="${CLIENT_DIR:-$PROJECT_DIR/web_new}"
DIST_DIR="${DIST_DIR:-$CLIENT_DIR/dist}"
TARBALL="${TARBALL:-/tmp/origin_web_dist.tgz}"

echo "[1/4] Building client"
(
  cd "$CLIENT_DIR"
  npm run proto
  npx vite build
)

echo "[2/4] Packaging dist"
if tar --help 2>/dev/null | grep -q -- '--disable-copyfile'; then
  COPYFILE_DISABLE=1 COPY_EXTENDED_ATTRIBUTES_DISABLE=1 tar \
    --disable-copyfile \
    --no-xattrs \
    --format=ustar \
    -C "$DIST_DIR" -czf "$TARBALL" .
elif tar --help 2>/dev/null | grep -q -- '--no-mac-metadata'; then
  COPYFILE_DISABLE=1 COPY_EXTENDED_ATTRIBUTES_DISABLE=1 tar \
    --no-mac-metadata \
    --no-xattrs \
    --format=ustar \
    -C "$DIST_DIR" -czf "$TARBALL" .
else
  COPYFILE_DISABLE=1 COPY_EXTENDED_ATTRIBUTES_DISABLE=1 tar \
    --format=ustar \
    -C "$DIST_DIR" -czf "$TARBALL" .
fi

echo "[3/4] Uploading to $SSH_HOST:$REMOTE_WEB_DIR"
ssh "$SSH_HOST" "set -euo pipefail; mkdir -p '$REMOTE_WEB_DIR'; rm -rf '$REMOTE_WEB_DIR'/*"
cat "$TARBALL" | ssh "$SSH_HOST" "set -euo pipefail; tar xzf - -C '$REMOTE_WEB_DIR'; find '$REMOTE_WEB_DIR' -name '._*' -type f -delete"

echo "[4/4] Reload nginx and verify files"
ssh "$SSH_HOST" "set -euo pipefail; (sudo systemctl reload nginx || systemctl reload nginx || true); ls -la '$REMOTE_WEB_DIR' | head -n 20"

echo "Done."
