// Module re2-wasm is a browser port of the original Resident Evil 2 (1998)
// built on top of the OpenBiohazard2 Go engine.
//
// The module targets GOOS=js GOARCH=wasm for the primary build but is also
// unit-testable on the host (GOOS=linux GOARCH=amd64 etc.) because every
// syscall/js call is isolated behind the packages under wasm/ and renderer/.
module github.com/aarvsn/re2-wasm

go 1.26
