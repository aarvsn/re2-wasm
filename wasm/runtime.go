//go:build js && wasm

// Package wasm is the browser-side runtime glue. It owns the
// requestAnimationFrame loop, the engine's ports, and the JS-callable API
// exposed on the global object so that loader.js can drive the module.
//
// This package is only compiled for GOOS=js,GOARCH=wasm.
package wasm

import (
        "context"
        "errors"
        "runtime"
        "sync"
        "syscall/js"
        "time"

        "github.com/aarvsn/re2-wasm/assets"
        "github.com/aarvsn/re2-wasm/audio"
        "github.com/aarvsn/re2-wasm/engine"
        "github.com/aarvsn/re2-wasm/engine/clock"
        "github.com/aarvsn/re2-wasm/filesystem/discfs"
        "github.com/aarvsn/re2-wasm/input"
        "github.com/aarvsn/re2-wasm/renderer/webgl"
        "github.com/aarvsn/re2-wasm/saves"
        "github.com/aarvsn/re2-wasm/ui"
)

// Runtime owns the engine and the rAF loop. There is one Runtime per WASM
// module instance.
type Runtime struct {
        mu          sync.Mutex
        engine      *engine.Engine
        renderer    *webgl.Renderer
        audio       *audio.Manager
        input       *input.Manager
        saves       engine.SaveStore
        ui          *ui.Overlay
        fs          *discfs.DiscFS
        assets      *assets.Loader

        cancel      context.CancelFunc
        ctx         context.Context
        rAFid       int
        running     bool
        filesMounted int
}

// New constructs a Runtime bound to the canvas with the given DOM id.
func New(canvasID string) (*Runtime, error) {
        r := &Runtime{}
        r.ui = ui.New()
        r.fs = discfs.New()

        rnd, err := webgl.New(canvasID)
        if err != nil {
                return nil, err
        }
        r.renderer = rnd

        r.audio = audio.New()
        r.input = input.New(r.renderer.Canvas())

        // Save store is opened lazily so that a missing IndexedDB never breaks
        // the engine boot.

        return r, nil
}

// Start boots the engine and begins the rAF loop. It is idempotent.
func (r *Runtime) Start() error {
        r.mu.Lock()
        defer r.mu.Unlock()
        if r.running {
                return nil
        }

        ports := engine.Ports{
                Renderer: r.renderer,
                Audio:    r.audio,
                Input:    r.input,
                UI:       r.ui,
                FS:       r.fs,
                Assets:   assets.New(nil), // Phase 3 wires a real loader; nil-safe
                Clock:    clock.SystemClock{},
        }
        eng, err := engine.New(ports)
        if err != nil {
                return err
        }
        r.engine = eng

        // Init ports synchronously so we surface errors before rAF starts.
        if err := eng.Init(); err != nil {
                return err
        }

        r.ctx, r.cancel = context.WithCancel(context.Background())
        r.running = true

        // Drive the loop via requestAnimationFrame. We do not call eng.Run
        // because that would block; instead we step the engine once per frame.
        r.scheduleFrame()

        return nil
}

// Stop cancels the loop and shuts the engine down. It is idempotent.
func (r *Runtime) Stop() {
        r.mu.Lock()
        defer r.mu.Unlock()
        if !r.running {
                return
        }
        if r.cancel != nil {
                r.cancel()
                r.cancel = nil
        }
        if r.rAFid != 0 {
                js.Global().Call("cancelAnimationFrame", r.rAFid)
                r.rAFid = 0
        }
        r.running = false
        if r.engine != nil {
                r.engine.Shutdown()
        }
}

// scheduleFrame requests a single animation frame and routes it to step.
func (r *Runtime) scheduleFrame() {
        cb := js.FuncOf(func(this js.Value, args []js.Value) any {
                go r.step()
                return nil
        })
        r.rAFid = js.Global().Call("requestAnimationFrame", cb).Int()
        // cb is intentionally leaked; the next scheduleFrame creates a fresh one.
        _ = cb
}

