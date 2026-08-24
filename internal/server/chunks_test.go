package server

import (
	"testing"

	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

type chunkCoordinateTest struct {
	position float64
	expected int32
}

func TestInitialChunksUseSessionTracking(t *testing.T) {
	session, connection := newChunkTestSession(game.Position{X: -0.01, Z: 32})

	err := session.sendInitialChunks()
	if err != nil {
		t.Fatalf("send initial chunks: %v", err)
	}

	expectedCenter := LoadedChunk{X: -1, Z: 2}
	expectedChunks := chunksInView(expectedCenter)

	packets := connection.packets(t)

	if len(packets) != len(expectedChunks)+4 {
		t.Fatalf("initial packet count = %d, want %d", len(packets), len(expectedChunks)+4)
	}

	if packets[0].ID != protocol.ClientboundGameEventID {
		t.Fatalf("first packet id = %#x, want game event", packets[0].ID)
	}

	assertCenterChunkPacket(t, packets[1], expectedCenter)

	if packets[2].ID != protocol.ClientboundChunkBatchBeginID {
		t.Fatalf("packet 2 id = %#x, want chunk batch begin", packets[2].ID)
	}

	assertLoadedChunkPackets(t, packets[3:3+len(expectedChunks)], expectedChunks)
	assertChunkBatchEndPacket(t, packets[len(packets)-1], len(expectedChunks))
	assertSessionChunks(t, session, expectedCenter, expectedChunks)

	connection.reset()

	err = session.updatePlayerChunks()
	if err != nil {
		t.Fatalf("update unchanged player chunks: %v", err)
	}

	if packets = connection.packets(t); len(packets) != 0 {
		t.Fatalf("unchanged center sent packets: %v", connection.packetIDs(t))
	}
}

func TestPlayerChunkTransitionStreamsChangedColumns(t *testing.T) {
	session, connection := newChunkTestSession(game.Position{X: 15.999, Z: -0.01})

	err := session.updatePlayerChunks()
	if err != nil {
		t.Fatalf("load initial view: %v", err)
	}

	connection.reset()

	session.Player.Position.X = 16

	err = session.updatePlayerChunks()
	if err != nil {
		t.Fatalf("stream changed view: %v", err)
	}

	center := LoadedChunk{X: 1, Z: -1}

	packets := connection.packets(t)

	expectedUnloaded := []LoadedChunk{
		{X: -2, Z: -3},
		{X: -2, Z: -2},
		{X: -2, Z: -1},
		{X: -2, Z: 0},
		{X: -2, Z: 1},
	}

	expectedLoaded := []LoadedChunk{
		{X: 3, Z: -3},
		{X: 3, Z: -2},
		{X: 3, Z: -1},
		{X: 3, Z: 0},
		{X: 3, Z: 1},
	}

	if len(packets) != 1+len(expectedUnloaded)+2+len(expectedLoaded) {
		t.Fatalf("transition packet count = %d", len(packets))
	}

	assertCenterChunkPacket(t, packets[0], center)
	assertForgottenChunkPackets(t, packets[1:1+len(expectedUnloaded)], expectedUnloaded)

	batchStart := 1 + len(expectedUnloaded)
	if packets[batchStart].ID != protocol.ClientboundChunkBatchBeginID {
		t.Fatalf("packet %d id = %#x, want chunk batch begin", batchStart, packets[batchStart].ID)
	}

	assertLoadedChunkPackets(t, packets[batchStart+1:len(packets)-1], expectedLoaded)
	assertChunkBatchEndPacket(t, packets[len(packets)-1], len(expectedLoaded))
	assertSessionChunks(t, session, center, chunksInView(center))

	connection.reset()

	session.Player.Position.X = 31.999

	err = session.updatePlayerChunks()
	if err != nil {
		t.Fatalf("update within chunk: %v", err)
	}

	if packets = connection.packets(t); len(packets) != 0 {
		t.Fatalf("movement within chunk sent packets: %v", connection.packetIDs(t))
	}
}

func TestChunkCoordinatesAtBoundaries(t *testing.T) {
	tests := []chunkCoordinateTest{
		{position: 0, expected: 0},
		{position: 15.999, expected: 0},
		{position: 16, expected: 1},
		{position: -0.001, expected: -1},
		{position: -16, expected: -1},
		{position: -16.001, expected: -2},
	}

	for _, test := range tests {
		actual := chunkCoordinate(test.position)
		if actual != test.expected {
			t.Errorf("chunkCoordinate(%v) = %d, want %d", test.position, actual, test.expected)
		}
	}
}

func TestNegativeChunkBoundaryTransition(t *testing.T) {
	session, connection := newChunkTestSession(game.Position{})

	err := session.updatePlayerChunks()
	if err != nil {
		t.Fatalf("load initial view: %v", err)
	}

	connection.reset()

	session.Player.Position.X = -0.001

	err = session.updatePlayerChunks()
	if err != nil {
		t.Fatalf("cross negative chunk boundary: %v", err)
	}

	packets := connection.packets(t)

	center := LoadedChunk{X: -1, Z: 0}

	expectedUnloaded := make([]LoadedChunk, 0, ChunkRadius*2+1)
	expectedLoaded := make([]LoadedChunk, 0, ChunkRadius*2+1)

	for chunkZ := int32(-ChunkRadius); chunkZ <= ChunkRadius; chunkZ++ {
		expectedUnloaded = append(expectedUnloaded, LoadedChunk{X: 2, Z: chunkZ})
		expectedLoaded = append(expectedLoaded, LoadedChunk{X: -3, Z: chunkZ})
	}

	if len(packets) != 1+len(expectedUnloaded)+2+len(expectedLoaded) {
		t.Fatalf("negative transition packet count = %d", len(packets))
	}

	assertCenterChunkPacket(t, packets[0], center)
	assertForgottenChunkPackets(t, packets[1:1+len(expectedUnloaded)], expectedUnloaded)

	batchStart := 1 + len(expectedUnloaded)
	if packets[batchStart].ID != protocol.ClientboundChunkBatchBeginID {
		t.Fatalf("packet %d id = %#x, want chunk batch begin", batchStart, packets[batchStart].ID)
	}

	assertLoadedChunkPackets(t, packets[2+len(expectedUnloaded):len(packets)-1], expectedLoaded)
	assertChunkBatchEndPacket(t, packets[len(packets)-1], len(expectedLoaded))
	assertSessionChunks(t, session, center, chunksInView(center))
}

func TestSessionsTrackLoadedChunksIndependently(t *testing.T) {
	first, firstConnection := newChunkTestSession(game.Position{})
	second, secondConnection := newChunkTestSession(game.Position{X: 160})

	for _, session := range []*Session{first, second} {
		err := session.updatePlayerChunks()
		if err != nil {
			t.Fatalf("load session chunks: %v", err)
		}
	}

	firstConnection.reset()
	secondConnection.reset()

	first.Player.Position.X = 16

	err := first.updatePlayerChunks()
	if err != nil {
		t.Fatalf("move first session: %v", err)
	}

	if len(firstConnection.packets(t)) == 0 {
		t.Fatal("moving session received no chunk updates")
	}

	if packets := secondConnection.packets(t); len(packets) != 0 {
		t.Fatalf("stationary session received packets: %v", secondConnection.packetIDs(t))
	}

	assertSessionChunks(t, first, LoadedChunk{X: 1, Z: 0}, chunksInView(LoadedChunk{X: 1, Z: 0}))
	assertSessionChunks(t, second, LoadedChunk{X: 10, Z: 0}, chunksInView(LoadedChunk{X: 10, Z: 0}))
}

func newChunkTestSession(position game.Position) (*Session, *recordingConnection) {
	connection := &recordingConnection{}

	session := &Session{
		Conn:    protocol.NewConnection(connection, nil),
		Runtime: NewRuntime(game.NewOverworld()),
		Player:  &game.Player{Position: position},
	}

	return session, connection
}

func assertCenterChunkPacket(t *testing.T, packet protocol.Packet, expected LoadedChunk) {
	t.Helper()

	if packet.ID != protocol.ClientboundSetCenterChunkID {
		t.Fatalf("packet id = %#x, want set center chunk", packet.ID)
	}

	reader := protocol.NewPacketReader(packet.Data)

	actual := LoadedChunk{X: reader.VarInt(), Z: reader.VarInt()}

	if err := reader.Err(); err != nil {
		t.Fatalf("decode center chunk: %v", err)
	}

	if actual != expected {
		t.Fatalf("center chunk = %+v, want %+v", actual, expected)
	}
}

func assertForgottenChunkPackets(t *testing.T, packets []protocol.Packet, expected []LoadedChunk) {
	t.Helper()

	for index, packet := range packets {
		if packet.ID != protocol.ClientboundForgetLevelChunkID {
			t.Fatalf("packet %d id = %#x, want forget level chunk", index, packet.ID)
		}

		reader := protocol.NewPacketReader(packet.Data)

		actual := LoadedChunk{Z: reader.Int(), X: reader.Int()}

		if err := reader.Err(); err != nil {
			t.Fatalf("decode forgotten chunk %d: %v", index, err)
		}

		if actual != expected[index] {
			t.Fatalf("forgotten chunk %d = %+v, want %+v", index, actual, expected[index])
		}
	}
}

func assertLoadedChunkPackets(t *testing.T, packets []protocol.Packet, expected []LoadedChunk) {
	t.Helper()

	if len(packets) != len(expected) {
		t.Fatalf("loaded chunk packet count = %d, want %d", len(packets), len(expected))
	}

	for index, packet := range packets {
		if packet.ID != protocol.ClientboundLevelChunkWithLightID {
			t.Fatalf("packet %d id = %#x, want level chunk with light", index, packet.ID)
		}

		reader := protocol.NewPacketReader(packet.Data)

		actual := LoadedChunk{X: reader.Int(), Z: reader.Int()}

		if err := reader.Err(); err != nil {
			t.Fatalf("decode loaded chunk %d: %v", index, err)
		}

		if actual != expected[index] {
			t.Fatalf("loaded chunk %d = %+v, want %+v", index, actual, expected[index])
		}
	}
}

func assertChunkBatchEndPacket(t *testing.T, packet protocol.Packet, expectedSize int) {
	t.Helper()

	if packet.ID != protocol.ClientboundChunkBatchEndID {
		t.Fatalf("packet id = %#x, want chunk batch end", packet.ID)
	}

	reader := protocol.NewPacketReader(packet.Data)

	actualSize := reader.VarInt()

	if err := reader.Err(); err != nil {
		t.Fatalf("decode chunk batch end: %v", err)
	}

	if actualSize != int32(expectedSize) {
		t.Fatalf("chunk batch size = %d, want %d", actualSize, expectedSize)
	}
}

func assertSessionChunks(t *testing.T, session *Session, expectedCenter LoadedChunk, expected []LoadedChunk) {
	t.Helper()

	if !session.hasChunkCenter {
		t.Fatal("session has no chunk center")
	}

	if session.centerChunk != expectedCenter {
		t.Fatalf("session center = %+v, want %+v", session.centerChunk, expectedCenter)
	}

	if len(session.loadedChunks) != len(expected) {
		t.Fatalf("session loaded chunk count = %d, want %d", len(session.loadedChunks), len(expected))
	}

	for _, chunk := range expected {
		if _, loaded := session.loadedChunks[chunk]; !loaded {
			t.Errorf("session does not track chunk %+v", chunk)
		}
	}
}
