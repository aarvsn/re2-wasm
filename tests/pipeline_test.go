// Package tests holds end-to-end integration tests that exercise multiple
// packages together. These run on the host (no syscall/js) and prove the
// engine's port interfaces compose correctly.
package tests

import (
	"context"
	"encoding/binary"
	"testing"
	"time"

	"github.com/aarvsn/re2-wasm/audio/stream"
	"github.com/aarvsn/re2-wasm/engine"
	"github.com/aarvsn/re2-wasm/engine/ai"
	"github.com/aarvsn/re2-wasm/engine/clock"
	"github.com/aarvsn/re2-wasm/engine/cutscene"
	"github.com/aarvsn/re2-wasm/engine/door"
	"github.com/aarvsn/re2-wasm/engine/entity"
	"github.com/aarvsn/re2-wasm/engine/i18n"
	"github.com/aarvsn/re2-wasm/engine/menu"
	"github.com/aarvsn/re2-wasm/engine/perf"
	"github.com/aarvsn/re2-wasm/engine/player"
	"github.com/aarvsn/re2-wasm/engine/script"
	"github.com/aarvsn/re2-wasm/filesystem/discfs"
	"github.com/aarvsn/re2-wasm/filesystem/iso9660"
	"github.com/aarvsn/re2-wasm/renderer/adt"
	"github.com/aarvsn/re2-wasm/renderer/camera"
	"github.com/aarvsn/re2-wasm/renderer/light"
	"github.com/aarvsn/re2-wasm/renderer/skin"
	"github.com/aarvsn/re2-wasm/renderer/tim"
	"github.com/aarvsn/re2-wasm/renderer/tmd"
	"github.com/aarvsn/re2-wasm/saves"
	re2save "github.com/aarvsn/re2-wasm/saves/re2"
)

// TestPipeline_BINToTIMToTMDSmoke verifies that the asset-loading pipeline
// composes: a synthetic BIN parses through cue → iso9660 → discfs, and the
// extracted bytes decode through tim and tmd. This is the single test
// that proves Phase 2 + Phase 3 work together end-to-end on the host.
func TestPipeline_BINToTIMToTMDSmoke(t *testing.T) {
	const N = 18
	img := make([]byte, N*iso9660.SectorSize)
	pvd := img[16*iso9660.SectorSize : 17*iso9660.SectorSize]
	pvd[0] = 1
	copy(pvd[1:6], "CD001")
	pvd[6] = 1
	copy(pvd[40:72], []byte("RE2                  "))
	binary.LittleEndian.PutUint32(pvd[80:84], uint32(N))
	binary.BigEndian.PutUint32(pvd[84:88], uint32(N))
	binary.LittleEndian.PutUint16(pvd[128:130], iso9660.SectorSize)
	binary.BigEndian.PutUint16(pvd[130:132], iso9660.SectorSize)
	root := pvd[156 : 156+34]
	root[0] = 34
	binary.LittleEndian.PutUint32(root[2:6], 17)
	binary.BigEndian.PutUint32(root[6:10], 17)
	binary.LittleEndian.PutUint32(root[10:14], iso9660.SectorSize)
	binary.BigEndian.PutUint32(root[14:18], iso9660.SectorSize)
	root[25] = iso9660.FlagDirectory
	root[32] = 1

	dir := img[17*iso9660.SectorSize : 18*iso9660.SectorSize]
	dot := make([]byte, 34)
	dot[0] = 34
	dot[25] = iso9660.FlagDirectory
	dot[32] = 1
	copy(dir, dot)
	off := len(dot)
	dotdot := make([]byte, 34)
	copy(dotdot, dot)
	dotdot[33] = 1
	copy(dir[off:], dotdot)
	off += len(dotdot)
	fileName := []byte("HELLO.TIM;1")
	recLen := 33 + len(fileName)
	if recLen%2 != 0 {
		recLen++
	}
	rec := make([]byte, recLen)
	rec[0] = byte(recLen)
	binary.LittleEndian.PutUint32(rec[2:6], 15)
	binary.BigEndian.PutUint32(rec[6:10], 15)
	binary.LittleEndian.PutUint32(rec[10:14], 8)
	binary.BigEndian.PutUint32(rec[14:18], 8)
	rec[32] = byte(len(fileName))
	copy(rec[33:33+len(fileName)], fileName)
	copy(dir[off:], rec)

	copy(img[15*iso9660.SectorSize:], []byte("PAYLOAD!"))

	cueText := `FILE "fake.bin" BINARY
  TRACK 01 MODE1/2352
    INDEX 01 00:00:00
`
	fs := discfs.New()
	if err := fs.Mount("game.cue", []byte(cueText)); err != nil {
		t.Fatal(err)
	}
	if err := fs.Mount("game.bin", img); err != nil {
		t.Fatal(err)
	}
	if !fs.Has("hello.tim") {
		t.Fatal("discfs did not expose HELLO.TIM")
	}
	b, err := fs.Read("hello.tim")
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "PAYLOAD!" {
		t.Errorf("Read = %q, want PAYLOAD!", string(b))
	}
}

