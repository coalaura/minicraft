package protocol

import (
	"bufio"
	"bytes"
	"compress/zlib"
	"crypto/aes"
	"encoding/hex"
	"errors"
	"io"
	"net"

	"github.com/coalaura/minicraft/crypto"
)

func NewConn(conn net.Conn) *MCConnection {
	return &MCConnection{
		conn: conn,
		wbuf: bufio.NewWriter(conn),
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

func (c *MCConnection) ReadByte() (byte, error) {
	var buf [1]byte

	if _, err := io.ReadFull(c.conn, buf[:]); err != nil {
		return 0, err
	}

	if c.dec != nil {
		c.dec.XORKeyStream(buf[:], buf[:])
	}

	return buf[0], nil
}

func (c *MCConnection) SetCompression(threshold int) {
	c.compThr = threshold
}

func (c *MCConnection) Read(p []byte) (int, error) {
	for i := range p {
		b, err := c.ReadByte()
		if err != nil {
			if i == 0 {
				return 0, err
			}

			return i, err
		}

		p[i] = b
	}

	return len(p), nil
}

func (c *MCConnection) ReadPacket() (*Packet, error) {
	ln, err := ReadVarInt(c)
	if err != nil {
		return nil, err
	}

	if ln < 0 {
		return nil, io.ErrUnexpectedEOF
	}

	payload := make([]byte, ln)

	if _, err := io.ReadFull(c, payload); err != nil {
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

	pkt := &Packet{
		ID:   id,
		Data: rest,
	}

	c.logPacket("RECV", pkt)

	return pkt, nil
}

func (c *MCConnection) WritePacket(packet Packet) error {
	c.wmu.Lock()
	defer c.wmu.Unlock()

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

			_, err = buf.Write(payload)
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

	c.logPacket("SEND", &packet)

	if c.enc != nil {
		c.enc.XORKeyStream(out, out)
	}

	_, err = c.wbuf.Write(out)
	if err != nil {
		return err
	}

	return c.wbuf.Flush()
}

func (c *MCConnection) logPacket(direction string, p *Packet) {
	if log == nil {
		return
	}

	// Don't log high-frequency packets
	if direction == "RECV" {
		switch p.ID {
		case SB_ClientTickEnd, SB_MovePlayerPos, SB_MovePlayerPosRot, SB_MovePlayerRot, SB_MoveStatusOnly:
			return
		}
	}

	data := p.Data

	if len(data) > 64 {
		data = data[:64]
	}

	log.Debugf(
		"[net] %s %s -> id=%d (0x%x) len=%d data=%s\n",
		direction,
		c.conn.RemoteAddr(),
		p.ID,
		p.ID,
		len(p.Data),
		hex.EncodeToString(data),
	)
}
