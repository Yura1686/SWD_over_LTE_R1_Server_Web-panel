# Backend AI Context

Version: `r1.2.0-server`

## Fast Summary
- Language: Go.
- Entry point: `cmd/lte-swd-server/main.go`.
- Storage: in-memory + JSON snapshot persistence.
- Fleet cap: 10 devices.

## Main API Groups
- Operator auth: `/api/v1/operator/login`.
- Fleet operations: `/api/v1/devices*`, `/api/v1/commands`, `/api/v1/artifacts`.
- Device runtime: `/api/v1/device/*`.

## Important Behaviors
- Device registration requires `enroll_key`.
- Device command flow is queue-based (`queued -> dispatched -> success/failed`).
- Telemetry/location updates set device online timestamp.
- Security middleware adds per-IP rate limiting and login lockout guard.
