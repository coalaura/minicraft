package protocol

import (
	"bufio"
	"bytes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"io"
	"unicode/utf8"
)

const (
	MaxVarIntBytes  = 5
	MaxVarLongBytes = 10
)

type MCConnection struct {
	rw *bufio.ReadWriter

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
		n      int
		result int32
	)

	for {
		if n >= MaxVarIntBytes {
			return 0, errors.New("varint too long")
		}

		b, err := r.ReadByte()
		if err != nil {
			return 0, err
		}

		value := int32(b & 0x7F)

		result |= value << (7 * n)

		n++

		if b&0x80 == 0 {
			break
		}
	}

	return result, nil
}

func WriteVarInt(w io.Writer, v int32) error {
	var (
		i   int
		buf [5]byte
	)

	for {
		temp := byte(v & 0x7F)

		v >>= 7
		if v != 0 {
			temp |= 0x80
		}

		buf[i] = temp

		i++

		if v == 0 {
			break
		}
	}

	_, err := w.Write(buf[:i])
	return err
}

func ReadStringBytes(b []byte) (string, error) {
	br := bytes.NewReader(b)
	rd := ByteReader{br}

	return ReadString(&rd, 32767)
}

func WriteString(w io.Writer, str string) error {
	ln := int32(utf8.RuneCountInString(str))

	err := WriteVarInt(w, ln)
	if err != nil {
		return err
	}

	_, err = w.Write([]byte(str))
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

func WriteBytes(w io.Writer, b []byte) error {
	err := WriteVarInt(w, int32(len(b)))
	if err != nil {
		return err
	}

	_, err = w.Write(b)
	return err
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