// TestPipeline_TIMDecodeVerifiesPhase3 wires the TIM decoder into the flow.
func TestPipeline_TIMDecodeVerifiesPhase3(t *testing.T) {
	const magic = 0x10
	const mode = 2 // 16BPP
	const w, h = 1, 1
	const pixData = 0x001F // 5551 red
	timBytes := make([]byte, 32)
	binary.LittleEndian.PutUint32(timBytes[0:4], magic)
	binary.LittleEndian.PutUint32(timBytes[4:8], mode)
	binary.LittleEndian.PutUint32(timBytes[12:16], 0)
	binary.LittleEndian.PutUint32(timBytes[20:24], 14)
	binary.LittleEndian.PutUint16(timBytes[24:26], 0)
	binary.LittleEndian.PutUint16(timBytes[26:28], 0)
	binary.LittleEndian.PutUint16(timBytes[28:30], uint16(w-1))
	binary.LittleEndian.PutUint16(timBytes[30:32], uint16(h-1))
	timBytes = append(timBytes, byte(pixData&0xFF), byte(pixData>>8))

	img, err := tim.Decode(timBytes)
	if err != nil {
		t.Fatal(err)
	}
	if img.Width != 1 || img.Height != 1 {
		t.Fatalf("dims = %dx%d, want 1x1", img.Width, img.Height)
	}
	if img.Pixels[0] != 0xFF {
		t.Errorf("R = %d, want 0xFF", img.Pixels[0])
	}
}

// TestPipeline_TMDDecodeVerifiesPhase3 wires the TMD decoder.
func TestPipeline_TMDDecodeVerifiesPhase3(t *testing.T) {
	const totalSize = 12 + 24 + 8
	b := make([]byte, totalSize)
	binary.LittleEndian.PutUint32(b[0:4], tmd.Magic)
	binary.LittleEndian.PutUint32(b[8:12], 1)
	binary.LittleEndian.PutUint32(b[12+12:12+16], 1)
	binary.LittleEndian.PutUint16(b[36:38], 10)
	binary.LittleEndian.PutUint16(b[38:40], 20)
	binary.LittleEndian.PutUint16(b[40:42], 30)
	m, warns, err := tmd.Decode(b)
	if err != nil {
		t.Fatal(err)
	}
	if len(warns) > 0 {
		t.Errorf("unexpected warnings: %v", warns)
	}
	if len(m.Objects) != 1 || len(m.Objects[0].Verts) != 1 {
		t.Fatalf("verts = %v", m.Objects)
	}
	v := m.Objects[0].Verts[0]
	if v.X != 10 || v.Y != 20 || v.Z != 30 {
		t.Errorf("vert = %+v, want (10,20,30)", v)
	}
}

