# Web Panel Developer Notes

Version: `R1`

## Purpose
The panel provides operator login, fleet overview, device details, command launch, firmware artifact upload, map view, and WebUSB provisioning.

## Implementation
- Vanilla JS modules (`app.js`, `api.js`, `map.js`, `webusb.js`).
- Static assets served by backend.
- Leaflet map with OpenStreetMap tiles.

## Key UI Sections
- Login panel.
- Fleet list and map.
- Device card and command history.
- SWD command form.
- Artifact upload form.
- WebUSB provisioning forms.
- WebUSB provisioning now includes `server_url` and `enroll_key`.
