package protocol

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"io"
	"math"
)

const (
	VarSegmentBits = 0x7F
	VarContinueBit = 0x80
)

type ExtendedWriter interface {
	io.Writer
	io.ByteWriter
}

type ByteReader struct {
	*bytes.Reader
}

func (r ByteReader) Read(b []byte) (int, error) {
	return r.Reader.Read(b)
}

func (r ByteReader) ReadByte() (byte, error) {
	return r.Reader.ReadByte()
}

func ReadVarInt(rd io.ByteReader) (int32, error) {
	var (
		value    int32
		position uint
	)

	for {
		currentByte, err := rd.ReadByte()
		if err != nil {
			return 0, err
		}

		value |= int32(currentByte&VarSegmentBits) << position

		if currentByte&VarContinueBit == 0 {
			break
		}

		position += 7

		if position >= 32 {
			return 0, errors.New("VarInt is too big")
		}
	}

	return value, nil
}

func WriteVarInt(wr io.ByteWriter, value int32) error {
	uValue := uint32(value)

	for {
		if (uValue & ^uint32(VarSegmentBits)) == 0 {
			return wr.WriteByte(byte(uValue))
		}

		err := wr.WriteByte(byte(uValue&VarSegmentBits) | VarContinueBit)
		if err != nil {
			return err
		}

		uValue >>= 7
	}
}

func ReadVarLong(rd io.ByteReader) (int64, error) {
	var (
		value    int64
		position uint
	)

	for {
		currentByte, err := rd.ReadByte()
		if err != nil {
			return 0, err
		}

		value |= int64(currentByte&VarSegmentBits) << position

		if currentByte&VarContinueBit == 0 {
			break
		}

		position += 7

		if position >= 64 {
			return 0, errors.New("VarLong is too big")
		}
	}

	return value, nil
}

func WriteVarLong(wr io.ByteWriter, value int64) error {
	uValue := uint64(value)

	for {
		if (uValue & ^uint64(VarSegmentBits)) == 0 {
			return wr.WriteByte(byte(uValue))
		}

		err := wr.WriteByte(byte(uValue&uint64(VarSegmentBits)) | VarContinueBit)
		if err != nil {
			return err
		}

		uValue >>= 7
	}
}

func ReadString(rd *ByteReader, max int) (string, error) {
	ln, err := ReadVarInt(rd)
	if err != nil {
		return "", err
	}

	if int(ln) < 0 || int(ln) > max*3+3 {
		return "", errors.New("string too long")
	}

	buf := make([]byte, int(ln))

	_, err = io.ReadFull(rd, buf)
	if err != nil {
		return "", err
	}

	return string(buf), nil
}

func WriteString(wr ExtendedWriter, str string) error {
	b := []byte(str)

	err := WriteVarInt(wr, int32(len(b)))
	if err != nil {
		return err
	}

	_, err = wr.Write(b)
	return err
}

func ReadBytes(rd *bytes.Reader) ([]byte, error) {
	ln, err := ReadVarInt(rd)
	if err != nil {
		return nil, err
	}

	if ln < 0 {
		return nil, io.ErrUnexpectedEOF
	}

	buf := make([]byte, ln)

	_, err = io.ReadFull(rd, buf)
	return buf, err
}

func WriteBytes(wr ExtendedWriter, b []byte) error {
	err := WriteVarInt(wr, int32(len(b)))
	if err != nil {
		return err
	}

	_, err = wr.Write(b)
	return err
}

func ReadInt(rd io.Reader) (int32, error) {
	var value int32

	err := binary.Read(rd, binary.BigEndian, &value)
	return value, err
}

func WriteInt(wr ExtendedWriter, value int32) error {
	var encoded [4]byte

	binary.BigEndian.PutUint32(encoded[:], uint32(value))

	_, err := wr.Write(encoded[:])
	return err
}

func ReadLong(rd io.Reader) (int64, error) {
	var value int64

	err := binary.Read(rd, binary.BigEndian, &value)
	return value, err
}

func WriteLong(wr ExtendedWriter, value int64) error {
	var encoded [8]byte

	binary.BigEndian.PutUint64(encoded[:], uint64(value))

	_, err := wr.Write(encoded[:])
	return err
}

func ReadShort(rd io.Reader) (int16, error) {
	var value int16

	err := binary.Read(rd, binary.BigEndian, &value)
	return value, err
}

func WriteShort(wr ExtendedWriter, value int16) error {
	var encoded [2]byte

	binary.BigEndian.PutUint16(encoded[:], uint16(value))

	_, err := wr.Write(encoded[:])
	return err
}

func ReadFloat(rd io.Reader) (float32, error) {
	var value float32

	err := binary.Read(rd, binary.BigEndian, &value)
	return value, err
}

func WriteFloat(wr ExtendedWriter, value float32) error {
	return WriteInt(wr, int32(math.Float32bits(value)))
}

func ReadDouble(rd io.Reader) (float64, error) {
	var value float64

	err := binary.Read(rd, binary.BigEndian, &value)
	return value, err
}

func WriteDouble(wr ExtendedWriter, value float64) error {
	return WriteLong(wr, int64(math.Float64bits(value)))
}

func ReadBool(rd io.ByteReader) (bool, error) {
	b, err := rd.ReadByte()
	if err != nil {
		return false, err
	}

	return b != 0, nil
}

func WriteBool(wr io.ByteWriter, value bool) error {
	if value {
		return wr.WriteByte(1)
	}

	return wr.WriteByte(0)
}

func ReadStringBytes(b []byte) (string, error) {
	br := bytes.NewReader(b)
	rd := ByteReader{br}

	return ReadString(&rd, 32767)
}

func RandomBytes(n int) ([]byte, error) {
	b := make([]byte, n)

	_, err := rand.Read(b)
	return b, err
}
