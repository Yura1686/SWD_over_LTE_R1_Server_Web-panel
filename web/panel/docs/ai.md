# Web Panel AI Context

Version: `R1`

## Fast Summary
- Static app, no build pipeline required.
- Main state lives in `web/panel/app.js`.
- API client wrapper is in `web/panel/api.js`.

## Runtime Flow
1. Login with operator password.
2. Load capabilities and fleet list.
3. Refresh devices and command history every 5 seconds.
4. Submit SWD commands and artifact uploads.
5. Use WebUSB provisioning for one-time setup (`device_id`, APN/operator, optional SIM PIN, `server_url`, `enroll_key`).