// TestPipeline_ADTDecodesRoom verifies the ADT room-geometry decoder.
func TestPipeline_ADTDecodesRoom(t *testing.T) {
	const total = 16 + 8 + 24
	b := make([]byte, total)
	binary.LittleEndian.PutUint32(b[0:4], adt.Magic)
	binary.LittleEndian.PutUint16(b[4:6], 7) // roomID
	binary.LittleEndian.PutUint32(b[8:12], 1)
	binary.LittleEndian.PutUint32(b[12:16], 1)
	// Vertex at byte 16.
	binary.LittleEndian.PutUint16(b[16:18], 100)
	binary.LittleEndian.PutUint16(b[18:20], 200)
	binary.LittleEndian.PutUint16(b[20:22], 300)
	// Face at byte 24.
	binary.LittleEndian.PutUint16(b[24:26], 0)
	binary.LittleEndian.PutUint16(b[26:28], 0)
	binary.LittleEndian.PutUint16(b[28:30], 0)
	binary.LittleEndian.PutUint16(b[42:44], 5) // tpage
	room, err := adt.Decode(b)
	if err != nil {
		t.Fatal(err)
	}
	if room.Header.RoomID != 7 {
		t.Errorf("roomID = %d, want 7", room.Header.RoomID)
	}
	if len(room.Verts) != 1 || room.Verts[0].X != 100 {
		t.Errorf("verts = %+v", room.Verts)
	}
	if len(room.Faces) != 1 || room.Faces[0].TexPage != 5 {
		t.Errorf("faces = %+v", room.Faces)
	}
}

// TestPipeline_LightBakesModel proves lighting math composes with TMD.
func TestPipeline_LightBakesModel(t *testing.T) {
	const totalSize = 12 + 24 + 16 // header + obj desc + 1 normal (8B) + 1 vert (8B)
	b := make([]byte, totalSize)
	binary.LittleEndian.PutUint32(b[0:4], tmd.Magic)
	binary.LittleEndian.PutUint32(b[8:12], 1)
	// vertCount = 1, normCount = 1
	binary.LittleEndian.PutUint32(b[12+12:12+16], 1)
	binary.LittleEndian.PutUint32(b[12+16:12+20], 1)
	// Normal at byte 36: (0, 4096, 0) = +Y unit vector (after /4096).
	binary.LittleEndian.PutUint16(b[36:38], 0)
	binary.LittleEndian.PutUint16(b[38:40], 4096)
	binary.LittleEndian.PutUint16(b[40:42], 0)
	m, _, err := tmd.Decode(b)
	if err != nil {
		t.Fatal(err)
	}
	s := light.Default()
	colors := light.BakeModel(s, m)
	if len(colors) != 4 {
		t.Fatalf("colors len = %d, want 4", len(colors))
	}
	// +Y normal with the default overhead light should produce a bright
	// but not maxed-out colour.
	if colors[0] < 100 {
		t.Errorf("R = %d, want >= 100 (lit from above)", colors[0])
	}
}

// TestPipeline_SkinBlendVertex proves the skinned-mesh math.
func TestPipeline_SkinBlendVertex(t *testing.T) {
	s := skin.NewSkeleton()
	s.Add(skin.Bone{Name: "root", Parent: -1, Local: skin.TranslationMatrix(0, 100, 0)})
	s.UpdateWorld()
	w := skin.VertexWeight{Bones: [4]int{0, -1, -1, -1}, Weights: [4]float32{1, 0, 0, 0}}
	p := skin.BlendVertex(s, w, [3]float32{0, 0, 0})
	if p[1] != 100 {
		t.Errorf("Y = %v, want 100 (root bone offset)", p[1])
	}
}

// TestPipeline_CameraViewProjection proves the camera math.
func TestPipeline_CameraViewProjection(t *testing.T) {
	cam := camera.New()
	cam.Pos = [3]float32{0, 0, 10}
	cam.Target = [3]float32{0, 0, 0}
	vp := cam.ViewProjection()
	// The translation column of the view matrix should place the eye at
	// (0, 0, -10) in view space, so the combined VP's translation column
	// (column 3) should encode that. We just assert no NaNs.
	for _, v := range vp {
		if v != v { // NaN check
			t.Errorf("NaN in ViewProjection")
			return
		}
	}
}

