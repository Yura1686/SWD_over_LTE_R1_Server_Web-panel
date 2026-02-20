# LTE_SWD Web Panel (R1)

Version: `R1`  
Design Version: `crt-futurism-R1`

## Purpose
The panel is the operator GUI for fleet status, map, command dispatch, artifact upload, and WebUSB provisioning.

## Runtime
- Static files are served by backend (`STATIC_DIR`).
- No local build toolchain is required in R1.
- WebUSB provisioning writes `device_id`, APN/operator, optional SIM PIN, `server_url`, and `enroll_key`.

## Documentation Levels
- Developer docs: `web/panel/docs/developer.md`
- AI docs: `web/panel/docs/ai.md`
- Design developer docs: `web/panel/docs/design/developer.md`
- Design AI docs: `web/panel/docs/design/ai.md`
