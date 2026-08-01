# BUILDING

Cross-platform build instructions for re2-wasm. The project supports Linux,
Windows, and macOS as build hosts; the produced `re2.wasm` is portable to
every browser that supports WebGL 2.

## Prerequisites

| Tool          | Min version | Required?                       |
| ------------- | ----------- | ------------------------------- |
| Go            | 1.25        | Yes — builds the WASM binary    |
| Make          | any         | Recommended (Linux/macOS)       |
| Python 3      | 3.8+        | Only for `make serve`           |
| CMake         | 3.18+       | Only for native asset tools     |
| C/C++ compiler| any         | Only when building native tools |

If your system doesn't have `make` (e.g. Windows without WSL), the
equivalent raw commands are:

```powershell
# Windows PowerShell
$env:GOOS="js"; $env:GOARCH="wasm"
go build -trimpath -ldflags="-s -w" -o web\re2.wasm .\cmd\re2-wasm
copy "$(go env GOROOT)\lib\wasm\wasm_exec.js" "web\wasm_exec.js"
```

## Build

From the repo root:

```bash
make wasm      # produces web/re2.wasm and web/wasm_exec.js
make serve     # serves web/ on http://localhost:8080
make test      # runs host unit tests
make vet       # runs go vet on both host and js/wasm targets
make check     # vet + test + wasm (the gate CI runs)
make clean     # removes build artifacts
make fmt       # formats Go source
make tidy      # runs `go mod tidy`
```

## Verifying the build

After `make wasm`, `web/` must contain exactly these files:

```
web/
├── index.html
├── styles.css
├── loader.js
├── wasm_exec.js
└── re2.wasm
```

If any file is missing the page will fail to load; the loader.js script
surfaces the specific failure as an error toast.

## Native asset-converter tools (Phase 3+)

Phase 3 will add a native C/C++ toolchain that converts original RE2 asset
formats (TIM, TMD, ADT) into engine-friendly blobs. The CMake build is
opt-in to keep the default build dependency-free:

```bash
cmake -S . -B build -DBUILD_NATIVE_TOOLS=ON
cmake --build build
```

When `BUILD_NATIVE_TOOLS` is OFF (the default), CMake prints a notice and
does nothing.

## Troubleshooting

### `wasm_exec.js failed to load`

You ran `go build` directly without copying `wasm_exec.js`. Either use
`make wasm` (which copies it for you) or copy it manually:

```bash
cp "$(go env GOROOT)/lib/wasm/wasm_exec.js" web/
```

### `WebGL2 is not available`

Your browser does not support WebGL 2 / GLES 3.0. Update to a current
Chromium, Firefox, Safari 16.4+, or Edge. On Linux, make sure your GPU
driver exposes WebGL2 — software rendering via SwiftShader also works.

### `re2wasm.start failed: renderer init failed`

The canvas with id `re2-canvas` could not be found, or the WebGL2 context
creation returned null. Check the browser console for the specific GL error.

### `WASM booted but did not register its API`

The Go `main()` function did not reach `RegisterAPI` in time. This usually
means `wasm.New` failed; check the `re2wasm_boot_error` global in the
browser console.

### Build is slow

The first `GOOS=js GOARCH=wasm go build` downloads the standard library's
`syscall/js` package and caches it. Subsequent builds are fast. If you are
on a slow connection, set `GOPROXY=https://goproxy.cn` (or your local mirror).

### `go vet` complains about syscall/js on host

The host `go vet` should skip js-only files automatically thanks to the
`//go:build js && wasm` constraint at the top of every browser-only file.
If you see this error, the constraint is missing — run `make vet` from a
clean checkout and re-check the file headers.

## Reproducible builds

The Makefile passes `-trimpath` and `-ldflags="-s -w"` so the output is
deterministic across machines given the same Go toolchain and source tree.
To verify, build twice on different hosts and compare SHA-256:

```bash
sha256sum web/re2.wasm
```
