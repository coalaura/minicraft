package protocol

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/coalaura/minicraft/config"
)

type LoginStart struct {
	Name string
}

type ProfileProperty struct {
	Name      string `json:"name"`
	Value     string `json:"value"`
	Signature string `json:"signature,omitempty"`
}

type SessionResponse struct {
	ID         string            `json:"id"`
	Name       string            `json:"name"`
	Properties []ProfileProperty `json:"properties"`
}

func HandleLogin(ctx context.Context, c *MCConnection, cfg *config.Config, clientProto int) {
	c.Print("login", "processing login (proto=%d)", clientProto)

	packet, err := c.ReadPacket()
	if err != nil || packet.ID != SB_LoginStart {
		HandleLoginError(c, "invalid first packet", err)

		return
	}

	name, err := ReadStringBytes(packet.Data)
	if err != nil {
		HandleLoginError(c, "failed to read name", err)

		return
	}

	if name == "" {
		SendLoginDisconnect(c, "Invalid username")

		return
	}

	var data bytes.Buffer

	// serverId is empty string since 1.7+
	WriteString(&data, "")

	WriteBytes(&data, cfg.Key.Public)

	verifyToken, err := RandomBytes(16)
	if err != nil {
		HandleLoginError(c, "failed to get random bytes", err)

		return
	}

	WriteBytes(&data, verifyToken)

	if cfg.OnlineMode {
		data.WriteByte(0x01)
	} else {
		data.WriteByte(0x00)
	}

	err = c.WritePacket(Packet{
		ID:   CB_EncryptionRequest,
		Data: data.Bytes(),
	})

	if err != nil {
		HandleLoginError(c, "failed to write encryption request", err)

		return
	}

	c.Print("login", "sent encryption request")

	packet, err = c.ReadPacket()
	if err != nil || packet.ID != SB_EncryptionResponse {
		HandleLoginError(c, "invalid encryption response packet", err)

		return
	}

	c.Print("login", "received encryption response")

	rd := bytes.NewReader(packet.Data)

	secretEnc, err := ReadBytes(rd)
	if err != nil {
		HandleLoginError(c, "failed to read encrypted secret", err)

		return
	}

	tokenEnc, err := ReadBytes(rd)
	if err != nil {
		HandleLoginError(c, "failed to read encrypted token", err)

		return
	}

	secret, err := rsa.DecryptPKCS1v15(rand.Reader, cfg.Key.Private, secretEnc)
	if err != nil || len(secret) != 16 {
		SendLoginDisconnect(c, "Bad encryption")

		return
	}

	token, err := rsa.DecryptPKCS1v15(rand.Reader, cfg.Key.Private, tokenEnc)
	if err != nil || !bytes.Equal(token, verifyToken) {
		SendLoginDisconnect(c, "Bad verify token")

		return
	}

	// Compute server hash (minecraft hexdigest)
	// sha1 of (serverId "" + secret + pubkey)
	h := sha1.New()

	h.Write([]byte{})
	h.Write(secret)
	h.Write(cfg.Key.Public)

	sum := h.Sum(nil)

	serverHash := MCHexDigest(sum)

	if !cfg.OnlineMode {
		SendLoginDisconnect(c, "This server requires online-mode")

		return
	}

	c.Print("login", "verifying login")

	ok, uuidStr, properties := HasJoined(ctx, name, serverHash)
	if !ok {
		SendLoginDisconnect(c, "Failed to verify username")

		return
	}

	c.Print("login", "verified login (uuid=%s)", uuidStr)

	// Enable encryption AFTER we validated token and before sending Login Success
	err = c.EnableEncryption(secret)
	if err != nil {
		SendLoginDisconnect(c, "Encryption not available on server")

		return
	}

	c.Print("login", "enabled encryption")

	// Optionally set compression before Login Success
	if cfg.CompressionThreshold > 0 {
		b, err := WriteVarIntToBytes(int32(cfg.CompressionThreshold))
		if err != nil {
			HandleLoginError(c, "failed to write compression threshold to bytes", err)
		}

		err = c.WritePacket(Packet{
			ID:   CB_SetCompression,
			Data: b,
		})

		if err != nil {
			HandleLoginError(c, "failed to write compression threshold", err)

			return
		}

		c.SetCompression(cfg.CompressionThreshold)

		c.Print("login", "set compression threshold to %d", cfg.CompressionThreshold)
	}

	var success bytes.Buffer

	WriteUUIDString(&success, uuidStr)
	WriteString(&success, name)
	WriteVarInt(&success, int32(len(properties)))

	for _, prop := range properties {
		WriteString(&success, prop.Name)
		WriteString(&success, prop.Value)

		if prop.Signature != "" {
			success.WriteByte(0x01)

			WriteString(&success, prop.Signature)
		} else {
			success.WriteByte(0x00)
		}
	}

	err = c.WritePacket(Packet{
		ID:   CB_LoginSuccess,
		Data: success.Bytes(),
	})

	if err != nil {
		HandleLoginError(c, "failed to write login success", err)

		return
	}

	c.Print("login", "sent login success")

	packet, err = c.ReadPacket()
	if err != nil {
		HandleLoginError(c, "failed to read ack packet", err)

		return
	}

	if packet.ID != SB_LoginAcknowledged {
		HandleLoginError(c, "client did not acknowledge login success", err)

		return
	}

	c.Print("login", "received login acknowledge")

	HandleConfiguration(ctx, c, cfg, uuidStr, name)
}

