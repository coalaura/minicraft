package server

import (
	"sync"
	"testing"

	"github.com/coalaura/minicraft/internal/game"
)

type runtimeTickLog struct {
	mu      sync.Mutex
	entries []string
}

type recordingRuntimeTicker struct {
	label    string
	log      *runtimeTickLog
	position game.BlockPosition
	state    RuntimeEntityState
}

type recordingBlockEntityInteraction struct {
	position game.BlockPosition
	calls    int
}

func (t *recordingRuntimeTicker) BlockEntityType() game.BlockEntityType {
	return game.BlockEntityTypeBarrel
}

func (t *recordingRuntimeTicker) BlockPosition() game.BlockPosition {
	return t.position
}

func (t *recordingRuntimeTicker) Tick(_ *Runtime, _ *ActiveChunk) {
	t.log.mu.Lock()
	defer t.log.mu.Unlock()

	t.log.entries = append(t.log.entries, t.label)
}

func (t *recordingRuntimeTicker) RuntimeEntityState() *RuntimeEntityState {
	return &t.state
}

func (interaction *recordingBlockEntityInteraction) BlockEntityType() game.BlockEntityType {
	return game.BlockEntityTypeBarrel
}

func (interaction *recordingBlockEntityInteraction) BlockPosition() game.BlockPosition {
	return interaction.position
}

func (interaction *recordingBlockEntityInteraction) InteractBlock(_ *Runtime, _ *Session) error {
	interaction.calls++

	return nil
}

func TestRuntimeBlockEntityCapabilitiesAreOptionalAndGeneric(t *testing.T) {
	position := game.BlockPosition{Y: 70}

	runtime := NewRuntime(&game.World{})

	session, _ := newPlacementTestSession(runtime, position)

	runtime.World.SetBlock(position, game.Barrel)

	runtime.setSessionActiveChunks(session, []LoadedChunk{blockLoadedChunk(position)})

	chunk, _ := runtime.ActiveChunk(blockLoadedChunk(position))

	interaction := &recordingBlockEntityInteraction{position: position}

	chunk.SetBlockEntity(position, interaction)

	if _, ticking := any(interaction).(RuntimeBlockEntityTicker); ticking {
		t.Fatal("interaction-only block entity unexpectedly ticks")
	}

	handled, result, _, err := runtime.InteractBlock(session, position)
	if err != nil {
		t.Fatalf("generic block entity interaction: %v", err)
	}

	if !handled || !result.Allowed || !result.Changed || interaction.calls != 1 {
		t.Fatalf("interaction result = handled %v, result %+v, calls %d", handled, result, interaction.calls)
	}

	runtime.World.SetBlock(position, game.Air)

	chunk.SetBlockEntity(position, interaction)

	handled, _, _, err = runtime.InteractBlock(session, position)
	if err != nil {
		t.Fatalf("stale generic block entity interaction: %v", err)
	}

	if handled || interaction.calls != 1 {
		t.Fatalf("stale interaction result = handled %v, calls %d", handled, interaction.calls)
	}
}

func TestRuntimeActiveChunksFollowSessionViews(t *testing.T) {
	runtime := NewRuntime(game.NewOverworld(nil))

	first := &Session{}
	second := &Session{}

	sharedPosition := LoadedChunk{X: 1, Z: 0}

	runtime.setSessionActiveChunks(first, []LoadedChunk{{X: 0, Z: 0}, sharedPosition})
	runtime.setSessionActiveChunks(second, []LoadedChunk{sharedPosition})

	count := runtime.ActiveChunkCount()
	if count != 2 {
		t.Fatalf("active chunk count = %d, want 2", count)
	}

	shared, active := runtime.ActiveChunk(sharedPosition)
	if !active {
		t.Fatal("shared chunk is not active")
	}

	shared.SetEntity(1, &recordingRuntimeTicker{log: &runtimeTickLog{}})

	runtime.setSessionActiveChunks(first, []LoadedChunk{{X: 2, Z: 0}})

	retained, active := runtime.ActiveChunk(sharedPosition)
	if !active || retained != shared || retained.EntityCount() != 1 {
		t.Fatal("shared chunk state was not retained for the second session")
	}

	_, active = runtime.ActiveChunk(LoadedChunk{})

	if active {
		t.Fatal("chunk left by all sessions is still active")
	}

	runtime.LeaveSession(second)

	_, active = runtime.ActiveChunk(sharedPosition)

	if active {
		t.Fatal("shared chunk remained active after its final session left")
	}

	runtime.LeaveSession(first)

	count = runtime.ActiveChunkCount()
	if count != 0 {
		t.Fatalf("active chunk count after leaves = %d, want 0", count)
	}
}

