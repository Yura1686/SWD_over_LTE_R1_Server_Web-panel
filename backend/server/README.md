# LTE_SWD Backend (R1)

Version: `r1.2.0-server`

## Purpose
The backend receives device telemetry/coordinates, manages SWD command queue, and serves operator web panel.

## Run
```bash
cd backend/server
GOCACHE=/tmp/go-cache go test ./...
GOCACHE=/tmp/go-cache go build -o lte-swd-server ./cmd/lte-swd-server
./lte-swd-server
```

## Environment
- `HTTP_ADDR` default `:8080`
- `HTTPS_ADDR` optional TLS bind address (example `:8443`)
- `TLS_CERT_FILE` TLS certificate path when `HTTPS_ADDR` is set
- `TLS_KEY_FILE` TLS private key path when `HTTPS_ADDR` is set
- `OPERATOR_PASSWORD` default `lte_swd_admin`
- `DEVICE_ENROLL_KEY` default `r1-enroll-key`
- `DATA_FILE` default `data/state.json`
- `STATIC_DIR` default `../../web/panel`
- `FLEET_LIMIT` default `10`
- `OPERATOR_TOKEN_TTL` default `12h`
- `DEVICE_OFFLINE_AFTER` default `90s`
- `MAX_JSON_BYTES` default `65536`
- `MAX_ARTIFACT_BYTES` default `12582912`
- `API_RATE_PER_MINUTE` default `180`
- `LOGIN_RATE_PER_MINUTE` default `20`
- `LOGIN_BURST` default `5`
- `TRUST_PROXY_HEADERS` default `false`

## Internet-Facing Security Controls
- Request body size limits for JSON/artifact API.
- Per-IP API rate limiting.
- Login brute-force guard with temporary block.
- Strict security headers (CSP, HSTS on HTTPS, frame deny, etc.).

## Main API Groups
- Operator auth and fleet control: `/api/v1/operator/*`, `/api/v1/devices*`, `/api/v1/commands`, `/api/v1/artifacts`.
- Device runtime API: `/api/v1/device/*`.

See `shared/protocol/device_api.md` for contract examples.

## Documentation Levels
- Developer docs: `backend/server/docs/developer.md`
- AI docs: `backend/server/docs/ai.md`