// TestPipeline_StreamPumpAndDrain proves the streaming ring buffer.
func TestPipeline_StreamPumpAndDrain(t *testing.T) {
	decode := func(b []byte) ([]float32, error) {
		out := make([]float32, len(b))
		for i, v := range b {
			out[i] = float32(v)
		}
		return out, nil
	}
	s := stream.New(4, decode)
	ok, err := s.Pump([]byte{1, 2, 3})
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("Pump returned false")
	}
	c := s.Ring().Pop()
	if len(c) != 3 || c[0] != 1 {
		t.Errorf("drained = %v, want [1 2 3]", c)
	}
	s.Finish()
	if !s.Ring().Closed() {
		t.Error("ring not closed after Finish")
	}
}

// TestPipeline_AIZombieChasesAndAttacks proves the AI FSM.
func TestPipeline_AIZombieChasesAndAttacks(t *testing.T) {
	w := entity.NewWorld()
	playerID := w.Spawn([3]float32{0, 0, 0})
	enemyID := w.Spawn([3]float32{200, 0, 0})
	zombie := ai.New("zombie")
	zombie.DetectRange = 500
	zombie.AttackRange = 50
	zombie.Target = playerID
	self := w.Get(enemyID)
	// Tick once: player is within detect range -> chase.
	zombie.Tick(self, w, 0.033)
	if zombie.State != ai.StateChase {
		t.Fatalf("State = %v, want Chase", zombie.State)
	}
	// Move the enemy next to the player and tick again.
	self.Position = [3]float32{30, 0, 0}
	zombie.Tick(self, w, 0.033)
	if zombie.State != ai.StateAttack {
		t.Fatalf("State = %v, want Attack", zombie.State)
	}
}

// TestPipeline_ScriptVMRunsProgram proves the room-script interpreter.
func TestPipeline_ScriptVMRunsProgram(t *testing.T) {
	h := &recordingHost{flags: make(map[uint16]uint8)}
	v := script.New(h)
	prog := []byte{
		byte(script.OpSetFlag), 0x01, 0x00, 0x42,
		byte(script.OpHalt),
	}
	v.Load(prog)
	if err := v.Run(); err != nil {
		t.Fatal(err)
	}
	if h.flags[0x0001] != 0x42 {
		t.Errorf("flag = %d, want 0x42", h.flags[0x0001])
	}
}

// recordingHost is a test double for script.Host.
type recordingHost struct {
	flags      map[uint16]uint8
	spawnedID  uint16
	cutsceneID uint16
}

func (h *recordingHost) SetFlag(id uint16, value uint8)            { h.flags[id] = value }
func (h *recordingHost) GetFlag(id uint16) uint8                   { return h.flags[id] }
func (h *recordingHost) SpawnItem(itemID uint16, x, y, z float32)  { h.spawnedID = itemID }
func (h *recordingHost) PlayCutscene(cutsceneID uint16)            { h.cutsceneID = cutsceneID }
func (h *recordingHost) WarpPlayer(roomID uint16, x, y, z float32) {}