// step drives one engine iteration on the rAF callback.
func (r *Runtime) step() {
        r.mu.Lock()
        if !r.running {
                r.mu.Unlock()
                return
        }
        eng := r.engine
        ctx := r.ctx
        r.mu.Unlock()

        if eng == nil {
                return
        }
        if err := eng.Step(ctx); err != nil {
                r.ui.ShowError(err.Error())
                r.Stop()
                return
        }
        // Pulse the clear colour so Phase 1 visibly proves the loop is live.
        t := time.Since(loopStart).Seconds()
        rR := float32(0.5 + 0.5*sin(t*0.5))
        g := float32(0.5 + 0.5*sin(t*0.7+1.0))
        b := float32(0.5 + 0.5*sin(t*0.9+2.0))
        r.renderer.SetClearColor(rR, g, b, 1.0)

        if ctx.Err() == nil {
                r.scheduleFrame()
        }
}

// loopStart is captured when the package initialises so the Phase 1 colour
// pulse is deterministic from the moment the module loads.
var loopStart = time.Now()

// sin is a tiny sin approximation that avoids pulling in math.Sin (which
// works but bloats the wasm binary). Accuracy here is irrelevant: the pulse
// is purely cosmetic to prove the renderer is alive.
func sin(x float64) float64 {
        const pi = 3.141592653589793
        for x < 0 {
                x += 2 * pi
        }
        for x > 2*pi {
                x -= 2 * pi
        }
        if x > pi {
                x = 2*pi - x
        }
        num := 16 * x * (pi - x)
        den := 5*pi*pi - 4*x*(pi-x)
        return num / den
}

// itoa is a tiny int-to-string helper that avoids pulling in strconv for a
// handful of small numbers used by the loading UI.
func itoa(n int) string {
        if n == 0 {
                return "0"
        }
        neg := n < 0
        if neg {
                n = -n
        }
        var buf [12]byte
        i := len(buf)
        for n > 0 {
                i--
                buf[i] = byte('0' + n%10)
                n /= 10
        }
        if neg {
                i--
                buf[i] = '-'
        }
        return string(buf[i:])
}

