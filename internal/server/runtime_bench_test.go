package server

import (
	"io"
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

type benchmarkDiscardConnection struct{}

type benchmarkCountingConnection struct {
	writes uint64
	bytes  uint64
}

type droppedItemFluidBenchmarkFixture struct {
	runtime     *Runtime
	items       []*runtimeItemEntity
	connections []*benchmarkCountingConnection
}

type benchmarkRuntimeTicker struct {
	state RuntimeEntityState
}

func (benchmarkDiscardConnection) Read([]byte) (int, error) {
	return 0, io.EOF
}

func (benchmarkDiscardConnection) Write(data []byte) (int, error) {
	return len(data), nil
}

func (benchmarkDiscardConnection) Close() error {
	return nil
}

func (benchmarkDiscardConnection) LocalAddr() net.Addr {
	return &net.TCPAddr{}
}

func (benchmarkDiscardConnection) RemoteAddr() net.Addr {
	return &net.TCPAddr{}
}

func (benchmarkDiscardConnection) SetDeadline(time.Time) error {
	return nil
}

func (benchmarkDiscardConnection) SetReadDeadline(time.Time) error {
	return nil
}

func (benchmarkDiscardConnection) SetWriteDeadline(time.Time) error {
	return nil
}

func (*benchmarkCountingConnection) Read([]byte) (int, error) {
	return 0, io.EOF
}

func (connection *benchmarkCountingConnection) Write(data []byte) (int, error) {
	connection.writes++
	connection.bytes += uint64(len(data))

	return len(data), nil
}

func (*benchmarkCountingConnection) Close() error {
	return nil
}

func (*benchmarkCountingConnection) LocalAddr() net.Addr {
	return &net.TCPAddr{}
}

func (*benchmarkCountingConnection) RemoteAddr() net.Addr {
	return &net.TCPAddr{}
}

func (*benchmarkCountingConnection) SetDeadline(time.Time) error {
	return nil
}

func (*benchmarkCountingConnection) SetReadDeadline(time.Time) error {
	return nil
}

func (*benchmarkCountingConnection) SetWriteDeadline(time.Time) error {
	return nil
}

func (fixture *droppedItemFluidBenchmarkFixture) reset() {
	for index, item := range fixture.items {
		item.State.mu.Lock()

		item.State.Position = droppedItemFluidBenchmarkPosition(index)
		item.mergePosition.Store(item.State.Position)
		item.State.Chunk = LoadedChunk{}
		item.State.Removed = false
		item.State.metadataDirty = false
		item.State.movementSyncDirty = false
		item.State.tracker = newRuntimeEntityTracker(item.runtimeEntityViewLocked())
		item.State.tracker.UpdateTick = 1
		item.Velocity = game.Velocity{X: 0.02}
		item.Age = 100
		item.PickupDelay = 40
		item.OnGround = false
		item.TickCount = 1

		item.State.mu.Unlock()
	}

	for _, connection := range fixture.connections {
		connection.writes = 0
		connection.bytes = 0
	}
}

func (ticker *benchmarkRuntimeTicker) RuntimeEntityState() *RuntimeEntityState {
	return &ticker.state
}

func (*benchmarkRuntimeTicker) Tick(*Runtime, *ActiveChunk) {}

func BenchmarkRuntimeMobTick(b *testing.B) {
	b.Run("stationary", func(b *testing.B) {
		runtime := benchmarkRuntime()

		runtime.entityRandom = func() float32 {
			return 0
		}

		zombie := runtime.SpawnZombie(game.Position{X: 0.5, Y: 1, Z: 0.5})

		zombie.PersistenceRequired = true

		for range 20 {
			zombie.Tick(runtime, nil)
		}

		b.ReportAllocs()
		b.ResetTimer()

		for b.Loop() {
			zombie.Tick(runtime, nil)
		}
	})

	b.Run("walking", func(b *testing.B) {
		runtime := benchmarkRuntime()

		runtime.entityRandom = func() float32 {
			return 0
		}

		viewer := benchmarkSession(b, runtime, "walking-viewer", game.Position{X: 8.5, Y: 1, Z: 0.5})

		zombie := runtime.SpawnZombie(game.Position{X: 0.5, Y: 1, Z: 0.5})

		zombie.PersistenceRequired = true
		zombie.GoalTarget = viewer

		for range 20 {
			zombie.Tick(runtime, nil)
		}

		b.ReportAllocs()
		b.ResetTimer()

		for b.Loop() {
			zombie.Tick(runtime, nil)
		}
	})
}

func BenchmarkActiveMobTick(b *testing.B) {
	counts := []int{100, 500}

	for _, count := range counts {
		b.Run("ordinary_mobs_"+itoa(count), func(b *testing.B) {
			runtime := benchmarkRuntime()

			runtime.entityRandom = func() float32 {
				return 0
			}

			session := &Session{}

			runtime.setSessionActiveChunks(session, []LoadedChunk{{}})

			for index := range count {
				position := game.Position{X: float64(index%16) + 0.5, Y: 1, Z: float64(index/16) + 0.5}

				zombie := runtime.SpawnZombie(position)

				zombie.PersistenceRequired = true
			}

			runtime.tickActiveChunks()

			b.ReportAllocs()
			b.ResetTimer()

			for b.Loop() {
				runtime.tickActiveChunks()
			}
		})
	}
}

func BenchmarkSkeletonCombatTick(b *testing.B) {
	runtime := benchmarkRuntime()

	runtime.entityRandom = func() float32 {
		return 0
	}

	player := benchmarkSession(b, runtime, "skeleton-target", game.Position{X: 8.5, Y: 1, Z: 0.5})

	skeleton := runtime.SpawnSkeleton(game.Position{X: 0.5, Y: 1, Z: 0.5})

	skeleton.PersistenceRequired = true
	skeleton.GoalTarget = runtimeLivingTarget{session: player}

	for range 20 {
		skeleton.Tick(runtime, nil)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		skeleton.Tick(runtime, nil)
	}
}

func BenchmarkDroppedItemTick(b *testing.B) {
	runtime := benchmarkRuntime()

	item := runtime.SpawnItemEntity(game.ItemStack{Item: game.ItemStone, Count: 1}, game.Position{X: 0.5, Y: 2, Z: 0.5}, game.Velocity{X: 0.1}, 32767)

	for range 20 {
		item.Tick(runtime, nil)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		item.Tick(runtime, nil)
	}
}

func BenchmarkDroppedItemsFlowingWaterTick(b *testing.B) {
	counts := []int{100, 500}
	viewerCounts := []int{0, 1, 4}

	for _, count := range counts {
		for _, viewers := range viewerCounts {
			name := itoa(count) + "_physics"

			if viewers != 0 {
				name = itoa(count) + "_tracking_" + itoa(viewers)
			}

			b.Run(name, func(b *testing.B) {
				fixture := newDroppedItemFluidBenchmarkFixture(b, count, viewers)

				fixture.reset()

				fixture.runtime.tickActiveChunks()

				var (
					writes uint64
					bytes  uint64
				)

				b.ReportAllocs()

				for b.Loop() {
					b.StopTimer()
					fixture.reset()
					b.StartTimer()

					fixture.runtime.tickActiveChunks()

					b.StopTimer()

					for _, connection := range fixture.connections {
						writes += connection.writes
						bytes += connection.bytes
					}

					b.StartTimer()
				}

				b.ReportMetric(float64(writes)/float64(b.N), "writes/tick")
				b.ReportMetric(float64(bytes)/float64(b.N), "packet-bytes/tick")
			})
		}
	}
}

func BenchmarkEntityTracking(b *testing.B) {
	viewerCounts := []int{1, 4}

	for _, viewers := range viewerCounts {
		b.Run("unchanged_viewers_"+itoa(viewers), func(b *testing.B) {
			runtime, item := benchmarkTrackedItem(b, viewers)

			runtime.synchronizeRuntimeEntity(item)

			b.ReportAllocs()
			b.ResetTimer()

			for b.Loop() {
				runtime.synchronizeRuntimeEntity(item)
			}
		})

		b.Run("movement_viewers_"+itoa(viewers), func(b *testing.B) {
			runtime, item := benchmarkTrackedItem(b, viewers)

			runtime.synchronizeRuntimeEntity(item)
			moved := false

			b.ReportAllocs()
			b.ResetTimer()

			for b.Loop() {
				item.State.mu.Lock()
				moved = !moved

				if moved {
					item.State.Position.X = 0.51
				} else {
					item.State.Position.X = 0.5
				}

				item.State.movementSyncDirty = true
				item.State.mu.Unlock()

				runtime.synchronizeRuntimeEntity(item)
			}
		})
	}
}

func BenchmarkPlayerScalarStateRead(b *testing.B) {
	runtime := benchmarkRuntime()
	session := benchmarkSession(b, runtime, "state-reader", game.Position{Y: 1})

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_ = session.playerAlive()
	}
}