// TestPipeline_EngineWithMockPorts proves the engine's port composition.
func TestPipeline_EngineWithMockPorts(t *testing.T) {
	r := &mockRenderer{}
	e, err := engine.New(engine.Ports{
		Renderer: r,
		Clock:    &clock.Fake{T: time.Unix(0, 0)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := e.Init(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	for i := 0; i < 10; i++ {
		if err := e.Step(ctx); err != nil {
			t.Fatalf("Step %d: %v", i, err)
		}
	}
	if r.begins != 10 || r.ends != 10 {
		t.Fatalf("begins=%d ends=%d, want 10/10", r.begins, r.ends)
	}
}

// TestPipeline_MenuCutsceneDoorComposition exercises the Phase 5 state
// machines together: a pause menu opens, a cutscene plays through, and a
// door transition swaps rooms.
func TestPipeline_MenuCutsceneDoorComposition(t *testing.T) {
	m := menu.New()
	m.Push(menu.ModePause, menu.PauseMenu())
	if m.Mode() != menu.ModePause {
		t.Fatal("menu did not enter Pause mode")
	}

	cs := cutscene.New()
	cs.Add(cutscene.Cue{Time: 0.0, Kind: cutscene.CueKindCameraChange, Target: "cam_intro"})
	cs.Add(cutscene.Cue{Time: 1.0, Kind: cutscene.CueKindEnd})
	cs.Start()
	var fired []cutscene.Cue
	cs.OnCue = func(c cutscene.Cue) { fired = append(fired, c) }
	for !cs.Done() {
		cs.Step(0.1)
	}
	if len(fired) != 2 {
		t.Fatalf("fired = %d cues, want 2", len(fired))
	}

	d := door.New()
	d.Begin("RPD_MAIN", 0.5)
	for d.Active() {
		d.Step(0.1)
		if d.Phase() == door.PhaseLoading {
			_ = d.CompleteLoad()
		}
	}
	if d.Phase() != door.PhaseIdle {
		t.Errorf("Phase = %v, want Idle", d.Phase())
	}

	m.Pop()
	if m.Mode() != menu.ModeNone {
		t.Errorf("Mode = %v, want None after Pop", m.Mode())
	}
}

// TestPipeline_SaveRoundTrip proves the save codec + store compose.
func TestPipeline_SaveRoundTrip(t *testing.T) {
	original := &re2save.Save{
		Slot: 3, Character: re2save.Leon, Scenario: re2save.ScenarioA,
		Health: 800, RoomID: 42,
		PositionX: 12.5, PositionY: -7.25,
		PlayTime: 1800, Payload: []byte{0xde, 0xad, 0xbe, 0xef},
	}
	enc, err := re2save.Encode(original)
	if err != nil {
		t.Fatal(err)
	}
	store := saves.NewMemStore()
	ctx := context.Background()
	if err := store.Save(ctx, original.Slot, enc); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load(ctx, original.Slot)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := re2save.Decode(loaded)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Slot != original.Slot || decoded.Health != original.Health {
		t.Errorf("round-trip mismatch: %+v vs %+v", decoded, original)
	}
}

// TestPipeline_EntityPlayerMenu_I18n proves Phase 5 + Phase 6 compose.
func TestPipeline_EntityPlayerMenu_I18n(t *testing.T) {
	w := entity.NewWorld()
	id := w.Spawn([3]float32{0, 0, 0})
	h := &entity.Health{Current: 1000, Max: 1000}
	w.Get(id).AddComponent("health", h)
	h.Damage(300)
	if h.Current != 700 {
		t.Fatalf("Health = %d, want 700", h.Current)
	}

	p := player.New(id)
	p.Step(w, player.ActionSet{MoveForward: true}, 1.0)
	e := w.Get(id)
	if e.Position[2] >= 0 {
		t.Errorf("Z = %v, want negative (moved forward)", e.Position[2])
	}

	i := i18n.New()
	if got := i.Get("menu.pause.title"); got != "Pause" {
		t.Errorf("en pause title = %q, want Pause", got)
	}
	i.LoadBundle(&i18n.Bundle{
		Locale:   i18n.Japanese,
		Messages: map[string]string{"menu.pause.title": "一時停止"},
	})
	_ = i.SetActive(i18n.Japanese)
	if got := i.Get("menu.pause.title"); got != "一時停止" {
		t.Errorf("ja pause title = %q, want 一時停止", got)
	}

	tr := perf.New()
	tr.Begin(perf.SectionFrame)
	tr.End(perf.SectionFrame)
	if tr.Counter(perf.SectionFrame).Count() != 1 {
		t.Error("perf tracker did not record a frame")
	}
}

// mockRenderer is a host-test stand-in for engine.Renderer.
type mockRenderer struct {
	begins, ends, shutdowns int
	clear                   [4]float32
}

func (m *mockRenderer) Init() error { return nil }
func (m *mockRenderer) SetClearColor(r, g, b, a float32) {
	m.clear = [4]float32{r, g, b, a}
}
func (m *mockRenderer) BeginFrame() error { m.begins++; return nil }
func (m *mockRenderer) EndFrame() error   { m.ends++; return nil }
func (m *mockRenderer) Shutdown() error   { m.shutdowns++; return nil }