func HandleLoginError(c *MCConnection, msg string, err error) {
	c.Warn("login", "%s: %v", msg, err)

	SendLoginDisconnect(c, "Something went wrong")
}

func SendLoginDisconnect(c *MCConnection, reason string) {
	js, _ := json.Marshal(map[string]any{
		"text": reason,
	})

	var b bytes.Buffer

	err := WriteString(&b, string(js))
	if err != nil {
		log.Warnf("failed to write login disconnect: %v\n", err)

		return
	}

	c.WritePacket(Packet{
		ID:   CB_DisconnectLogin,
		Data: b.Bytes(),
	})
}

func HasJoined(ctx context.Context, username, serverHash string) (bool, string, []ProfileProperty) {
	// https://sessionserver.mojang.com/session/minecraft/hasJoined?username=<>&serverId=<>
	// For privacy/modern changes, the endpoint and params remain; docs still reflect this flow.
	url := fmt.Sprintf("https://sessionserver.mojang.com/session/minecraft/hasJoined?username=%s&serverId=%s", username, serverHash)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return false, "", nil
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false, "", nil
	}

	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return false, "", nil
	}

	var out SessionResponse

	err = json.NewDecoder(resp.Body).Decode(&out)
	if err != nil {
		return false, "", nil
	}

	return out.ID != "", FormatUUID(out.ID), out.Properties
}

// MCHexDigest renders SHA1 sum as Minecraft's signed big-int hex (may be prefixed with '-')
func MCHexDigest(b []byte) string {
	// Interpret as two's complement big-endian, then base16 with sign if negative.
	neg := (b[0] & 0x80) != 0

	if neg {
		// two's complement negate
		two := make([]byte, len(b))

		copy(two, b)

		// invert
		for i := range two {
			two[i] = ^two[i]
		}

		// add 1
		for i := len(two) - 1; i >= 0; i-- {
			two[i]++

			if two[i] != 0 {
				break
			}
		}

		// strip leading zeros
		two = StripLeadingZeros(two)

		return "-" + hex.EncodeToString(two)
	}

	b = StripLeadingZeros(b)

	return hex.EncodeToString(b)
}

func StripLeadingZeros(b []byte) []byte {
	var i int

	for i < len(b)-1 && b[i] == 0 {
		i++
	}

	return b[i:]
}

func FormatUUID(nodash string) string {
	// 32 hex => 8-4-4-4-12
	if len(nodash) != 32 {
		return nodash
	}

	return fmt.Sprintf("%s-%s-%s-%s-%s", nodash[0:8], nodash[8:12], nodash[12:16], nodash[16:20], nodash[20:])
}

func WriteUUIDString(w io.Writer, str string) error {
	// UUID as binary 16 bytes (most 64, least 64 big-endian)
	var uuid [16]byte

	hexBytes, err := hex.DecodeString(strings.ReplaceAll(str, "-", ""))
	if err != nil {
		return err
	}

	if len(hexBytes) == 16 {
		copy(uuid[:], hexBytes)
	} else {
		return errors.New("malformed uuid")
	}

	_, err = w.Write(uuid[:])
	return err
}