func BenchmarkActiveChunkTick(b *testing.B) {
	runtime := benchmarkRuntime()

	session := &Session{}

	runtime.setSessionActiveChunks(session, []LoadedChunk{{}})

	chunk, active := runtime.ActiveChunk(LoadedChunk{})
	if !active {
		b.Fatal("active chunk was not created")
	}

	for index := range 100 {
		chunk.SetEntity(int32(index), &benchmarkRuntimeTicker{})
	}

	runtime.tickActiveChunks()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		runtime.tickActiveChunks()
	}
}

func BenchmarkGroundCollisionMovement(b *testing.B) {
	runtime := benchmarkRuntime()

	runtime.World.SetBlock(game.BlockPosition{Y: 0}, game.Stone)

	position := game.Position{X: 0.5, Y: 1, Z: 0.5}
	velocity := game.Velocity{X: 0.15, Y: -0.1, Z: 0.1}

	for range 20 {
		runtime.moveGroundEntity(position, velocity, 0.6, 1.8, 0.6, true)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_ = runtime.moveGroundEntity(position, velocity, 0.6, 1.8, 0.6, true)
	}
}

func BenchmarkNearbyEntityLookup(b *testing.B) {
	runtime := benchmarkRuntime()

	session := &Session{}

	runtime.setSessionActiveChunks(session, []LoadedChunk{{}})

	for index := range 100 {
		position := game.Position{X: float64(index%16) + 0.5, Y: 1, Z: float64(index/16) + 0.5}

		runtime.SpawnItemEntity(game.ItemStack{Item: game.ItemStone, Count: 1}, position, game.Velocity{}, 32767)
	}

	box := game.AABB{MinX: 0, MinY: 0, MinZ: 0, MaxX: 16, MaxY: 2, MaxZ: 16}

	items := make([]*runtimeItemEntity, 0, 100)
	items = runtime.appendItemEntitiesInBox(items, box)

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		items = runtime.appendItemEntitiesInBox(items[:0], box)
	}
}