// RegisterAPI exposes the JS-callable API on the global object. The returned
// function releases every callback; the WASM main function defers it.
func (r *Runtime) RegisterAPI() func() {
        start := js.FuncOf(func(this js.Value, args []js.Value) any {
                if err := r.Start(); err != nil {
                        r.ui.ShowError(err.Error())
                        return map[string]any{"ok": false, "error": err.Error()}
                }
                return map[string]any{"ok": true}
        })
        stop := js.FuncOf(func(this js.Value, args []js.Value) any {
                r.Stop()
                return map[string]any{"ok": true}
        })
        mountFile := js.FuncOf(func(this js.Value, args []js.Value) any {
                if len(args) < 2 {
                        return map[string]any{"ok": false, "error": "expected (name, data)"}
                }
                name := args[0].String()
                b := make([]byte, args[1].Get("byteLength").Int())
                js.CopyBytesToGo(b, args[1])
                if err := r.fs.Mount(name, b); err != nil {
                        r.ui.ShowError("mount " + name + ": " + err.Error())
                        return map[string]any{"ok": false, "error": err.Error(), "size": len(b)}
                }
                r.mu.Lock()
                r.filesMounted++
                n := r.filesMounted
                r.mu.Unlock()
                // Update loading bar so users see the file was accepted. The
                // 0.1..0.9 range leaves headroom for the engine init that
                // follows once both halves of a BIN/CUE pair have landed.
                progress := float32(0.1 + 0.8*float64(n)/10.0)
                if progress > 0.9 {
                        progress = 0.9
                }
                r.ui.SetLoading(progress, "Mounted "+name+" ("+itoa(len(b))+" bytes)")
                return map[string]any{"ok": true, "size": len(b), "mounted": n}
        })
        listFiles := js.FuncOf(func(this js.Value, args []js.Value) any {
                list, err := r.fs.List()
                if err != nil {
                        return map[string]any{"ok": false, "error": err.Error()}
                }
                out := js.Global().Get("Array").New()
                for _, p := range list {
                        out.Call("push", p)
                }
                return map[string]any{"ok": true, "files": out}
        })
        hasFile := js.FuncOf(func(this js.Value, args []js.Value) any {
                if len(args) < 1 {
                        return map[string]any{"ok": false, "error": "expected (path)"}
                }
                return map[string]any{"ok": true, "has": r.fs.Has(args[0].String())}
        })
        readFile := js.FuncOf(func(this js.Value, args []js.Value) any {
                if len(args) < 1 {
                        return map[string]any{"ok": false, "error": "expected (path)"}
                }
                b, err := r.fs.Read(args[0].String())
                if err != nil {
                        return map[string]any{"ok": false, "error": err.Error()}
                }
                arr := js.Global().Get("Uint8Array").New(len(b))
                js.CopyBytesToJS(arr, b)
                return map[string]any{"ok": true, "data": arr, "size": len(b)}
        })
        // ---- Save API (Phase 4) ------------------------------------------------
        // The store is opened lazily on first call so that a missing or
        // unavailable IndexedDB never blocks the engine boot.
        saveSlot := js.FuncOf(func(this js.Value, args []js.Value) any {
                if len(args) < 2 {
                        return map[string]any{"ok": false, "error": "expected (slot, data)"}
                }
                slot := args[0].Int()
                b := make([]byte, args[1].Get("byteLength").Int())
                js.CopyBytesToGo(b, args[1])
                store, err := r.saveStore()
                if err != nil {
                        return map[string]any{"ok": false, "error": err.Error()}
                }
                if err := store.Save(r.ctx, slot, b); err != nil {
                        return map[string]any{"ok": false, "error": err.Error()}
                }
                return map[string]any{"ok": true, "slot": slot, "size": len(b)}
        })
        loadSlot := js.FuncOf(func(this js.Value, args []js.Value) any {
                if len(args) < 1 {
                        return map[string]any{"ok": false, "error": "expected (slot)"}
                }
                slot := args[0].Int()
                store, err := r.saveStore()
                if err != nil {
                        return map[string]any{"ok": false, "error": err.Error()}
                }
                b, err := store.Load(r.ctx, slot)
                if err != nil {
                        return map[string]any{"ok": false, "error": err.Error()}
                }
                arr := js.Global().Get("Uint8Array").New(len(b))
                js.CopyBytesToJS(arr, b)
                return map[string]any{"ok": true, "data": arr, "size": len(b)}
        })
        listSaves := js.FuncOf(func(this js.Value, args []js.Value) any {
                store, err := r.saveStore()
                if err != nil {
                        return map[string]any{"ok": false, "error": err.Error()}
                }
                slots, err := store.List(r.ctx)
                if err != nil {
                        return map[string]any{"ok": false, "error": err.Error()}
                }
                out := js.Global().Get("Array").New()
                for _, s := range slots {
                        out.Call("push", s)
                }
                return map[string]any{"ok": true, "slots": out}
        })
        exportSave := js.FuncOf(func(this js.Value, args []js.Value) any {
                if len(args) < 1 {
                        return map[string]any{"ok": false, "error": "expected (slot)"}
                }
                store, err := r.saveStore()
                if err != nil {
                        return map[string]any{"ok": false, "error": err.Error()}
                }
                b, err := store.Export(args[0].Int())
                if err != nil {
                        return map[string]any{"ok": false, "error": err.Error()}
                }
                arr := js.Global().Get("Uint8Array").New(len(b))
                js.CopyBytesToJS(arr, b)
                return map[string]any{"ok": true, "data": arr}
        })
        importSave := js.FuncOf(func(this js.Value, args []js.Value) any {
                if len(args) < 2 {
                        return map[string]any{"ok": false, "error": "expected (slot, data)"}
                }
                slot := args[0].Int()
                b := make([]byte, args[1].Get("byteLength").Int())
                js.CopyBytesToGo(b, args[1])
                store, err := r.saveStore()
                if err != nil {
                        return map[string]any{"ok": false, "error": err.Error()}
                }
                if err := store.Import(slot, b); err != nil {
                        return map[string]any{"ok": false, "error": err.Error()}
                }
                return map[string]any{"ok": true, "slot": slot}
        })
        js.Global().Set("re2wasm", js.Global().Get("Object").New())
        api := js.Global().Get("re2wasm")
        api.Set("start", start)
        api.Set("stop", stop)
        api.Set("mountFile", mountFile)
        api.Set("listFiles", listFiles)
        api.Set("hasFile", hasFile)
        api.Set("readFile", readFile)
        api.Set("saveSlot", saveSlot)
        api.Set("loadSlot", loadSlot)
        api.Set("listSaves", listSaves)
        api.Set("exportSave", exportSave)
        api.Set("importSave", importSave)
        return func() {
                start.Release()
                stop.Release()
                mountFile.Release()
                listFiles.Release()
                hasFile.Release()
                readFile.Release()
                saveSlot.Release()
                loadSlot.Release()
                listSaves.Release()
                exportSave.Release()
                importSave.Release()
        }
}

