# Design System Developer Notes

Design Version: `crt-futurism-R1`

## Visual Direction
- Retro CRT console mood with modern control panel grid.
- Green phosphor text, scanline overlay, low-intensity flicker.
- Mechanical indicator LEDs with glow and blink states.
- Steel-style action buttons with metallic gradient and specular highlight.

## Functional Design Rules
- Critical statuses should be visible without opening device card.
- High-contrast monospaced telemetry text should remain readable.
- Motion is subtle and informative, not decorative only.
- Layout remains usable on desktop and mobile.

## Design Tokens
- Core colors and glow tokens are defined in `styles.css` root variables.
- CRT effects are layered through pseudo-elements and background overlays.
