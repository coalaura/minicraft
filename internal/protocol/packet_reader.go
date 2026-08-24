package protocol

import (
	"bytes"

	"github.com/coalaura/minicraft/internal/game"
)

type PacketReader struct {
	*bytes.Reader
	err error
}

func NewPacketReader(b []byte) *PacketReader {
	return &PacketReader{
		Reader: bytes.NewReader(b),
	}
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

func (r *PacketReader) Err() error {
	return r.err
}
