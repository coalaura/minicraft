package protocol

import (
	"bufio"
	"bytes"
	"compress/zlib"
	"crypto/aes"
	"errors"
	"io"

	"github.com/coalaura/minicraft/crypto"
)

func NewConn(rwc io.ReadWriteCloser) *MCConnection {
	return &MCConnection{
		rw: bufio.NewReadWriter(bufio.NewReader(rwc), bufio.NewWriter(rwc)),
	}
}

func (c *MCConnection) EnableEncryption(secret []byte) error {
	if len(secret) != 16 {
		return errors.New("secret must be 16 bytes")
	}

	block, err := aes.NewCipher(secret)
	if err != nil {
		return err
	}

	c.enc = crypto.NewCFB8(block, secret, false)
	c.dec = crypto.NewCFB8(block, secret, true)

	return nil
}

func (c *MCConnection) Flush() error {
	return c.rw.Flush()
}

func (c *MCConnection) SetCompression(threshold int) {
	c.compThr = threshold
}

func (c *MCConnection) ReadRaw(n int) ([]byte, error) {
	buf := make([]byte, n)

	_, err := io.ReadFull(c.rw, buf)
	if err != nil {
		return nil, err
	}

	if c.dec != nil {
		c.dec.XORKeyStream(buf, buf)
	}

	return buf, nil
}

func (c *MCConnection) ReadPacket() (*Packet, error) {
	// Outer length (VarInt), possibly encrypted
	ln, err := ReadVarInt(c.rw)
	if err != nil {
		return nil, err
	}

	if ln < 0 {
		return nil, io.ErrUnexpectedEOF
	}

	payload, err := c.ReadRaw(int(ln))
	if err != nil {
		return nil, err
	}

	br := bytes.NewReader(payload)
	rd := ByteReader{br}

	if c.compThr > 0 {
		ulen, err := ReadVarInt(rd)
		if err != nil {
			return nil, err
		}

		if ulen != 0 {
			zr, err := zlib.NewReader(br)
			if err != nil {
				return nil, err
			}

			defer zr.Close()

			decompressed := make([]byte, ulen)

			_, err = io.ReadFull(zr, decompressed)
			if err != nil {
				return nil, err
			}

			br = bytes.NewReader(decompressed)
			rd = ByteReader{br}
		}
	}

	id, err := ReadVarInt(rd)
	if err != nil {
		return nil, err
	}

	rest, _ := io.ReadAll(br)

	return &Packet{
		ID:   id,
		Data: rest,
	}, nil
}

func (c *MCConnection) WritePacket(packet Packet) error {
	var inner bytes.Buffer

	err := WriteVarInt(&inner, packet.ID)
	if err != nil {
		return err
	}

	_, err = inner.Write(packet.Data)
	if err != nil {
		return err
	}

	payload := inner.Bytes()

	if c.compThr > 0 {
		var buf bytes.Buffer

		if len(payload) >= c.compThr {
			// write ulen, then compress payload
			err = WriteVarInt(&buf, int32(len(payload)))
			if err != nil {
				return err
			}

			zw := zlib.NewWriter(&buf)

			_, err = zw.Write(payload)
			if err != nil {
				return err
			}

			err = zw.Close()
			if err != nil {
				return err
			}
		} else {
			// uncompressed payload: ulen=0 then id+data
			err = WriteVarInt(&buf, 0)
			if err != nil {
				return err
			}

			_, err = inner.Write(payload)
			if err != nil {
				return err
			}
		}

		payload = buf.Bytes()
	}

	var pkt bytes.Buffer

	// Prefix length
	if err := WriteVarInt(&pkt, int32(len(payload))); err != nil {
		return err
	}

	if _, err := pkt.Write(payload); err != nil {
		return err
	}

	out := pkt.Bytes()

	if c.enc != nil {
		c.enc.XORKeyStream(out, out)
	}

	_, err = c.rw.Write(out)
	if err != nil {
		return err
	}

	return c.rw.Flush()
}
