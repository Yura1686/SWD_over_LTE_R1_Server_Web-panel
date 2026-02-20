#!/usr/bin/env bash
set -euo pipefail

if [[ ${EUID:-$(id -u)} -ne 0 ]]; then
  echo "Run as root: sudo $0 --domain <fqdn> [--email <mail>]"
  exit 1
fi

DOMAIN=""
EMAIL=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --domain)
      DOMAIN="${2:-}"
      shift 2
      ;;
    --email)
      EMAIL="${2:-}"
      shift 2
      ;;
    *)
      echo "Unknown argument: $1"
      exit 1
      ;;
  esac
done

if [[ -z "$DOMAIN" ]]; then
  echo "Missing --domain <fqdn>"
  exit 1
fi

apt-get update
apt-get install -y caddy ufw fail2ban

mkdir -p /etc/caddy

if [[ -n "$EMAIL" ]]; then
  cat >/etc/caddy/Caddyfile <<EOF
{
  email $EMAIL
}

$DOMAIN {
  encode zstd gzip
  header {
    Strict-Transport-Security "max-age=31536000; includeSubDomains"
    X-Frame-Options "DENY"
    X-Content-Type-Options "nosniff"
    Referrer-Policy "no-referrer"
  }
  reverse_proxy 127.0.0.1:8080
}
EOF
else
  cat >/etc/caddy/Caddyfile <<EOF
$DOMAIN {
  encode zstd gzip
  header {
    Strict-Transport-Security "max-age=31536000; includeSubDomains"
    X-Frame-Options "DENY"
    X-Content-Type-Options "nosniff"
    Referrer-Policy "no-referrer"
  }
  reverse_proxy 127.0.0.1:8080
}
EOF
fi

systemctl enable --now caddy
systemctl restart caddy

ufw --force reset
ufw default deny incoming
ufw default allow outgoing
ufw allow 22/tcp
ufw allow 80/tcp
ufw allow 443/tcp
ufw --force enable

systemctl enable --now fail2ban

echo "Public edge hardening applied."
echo "Check: https://$DOMAIN"
