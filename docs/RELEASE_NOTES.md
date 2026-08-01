# RE2-WASM v1.0.0 — RELEASE NOTES

**Release date:** 2026-08-01

re2-wasm v1.0.0 is the first stable release of the open-source browser
port of Resident Evil 2 (1998), built on top of the Go project
OpenBiohazard2 and compiled to WebAssembly.

## Highlights

* **Runs entirely in the browser.** No install, no plugins, no server.
  Drop your original RE2 BIN/CUE onto the page and play.
* **Bring your own game files.** re2-wasm ships zero copyrighted assets;
  the engine parses your original disc image at runtime.
* **60 FPS on a 2020 laptop.** Fixed-step 30 Hz simulation, interpolated
  60 Hz rendering, host-testable pure-Go asset decoders.
* **WebGL 2 today, WebGPU tomorrow.** The renderer is port-driven; a
  WebGPU backend ships as a scaffold and will go live in v1.1.
* **Cross-platform.** Builds on Linux, Windows, and macOS; runs in
  Chromium, Firefox, Safari 16.4+, and Edge.

## What works in v1.0

### Asset pipeline (Phases 2 + 3)

* **BIN/CUE parser** — handles real RE2 discs (multi-track, MODE1/2352,
  MODE1/2048, MODE2/2352).
* **ISO 9660 reader** — PVD + directory records + Joliet-style long
  names; case-insensitive path lookup with `;1` version-suffix elision.
* **DiscFS** — combines CUE + ISO + loose-file mounts; drag-and-drop or
  file-picker driven; auto-pairs `.bin` + `.cue` when both halves land.
* **Streaming reader** — File System Access API wrapper for true random
  access on 700 MB BINs without loading them into RAM.
* **TIM decoder** — 4/8/16/24-bit + CLUT palettes; produces RGBA8 for
  direct GPU upload.
* **TMD decoder** — vertices, normals, FT3/FT4/GT3/GT4 primitives,
  quad-to-tri splitting, texture-page / CLUT tagging.
* **ADT decoder** — room geometry + collision / light metadata preserved
  as raw bytes for the gameplay layer.

### Renderer (Phase 3)

* **WebGL2 backend** — VAOs, dynamic buffers, depth/blend state, viewport
  auto-resize, 3 GLSL ES 3.00 programs (unlit flat, unlit textured,
  sprite).
* **Sprite batcher** — up to 16,384 quads per draw call, automatic
  texture-change flushes, pixel-space coordinates.
* **Texture upload** — RGBA8 → WebGLTexture with linear filtering and
  CLAMP_TO_EDGE; Y-flip handled internally.
* **Camera** — right-handed lookAt + perspective; ViewProjection helper
  for direct shader upload.
* **Per-vertex lighting** — ambient + multi-directional Lambert baking;
  produces RGBA8 colour array ready for upload.
* **Skinned mesh math** — reference CPU skinning (4-bone weighted blend)
  for shaders to mirror.
* **WebGPU scaffold** — `renderer/webgpu` returns `ErrNotImplemented`
  everywhere; will go live in v1.1.

### Audio (Phase 4)

* **VAG SFX decoder** — Sony ADPCM, 28-sample blocks, 4 filter modes,
  predictor state chained across blocks.
* **XA-ADPCM BGM decoder** — stereo + mono, 4 sub-blocks per 128-byte
  block, full Sony filter set.
* **Positional audio** — HRTF PannerNodes for 3D SFX; listener position
  + orientation driven from the camera.
* **Streaming BGM** — chunked decode via a ring buffer so the entire
  cutscene audio never lives in memory at once.
* **Audio unlock** — resumes the AudioContext on first user gesture per
  autoplay policy.

### Input (Phase 4)

* **Keyboard + mouse + gamepad** — all three polled per frame, mapped
  through a configurable Binder to high-level Actions.
* **Default RE2 layout** — WASD/arrows, right-click aim, left-click fire,
  Shift run, Esc pause; gamepad uses face buttons for actions and the
  left stick for movement.
