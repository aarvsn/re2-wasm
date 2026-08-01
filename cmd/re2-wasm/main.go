//go:build js && wasm

// Command re2-wasm is the WebAssembly entry point. It is built with
// GOOS=js GOARCH=wasm and loaded by web/index.html via wasm_exec.js.
//
// The binary is intentionally tiny: it constructs a wasm.Runtime, exposes its
// JS-callable API on the global object, and then blocks forever so the Go
// runtime does not exit (which would tear down every goroutine and release
// the registered callbacks).
package main

import (
        "syscall/js"

        "github.com/aarvsn/re2-wasm/wasm"
)

func main() {
        rt, err := wasm.New("re2-canvas")
        if err != nil {
                // We have no UI overlay yet at this point; surface the error to
                // the console and to a global that loader.js can poll for.
                js.Global().Set("re2wasm_boot_error", err.Error())
                return
        }
        // RegisterAPI returns a release function we never call (the module
        // runs for the lifetime of the page), but we keep a reference to it
        // so the Go compiler does not flag the registered callbacks as dead.
        release := rt.RegisterAPI()
        defer release()

        // Auto-start the renderer so Phase 1 is visible the moment the module
        // loads. Loader.js still calls re2wasm.start() to be safe.
        if v, _ := jsEval(`re2wasm && re2wasm.start && re2wasm.start()`); !v.Truthy() {
                // Not fatal: loader.js will retry once wasm_exec finishes.
        }

        // Block forever. The browser will tear us down when the page unloads.
        select {}
}

// jsEval runs a snippet of JavaScript and returns its result. It exists so
// the main function can call into the API it just registered without having
// to duplicate the call logic.
func jsEval(src string) (js.Value, error) {
        v := js.Global().Call("eval", src)
        return v, nil
}
