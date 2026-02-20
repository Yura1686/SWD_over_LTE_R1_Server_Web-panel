#!/usr/bin/env bash
set -euo pipefail

DOMAIN="${1:-178.165.38.105.nip.io}"
EMAIL="${2:-admin@178.165.38.105.nip.io}"
LEGO_PATH="${LEGO_PATH:-/etc/lego}"

echo "[cert-renew] renewing certificate for ${DOMAIN}"
systemctl stop nginx
if lego --path "${LEGO_PATH}" --email "${EMAIL}" --accept-tos --domains "${DOMAIN}" --tls renew --days 30; then
  echo "[cert-renew] renewal completed"
else
  echo "[cert-renew] renewal skipped or failed, attempting one-time run"
  lego --path "${LEGO_PATH}" --email "${EMAIL}" --accept-tos --domains "${DOMAIN}" --tls run
fi
systemctl start nginx
systemctl reload nginx
echo "[cert-renew] nginx reloaded"
