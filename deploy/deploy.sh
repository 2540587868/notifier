#!/bin/bash
set -euo pipefail

DEPLOY_DIR="/opt/notifier"
NOTIFIER_BIN="${DEPLOY_DIR}/notifier/notifier"
SRC_DIR="${DEPLOY_DIR}/src"
REPO_URL="https://github.com/ysqss/notifier.git"
BRANCH="${1:-main}"

echo "==> Deploying Notifier (branch: ${BRANCH})"

if [ ! -d "${SRC_DIR}/.git" ]; then
    echo "==> Cloning repository..."
    git clone -b "${BRANCH}" "${REPO_URL}" "${SRC_DIR}"
else
    echo "==> Updating repository..."
    cd "${SRC_DIR}"
    git fetch origin
    git reset --hard "origin/${BRANCH}"
fi

echo "==> Building Notifier..."
cd "${SRC_DIR}"
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -trimpath \
    -ldflags "-s -w" \
    -o "${NOTIFIER_BIN}.new" ./cmd/notifier/

echo "==> Stopping Notifier service..."
systemctl stop notifier || true

echo "==> Replacing binary..."
mv "${NOTIFIER_BIN}.new" "${NOTIFIER_BIN}"
chmod +x "${NOTIFIER_BIN}"

echo "==> Starting Notifier service..."
systemctl start notifier

echo "==> Waiting for health check..."
for i in $(seq 1 15); do
    if curl -sf http://127.0.0.1:8081/health > /dev/null 2>&1; then
        echo "==> Notifier is healthy!"
        systemctl status notifier --no-pager
        exit 0
    fi
    sleep 2
done

echo "==> Health check failed, showing logs..."
journalctl -u notifier --no-pager -n 20
exit 1
