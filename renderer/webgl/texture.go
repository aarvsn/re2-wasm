//go:build js && wasm

// Package webgl (texture.go) uploads decoded RGBA8 pixel data to the GPU
// as a WebGL2Texture and returns a TextureID the renderer can reference.
package webgl

import (
	"errors"
	"syscall/js"

	"github.com/aarvsn/re2-wasm/renderer/common"
)

// Texture is a GPU-side texture handle.
type Texture struct {
	id      common.TextureID
	glObj   js.Value
	width   int
	height  int
	format  common.PixelFormat
}

// UploadTexture uploads RGBA8 pixel data to a fresh GPU texture. The data
// slice must be width*height*4 bytes.
func (r *Renderer) UploadTexture(width, height int, rgba []byte) (*Texture, error) {
	if !r.gl.Truthy() {
		return nil, errors.New("webgl: GL context not available")
	}
	if width <= 0 || height <= 0 {
		return nil, errors.New("webgl: texture dimensions must be positive")
	}
	if len(rgba) != width*height*4 {
		return nil, errors.New("webgl: rgba slice length mismatch")
	}
	tex := r.gl.Call("createTexture")
	r.gl.Call("activeTexture", r.gl.Get("TEXTURE0"))
	r.gl.Call("bindTexture", r.gl.Get("TEXTURE_2D"), tex)
	// Use UNPACK_FLIP_Y_WEBGL so the image renders the right way up.
	r.gl.Call("pixelStorei", r.gl.Get("UNPACK_FLIP_Y_WEBGL"), true)
	r.gl.Call("texImage2D",
		r.gl.Get("TEXTURE_2D"), 0, r.gl.Get("RGBA"),
		width, height, 0,
		r.gl.Get("RGBA"), r.gl.Get("UNSIGNED_BYTE"),
		arrayToJS(rgba))
	r.gl.Call("texParameteri", r.gl.Get("TEXTURE_2D"), r.gl.Get("TEXTURE_MIN_FILTER"), r.gl.Get("LINEAR"))
	r.gl.Call("texParameteri", r.gl.Get("TEXTURE_2D"), r.gl.Get("TEXTURE_MAG_FILTER"), r.gl.Get("LINEAR"))
	r.gl.Call("texParameteri", r.gl.Get("TEXTURE_2D"), r.gl.Get("TEXTURE_WRAP_S"), r.gl.Get("CLAMP_TO_EDGE"))
	r.gl.Call("texParameteri", r.gl.Get("TEXTURE_2D"), r.gl.Get("TEXTURE_WRAP_T"), r.gl.Get("CLAMP_TO_EDGE"))

	r.stats.Textures++
	t := &Texture{
		id:     common.TextureID(r.stats.Textures),
		glObj:  tex,
		width:  width,
		height: height,
		format: common.PixelFormatRGBA8,
	}
	return t, nil
}

// Delete releases the texture on the GPU.
func (t *Texture) Delete(gl js.Value) {
	if t.glObj.Truthy() {
		gl.Call("deleteTexture", t.glObj)
		t.glObj = js.Value{}
	}
}

// ID returns the texture's opaque handle.
func (t *Texture) ID() common.TextureID { return t.id }

// Width / Height return the texture's dimensions.
func (t *Texture) Width() int  { return t.width }
func (t *Texture) Height() int { return t.height }

// arrayToJS copies a Go []byte into a fresh JS Uint8Array view. We can't
// use js.CopyBytesToJS for texImage2D because it expects an ArrayBuffer
// view that the GL driver can read directly.
func arrayToJS(b []byte) js.Value {
	arr := js.Global().Get("Uint8Array").New(len(b))
	js.CopyBytesToJS(arr, b)
	return arr
}
