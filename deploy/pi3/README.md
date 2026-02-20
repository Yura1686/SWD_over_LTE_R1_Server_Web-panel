# Pi 3 B+ Deployment (Ubuntu Server)

## 1. Copy Project
Copy repository to `/opt/lte_swd` on Raspberry Pi 3 B+.

## 2. Build Backend
```bash
cd /opt/lte_swd/backend/server
GOCACHE=/tmp/go-cache go test ./...
GOCACHE=/tmp/go-cache go build -o lte-swd-server ./cmd/lte-swd-server
```

## 3. Configure Environment
```bash
cp /opt/lte_swd/deploy/pi3/env.example /opt/lte_swd/deploy/pi3/env
nano /opt/lte_swd/deploy/pi3/env
```

Recommended for internet-facing mode:
- keep backend bound to localhost: `HTTP_ADDR=127.0.0.1:8080`
- use a strong operator password and random enroll key
- keep `TRUST_PROXY_HEADERS=true` only behind reverse proxy

## 4. Start Manually
```bash
chmod +x /opt/lte_swd/deploy/pi3/run_server.sh
/opt/lte_swd/deploy/pi3/run_server.sh
```

## 5. Run as systemd Service
```bash
sudo cp /opt/lte_swd/deploy/pi3/lte-swd.service /etc/systemd/system/lte-swd.service
sudo systemctl daemon-reload
sudo systemctl enable --now lte-swd.service
sudo systemctl status lte-swd.service
```

## 6. Open Panel
LAN mode: `http://<pi-ip>:8080`

## 7. Public Internet Mode (TLS + Firewall)
1. Point DNS `A`/`AAAA` record to Raspberry Pi public IP.
2. Run hardening script:
```bash
sudo chmod +x /opt/lte_swd/deploy/pi3/public_hardening.sh
sudo /opt/lte_swd/deploy/pi3/public_hardening.sh --domain your.domain.example --email ops@your.domain.example
```
3. Verify:
```bash
systemctl status caddy
sudo ufw status verbose
```
4. Open panel over HTTPS:
- `https://your.domain.example`

## 8. Public HTTPS Without Own Domain (NIP)
If you do not have your own domain yet, use `nip.io` host that maps to your public IP.

Example public host:
- `https://178.165.38.105.nip.io`

Issue trusted cert over port `443` (TLS-ALPN challenge):
```bash
sudo systemctl stop nginx
sudo lego --path /etc/lego --email "admin@178.165.38.105.nip.io" --accept-tos --domains "178.165.38.105.nip.io" --tls run
sudo systemctl start nginx
```

Then point nginx cert paths to:
- `/etc/lego/certificates/178.165.38.105.nip.io.crt`
- `/etc/lego/certificates/178.165.38.105.nip.io.key`

Optional auto-renew:
```bash
sudo chmod +x /opt/lte_swd/deploy/pi3/renew_lego_tls.sh
sudo cp /opt/lte_swd/deploy/pi3/lte-swd-cert-renew.service /etc/systemd/system/
sudo cp /opt/lte_swd/deploy/pi3/lte-swd-cert-renew.timer /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now lte-swd-cert-renew.timer
```
