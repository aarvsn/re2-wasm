//go:build js && wasm

// Package webgl (sprite.go) implements a 2D sprite batcher. The batcher
// accumulates quads (each with a texture, screen-space rect, UV rect, and
// tint) into a single dynamic vertex buffer and flushes them in one draw
// call. This is the workhorse used for RE2's HUD, menus, and pre-rendered
// backgrounds.
package webgl

import (
	"errors"
	"syscall/js"
)

// SpriteBatch is a dynamic quad batcher. Create one with NewSpriteBatch,
// call Begin to bind the sprite shader, queue sprites via Quad, and call
// End to flush.
type SpriteBatch struct {
	r    *Renderer
	prog *Program
	vbo  js.Value
	ibuf js.Value
	vao  js.Value

	// CPU-side vertex buffer. 8 floats per vertex (pos.xy, uv.xy, col.rgba),
	// 4 vertices per quad. We grow capacity lazily.
	verts    []float32
	maxQuads int

	// Active texture; we flush when the caller changes textures.
	activeTex *Texture
}

// Vertex layout for the sprite shader. Keep in sync with ShaderSprite.
const (
	spriteFloatsPerVertex = 8
	spriteVertsPerQuad    = 4
	spriteIndicesPerQuad  = 6
)

// NewSpriteBatch constructs a batcher with the given maximum quad count.
// maxQuads must be at least 1; values above 16384 are clamped.
func NewSpriteBatch(r *Renderer, maxQuads int) (*SpriteBatch, error) {
	if r == nil || !r.gl.Truthy() {
		return nil, errors.New("webgl: renderer not ready")
	}
	if maxQuads < 1 {
		maxQuads = 1
	}
	if maxQuads > 16384 {
		maxQuads = 16384
	}
	prog, err := r.CompileProgram(ShaderSprite)
	if err != nil {
		return nil, err
	}
	gl := r.gl

	// Build a static index buffer for the max quad count.
	indices := make([]uint16, maxQuads*spriteIndicesPerQuad)
	for i := 0; i < maxQuads; i++ {
		v := uint16(i * 4)
		off := i * 6
		indices[off+0] = v + 0
		indices[off+1] = v + 1
		indices[off+2] = v + 2
		indices[off+3] = v + 0
		indices[off+4] = v + 2
		indices[off+5] = v + 3
	}
	ibuf := gl.Call("createBuffer")
	gl.Call("bindBuffer", gl.Get("ELEMENT_ARRAY_BUFFER"), ibuf)
	idxArr := js.Global().Get("Uint16Array").New(len(indices))
	for i, v := range indices {
		idxArr.SetIndex(i, v)
	}
	gl.Call("bufferData", gl.Get("ELEMENT_ARRAY_BUFFER"), idxArr, gl.Get("STATIC_DRAW"))

	vbo := gl.Call("createBuffer")
	vao := gl.Call("createVertexArray")
	gl.Call("bindVertexArray", vao)
	gl.Call("bindBuffer", gl.Get("ARRAY_BUFFER"), vbo)
	gl.Call("bufferData", gl.Get("ARRAY_BUFFER"),
		maxQuads*spriteVertsPerQuad*spriteFloatsPerVertex*4,
		gl.Get("DYNAMIC_DRAW"))

	// a_pos = location 0, 2 floats
	gl.Call("enableVertexAttribArray", 0)
	gl.Call("vertexAttribPointer", 0, 2, gl.Get("FLOAT"), false, spriteFloatsPerVertex*4, 0)
	// a_uv = location 1, 2 floats
	gl.Call("enableVertexAttribArray", 1)
	gl.Call("vertexAttribPointer", 1, 2, gl.Get("FLOAT"), false, spriteFloatsPerVertex*4, 8)
	// a_col = location 2, 4 floats
	gl.Call("enableVertexAttribArray", 2)
	gl.Call("vertexAttribPointer", 2, 4, gl.Get("FLOAT"), false, spriteFloatsPerVertex*4, 16)

	gl.Call("bindVertexArray", js.Null())
	gl.Call("bindBuffer", gl.Get("ARRAY_BUFFER"), js.Null())
	gl.Call("bindBuffer", gl.Get("ELEMENT_ARRAY_BUFFER"), js.Null())

	return &SpriteBatch{
		r:        r,
		prog:     prog,
		vbo:      vbo,
		ibuf:     ibuf,
		vao:      vao,
		maxQuads: maxQuads,
		verts:    make([]float32, 0, maxQuads*spriteVertsPerQuad*spriteFloatsPerVertex),
	}, nil
}

