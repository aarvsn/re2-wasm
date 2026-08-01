# ARCHITECTURE

This document explains *why* re2-wasm is structured the way it is. For
*what* each package does, see the package-level doc comments.

## Design principles

1. **One concern per package.** Each directory under the repo root owns
   exactly one subsystem. Cross-package imports flow downward through the
   dependency graph: `cmd/re2-wasm` → `wasm` → `engine` → ports
   (`renderer`, `audio`, `input`, `assets`, `filesystem`, `saves`, `ui`).

2. **syscall/js stays at the edges.** Only five packages import `syscall/js`
   directly: `renderer/webgl`, `audio`, `input`, `saves` (the IDB file
   only), `ui`, and `wasm`. Everything else is host-testable, which is why
   `go test ./...` runs without a browser.

3. **The engine loop is port-driven.** `engine.Engine` does not know what a
   "canvas" or "AudioContext" is. It holds a `Ports` struct whose fields
   are interfaces; the WASM runtime supplies concrete implementations. A
   host test can swap in fakes and exercise the entire loop.

4. **Fixed-step simulation, variable-step rendering.** The original RE2
   runs at 30 Hz. We keep that cadence (`engine.DefaultSimStep`) and
   decouple it from the browser's rAF (~60 Hz) via a fixed-step accumulator
   (`engine/clock.FrameTimer`). This matches what OpenBiohazard2 already
   does, so reverse-engineered frame timings carry over unchanged.

5. **Build constraints, not preprocessor.** Every js-only file starts with
   `//go:build js && wasm`. Host-only tests have no constraint. This means
   `go vet ./...` and `go test ./...` on the host never see `syscall/js`
   imports and never need a browser.

## The loop

```
                ┌────────────────────────────────────────┐
                │              cmd/re2-wasm/main          │
                │   constructs wasm.Runtime, blocks      │
                └──────────────────┬─────────────────────┘
                                   │
                                   ▼
                ┌────────────────────────────────────────┐
                │              wasm.Runtime              │
                │  • registers JS API on global          │
                │  • owns requestAnimationFrame loop     │
                │  • calls engine.Step() once per frame  │
                └──────────────────┬─────────────────────┘
                                   │
                                   ▼
                ┌────────────────────────────────────────┐
                │              engine.Engine             │
                │  • FrameTimer.Tick()                   │
                │  • Renderer.BeginFrame() / EndFrame()  │
                │  • (Phase 2+) sim steps, draw calls    │
                └──────────────────┬─────────────────────┘
                                   │
              ┌─────────────┬──────┴───────┬─────────────┐
              ▼             ▼              ▼             ▼
       renderer/webgl   audio.Manager  input.Manager  ui.Overlay
       (WebGL2 ctx)     (AudioContext)  (kb/mouse/gp)  (DOM overlay)
```

The browser drives the loop. `wasm.Runtime.scheduleFrame()` calls
`requestAnimationFrame`; when the browser fires the callback, the runtime
calls `engine.Step(ctx)`, which advances the simulation timer and renders
one frame. There is no Go-side `for {}` loop on the WASM path because that
would block the JS event loop.

## Why not TinyGo?

TinyGo produces smaller WASM binaries and supports `syscall/js`, but it
lags the main Go release and lacks support for some reflect-heavy patterns
that OpenBiohazard2 uses for asset decoding. The official Go compiler
produces a ~3 MB WASM file for the Phase 1 binary, which is acceptable;
if size becomes a problem in later phases we will revisit.

## Why WebGL 2 and not WebGPU?

WebGPU is the future, but as of 2026 its browser support is still uneven
(Safari in particular trails). WebGL 2 is universally available and gives
us GLES 3.0, which is enough for the original RE2's rendering pipeline.

The renderer is split so that a `renderer/webgpu` package can be added
without touching `engine.Engine`. The `renderer/common` package owns the
shared types (`PixelFormat`, `Mesh`, `TextureDesc`); each backend
translates them into its own native handles.

## Why IndexedDB for saves?

The original RE2 stores 20 save slots. IndexedDB gives us:

* Larger per-record storage than `localStorage` (RE2 saves are ~8 KB)
* Async I/O so saving never blocks the render loop
* Per-origin isolation so saves from one site don't leak to another
* Easy export/import: read the slot's `Uint8Array` and hand it to the user
  as a downloadable file

`saves.IDBStore` is the browser implementation; `saves.MemStore` is an
in-memory fallback used by host tests and when IndexedDB is unavailable.

## Why a fixed-step accumulator?

A naive `dt`-based simulation drifts: physics integration gets less stable
as `dt` varies, and re-renders of old frames (e.g. after a tab is
backgrounded) cause huge `dt` spikes that fling objects through walls.

The fixed-step accumulator runs the simulation at exactly 30 Hz and
accumulates the leftover time. If the render rate is 60 Hz, the sim runs
once every other frame; if it drops to 20 Hz, the sim runs 1–2 times per
frame to catch up, capped at 5 steps to avoid the spiral of death. The
`FrameTimer.Alpha()` value gives renderers a fractional interpolation
factor so motion stays smooth between sim steps.

## Asset pipeline (Phase 2+)

```
   Browser file picker / drag-and-drop
                │
                ▼
       loader.js reads as ArrayBuffer
                │
                ▼
       re2wasm.mountFile(name, Uint8Array)
                │
                ▼
       filesystem.MemoryFS.Mount(name, []byte)
                │
                ▼
       assets.Loader.Open(ctx, name) → Reader
                │
                ▼
       (Phase 3) format-specific decoders
```

The `MemoryFS` is intentionally simple in Phase 1: it normalises paths
(lower-case, forward-slashes, no leading slash) so BIN/CUE / Windows paths
behave identically. Phase 2 will add a virtual directory tree so the
engine can ask for `STAGE1/ROOM1.TIM` without knowing it came from
`rea1.bin`.

## Why a `//go:build js && wasm` constraint on every js-only file?

Without the constraint, `go vet ./...` on the host fails because the host
toolchain cannot import `syscall/js`. With the constraint, those files
compile only when `GOOS=js GOARCH=wasm`, and host tooling silently skips
them. This is what lets us run the full test suite on every commit.

## Why does the main function call `select {}`?

A Go WASM module exits as soon as `main()` returns, which would tear down
every registered `js.FuncOf` callback and effectively freeze the engine.
`select {}` blocks forever; the browser destroys the module when the page
unloads. This is the same pattern Go's own `wasm_exec.js` examples use.

## Performance budget

Phase 1 has no budget-sensitive code yet, but the budget we are aiming for
once gameplay is in:

* 60 FPS render rate on a mid-range 2020 laptop
* < 16 ms per frame (engine + render + browser compositor)
* < 100 MB resident heap (original RE2 fits in 8 MB of PS1 RAM; we have
  headroom for decoded textures and audio buffers)
* < 5 s cold start (WASM compile + first frame)

The `FrameTimer` cap of 5 simulation steps per render frame is the main
defence against the spiral-of-death; if we fall behind, we drop simulation
time rather than rendering fewer frames.
