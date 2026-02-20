# Backend Developer Notes

Version: `r1.2.0-server`

## Purpose
The backend serves operator API, device runtime API, command queue, artifact storage, and static web panel delivery.

## Core Packages
- `internal/httpapi`: routing and HTTP handlers.
- `internal/service`: business rules.
- `internal/store`: state store and persistence.
- `internal/auth`: operator token management.
- `internal/model`: domain entities.

## Runtime Constraints
- Fleet hard limit defaults to 10 devices.
- Local JSON state file is used in R1 (`data/state.json`).
- Operator authentication uses static password and short-lived token.
- Request-size limits and per-IP rate limits are enabled by default.
- Login endpoint has brute-force guard with temporary lockout.

## Extension Targets
- Replace JSON store with PostgreSQL implementation.
- Replace device polling bridge with MQTT transport.
- Add RBAC and multi-tenant authorization model.