// GoVersion returns the Go toolchain version for the debug HUD. Exported so
// loader.js can display it in the corner.
func GoVersion() string { return runtime.Version() }

// errMissing is returned when a required JS global is missing.
var errMissing = errors.New("wasm: required browser API is missing")

// saveStore returns the IndexedDB-backed save store, opening it lazily on
// first call. If IndexedDB is unavailable, the in-memory fallback is used
// so the engine still works for testing.
func (r *Runtime) saveStore() (engine.SaveStore, error) {
        r.mu.Lock()
        defer r.mu.Unlock()
        if r.saves != nil {
                return r.saves, nil
        }
        store, err := saves.OpenIDB()
        if err != nil {
                // Fall back to in-memory so the engine still runs in environments
                // without IndexedDB (e.g. private browsing, file:// origins).
                ConsoleLog("warn", "IndexedDB unavailable; using in-memory save store: "+err.Error())
                mem := saves.NewMemStore()
                r.saves = memSaveShim{inner: mem}
                return r.saves, nil
        }
        r.saves = store
        return store, nil
}

// memSaveShim adapts saves.MemStore to the engine.SaveStore interface
// without dragging the engine import into saves.MemStore (which would
// create a cycle).
type memSaveShim struct{ inner *saves.MemStore }

func (m memSaveShim) Load(ctx context.Context, slot int) ([]byte, error) {
        if m.inner == nil {
                return nil, errors.New("saves: not initialised")
        }
        return m.inner.Load(ctx, slot)
}
func (m memSaveShim) Save(ctx context.Context, slot int, data []byte) error {
        if m.inner == nil {
                return errors.New("saves: not initialised")
        }
        return m.inner.Save(ctx, slot, data)
}
func (m memSaveShim) List(ctx context.Context) ([]int, error) {
        if m.inner == nil {
                return nil, errors.New("saves: not initialised")
        }
        return m.inner.List(ctx)
}
func (m memSaveShim) Export(slot int) ([]byte, error) {
        if m.inner == nil {
                return nil, errors.New("saves: not initialised")
        }
        return m.inner.Export(slot)
}
func (m memSaveShim) Import(slot int, data []byte) error {
        if m.inner == nil {
                return errors.New("saves: not initialised")
        }
        return m.inner.Import(slot, data)
}

// ConsoleLog is a thin shim around wasmglue.ConsoleLog so this package can
// emit warnings without importing wasmglue (which doesn't exist; we log
// directly).
func ConsoleLog(level, msg string) {
        if !js.Global().Get("console").Truthy() {
                return
        }
        switch level {
        case "warn":
                js.Global().Get("console").Call("warn", msg)
        case "error":
                js.Global().Get("console").Call("error", msg)
        default:
                js.Global().Get("console").Call("log", msg)
        }
}