func BenchmarkItemPickup(b *testing.B) {
	runtime := benchmarkRuntime()
	session := benchmarkSession(b, runtime, "pickup-player", game.Position{X: 0.5, Y: 1, Z: 0.5})

	stack := game.ItemStack{Item: game.ItemStone, Count: 1}

	item := runtime.SpawnItemEntity(stack, session.Player.Position, game.Velocity{}, 0)

	if !runtime.pickUpItemEntity(item) {
		b.Fatal("warm item was not picked up")
	}

	b.ReportAllocs()

	for b.Loop() {
		b.StopTimer()
		resetBenchmarkItemPickup(runtime, session, item, stack)
		b.StartTimer()

		if !runtime.pickUpItemEntity(item) {
			b.Fatal("item was not picked up")
		}
	}
}

func BenchmarkInventoryMutation(b *testing.B) {
	runtime := benchmarkRuntime()
	session := benchmarkSession(b, runtime, "inventory-player", game.Position{Y: 1})

	session.Player.GameMode = game.GameModeCreative

	updates := [...]protocol.SetCreativeModeSlot{
		{Slot: 36, Item: protocol.UntrustedSlot{ItemID: int32(game.ItemStone), ItemCount: 1}},
		{Slot: 36, Item: protocol.UntrustedSlot{ItemID: int32(game.ItemDirt), ItemCount: 1}},
	}

	session.handleSetCreativeModeSlot(updates[0])
	session.handleSetCreativeModeSlot(updates[1])

	b.ReportAllocs()
	b.ResetTimer()

	for index := 0; b.Loop(); index++ {
		session.handleSetCreativeModeSlot(updates[index&1])
	}
}

func benchmarkRuntime() *Runtime {
	world := &game.World{}

	world.SetTime(18000, false)

	return NewRuntime(world)
}

