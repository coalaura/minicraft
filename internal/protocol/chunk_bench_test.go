package protocol

import (
	"io"
	"net"
	"testing"
	"time"
)

type benchmarkConnection struct{}

func (benchmarkConnection) Read(_ []byte) (int, error) {
	return 0, io.EOF
}

func (benchmarkConnection) Write(data []byte) (int, error) {
	return len(data), nil
}

func (benchmarkConnection) Close() error {
	return nil
}

func (benchmarkConnection) LocalAddr() net.Addr {
	return benchmarkAddress{}
}

func (benchmarkConnection) RemoteAddr() net.Addr {
	return benchmarkAddress{}
}

func (benchmarkConnection) SetDeadline(time.Time) error {
	return nil
}

func (benchmarkConnection) SetReadDeadline(time.Time) error {
	return nil
}

func (benchmarkConnection) SetWriteDeadline(time.Time) error {
	return nil
}

type benchmarkAddress struct{}

func (benchmarkAddress) Network() string {
	return "benchmark"
}

func (benchmarkAddress) String() string {
	return "benchmark"
}

func BenchmarkSectionBlocksToSection(b *testing.B) {
	benchmarks := []struct {
		name   string
		states func(int) int32
	}{
		{name: "empty", states: func(int) int32 { return AirBlockState }},
		{name: "uniform", states: func(int) int32 { return StoneBlockState }},
		{name: "small_palette", states: func(index int) int32 { return int32(index % 8) }},
		{name: "large_palette", states: func(index int) int32 { return int32(index % 300) }},
	}

	for _, benchmark := range benchmarks {
		b.Run(benchmark.name, func(b *testing.B) {
			var blocks SectionBlocks

			for index := range blocks.States {
				blocks.States[index] = benchmark.states(index)
			}

			b.ReportAllocs()
			b.ResetTimer()

			for range b.N {
				_ = blocks.ToSection(0)
			}
		})
	}
}

func BenchmarkLevelChunkWithLightEncode(b *testing.B) {
	chunk := NewEmptyOverworldChunk(0, 0, 0)

	var blocks SectionBlocks

	for index := range blocks.States {
		blocks.States[index] = int32(index % 8)
	}

	for index := range chunk.Sections {
		chunk.Sections[index] = blocks.ToSection(0)
	}

	b.ReportAllocs()

	for b.Loop() {
		var writer PacketWriter

		chunk.Encode(&writer)

		err := writer.Err()
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkConnectionWriteCompressedChunk(b *testing.B) {
	chunk := NewEmptyOverworldChunk(0, 0, 0)

	var blocks SectionBlocks

	for index := range blocks.States {
		blocks.States[index] = int32(index % 8)
	}

	for index := range chunk.Sections {
		chunk.Sections[index] = blocks.ToSection(0)
	}

	var writer PacketWriter

	chunk.Encode(&writer)

	packet := Packet{ID: ClientboundLevelChunkWithLightID, Data: writer.Buffer.Bytes()}

	connection := NewConnection(benchmarkConnection{}, nil)

	connection.SetCompression(256)

	b.ReportAllocs()

	for b.Loop() {
		err := connection.WritePacket(packet)
		if err != nil {
			b.Fatal(err)
		}
	}
}
