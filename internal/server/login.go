package server

import (
	"context"
	"crypto/md5"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

type SessionProfileResponse struct {
	ID         string                 `json:"id"`
	Name       string                 `json:"name"`
	Properties []game.ProfileProperty `json:"properties"`
}

func (s *Session) handleLogin(ctx context.Context) error {
	s.Log.Printf("[login] %s - processing login\n", s.Conn.RemoteAddr())

	packet, err := s.readPacket()
	if err != nil {
		return fmt.Errorf("read login start: %w", err)
	}

	if packet.ID != protocol.ServerboundLoginStartID {
		return errors.New("invalid first login packet")
	}

	start, err := protocol.DecodeLoginStart(packet.Data)
	if err != nil {
		return s.sendLoginDisconnect("Invalid username")
	}

	if start.Name == "" {
		return s.sendLoginDisconnect("Invalid username")
	}

	if s.Config.Server.OnlineMode {
		err = s.handleOnlineLogin(ctx, start)
	} else {
		err = s.handleOfflineLogin(start)
	}

	if err != nil {
		return err
	}

	if !s.Runtime.ReservePlayerSlot(s.Config.MaxPlayers()) {
		return s.sendLoginDisconnect("Server is full")
	}

	defer s.Runtime.ReleasePlayerSlot()

	err = s.sendLoginSuccess()
	if err != nil {
		return fmt.Errorf("send login success: %w", err)
	}

	s.Log.Printf("[login] %s - sent login success\n", s.Conn.RemoteAddr())

	packet, err = s.readPacket()
	if err != nil {
		return fmt.Errorf("read login acknowledged: %w", err)
	}

	if packet.ID != protocol.ServerboundLoginAcknowledgedID {
		return errors.New("client did not acknowledge login success")
	}

	s.Log.Printf("[login] %s - received login acknowledge\n", s.Conn.RemoteAddr())

	return s.handleConfiguration(ctx)
}

func (s *Session) handleOnlineLogin(ctx context.Context, start protocol.LoginStart) error {
	verifyToken, err := protocol.RandomBytes(16)
	if err != nil {
		return fmt.Errorf("get random bytes: %w", err)
	}

	err = s.sendEncryptionRequest(verifyToken)
	if err != nil {
		return fmt.Errorf("send encryption request: %w", err)
	}

	s.Log.Printf("[login] %s - sent encryption request\n", s.Conn.RemoteAddr())

	packet, err := s.readPacket()
	if err != nil {
		return fmt.Errorf("read encryption response: %w", err)
	}

	if packet.ID != protocol.ServerboundEncryptionResponseID {
		return errors.New("invalid encryption response packet")
	}

	response, err := protocol.DecodeEncryptionResponse(packet.Data)
	if err != nil {
		return fmt.Errorf("decode encryption response: %w", err)
	}

	secret, err := protocol.RandomBytes(16)
	if err != nil {
		return fmt.Errorf("get fallback shared secret: %w", err)
	}

	token, err := protocol.RandomBytes(len(verifyToken))
	if err != nil {
		return fmt.Errorf("get fallback verify token: %w", err)
	}

	//lint:ignore SA1019 Minecraft's login protocol requires RSA PKCS #1 v1.5 encryption.
	secretErr := rsa.DecryptPKCS1v15SessionKey(rand.Reader, s.Config.Key.Private, response.SharedSecret, secret)

	//lint:ignore SA1019 Minecraft's login protocol requires RSA PKCS #1 v1.5 encryption.
	tokenErr := rsa.DecryptPKCS1v15SessionKey(rand.Reader, s.Config.Key.Private, response.VerifyToken, token)

	tokenMatches := subtle.ConstantTimeCompare(token, verifyToken)

	if secretErr != nil || tokenErr != nil || tokenMatches != 1 {
		return s.sendLoginDisconnect("Bad encryption response")
	}

	// sha1 of (serverId "" + secret + pubkey), rendered as minecraft hexdigest
	hs := sha1.New()

	hs.Write(secret)
	hs.Write(s.Config.Key.Public)

	sum := hs.Sum(nil)

	neg := (sum[0] & 0x80) != 0

	if neg {
		for i := range sum {
			sum[i] = ^sum[i]
		}

		for i := len(sum) - 1; i >= 0; i-- {
			sum[i]++

			if sum[i] != 0 {
				break
			}
		}
	}

	var first int

	for first < len(sum)-1 && sum[first] == 0 {
		first++
	}

	var serverHash string

	if neg {
		serverHash = "-" + hex.EncodeToString(sum[first:])
	} else {
		serverHash = hex.EncodeToString(sum[first:])
	}

	s.Log.Printf("[login] %s - verifying login\n", s.Conn.RemoteAddr())

	player, err := authenticatePlayer(start.Name, serverHash, "")
	if err != nil {
		s.Log.Warnf("[login] %s - authentication failed: %v\n", s.Conn.RemoteAddr(), err)

		return s.sendLoginDisconnect("Failed to verify username")
	}

	player.Position = s.Runtime.World.Spawn
	player.GameMode = s.Config.GameMode()

	s.Player = player

	s.Log.Printf("[login] %s - verified login (uuid=%s)\n", s.Conn.RemoteAddr(), player.UUID)

	err = s.Conn.EnableEncryption(secret)
	if err != nil {
		return s.sendLoginDisconnect("Encryption not available on server")
	}

	s.Log.Printf("[login] %s - enabled encryption\n", s.Conn.RemoteAddr())

	if s.Config.Network.CompressionThreshold > 0 {
		var wr protocol.PacketWriter

		compression := protocol.SetCompression{Threshold: int32(s.Config.Network.CompressionThreshold)}

		compression.Encode(&wr)

		err = wr.Err()
		if err != nil {
			return fmt.Errorf("encode set compression: %w", err)
		}

		err = s.writeRawPacket(protocol.Packet{
			ID:   protocol.ClientboundSetCompressionID,
			Data: wr.Buffer.Bytes(),
		})

		if err != nil {
			return fmt.Errorf("send set compression: %w", err)
		}

		s.Conn.SetCompression(s.Config.Network.CompressionThreshold)

		s.Log.Printf("[login] %s - set compression threshold to %d\n", s.Conn.RemoteAddr(), s.Config.Network.CompressionThreshold)
	}

	return nil
}

func (s *Session) handleOfflineLogin(start protocol.LoginStart) error {
	sum := md5.Sum([]byte("OfflinePlayer:" + start.Name))

	sum[6] = (sum[6] & 0x0F) | 0x30
	sum[8] = (sum[8] & 0x3F) | 0x80

	raw := hex.EncodeToString(sum[:])

	uuid := fmt.Sprintf("%s-%s-%s-%s-%s", raw[0:8], raw[8:12], raw[12:16], raw[16:20], raw[20:])

	player := &game.Player{
		UUID: uuid,
		Name: start.Name,

		Position: s.Runtime.World.Spawn,

		GameMode: s.Config.GameMode(),
	}

	s.Player = player

	return nil
}

func (s *Session) sendEncryptionRequest(verifyToken []byte) error {
	var wr protocol.PacketWriter

	request := protocol.EncryptionRequest{
		ServerID:    "",
		PublicKey:   s.Config.Key.Public,
		VerifyToken: verifyToken,
	}

	request.Encode(&wr)

	err := wr.Err()
	if err != nil {
		return err
	}

	return s.writeRawPacket(protocol.Packet{
		ID:   protocol.ClientboundEncryptionRequestID,
		Data: wr.Buffer.Bytes(),
	})
}

func (s *Session) sendLoginSuccess() error {
	var wr protocol.PacketWriter

	success := protocol.LoginSuccess{
		UUID:       s.Player.UUID,
		Username:   s.Player.Name,
		Properties: s.Player.Properties,
	}

	success.Encode(&wr)

	err := wr.Err()
	if err != nil {
		return err
	}

	return s.writeRawPacket(protocol.Packet{
		ID:   protocol.ClientboundLoginSuccessID,
		Data: wr.Buffer.Bytes(),
	})
}

func (s *Session) sendLoginDisconnect(reason string) error {
	js, _ := json.Marshal(map[string]any{
		"text": reason,
	})

	var wr protocol.PacketWriter

	wr.String(string(js))

	err := wr.Err()
	if err != nil {
		s.Log.Warnf("[login] failed to write login disconnect: %v\n", err)

		return err
	}

	err = s.writeRawPacket(protocol.Packet{
		ID:   protocol.ClientboundLoginDisconnectID,
		Data: wr.Buffer.Bytes(),
	})
	if err != nil {
		return fmt.Errorf("send login disconnect: %w", err)
	}

	return fmt.Errorf("disconnected client: %s", reason)
}

func authenticatePlayer(username, serverHash, ip string) (*game.Player, error) {
	url := fmt.Sprintf(
		"https://sessionserver.mojang.com/session/minecraft/hasJoined?username=%s&serverId=%s",
		username, serverHash,
	)

	if ip != "" {
		url += "&ip=" + ip
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var profile SessionProfileResponse

	err = json.NewDecoder(resp.Body).Decode(&profile)
	if err != nil {
		return nil, err
	}

	if profile.ID == "" {
		return nil, errors.New("session verification failed")
	}

	if profile.Name == "" {
		profile.Name = username
	}

	rawUUID, err := hex.DecodeString(profile.ID)
	if err != nil || len(rawUUID) != 16 {
		return nil, errors.New("malformed uuid")
	}

	formatted := fmt.Sprintf("%x-%x-%x-%x-%x",
		rawUUID[0:4], rawUUID[4:6], rawUUID[6:8], rawUUID[8:10], rawUUID[10:16],
	)

	return &game.Player{
		UUID:       formatted,
		Name:       profile.Name,
		Properties: profile.Properties,

		GameMode: game.GameModeCreative,
	}, nil
}
