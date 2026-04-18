#!/usr/bin/env bash

# Example script to deploy the hertz-track binary to a remote Linux server
# using systemd. This script is for demonstration only and does not contain
# any real host information. Replace placeholders before using in production.

set -euo pipefail

APP_NAME="hertz-track"
BINARY_NAME="${APP_NAME}"

# ---- Remote host configuration (placeholders, edit before use) ----
REMOTE_USER="your-ssh-user"
REMOTE_HOST="your.server.example.com"
REMOTE_PORT="22"

REMOTE_APP_DIR="/opt/hertz-track"
REMOTE_ENV_FILE="/etc/hertz-track.env"
REMOTE_SERVICE_FILE="/etc/systemd/system/hertz-track.service"

# Local paths (relative to repository root `server/`)
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
LOCAL_BINARY_PATH="${REPO_ROOT}/build/${BINARY_NAME}"
LOCAL_ENV_TEMPLATE="${REPO_ROOT}/.env.example"
LOCAL_SERVICE_FILE="${REPO_ROOT}/deploy/systemd/hertz-track.service"

# ---- Build binary ----

echo "[local] Building ${APP_NAME} binary..."
cd "${REPO_ROOT}"
mkdir -p build
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o "${LOCAL_BINARY_PATH}" ./cmd/server

echo "[local] Binary built at ${LOCAL_BINARY_PATH}"

echo "[local] Preparing environment file..."
if [[ ! -f "${REPO_ROOT}/.env" ]]; then
  echo "WARNING: ${REPO_ROOT}/.env not found. Copying from .env.example as a starting point."
  cp "${LOCAL_ENV_TEMPLATE}" "${REPO_ROOT}/.env"
fi

# Use the user-maintained .env as the source for remote EnvironmentFile
LOCAL_ENV_FILE="${REPO_ROOT}/.env"

# ---- Copy files to remote host ----

echo "[remote] Copying files to ${REMOTE_USER}@${REMOTE_HOST}..."
scp -P "${REMOTE_PORT}" "${LOCAL_BINARY_PATH}" "${REMOTE_USER}@${REMOTE_HOST}:/tmp/${BINARY_NAME}"
scp -P "${REMOTE_PORT}" "${LOCAL_ENV_FILE}" "${REMOTE_USER}@${REMOTE_HOST}:/tmp/hertz-track.env"
scp -P "${REMOTE_PORT}" "${LOCAL_SERVICE_FILE}" "${REMOTE_USER}@${REMOTE_HOST}:/tmp/hertz-track.service"

# ---- Remote installation steps ----

echo "[remote] Installing binary and systemd unit..."
ssh -p "${REMOTE_PORT}" "${REMOTE_USER}@${REMOTE_HOST}" bash -s << 'EOF'
set -euo pipefail

APP_NAME="hertz-track"
REMOTE_APP_DIR="/opt/hertz-track"
REMOTE_ENV_FILE="/etc/hertz-track.env"
REMOTE_SERVICE_FILE="/etc/systemd/system/hertz-track.service"

sudo mkdir -p "${REMOTE_APP_DIR}"

# Move binary
sudo mv "/tmp/${APP_NAME}" "${REMOTE_APP_DIR}/${APP_NAME}"
sudo chmod 755 "${REMOTE_APP_DIR}/${APP_NAME}"

# Install env file (edit on remote as needed)
sudo mv "/tmp/hertz-track.env" "${REMOTE_ENV_FILE}"
sudo chmod 640 "${REMOTE_ENV_FILE}"

# Install systemd unit
sudo mv "/tmp/hertz-track.service" "${REMOTE_SERVICE_FILE}"
sudo chmod 644 "${REMOTE_SERVICE_FILE}"

sudo systemctl daemon-reload
sudo systemctl enable --now hertz-track

sudo systemctl status hertz-track --no-pager
EOF

echo "Deployment completed. Verify logs with: ssh -p ${REMOTE_PORT} ${REMOTE_USER}@${REMOTE_HOST} 'sudo journalctl -u hertz-track -f'"
