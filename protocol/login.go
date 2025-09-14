package protocol

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/coalaura/minicraft/config"
)

type LoginStart struct {
	Name string
}

type SessionResponse struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func HandleLogin(ctx context.Context, c *MCConnection, cfg *config.Config, clientProto int) {
	packet, err := c.ReadPacket()
	if err != nil || packet.ID != SB_LoginStart {
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

	verifyToken, err := RandomBytes(4)
	if err != nil {
		HandleLoginError(c, "failed to get random bytes", err)

		return
	}

	var data bytes.Buffer

	// serverId is empty string since 1.7+
	err = WriteString(&data, "")
	if err != nil {
		HandleLoginError(c, "failed to write server id", err)

		return
	}

	err = WriteBytes(&data, cfg.Key.Public)
	if err != nil {
		HandleLoginError(c, "failed to write server public key", err)

		return
	}

	err = WriteBytes(&data, verifyToken)
	if err != nil {
		HandleLoginError(c, "failed to write verify token", err)

		return
	}

	err = c.WritePacket(Packet{
		ID:   CB_EncryptionRequest,
		Data: data.Bytes(),
	})

	if err != nil {
		HandleLoginError(c, "failed to write encryption request", err)

		return
	}

	// Expect Encryption Response
	packet, err = c.ReadPacket()
	if err != nil || packet.ID != SB_EncryptionResponse {
		return
	}

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

	ok, uuidStr := HasJoined(ctx, name, serverHash)
	if !ok {
		SendLoginDisconnect(c, "Failed to verify username")

		return
	}

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
	}

	// Enable encryption AFTER we validated token and before sending Login Success
	err = c.EnableEncryption(secret)
	if err != nil {
		SendLoginDisconnect(c, "Encryption not available on server")

		return
	}

	var success bytes.Buffer

	err = WriteUUIDString(&success, uuidStr)
	if err != nil {
		HandleLoginError(c, "failed to write uuid", err)

		return
	}

	WriteString(&success, name)
	if err != nil {
		HandleLoginError(c, "failed to write name", err)

		return
	}

	// properties: 0 (for now, we'll send skins/profile later via Player Info Update in play)
	err = WriteVarInt(&success, 0)
	if err != nil {
		HandleLoginError(c, "failed to write properties", err)

		return
	}

	err = c.WritePacket(Packet{
		ID:   CB_LoginSuccess,
		Data: success.Bytes(),
	})

	if err != nil {
		HandleLoginError(c, "failed to write login success", err)

		return
	}

	packet, err = c.ReadPacket() // expect SB_LoginAcknowledged
	if err != nil {
		HandleLoginError(c, "failed to read ack packet", err)

		return
	}

	if packet.ID != SB_LoginAcknowledged {
		HandleLoginError(c, "client did not acknowledge login success", err)

		return
	}

	HandleConfiguration(ctx, c, cfg, uuidStr, name)
}

func HandleLoginError(c *MCConnection, msg string, err error) {
	log.Warnf("[login] %s: %v\n", msg, err)

	SendLoginDisconnect(c, "Something went wrong")
}

func SendLoginDisconnect(c *MCConnection, msg string) {
	js, _ := json.Marshal(map[string]any{
		"text": msg,
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

func HasJoined(ctx context.Context, username, serverHash string) (bool, string) {
	// https://sessionserver.mojang.com/session/minecraft/hasJoined?username=<>&serverId=<>
	// For privacy/modern changes, the endpoint and params remain; docs still reflect this flow.
	url := fmt.Sprintf("https://sessionserver.mojang.com/session/minecraft/hasJoined?username=%s&serverId=%s", username, serverHash)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return false, ""
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false, ""
	}

	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return false, ""
	}

	var out SessionResponse

	err = json.NewDecoder(resp.Body).Decode(&out)
	if err != nil {
		return false, ""
	}

	return out.ID != "", FormatUUID(out.ID)
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
	}

	_, err = w.Write(uuid[:])
	return err
}
