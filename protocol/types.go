package protocol

import (
	"bufio"
	"bytes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"sync"
)

const (
	VarSegmentBits = 0x7F
	VarContinueBit = 0x80
)

type ExtendedWriter interface {
	io.Writer
	io.ByteWriter
}

type MCConnection struct {
	conn net.Conn
	wbuf *bufio.Writer

	wmu sync.Mutex // serializes outbound writes (keep-alive goroutine + handler loop)

	enc cipher.Stream // outbound encrypt
	dec cipher.Stream // inbound decrypt

	zlibWr io.WriteCloser
	zlibRd io.ReadCloser

	compThr int
}

type Packet struct {
	ID   int32
	Data []byte
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

func ReadVarInt(r io.ByteReader) (int32, error) {
	var (
		value    int32
		position uint
	)

	for {
		currentByte, err := r.ReadByte()
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

func WriteVarInt(w io.ByteWriter, value int32) error {
	uValue := uint32(value)

	for {
		if (uValue & ^uint32(VarSegmentBits)) == 0 {
			return w.WriteByte(byte(uValue))
		}

		err := w.WriteByte(byte(uValue&VarSegmentBits) | VarContinueBit)
		if err != nil {
			return err
		}

		uValue >>= 7
	}
}

func ReadVarLong(r io.ByteReader) (int64, error) {
	var (
		value    int64
		position uint
	)

	for {
		currentByte, err := r.ReadByte()
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

func WriteVarLong(w io.ByteWriter, value int64) error {
	uValue := uint64(value)

	for {
		if (uValue & ^uint64(VarSegmentBits)) == 0 {
			return w.WriteByte(byte(uValue))
		}

		err := w.WriteByte(byte(uValue&uint64(VarSegmentBits)) | VarContinueBit)
		if err != nil {
			return err
		}

		uValue >>= 7
	}
}

func ReadStringBytes(b []byte) (string, error) {
	br := bytes.NewReader(b)
	rd := ByteReader{br}

	return ReadString(&rd, 32767)
}

func WriteString(w ExtendedWriter, str string) error {
	b := []byte(str)

	if err := WriteVarInt(w, int32(len(b))); err != nil {
		return err
	}

	_, err := w.Write(b)
	return err
}

func ReadBytes(r *bytes.Reader) ([]byte, error) {
	ln, err := ReadVarInt(r)
	if err != nil {
		return nil, err
	}

	if ln < 0 {
		return nil, io.ErrUnexpectedEOF
	}

	buf := make([]byte, ln)

	_, err = io.ReadFull(r, buf)
	return buf, err
}

func WriteBytes(w ExtendedWriter, b []byte) error {
	err := WriteVarInt(w, int32(len(b)))
	if err != nil {
		return err
	}

	_, err = w.Write(b)
	return err
}

func WriteInt(w ExtendedWriter, value int32) error {
	return binary.Write(w, binary.BigEndian, value)
}

func WriteLong(w ExtendedWriter, value int64) error {
	return binary.Write(w, binary.BigEndian, value)
}

func WriteShort(w ExtendedWriter, value int16) error {
	return binary.Write(w, binary.BigEndian, value)
}

func WriteFloat(w ExtendedWriter, value float32) error {
	return binary.Write(w, binary.BigEndian, value)
}

func WriteDouble(w ExtendedWriter, value float64) error {
	return binary.Write(w, binary.BigEndian, value)
}

func WriteBool(w io.ByteWriter, value bool) error {
	if value {
		return w.WriteByte(1)
	}

	return w.WriteByte(0)
}

func WriteVarIntToBytes(v int32) ([]byte, error) {
	var buf bytes.Buffer

	err := WriteVarInt(&buf, v)
	if err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// RandomBytes returns n random bytes.
func RandomBytes(n int) ([]byte, error) {
	b := make([]byte, n)

	_, err := rand.Read(b)
	return b, err
}
