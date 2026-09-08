package protocol

import (
	"io"
	"net"
	"testing"
	"time"
)

const benchmarkEntityPacketsPerTick = 500

type benchmarkLoopConnection struct {
	data       []byte
	offset     int
	reads      uint64
	writes     uint64
	writeBytes uint64
}

var benchmarkPacketSink *Packet

func (connection *benchmarkLoopConnection) Read(data []byte) (int, error) {
	connection.reads++

	for index := range data {
		data[index] = connection.data[connection.offset]
		connection.offset++

		if connection.offset == len(connection.data) {
			connection.offset = 0
		}
	}

	return len(data), nil
}

func (connection *benchmarkLoopConnection) Write(data []byte) (int, error) {
	connection.writes++
	connection.writeBytes += uint64(len(data))

	return len(data), nil
}

func (*benchmarkLoopConnection) Close() error {
	return nil
}

func (*benchmarkLoopConnection) LocalAddr() net.Addr {
	return &net.TCPAddr{}
}

func (*benchmarkLoopConnection) RemoteAddr() net.Addr {
	return &net.TCPAddr{}
}

func (*benchmarkLoopConnection) SetDeadline(time.Time) error {
	return nil
}

func (*benchmarkLoopConnection) SetReadDeadline(time.Time) error {
	return nil
}

func (*benchmarkLoopConnection) SetWriteDeadline(time.Time) error {
	return nil
}

func BenchmarkConnectionReadPacket(b *testing.B) {
	payload := make([]byte, 128)
	payload[0] = 0x2A

	frame := frameTestPayload(b, payload)

	raw := &benchmarkLoopConnection{data: frame}

	connection := NewConnection(raw, nil)

	b.ReportAllocs()
	b.SetBytes(int64(len(frame)))
	b.ResetTimer()

	for b.Loop() {
		packet, err := connection.ReadPacket()
		if err != nil {
			b.Fatal(err)
		}

		benchmarkPacketSink = packet
	}

	b.ReportMetric(float64(raw.reads)/float64(b.N), "reads/packet")
}

func BenchmarkConnectionWriteEntityPackets(b *testing.B) {
	packet := Packet{ID: 0x2F, Data: make([]byte, 16)}

	b.Run("immediate", func(b *testing.B) {
		raw := &benchmarkLoopConnection{data: []byte{0}}

		connection := NewConnection(raw, nil)

		b.ReportAllocs()
		b.ResetTimer()

		for b.Loop() {
			for range benchmarkEntityPacketsPerTick {
				err := connection.WritePacket(packet)
				if err != nil {
					b.Fatal(err)
				}
			}
		}

		b.ReportMetric(float64(raw.writes)/float64(b.N), "writes/tick")
		b.ReportMetric(float64(raw.writeBytes)/float64(b.N), "packet-bytes/tick")
	})

	b.Run("buffered", func(b *testing.B) {
		raw := &benchmarkLoopConnection{data: []byte{0}}

		connection := NewConnection(raw, nil)

		b.ReportAllocs()
		b.ResetTimer()

		for b.Loop() {
			for range benchmarkEntityPacketsPerTick {
				err := connection.WritePacketBuffered(packet)
				if err != nil {
					b.Fatal(err)
				}
			}

			err := connection.Flush()
			if err != nil {
				b.Fatal(err)
			}
		}

		b.ReportMetric(float64(raw.writes)/float64(b.N), "writes/tick")
		b.ReportMetric(float64(raw.writeBytes)/float64(b.N), "packet-bytes/tick")
	})
}

var _ io.ReadWriter = (*benchmarkLoopConnection)(nil)