// Begin binds the sprite shader and VAO. Call End to flush.
func (b *SpriteBatch) Begin() {
	gl := b.r.gl
	b.prog.Use(gl)
	gl.Call("bindVertexArray", b.vao)
	gl.Call("bindBuffer", gl.Get("ARRAY_BUFFER"), b.vbo)
	gl.Call("bindBuffer", gl.Get("ELEMENT_ARRAY_BUFFER"), b.ibuf)
	gl.Call("disable", gl.Get("DEPTH_TEST"))
	gl.Call("enable", gl.Get("BLEND"))
	gl.Call("blendFunc", gl.Get("SRC_ALPHA"), gl.Get("ONE_MINUS_SRC_ALPHA"))
	w := b.r.canvas.Get("width").Int()
	h := b.r.canvas.Get("height").Int()
	loc := b.prog.Uniform(gl, "u_resolution")
	gl.Call("uniform2f", loc, float32(w), float32(h))
}

// Quad queues one sprite. x/y are top-left in pixel space; w/h are size in
// pixels; u0/v0/u1/v1 are the UV rectangle; r/g/b/a is the tint (0..1).
// If the active texture differs from tex, the batch flushes first.
func (b *SpriteBatch) Quad(tex *Texture, x, y, w, h float32, u0, v0, u1, v1 float32, r, g, bl, a float32) {
	if tex != b.activeTex {
		b.flush()
		b.activeTex = tex
		if tex != nil {
			b.r.gl.Call("activeTexture", b.r.gl.Get("TEXTURE0"))
			b.r.gl.Call("bindTexture", b.r.gl.Get("TEXTURE_2D"), tex.glObj)
			loc := b.prog.Uniform(b.r.gl, "u_tex")
			b.r.gl.Call("uniform1i", loc, 0)
		}
	}
	if len(b.verts)/8/4 >= b.maxQuads {
		b.flush()
	}
	x1, y1 := x+w, y+h
	// 4 vertices, each 8 floats (pos.xy, uv.xy, col.rgba).
	b.verts = append(b.verts,
		x, y, u0, v0, r, g, bl, a,
		x, y1, u0, v1, r, g, bl, a,
		x1, y1, u1, v1, r, g, bl, a,
		x1, y, u1, v0, r, g, bl, a,
	)
}

// End flushes any pending sprites and unbinds the VAO.
func (b *SpriteBatch) End() {
	b.flush()
	b.r.gl.Call("bindVertexArray", js.Null())
}

// flush uploads the CPU vertex buffer and issues one draw call.
func (b *SpriteBatch) flush() {
	if len(b.verts) == 0 {
		return
	}
	gl := b.r.gl
	arr := js.Global().Get("Float32Array").New(len(b.verts))
	for i, v := range b.verts {
		arr.SetIndex(i, v)
	}
	gl.Call("bufferSubData", gl.Get("ARRAY_BUFFER"), 0, arr)
	quads := len(b.verts) / (spriteFloatsPerVertex * spriteVertsPerQuad)
	gl.Call("drawElements", gl.Get("TRIANGLES"), quads*spriteIndicesPerQuad, gl.Get("UNSIGNED_SHORT"), 0, 0)
	b.r.stats.DrawCalls++
	b.verts = b.verts[:0]
}

// Delete releases the GPU resources.
func (b *SpriteBatch) Delete() {
	gl := b.r.gl
	if b.vbo.Truthy() {
		gl.Call("deleteBuffer", b.vbo)
	}
	if b.ibuf.Truthy() {
		gl.Call("deleteBuffer", b.ibuf)
	}
	if b.vao.Truthy() {
		gl.Call("deleteVertexArray", b.vao)
	}
	b.prog.Delete(gl)
}
