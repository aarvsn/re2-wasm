// loader.js — tiny glue between the DOM and the re2-wasm Go module.
//
// Responsibilities:
//   1. Copy Go's wasm_exec.js helper next to this file (the Makefile does it).
//   2. Detect browser features (WebGL2, Audio, Gamepad) and update badges.
//   3. Fetch re2.wasm and instantiate it via WebAssembly.instantiateStreaming.
//   4. Wire the DOM buttons (fullscreen, pointer lock, mute, stop).
//   5. Handle drag-and-drop + file-picker for user-provided game files.
//   6. Resume the AudioContext on first user interaction (autoplay policy).
//
// The file is deliberately framework-free and dependency-free so it works
// from a file:// URL too (useful for local testing).

(function () {
  "use strict";

  // ---- Boot badges --------------------------------------------------------

  const badges = {
    webgl:   document.getElementById("badge-webgl"),
    audio:   document.getElementById("badge-audio"),
    gamepad: document.getElementById("badge-gamepad"),
    wasm:    document.getElementById("badge-wasm"),
  };

  function setBadge(el, ok, labelOk, labelFail) {
    el.textContent = ok ? labelOk : labelFail;
    el.classList.remove("ok", "warn", "err");
    el.classList.add(ok ? "ok" : "err");
  }

  function detectFeatures() {
    const c = document.createElement("canvas");
    const gl = c.getContext("webgl2");
    setBadge(badges.webgl, !!gl,
      "WebGL2: ready", "WebGL2: missing");

    const audioOk = !!(window.AudioContext || window.webkitAudioContext);
    setBadge(badges.audio, audioOk,
      "Audio: ready", "Audio: missing");

    setBadge(badges.gamepad, !!navigator.getGamepads,
      "Gamepad: ready", "Gamepad: missing");

    setBadge(badges.wasm,
      typeof WebAssembly === "object" && typeof WebAssembly.instantiateStreaming === "function",
      "WASM: ready", "WASM: missing");

    return { gl, audioOk, gamepadOk: !!navigator.getGamepads };
  }

  // ---- Loading UI ---------------------------------------------------------

  const loadingScreen = document.getElementById("loading-screen");
  const loadingBar    = document.getElementById("loading-bar");
  const loadingLabel  = document.getElementById("loading-label");
  const loadingPct    = document.getElementById("loading-percent");
  const errorToast    = document.getElementById("error-toast");

  function setLoading(pct, label) {
    const p = Math.max(0, Math.min(1, pct));
    loadingBar.style.width = (p * 100).toFixed(1) + "%";
    loadingPct.textContent = (p * 100).toFixed(0) + "%";
    if (label) loadingLabel.textContent = label;
  }
  function hideLoading() {
    loadingScreen.classList.remove("visible");
  }
  function showError(msg) {
    errorToast.textContent = msg;
    errorToast.classList.add("visible");
    console.error("[re2-wasm]", msg);
    setTimeout(() => errorToast.classList.remove("visible"), 6000);
  }

  // Export so the Go module can call back into us if it needs to.
  window.re2ui = { setLoading, hideLoading, showError };

  // ---- Audio unlock -------------------------------------------------------

  let audioCtx;
  function unlockAudio() {
    if (!audioCtx) {
      const Ctor = window.AudioContext || window.webkitAudioContext;
      if (Ctor) audioCtx = new Ctor();
    }
    if (audioCtx && audioCtx.state === "suspended") {
      audioCtx.resume().catch(() => {});
    }
  }
  window.addEventListener("pointerdown", unlockAudio, { once: false });
  window.addEventListener("keydown",     unlockAudio, { once: false });

  // ---- Gamepad polling ----------------------------------------------------
  // (Phase 1 just shows whether one is connected; the Go side polls in
  // Phase 4.)

  function refreshGamepadBadge() {
    const pads = navigator.getGamepads ? navigator.getGamepads() : [];
    let n = 0;
    for (let i = 0; i < pads.length; i++) if (pads[i]) n++;
    badges.gamepad.textContent = n > 0 ? ("Gamepad: " + n + " connected") : "Gamepad: none";
    badges.gamepad.classList.remove("ok", "warn", "err");
    badges.gamepad.classList.add(n > 0 ? "ok" : "warn");
  }
  window.addEventListener("gamepadconnected",    refreshGamepadBadge);
  window.addEventListener("gamepaddisconnected", refreshGamepadBadge);
  setInterval(refreshGamepadBadge, 1000);

  // ---- Drag-and-drop + file picker ---------------------------------------

  const dropzone   = document.getElementById("dropzone");
  const fileInput  = document.getElementById("file-input");
  const browseBtn  = document.getElementById("browse-btn");

  function handleFiles(fileList) {
    const files = Array.from(fileList || []);
    if (files.length === 0) return;
    setLoading(0.05, "Reading " + files.length + " file(s)…");
    let done = 0;
    files.forEach((f) => {
      const reader = new FileReader();
      reader.onload = () => {
        const bytes = new Uint8Array(reader.result);
        if (window.re2wasm && window.re2wasm.mountFile) {
          const r = window.re2wasm.mountFile(f.name, bytes);
          if (!r || !r.ok) {
            showError("mountFile failed for " + f.name + ": " + (r && r.error));
          }
        }
        done++;
        setLoading(0.05 + 0.9 * (done / files.length), "Mounted " + f.name);
        if (done === files.length) {
          setLoading(1.0, "All files mounted");
          setTimeout(hideLoading, 400);
        }
      };
      reader.onerror = () => showError("Could not read " + f.name);
      reader.readAsArrayBuffer(f);
    });
  }

  dropzone.addEventListener("click", () => fileInput.click());
  browseBtn.addEventListener("click", (e) => { e.stopPropagation(); fileInput.click(); });
  fileInput.addEventListener("change", () => handleFiles(fileInput.files));

  ["dragenter", "dragover"].forEach((ev) => {
    dropzone.addEventListener(ev, (e) => {
      e.preventDefault(); e.stopPropagation();
      dropzone.classList.add("drag");
    });
  });
  ["dragleave", "drop"].forEach((ev) => {
    dropzone.addEventListener(ev, (e) => {
      e.preventDefault(); e.stopPropagation();
      dropzone.classList.remove("drag");
    });
  });
  dropzone.addEventListener("drop", (e) => handleFiles(e.dataTransfer.files));

  // Whole-window drop zone so users can drop anywhere.
  ["dragenter", "dragover", "drop"].forEach((ev) => {
    window.addEventListener(ev, (e) => { e.preventDefault(); });
  });
  window.addEventListener("drop", (e) => handleFiles(e.dataTransfer.files));

  // ---- Toolbar buttons ----------------------------------------------------

  const canvas = document.getElementById("re2-canvas");
  document.getElementById("btn-fullscreen").addEventListener("click", () => {
    if (document.fullscreenElement) {
      document.exitFullscreen();
    } else {
      document.documentElement.requestFullscreen().catch(showError);
    }
  });
  document.getElementById("btn-pointerlock").addEventListener("click", () => {
    canvas.requestPointerLock && canvas.requestPointerLock();
  });
  let muted = false;
  document.getElementById("btn-mute").addEventListener("click", (e) => {
    muted = !muted;
    if (audioCtx) audioCtx [(muted ? "suspend" : "resume")]();
    e.target.textContent = muted ? "Unmute" : "Mute";
  });
  document.getElementById("btn-stop").addEventListener("click", () => {
    if (window.re2wasm && window.re2wasm.stop) window.re2wasm.stop();
  });

  // HUD toggle on backtick.
  const hud = document.getElementById("hud");
  window.addEventListener("keydown", (e) => {
    if (e.code === "Backquote") hud.classList.toggle("visible");
  });

  // ---- WASM bootstrap -----------------------------------------------------

  const feats = detectFeatures();
  if (!feats.gl) {
    showError("WebGL2 is required. Try a current Chromium, Firefox, Safari, or Edge.");
    setLoading(1.0, "WebGL2 missing — cannot start");
    return;
  }

  // WebAssembly.instantiateStreaming requires the correct MIME type; some
  // local servers do not set it, so we fall back to fetch + instantiate.
  async function bootWasm() {
    setLoading(0.02, "Loading re2.wasm…");
    if (typeof go === "undefined") {
      showError("wasm_exec.js failed to load — did `make wasm` run?");
      return;
    }
    const goInstance = new Go();
    let result;
    try {
      const resp = await fetch("re2.wasm");
      if (!resp.ok) throw new Error("HTTP " + resp.status);
      const buf = await resp.arrayBuffer();
      result = await WebAssembly.instantiate(buf, goInstance.importObject);
    } catch (e) {
      showError("Failed to load re2.wasm: " + e.message);
      return;
    }
    setLoading(0.4, "Instantiating…");
    // goInstance.run returns a promise that resolves when main() returns.
    // Our main() blocks forever, so this promise stays pending.
    goInstance.run(result.instance).catch((e) => {
      showError("WASM main exited: " + (e && e.message ? e.message : e));
    });

    // Wait for the API to appear (main() registers it on the global).
    const t0 = performance.now();
    while (!window.re2wasm && performance.now() - t0 < 4000) {
      await new Promise((r) => setTimeout(r, 30));
    }
    if (!window.re2wasm) {
      showError("WASM booted but did not register its API");
      return;
    }
    if (window.re2wasm_boot_error) {
      showError("WASM boot error: " + window.re2wasm_boot_error);
      return;
    }
    setLoading(0.6, "Starting engine…");
    if (window.re2wasm.start) {
      const r = window.re2wasm.start();
      if (!r || !r.ok) {
        showError("re2wasm.start failed: " + (r && r.error));
        return;
      }
    }
    setLoading(1.0, "Ready");
    setTimeout(hideLoading, 250);

    // FPS counter.
    let frames = 0, last = performance.now();
    function tick(now) {
      frames++;
      if (now - last >= 1000) {
        document.getElementById("hud-fps").textContent = frames;
        document.getElementById("hud-frame").textContent =
          (window.__re2wasmFrame = (window.__re2wasmFrame || 0) + frames);
        frames = 0; last = now;
      }
      requestAnimationFrame(tick);
    }
    requestAnimationFrame(tick);
  }

  // Defer boot until the DOM is ready (the script is loaded with `defer`).
  bootWasm();
})();