func benchmarkSession(b *testing.B, runtime *Runtime, uuid string, position game.Position) *Session {
	connection := protocol.NewConnection(benchmarkDiscardConnection{}, nil)

	session := &Session{
		Conn:         connection,
		Runtime:      runtime,
		Player:       &game.Player{UUID: "00010203-0405-0607-0809-0a0b0c0d0e0f", Name: uuid, Position: position},
		loadedChunks: map[LoadedChunk]struct{}{{}: {}},
	}

	runtime.AssignEntityID(session)

	err := runtime.JoinSession(session)
	if err != nil {
		b.Fatal(err)
	}

	return session
}

func benchmarkTrackedItem(b *testing.B, viewers int) (*Runtime, *runtimeItemEntity) {
	runtime := benchmarkRuntime()

	for index := range viewers {
		benchmarkSession(b, runtime, "tracking-viewer-"+itoa(index), game.Position{Y: 1})
	}

	item := runtime.SpawnItemEntity(game.ItemStack{Item: game.ItemStone, Count: 1}, game.Position{X: 0.5, Y: 1, Z: 0.5}, game.Velocity{}, 32767)

	return runtime, item
}

func newDroppedItemFluidBenchmarkFixture(b *testing.B, count, viewers int) *droppedItemFluidBenchmarkFixture {
	b.Helper()

	world := &game.World{}

	changes := make([]game.BlockChange, 0, count*2)

	flowingWater, valid := game.Water.WithProperties(game.BlockPropertyValue{Name: "level", Value: "1"})
	if !valid {
		b.Fatal("resolve flowing water")
	}

	for index := range count {
		position := droppedItemFluidBenchmarkPosition(index)

		blockY := int32(position.Y)
		blockZ := int32(position.Z)

		changes = append(changes,
			game.BlockChange{Position: game.BlockPosition{Y: blockY, Z: blockZ}, Replacement: game.Water},
			game.BlockChange{Position: game.BlockPosition{X: 1, Y: blockY, Z: blockZ}, Replacement: flowingWater},
		)
	}

	world.SetBlocks(changes)

	runtime := NewRuntime(world)

	fixture := &droppedItemFluidBenchmarkFixture{
		runtime: runtime,
		items:   make([]*runtimeItemEntity, 0, count),
	}

	if viewers == 0 {
		runtime.setSessionActiveChunks(&Session{}, []LoadedChunk{{}})
	} else {
		fixture.connections = make([]*benchmarkCountingConnection, 0, viewers)

		for index := range viewers {
			connection := &benchmarkCountingConnection{}

			session := benchmarkSessionWithConnection(b, runtime, "fluid-viewer-"+itoa(index), game.Position{X: 8.5, Y: 1}, connection)

			runtime.setSessionActiveChunks(session, []LoadedChunk{{}})

			fixture.connections = append(fixture.connections, connection)
		}
	}

	for index := range count {
		item := runtime.SpawnItemEntity(game.ItemStack{Item: game.ItemStone, Count: 1}, droppedItemFluidBenchmarkPosition(index), game.Velocity{X: 0.02}, 40)

		fixture.items = append(fixture.items, item)
	}

	return fixture
}

func benchmarkSessionWithConnection(b *testing.B, runtime *Runtime, uuid string, position game.Position, raw net.Conn) *Session {
	b.Helper()

	connection := protocol.NewConnection(raw, nil)

	session := &Session{
		Conn:         connection,
		Runtime:      runtime,
		Player:       &game.Player{UUID: "00010203-0405-0607-0809-0a0b0c0d0e0f", Name: uuid, Position: position},
		loadedChunks: map[LoadedChunk]struct{}{{}: {}},
	}

	runtime.AssignEntityID(session)

	err := runtime.JoinSession(session)
	if err != nil {
		b.Fatal(err)
	}

	return session
}

func droppedItemFluidBenchmarkPosition(index int) game.Position {
	return game.Position{X: 0.99, Y: float64(index/16) + 0.5, Z: float64(index%16) + 0.5}
}

func resetBenchmarkItemPickup(runtime *Runtime, session *Session, item *runtimeItemEntity, stack game.ItemStack) {
	session.Player.Inventory = game.PlayerInventory{}

	item.State.mu.Lock()
	item.State.Removed = false
	item.Stack = stack
	item.State.mu.Unlock()

	runtime.entityMu.Lock()
	runtime.entities[item.State.ID] = item
	runtime.runtimeEntities = []RuntimeEntity{item}
	runtime.entitiesByChunk[item.State.Chunk] = []RuntimeEntity{item}
	runtime.entityMu.Unlock()
}

func itoa(value int) string {
	return strconv.Itoa(value)
}
