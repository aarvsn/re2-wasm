//go:build js && wasm

// Package audio (positional.go) wires up positional 3D audio via the Web
// Audio API's PannerNode. The engine places the listener at the camera
// position and positions sound sources in world space; the browser's
// spatialiser applies HRTF / panning automatically.
package audio

import (
	"errors"
	"syscall/js"
)

// Panner wraps a PannerNode and its source BufferSourceNode. Create one
// via Manager.NewPanner, queue samples with Play, and call UpdatePosition
// each frame to move the source.
type Panner struct {
	ctx     js.Value
	panner  js.Value
	source  js.Value
	master  js.Value
	playing bool
}

// NewPanner creates a positional source connected to the master gain.
// The panner is configured for HRTF spatialisation at 44.1 kHz.
func (m *Manager) NewPanner() (*Panner, error) {
	m.mu.Lock()
	ctx := m.ctx
	master := m.master
	m.mu.Unlock()
	if !ctx.Truthy() {
		return nil, errors.New("audio: AudioContext not initialised")
	}
	panner := ctx.Call("createPanner")
	panner.Set("panningModel", "HRTF")
	panner.Set("distanceModel", "inverse")
	panner.Set("refDistance", 1)
	panner.Set("maxDistance", 100)
	panner.Set("rolloffFactor", 1)
	panner.Call("connect", master)
	return &Panner{ctx: ctx, panner: panner, master: master}, nil
}

// SetListenerPosition places the listener (the camera) in world space.
// All PannerNodes are positioned relative to this point.
func (m *Manager) SetListenerPosition(x, y, z float32) {
	m.mu.Lock()
	ctx := m.ctx
	m.mu.Unlock()
	if !ctx.Truthy() {
		return
	}
	listener := ctx.Get("listener")
	if !listener.Truthy() {
		return
	}
	listener.Call("setPosition", float64(x), float64(y), float64(z))
}

// SetListenerOrientation points the listener. fx/fy/fz is the forward
// vector; ux/uy/uz is the up vector. Both must be normalised.
func (m *Manager) SetListenerOrientation(fx, fy, fz, ux, uy, uz float32) {
	m.mu.Lock()
	ctx := m.ctx
	m.mu.Unlock()
	if !ctx.Truthy() {
		return
	}
	listener := ctx.Get("listener")
	if !listener.Truthy() {
		return
	}
	listener.Call("setOrientation", float64(fx), float64(fy), float64(fz),
		float64(ux), float64(uy), float64(uz))
}

// UpdatePosition moves the panner to a new world-space location.
func (p *Panner) UpdatePosition(x, y, z float32) {
	if !p.panner.Truthy() {
		return
	}
	p.panner.Call("setPosition", float64(x), float64(y), float64(z))
}

// Play decodes the given mono float32 samples and starts playback. The
// panner must already be connected to the master gain.
func (p *Panner) Play(samples []float32, sampleRate int) error {
	if !p.ctx.Truthy() {
		return errors.New("audio: context not ready")
	}
	if p.playing {
		return errors.New("audio: panner already playing")
	}
	buf := p.ctx.Call("createBuffer", 1, len(samples), sampleRate)
	ch := buf.Call("getChannelData", 0)
	// Copy samples into the AudioBuffer's Float32Array.
	arr := js.Global().Get("Float32Array").New(len(samples))
	for i, v := range samples {
		arr.SetIndex(i, v)
	}
	ch.Call("set", arr)
	src := p.ctx.Call("createBufferSource")
	src.Set("buffer", buf)
	src.Call("connect", p.panner)
	src.Call("start")
	p.source = src
	p.playing = true
	// Auto-release when playback ends.
	src.Call("addEventListener", "ended", js.FuncOf(func(this js.Value, args []js.Value) any {
		p.playing = false
		return nil
	}))
	return nil
}

// Stop halts playback and releases the source node.
func (p *Panner) Stop() {
	if p.source.Truthy() && p.playing {
		p.source.Call("stop")
		p.playing = false
	}
}

// Delete disconnects the panner from the master gain. The browser GCs the
// node after the source's "ended" event fires.
func (p *Panner) Delete() {
	if p.panner.Truthy() {
		p.panner.Call("disconnect")
		p.panner = js.Value{}
	}
}