func TestRuntimeTickProcessesOnlyActiveChunkStateInStableOrder(t *testing.T) {
	runtime := NewRuntime(game.NewOverworld(nil))

	session := &Session{}

	log := &runtimeTickLog{}

	runtime.setSessionActiveChunks(session, []LoadedChunk{{X: 2, Z: 0}, {X: -1, Z: 3}})

	first, _ := runtime.ActiveChunk(LoadedChunk{X: -1, Z: 3})

	first.SetEntity(9, &recordingRuntimeTicker{label: "first entity 9", log: log})
	first.SetEntity(2, &recordingRuntimeTicker{label: "first entity 2", log: log})

	blockPosition := game.BlockPosition{X: -15, Y: 70, Z: 50}

	first.SetBlockEntity(blockPosition, &recordingRuntimeTicker{label: "first block", log: log, position: blockPosition})

	second, _ := runtime.ActiveChunk(LoadedChunk{X: 2, Z: 0})

	second.SetEntity(1, &recordingRuntimeTicker{label: "second entity", log: log})

	runtime.Tick()

	expected := []string{
		"first entity 2",
		"first entity 9",
		"first block",
		"second entity",
	}

	assertRuntimeTickLog(t, log, expected)

	runtime.LeaveSession(session)
	runtime.Tick()

	assertRuntimeTickLog(t, log, expected)
}

func TestVisibleChunksActivateRuntimeStateBeforeDelivery(t *testing.T) {
	session, _ := newChunkTestSession(game.Position{})

	session.startChunkStream(t.Context())

	err := session.updatePlayerChunks()
	if err != nil {
		t.Fatalf("queue visible chunks: %v", err)
	}

	expected := len(chunksInView(LoadedChunk{}, 2))
	count := session.Runtime.ActiveChunkCount()

	if count != expected {
		t.Fatalf("active chunk count = %d, want %d", count, expected)
	}

	session.Runtime.LeaveSession(session)

	count = session.Runtime.ActiveChunkCount()
	if count != 0 {
		t.Fatalf("active chunk count after leave = %d, want 0", count)
	}
}

func TestReleasedSessionCannotReactivateChunks(t *testing.T) {
	runtime := NewRuntime(game.NewOverworld(nil))

	session := &Session{Runtime: runtime}

	runtime.setSessionActiveChunks(session, []LoadedChunk{{X: 1, Z: 1}})

	runtime.LeaveSession(session)

	err := session.updateVisibleChunks(LoadedChunk{X: 2, Z: 2})
	if err != nil {
		t.Fatalf("late visible chunk update: %v", err)
	}

	count := runtime.ActiveChunkCount()
	if count != 0 {
		t.Fatalf("active chunk count after late update = %d, want 0", count)
	}
}

func assertRuntimeTickLog(t *testing.T, log *runtimeTickLog, expected []string) {
	t.Helper()

	log.mu.Lock()
	defer log.mu.Unlock()

	if len(log.entries) != len(expected) {
		t.Fatalf("tick log = %v, want %v", log.entries, expected)
	}

	for index := range expected {
		if log.entries[index] != expected[index] {
			t.Fatalf("tick log = %v, want %v", log.entries, expected)
		}
	}
}
