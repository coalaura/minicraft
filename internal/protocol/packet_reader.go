package protocol

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"io"

	"github.com/coalaura/minicraft/internal/game"
)

type PacketReader struct {
	*bytes.Reader
	err error
}

func (r *PacketReader) VarInt() int32 {
	if r.err != nil {
		return 0
	}

	value, err := ReadVarInt(r)
	if err != nil {
		r.err = err

		return 0
	}

	return value
}

func (r *PacketReader) VarLong() int64 {
	if r.err != nil {
		return 0
	}

	value, err := ReadVarLong(r)
	if err != nil {
		r.err = err

		return 0
	}

	return value
}

func (r *PacketReader) Int() int32 {
	if r.err != nil {
		return 0
	}

	value, err := ReadInt(r)
	if err != nil {
		r.err = err

		return 0
	}

	return value
}

func (r *PacketReader) Long() int64 {
	if r.err != nil {
		return 0
	}

	value, err := ReadLong(r)
	if err != nil {
		r.err = err

		return 0
	}

	return value
}

func (r *PacketReader) BlockPosition() game.BlockPosition {
	packed := r.Long()

	return game.BlockPosition{
		X: int32(packed >> 38),
		Y: int32(packed << 52 >> 52),
		Z: int32(packed << 26 >> 38),
	}
}

func (r *PacketReader) Short() int16 {
	if r.err != nil {
		return 0
	}

	value, err := ReadShort(r)
	if err != nil {
		r.err = err

		return 0
	}

	return value
}

func (r *PacketReader) Float() float32 {
	if r.err != nil {
		return 0
	}

	value, err := ReadFloat(r)
	if err != nil {
		r.err = err

		return 0
	}

	return value
}

func (r *PacketReader) Double() float64 {
	if r.err != nil {
		return 0
	}

	value, err := ReadDouble(r)
	if err != nil {
		r.err = err

		return 0
	}

	return value
}

func (r *PacketReader) Bool() bool {
	if r.err != nil {
		return false
	}

	value, err := ReadBool(r)
	if err != nil {
		r.err = err

		return false
	}

	return value
}

func (r *PacketReader) String(max int) string {
	if r.err != nil {
		return ""
	}

	br := ByteReader{r.Reader}

	value, err := ReadString(&br, max)
	if err != nil {
		r.err = err

		return ""
	}

	return value
}

func (r *PacketReader) Byte() byte {
	if r.err != nil {
		return 0
	}

	value, err := r.ReadByte()
	if err != nil {
		r.err = err

		return 0
	}

	return value
}

func (r *PacketReader) Bytes() []byte {
	if r.err != nil {
		return nil
	}

	value, err := ReadBytes(r.Reader)
	if err != nil {
		r.err = err

		return nil
	}

	return value
}

func (r *PacketReader) BytesMax(max int) []byte {
	if r.err != nil {
		return nil
	}

	length := r.VarInt()

	if r.err != nil {
		return nil
	}

	if length < 0 || length > int32(max) {
		r.err = fmt.Errorf("byte array length %d exceeds maximum %d", length, max)

		return nil
	}

	value := make([]byte, length)

	_, err := io.ReadFull(r.Reader, value)
	if err != nil {
		r.err = err

		return nil
	}

	return value
}

func (r *PacketReader) UUID() string {
	if r.err != nil {
		return ""
	}

	var value [16]byte

	_, err := io.ReadFull(r.Reader, value[:])
	if err != nil {
		r.err = err

		return ""
	}

	raw := hex.EncodeToString(value[:])

	return fmt.Sprintf("%s-%s-%s-%s-%s", raw[0:8], raw[8:12], raw[12:16], raw[16:20], raw[20:])
}

func (r *PacketReader) Err() error {
	return r.err
}

func (r *PacketReader) Done(packetName string) error {
	if r.err != nil {
		return r.err
	}

	if r.Len() != 0 {
		return fmt.Errorf("%s has %d trailing bytes", packetName, r.Len())
	}

	return nil
}

func NewPacketReader(b []byte) *PacketReader {
	return &PacketReader{
		Reader: bytes.NewReader(b),
	}
}

func DecodeEmptyPacket(data []byte, packetName string) error {
	rd := NewPacketReader(data)

	return rd.Done(packetName)
}