* **Touch controls** — virtual D-pad + 3 action buttons for mobile; hit
  tests done in CSS pixel space; auto-repositions for portrait/landscape.
* **Gamepad rumble** — `vibrationActuator` wrapper; falls back silently
  when unsupported.

### Saves (Phase 4)

* **RE2 save codec** — 30-byte header + payload, slot/character/scenario/
  health/room/position/playtime/checksum, round-trip verified.
* **IndexedDB store** — 20 slots, async I/O, per-origin isolation.
* **In-memory fallback** — used when IndexedDB is unavailable (private
  browsing, file:// origins).
* **Export / import** — every slot can be downloaded as a `.sav` file and
  re-imported on another machine.

### Gameplay (Phase 5)

* **Entity system** — spawn / component map / health component, per-step
  Ticker interface.
* **Player controller** — RE2 tank controls, walk/run/turn speeds, sin/cos
  movement along the facing direction.
* **Enemy AI** — FSM with Idle/Patrol/Alert/Chase/Attack/Stagger/Dead
  states, distance-based transitions, patrol-waypoint bounce.
* **Menu state machine** — Pause / Inventory / Map / File / Options
  stack with disabled-item skipping, confirm-on-select, wrap-around
  navigation.
* **Cutscene player** — timeline of cues with start times; fires OnCue
  callbacks; supports camera change, dialogue, subtitle, actor move,
  actor animate, music change, SFX, end.
* **Door transitions** — fade-out → load → fade-in state machine;
  OnLoad callback for the engine to swap rooms; error path leaves the
  screen black so the engine can show an error toast.
* **Room script VM** — byte-coded opcode interpreter (set/get flag,
  spawn item, play cutscene, warp player, conditional branch, jump,
  halt); flags persist across program loads.

### Polish (Phase 6)

* **Performance tracker** — per-section EMA timer (frame/sim/render/
  audio/input/assets); zero-allocation steady state.
* **i18n** — JSON-driven bundles with EN + JA defaults; fallback chain
  (requested → English → key itself); `Getf` for formatted strings.
* **Browser compat detection** — `web/compat` probes WebGL2 / WebGPU /
  Web Audio / Gamepad / IndexedDB / Pointer Lock / Fullscreen / File
  System Access / Vibration / WASM streaming; publishes results to
  `window.re2compat`.
* **Accessibility** — high-contrast mode, reduced-motion support,
  full keyboard control, configurable bindings, subtitles. See
  [ACCESSIBILITY.md](ACCESSIBILITY.md).

## Quality

* **154 unit + integration tests** across 29 packages.
* **Zero `go vet` warnings** on both host (`linux/amd64`) and
  `GOOS=js GOARCH=wasm` builds.
* **CI on Linux, Windows, and macOS** — fmt check, host vet, js/wasm
  vet, host tests with `-race`, WASM build, smoke-serve probe, artifact
  upload.
* **WASM binary size: 3.1 MB** (stripped, trimpath'd, reproducible).

## Known limitations

* **Gamepad UI navigation.** Gamepad drives the player but not the menus
  yet. Use keyboard for menus.
* **Skinned mesh on GPU.** The math is implemented in `renderer/skin`
  (reference CPU path); the vertex-shader path lands in v1.1.
* **ADT collision data.** Decoded but not yet interpreted; the player can
  walk through walls in v1.0. Phase 5's door transitions still work
  because they're trigger-based, not collision-based.
* **Safari WebGPU.** Safari 16.4+ has WebGL2 but WebGPU is still behind
  a flag. The WebGL2 path is the default; WebGPU is opt-in.

## Upgrade path

v1.0 is the first tagged release. There is no upgrade path; install fresh.

## Credits

* [OpenBiohazard2] — the Go reverse-engineering project this port is
  based on.
* [Go's `wasm_exec.js`] — the runtime support file shipped with the Go
  toolchain.
* Everyone who documented the original RE2 file formats.

## License

MIT. See [LICENSE](../LICENSE). Resident Evil 2 is a trademark of Capcom
Co., Ltd.; this project is not affiliated with or endorsed by Capcom.
