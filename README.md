## re2-wasm

An open-source browser port of the original Resident Evil 2 (1998) based on OpenBiohazard2. It compiles the engine to WebAssembly and runs entirely in modern web browsers.

## Features

- WebAssembly (WASM)
- WebGL 2 renderer
- Web Audio API
- Keyboard, mouse, and gamepad support
- IndexedDB save storage
- Cross-platform

## Build
```bash
git clone https://github.com/aarvsn/re2-wasm.git
cd re2-wasm

make wasm
make serve
```

Then open http://localhost:8080 in your browser.


## Browser Support

- Chromium
- Firefox
- Safari
- Microsoft Edge

## Legal

This project contains no Resident Evil 2 assets. Players must provide their own original game files.

Resident Evil is a trademark of Capcom. This project is not affiliated with or endorsed by Capcom.

## License

MIT License.

## Credits

- OpenBiohazard2
- Go WebAssembly