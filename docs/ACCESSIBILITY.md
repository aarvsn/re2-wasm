# ACCESSIBILITY

re2-wasm targets WCAG 2.1 Level AA. This document records the audit
performed at the end of Phase 6, the gaps that remain, and the
user-facing controls that ship with v1.0.

## What we ship

### Visual

* **High-contrast mode** — toggle with `H`. Swaps the dark theme for a
  black-on-white palette and disables the loading-screen gradient.
* **Text scaling** — the menu / HUD text scales with the browser's
  default font size. Set `font-size: 18px` in your browser settings and
  every UI element grows proportionally.
* **Reduced motion** — when the OS reports `prefers-reduced-motion:
  reduce`, the loading-screen pulse and the menu fade are disabled.
* **Colour-blind safe palette** — the accent red (`#c43c2a`) is
  distinguishable from green and blue for all three common colour-blindness
  types (protanopia, deuteranopia, tritanopia).

### Auditory

* **Subtitles** — every cutscene line that ships through the audio layer
  is mirrored as a DOM-text subtitle (Phase 5 cutscene cues carry a
  `CueKindSubtitle`). Subtitles can be toggled in the Options menu.
* **Volume controls** — master, BGM, and SFX sliders in Options. Each is
  independent; muting master does not silence UI beeps.
* **Visual cue for SFX** — when audio is muted, on-screen flashes appear
  for gunshots, door opens, and item pickups. Off by default; enable in
  Options.

### Motor

* **Full keyboard control** — every UI element reachable with Tab,
  activated with Enter. Focus ring is visible (2px accent border).
* **Configurable bindings** — every input action can be rebound from the
  Options → Controls screen. Mouse, keyboard, and gamepad each have their
  own binding columns.
* **No quick-time events** — RE2's original QTEs are re-implemented as
  hold-to-confirm so single-frame inputs are never required.
* **Pause anywhere** — `Esc` pauses the simulation regardless of what
  screen is active. The menu renders on top; the simulation clock stops.

### Cognitive

* **Save anytime** — the original game's typewriter-limited saves are
  preserved as the "canonical" mode, but an Options toggle enables
  quick-save from the pause menu (Ctrl+S).
* **Objective reminder** — pressing `O` shows the current objective as a
  non-modal toast.

## Known gaps (v1.0)

These items are tracked for v1.1:

* **Screen-reader support for the canvas.** The canvas is currently a
  black box to assistive tech; we need an off-screen live region that
  narrates scene changes ("Leon entered the RPD main hall").
* **Gamepad UI navigation.** Gamepad input drives the player but not the
  menu yet; players must use keyboard for menus.
* **Custom colour-blind filters.** The palette is safe but we do not ship
  per-type filter modes (deuteranopia mode, etc.).
* **Text-to-speech for subtitles.** Subtitles are visual-only; we do not
  yet bridge to the OS speech synthesiser.

## Audit methodology

The audit was performed against the WCAG 2.1 AA checklist using:

1. **Automated scan** — axe-core 4.9 run against the loading screen and
   every menu screen. Zero violations.
2. **Manual keyboard-only test** — completed the title screen →
   inventory → map → pause → quit flow without touching the mouse.
3. **Reduced-motion test** — macOS "Reduce motion" enabled; every
   animation either stopped or shortened.
4. **High-contrast test** — Windows High Contrast mode + Firefox
   `browser.display.document_color_use = 2`. All text readable.
5. **Colour-blind simulation** — Sim Daltonism (all three types) showed
   the accent colour is distinguishable from the background and from
   every state indicator (ok/warn/err badges).

## Reporting accessibility bugs

Open an issue with the `a11y` label. Include:

* The WCAG criterion you believe is violated (e.g. "1.4.3 Contrast
  (Minimum)").
* A screenshot or screen recording.
* The browser + OS + assistive tech you used.

We aim to respond to a11y issues within 2 business days.
