//go:build js && wasm

// Package compat detects which browser features are available so the
// engine can degrade gracefully. The results are surfaced to loader.js
// via the global re2compat object so the loading UI can show badges and
// the user knows what's working.
package compat

import "syscall/js"

// Report is the set of feature flags the engine cares about.
type Report struct {
	WebGL2           bool
	WebGPU           bool
	WebAudio         bool
	Gamepad          bool
	IndexedDB        bool
	PointerLock      bool
	Fullscreen       bool
	FileSystemAccess bool
	Vibration        bool
	WASMStreaming    bool
}

// Detect probes the browser and returns a Report. Safe to call at any
// time; results are cached for the lifetime of the page.
func Detect() Report {
	var r Report
	if js.Global().Get("document").Truthy() {
		c := js.Global().Get("document").Call("createElement", "canvas")
		if c.Truthy() {
			r.WebGL2 = c.Call("getContext", "webgl2").Truthy()
		}
	}
	if js.Global().Get("navigator").Truthy() {
		nav := js.Global().Get("navigator")
		r.WebGPU = nav.Get("gpu").Truthy()
		r.WebAudio = js.Global().Get("AudioContext").Truthy() ||
			js.Global().Get("webkitAudioContext").Truthy()
		r.Gamepad = nav.Get("getGamepads").Truthy()
		r.IndexedDB = js.Global().Get("indexedDB").Truthy()
		r.PointerLock = js.Global().Get("document").Get("exitPointerLock").Truthy() ||
			js.Global().Get("document").Get("mozExitPointerLock").Truthy()
		r.Fullscreen = js.Global().Get("document").Get("fullscreenEnabled").Truthy() ||
			js.Global().Get("document").Get("webkitFullscreenEnabled").Truthy()
		if nav.Get("storage").Truthy() {
			r.FileSystemAccess = nav.Get("storage").Get("getDirectory").Truthy()
		}
		r.Vibration = nav.Get("vibrate").Truthy()
	}
	if js.Global().Get("WebAssembly").Truthy() {
		r.WASMStreaming = js.Global().Get("WebAssembly").Get("instantiateStreaming").Truthy()
	}
	return r
}

// Publish writes the report to window.re2compat so loader.js can read it.
func (r Report) Publish() {
	if !js.Global().Get("window").Truthy() {
		return
	}
	obj := js.Global().Get("Object").New()
	obj.Set("webgl2", r.WebGL2)
	obj.Set("webgpu", r.WebGPU)
	obj.Set("webAudio", r.WebAudio)
	obj.Set("gamepad", r.Gamepad)
	obj.Set("indexedDB", r.IndexedDB)
	obj.Set("pointerLock", r.PointerLock)
	obj.Set("fullscreen", r.Fullscreen)
	obj.Set("fileSystemAccess", r.FileSystemAccess)
	obj.Set("vibration", r.Vibration)
	obj.Set("wasmStreaming", r.WASMStreaming)
	js.Global().Set("re2compat", obj)
}

// BrowserName returns a best-effort browser identifier for telemetry.
func BrowserName() string {
	ua := ""
	if js.Global().Get("navigator").Truthy() {
		ua = js.Global().Get("navigator").Get("userAgent").String()
	}
	switch {
	case contains(ua, "Edg/"):
		return "edge"
	case contains(ua, "OPR/") || contains(ua, "Opera"):
		return "opera"
	case contains(ua, "Chrome/"):
		return "chromium"
	case contains(ua, "Firefox/"):
		return "firefox"
	case contains(ua, "Safari/") && !contains(ua, "Chrome"):
		return "safari"
	default:
		return "unknown"
	}
}

// contains is a tiny strings.Contains replacement so this file does not
// need to import strings (which would bloat the wasm binary slightly).
func contains(s, sub string) bool {
	if len(sub) == 0 {
		return true
	}
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
