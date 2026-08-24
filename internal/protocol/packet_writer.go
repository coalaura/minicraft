package protocol

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"math"
	"strings"
	"unicode/utf16"

	"github.com/coalaura/minicraft/internal/game"
)

const (
	lowPrecisionVectorMaxQuantized = 32766
	lowPrecisionVectorMinValue     = 3.051944088384301e-5
	lowPrecisionVectorMaxValue     = 1.7179869183e10
)

type PacketWriter struct {
	bytes.Buffer
	err error
}

func (w *PacketWriter) VarInt(value int32) {
	if w.err != nil {
		return
	}

	w.err = WriteVarInt(&w.Buffer, value)
}

func (w *PacketWriter) VarLong(value int64) {
	if w.err != nil {
		return
	}

	w.err = WriteVarLong(&w.Buffer, value)
}

func (w *PacketWriter) Int(value int32) {
	if w.err != nil {
		return
	}

	var encoded [4]byte

	binary.BigEndian.PutUint32(encoded[:], uint32(value))

	_, w.err = w.Buffer.Write(encoded[:])
}

func (w *PacketWriter) Long(value int64) {
	if w.err != nil {
		return
	}

	var encoded [8]byte

	binary.BigEndian.PutUint64(encoded[:], uint64(value))

	_, w.err = w.Buffer.Write(encoded[:])
}

func (w *PacketWriter) BlockPosition(position game.BlockPosition) {
	packed := (int64(position.X)&0x3FFFFFF)<<38 |
		(int64(position.Z)&0x3FFFFFF)<<12 |
		int64(position.Y)&0xFFF

	w.Long(packed)
}

func (w *PacketWriter) Short(value int16) {
	if w.err != nil {
		return
	}

	var encoded [2]byte

	binary.BigEndian.PutUint16(encoded[:], uint16(value))

	_, w.err = w.Buffer.Write(encoded[:])
}

func (w *PacketWriter) Float(value float32) {
	if w.err != nil {
		return
	}

	w.Int(int32(math.Float32bits(value)))
}

func (w *PacketWriter) Double(value float64) {
	if w.err != nil {
		return
	}

	w.Long(int64(math.Float64bits(value)))
}

func (w *PacketWriter) LowPrecisionVector(x, y, z float64) {
	if w.err != nil {
		return
	}

	x = sanitizeLowPrecisionVectorValue(x)
	y = sanitizeLowPrecisionVectorValue(y)
	z = sanitizeLowPrecisionVectorValue(z)

	maxValue := max(math.Abs(x), math.Abs(y), math.Abs(z))
	if maxValue < lowPrecisionVectorMinValue {
		w.Byte(0)

		return
	}

	scale := uint64(math.Ceil(maxValue))
	markers := scale

	if scale > 3 {
		markers = scale%4 | 4
	}

	packed := markers +
		packLowPrecisionVectorValue(x/float64(scale))*0x8 +
		packLowPrecisionVectorValue(y/float64(scale))*0x40000 +
		packLowPrecisionVectorValue(z/float64(scale))*0x200000000

	w.Byte(byte(packed))
	w.Byte(byte(packed >> 8))
	w.Int(int32(packed >> 16))

	if scale > 3 {
		w.VarInt(int32(uint32(scale / 4)))
	}
}

func (w *PacketWriter) Bool(value bool) {
	if w.err != nil {
		return
	}

	w.err = WriteBool(&w.Buffer, value)
}

func (w *PacketWriter) String(value string) {
	if w.err != nil {
		return
	}

	w.err = WriteString(&w.Buffer, value)
}

func (w *PacketWriter) AnonymousNBTString(value string) {
	if w.err != nil {
		return
	}

	encoded := modifiedUTF8(value)
	if len(encoded) > math.MaxUint16 {
		w.err = errors.New("nbt string too long")

		return
	}

	w.Byte(8)
	w.Short(int16(len(encoded)))

	if w.err != nil {
		return
	}

	_, w.err = w.Write(encoded)
}

func (w *PacketWriter) Byte(value byte) {
	if w.err != nil {
		return
	}

	w.err = w.WriteByte(value)
}

func (w *PacketWriter) Bytes(value []byte) {
	if w.err != nil {
		return
	}

	w.VarInt(int32(len(value)))

	if w.err != nil {
		return
	}

	_, w.err = w.Write(value)
}

func (w *PacketWriter) UUID(value string) {
	if w.err != nil {
		return
	}

	raw, err := hex.DecodeString(strings.ReplaceAll(value, "-", ""))
	if err != nil || len(raw) != 16 {
		w.err = errors.New("malformed uuid")

		return
	}

	_, w.err = w.Write(raw)
}

func (w *PacketWriter) Err() error {
	return w.err
}

func sanitizeLowPrecisionVectorValue(value float64) float64 {
	if math.IsNaN(value) {
		return 0
	}

	return max(-lowPrecisionVectorMaxValue, min(value, lowPrecisionVectorMaxValue))
}

func packLowPrecisionVectorValue(value float64) uint64 {
	return uint64(math.Round((value*0.5 + 0.5) * lowPrecisionVectorMaxQuantized))
}

func modifiedUTF8(value string) []byte {
	codeUnits := utf16.Encode([]rune(value))

	encoded := make([]byte, 0, len(value))

	for _, codeUnit := range codeUnits {
		switch {
		case codeUnit >= 0x0001 && codeUnit <= 0x007F:
			encoded = append(encoded, byte(codeUnit))
		case codeUnit <= 0x07FF:
			encoded = append(encoded, 0xC0|byte(codeUnit>>6), 0x80|byte(codeUnit&0x3F))
		default:
			encoded = append(encoded, 0xE0|byte(codeUnit>>12), 0x80|byte(codeUnit>>6&0x3F), 0x80|byte(codeUnit&0x3F))
		}
	}

	return encoded
}
