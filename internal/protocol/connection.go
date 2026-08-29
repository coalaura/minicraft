package protocol

import (
	"bufio"
	"bytes"
	"compress/zlib"
	"crypto/aes"
	"crypto/cipher"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/coalaura/minicraft/internal/crypto"
)

const (
	maxPacketFrameLength = 0x1FFFFF
	maxPacketDataLength  = 8 * 1024 * 1024
)

type Connection struct {
	conn net.Conn
	wbuf *bufio.Writer

	log Logger

	wmu sync.Mutex

	enc cipher.Stream
	dec cipher.Stream

	compThr int

	writeInner      bytes.Buffer
	writeCompressed bytes.Buffer
	writePacket     bytes.Buffer
	zlibWriter      *zlib.Writer
}

func NewConnection(conn net.Conn, log Logger) *Connection {
	return &Connection{
		conn: conn,
		wbuf: bufio.NewWriter(conn),
		log:  log,
	}
}

func (c *Connection) EnableEncryption(secret []byte) error {
	c.wmu.Lock()
	defer c.wmu.Unlock()

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

func (c *Connection) SetCompression(threshold int) {
	c.wmu.Lock()
	defer c.wmu.Unlock()

	c.compThr = threshold
}

func (c *Connection) ReadByte() (byte, error) {
	var buf [1]byte

	_, err := io.ReadFull(c.conn, buf[:])
	if err != nil {
		return 0, err
	}

	if c.dec != nil {
		c.dec.XORKeyStream(buf[:], buf[:])
	}

	return buf[0], nil
}

func (c *Connection) Read(p []byte) (int, error) {
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

func (c *Connection) ReadPacket() (*Packet, error) {
	frameLength, err := readPacketFrameLength(c)
	if err != nil {
		return nil, err
	}

	payload := make([]byte, frameLength)

	_, err = io.ReadFull(c, payload)
	if err != nil {
		return nil, err
	}

	br := bytes.NewReader(payload)
	rd := ByteReader{br}

	if c.compThr > 0 {
		uncompressedLength, err := ReadVarInt(rd)
		if err != nil {
			return nil, err
		}

		if uncompressedLength < 0 || uncompressedLength > maxPacketDataLength {
			return nil, fmt.Errorf("invalid uncompressed packet length %d", uncompressedLength)
		}

		if uncompressedLength != 0 {
			if int(uncompressedLength) < c.compThr {
				return nil, fmt.Errorf("compressed packet length %d is below threshold %d", uncompressedLength, c.compThr)
			}

			zr, err := zlib.NewReader(br)
			if err != nil {
				return nil, err
			}

			defer zr.Close()

			decompressed := make([]byte, uncompressedLength)

			_, err = io.ReadFull(zr, decompressed)
			if err != nil {
				return nil, fmt.Errorf("decompressed packet length does not match declared length %d: %w", uncompressedLength, err)
			}

			var extra [1]byte

			extraLength, extraErr := zr.Read(extra[:])
			if extraLength != 0 || !errors.Is(extraErr, io.EOF) {
				if extraErr == nil {
					extraErr = errors.New("decompressed packet contains extra data")
				}

				return nil, fmt.Errorf("decompressed packet length does not match declared length %d: %w", uncompressedLength, extraErr)
			}

			if br.Len() != 0 {
				return nil, errors.New("compressed packet contains trailing data")
			}

			br = bytes.NewReader(decompressed)
			rd = ByteReader{br}
		} else if br.Len() >= c.compThr {
			return nil, fmt.Errorf("uncompressed packet length %d meets compression threshold %d", br.Len(), c.compThr)
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

func readPacketFrameLength(rd io.ByteReader) (int, error) {
	var frameLength int

	for index := range 3 {
		currentByte, err := rd.ReadByte()
		if err != nil {
			return 0, err
		}

		frameLength |= int(currentByte&VarSegmentBits) << (7 * index)

		if currentByte&VarContinueBit == 0 {
			if frameLength == 0 {
				return 0, errors.New("packet frame length must be positive")
			}

			if frameLength > maxPacketFrameLength {
				return 0, fmt.Errorf("packet frame length %d exceeds maximum %d", frameLength, maxPacketFrameLength)
			}

			return frameLength, nil
		}
	}

	return 0, errors.New("packet frame length VarInt is too big")
}

func (c *Connection) WritePacket(packet Packet) error {
	c.wmu.Lock()
	defer c.wmu.Unlock()

	c.writeInner.Reset()

	err := WriteVarInt(&c.writeInner, packet.ID)
	if err != nil {
		return err
	}

	_, err = c.writeInner.Write(packet.Data)
	if err != nil {
		return err
	}

	payload := c.writeInner.Bytes()

	if c.compThr > 0 {
		c.writeCompressed.Reset()

		if len(payload) >= c.compThr {
			// write ulen, then compress payload
			err = WriteVarInt(&c.writeCompressed, int32(len(payload)))
			if err != nil {
				return err
			}

			if c.zlibWriter == nil {
				c.zlibWriter = zlib.NewWriter(&c.writeCompressed)
			} else {
				c.zlibWriter.Reset(&c.writeCompressed)
			}

			_, err = c.zlibWriter.Write(payload)
			if err != nil {
				return err
			}

			err = c.zlibWriter.Close()
			if err != nil {
				return err
			}
		} else {
			// uncompressed payload: ulen=0 then id+data
			err = WriteVarInt(&c.writeCompressed, 0)
			if err != nil {
				return err
			}

			_, err = c.writeCompressed.Write(payload)
			if err != nil {
				return err
			}
		}

		payload = c.writeCompressed.Bytes()
	}

	c.writePacket.Reset()

	// Prefix length
	err = WriteVarInt(&c.writePacket, int32(len(payload)))
	if err != nil {
		return err
	}

	_, err = c.writePacket.Write(payload)
	if err != nil {
		return err
	}

	out := c.writePacket.Bytes()

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

func (c *Connection) RemoteAddr() net.Addr {
	return c.conn.RemoteAddr()
}

func (c *Connection) Close() error {
	return c.conn.Close()
}

func (c *Connection) SetReadDeadline(deadline time.Time) error {
	return c.conn.SetReadDeadline(deadline)
}

func (c *Connection) SetWriteDeadline(deadline time.Time) error {
	return c.conn.SetWriteDeadline(deadline)
}

// TODO: implement correctly later, not now
func (c *Connection) logPacket(direction string, p *Packet) {
	if c.log == nil {
		return
	}

	// Don't log high-frequency packets
	if direction == "RECV" {
		switch p.ID {
		case ServerboundClientTickEndID,
			ServerboundMovePlayerPositionID,
			ServerboundMovePlayerRotationID,
			ServerboundMovePlayerPositionRotationID,
			ServerboundMovePlayerStatusID:
			return
		}
	}

	data := p.Data

	if len(data) > 64 {
		data = data[:64]
	}

	c.log.Debugf(
		"[net] %s %s -> id=%d (0x%x) len=%d data=%s\n",
		direction,
		c.conn.RemoteAddr(),
		p.ID,
		p.ID,
		len(p.Data),
		hex.EncodeToString(data),
	)
}
