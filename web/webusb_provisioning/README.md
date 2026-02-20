# WebUSB Provisioning Module (R1)

The R1 web panel imports `web/panel/webusb.js` as provisioning runtime.
This folder documents the protocol and keeps module boundary explicit.

## Browser Requirements
- Chromium-based browser with WebUSB support.
- User gesture required for `requestDevice`.

## Provisioning Actions
- `set_config`: writes device identity and modem profile.
- `get_config`: reads stored config and requires password.
