//go:build js && wasm

// Package webgl (shaders.go) holds the GLSL source strings and the helper
// functions that compile/link them into WebGLPrograms. Keeping shader source
// in one place lets Phase 3 extend the catalogue without touching the
// renderer core.
package webgl

import (
	"errors"
	"syscall/js"
)

// ShaderID names a known shader program. Phase 1 has only the clear-colour
// renderer; Phase 3 adds textured and unlit programs.
type ShaderID int

// Catalogue of known shaders.
const (
	ShaderNone ShaderID = iota
	ShaderUnlitFlat   // flat-colour triangle, no lighting
	ShaderUnlitTextured // textured, no lighting
	ShaderSprite       // 2D sprite batcher
)

// vertexSource returns the GLSL ES 3.00 vertex shader for the given program.
func vertexSource(id ShaderID) string {
	switch id {
	case ShaderUnlitFlat:
		return `#version 300 es
in vec3 a_pos;
in vec4 a_col;
uniform mat4 u_mvp;
out vec4 v_col;
void main() {
  gl_Position = u_mvp * vec4(a_pos, 1.0);
  v_col = a_col;
}`
	case ShaderUnlitTextured:
		return `#version 300 es
in vec3 a_pos;
in vec2 a_uv;
uniform mat4 u_mvp;
out vec2 v_uv;
void main() {
  gl_Position = u_mvp * vec4(a_pos, 1.0);
  v_uv = a_uv;
}`
	case ShaderSprite:
		return `#version 300 es
in vec2 a_pos;
in vec2 a_uv;
in vec4 a_col;
uniform vec2 u_resolution;
out vec2 v_uv;
out vec4 v_col;
void main() {
  // Pixel-space to NDC: ((pos / res) * 2 - 1) with Y flipped.
  vec2 ndc = vec2(
    (a_pos.x / u_resolution.x) * 2.0 - 1.0,
    1.0 - (a_pos.y / u_resolution.y) * 2.0
  );
  gl_Position = vec4(ndc, 0.0, 1.0);
  v_uv = a_uv;
  v_col = a_col;
}`
	default:
		return ""
	}
}

// fragmentSource returns the GLSL ES 3.00 fragment shader for the given
// program.
func fragmentSource(id ShaderID) string {
	switch id {
	case ShaderUnlitFlat:
		return `#version 300 es
precision mediump float;
in vec4 v_col;
out vec4 frag;
void main() { frag = v_col; }`
	case ShaderUnlitTextured:
		return `#version 300 es
precision mediump float;
in vec2 v_uv;
uniform sampler2D u_tex;
out vec4 frag;
void main() { frag = texture(u_tex, v_uv); }`
	case ShaderSprite:
		return `#version 300 es
precision mediump float;
in vec2 v_uv;
in vec4 v_col;
uniform sampler2D u_tex;
out vec4 frag;
void main() { frag = texture(u_tex, v_uv) * v_col; }`
	default:
		return ""
	}
}

// Program is a compiled and linked WebGLProgram plus its attribute /
// uniform locations.
type Program struct {
	program js.Value
	id      ShaderID
	// Cached uniform locations.
	uniforms map[string]js.Value
	// Cached attribute locations.
	attributes map[string]int
}

// CompileProgram compiles and links the named program. Returns a
// ShaderCompileError if either stage fails.
func (r *Renderer) CompileProgram(id ShaderID) (*Program, error) {
	if !r.gl.Truthy() {
		return nil, errors.New("webgl: GL context not available")
	}
	vs, err := r.compileStage(id, "vertex")
	if err != nil {
		return nil, err
	}
	fs, err := r.compileStage(id, "fragment")
	if err != nil {
		r.gl.Call("deleteShader", vs)
		return nil, err
	}
	prog := r.gl.Call("createProgram")
	r.gl.Call("attachShader", prog, vs)
	r.gl.Call("attachShader", prog, fs)
	r.gl.Call("linkProgram", prog)
	r.gl.Call("deleteShader", vs)
	r.gl.Call("deleteShader", fs)
	if !r.gl.Call("getProgramParameter", prog, r.gl.Get("LINK_STATUS")).Bool() {
		log := r.gl.Call("getProgramInfoLog", prog).String()
		r.gl.Call("deleteProgram", prog)
		return nil, errors.New("webgl: link failed: " + log)
	}
	p := &Program{
		program:    prog,
		id:         id,
		uniforms:   make(map[string]js.Value),
		attributes: make(map[string]int),
	}
	return p, nil
}

// compileStage compiles one shader stage (vertex or fragment).
func (r *Renderer) compileStage(id ShaderID, stage string) (js.Value, error) {
	var src string
	var shaderType js.Value
	switch stage {
	case "vertex":
		src = vertexSource(id)
		shaderType = r.gl.Get("VERTEX_SHADER")
	case "fragment":
		src = fragmentSource(id)
		shaderType = r.gl.Get("FRAGMENT_SHADER")
	default:
		return js.Value{}, errors.New("webgl: unknown stage " + stage)
	}
	if src == "" {
		return js.Value{}, errors.New("webgl: no source for shader")
	}
	sh := r.gl.Call("createShader", shaderType)
	r.gl.Call("shaderSource", sh, src)
	r.gl.Call("compileShader", sh)
	if !r.gl.Call("getShaderParameter", sh, r.gl.Get("COMPILE_STATUS")).Bool() {
		log := r.gl.Call("getShaderInfoLog", sh).String()
		r.gl.Call("deleteShader", sh)
		return js.Value{}, ShaderCompileError{Stage: stage, InfoLog: log}
	}
	return sh, nil
}

// Use makes this program the active one.
func (p *Program) Use(gl js.Value) { gl.Call("useProgram", p.program) }

// Uniform looks up (and caches) a uniform location by name.
func (p *Program) Uniform(gl js.Value, name string) js.Value {
	if loc, ok := p.uniforms[name]; ok {
		return loc
	}
	loc := gl.Call("getUniformLocation", p.program, name)
	p.uniforms[name] = loc
	return loc
}

// Attrib looks up (and caches) an attribute location by name. Returns -1
// if the attribute was optimised out by the compiler.
func (p *Program) Attrib(gl js.Value, name string) int {
	if loc, ok := p.attributes[name]; ok {
		return loc
	}
	loc := gl.Call("getAttribLocation", p.program, name).Int()
	p.attributes[name] = loc
	return loc
}

// Delete releases the program on the GPU.
func (p *Program) Delete(gl js.Value) {
	if p.program.Truthy() {
		gl.Call("deleteProgram", p.program)
		p.program = js.Value{}
	}
}
