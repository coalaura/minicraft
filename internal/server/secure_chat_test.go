package server

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coalaura/minicraft/internal/config"
	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

type chatTestLogger struct {
	mu     sync.Mutex
	prints []string
}

type secureChatTestFixture struct {
	runtime             *Runtime
	sender              *Session
	recipient           *Session
	senderConnection    *recordingConnection
	recipientConnection *recordingConnection
	logger              *chatTestLogger
	now                 time.Time
	chatKey             *rsa.PrivateKey
	chatSession         protocol.ChatSession
}

type chatTestRoundTripper struct {
	calls int
}

type chatTestResponseRoundTripper struct {
	body string
}

type tamperedPlayerChatTestCase struct {
	name   string
	mutate func(*protocol.ChatMessage)
}

func (t *chatTestRoundTripper) RoundTrip(*http.Request) (*http.Response, error) {
	t.calls++

	return nil, fmt.Errorf("unexpected request")
}

func (t *chatTestResponseRoundTripper) RoundTrip(*http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: http.StatusOK,
		Status:     "200 OK",
		Body:       io.NopCloser(strings.NewReader(t.body)),
		Header:     make(http.Header),
	}, nil
}

func (l *chatTestLogger) Debugf(string, ...any) {}

func (l *chatTestLogger) Debugln(...any) {}

func (l *chatTestLogger) Println(...any) {}

func (l *chatTestLogger) Warnf(string, ...any) {}

func (l *chatTestLogger) Warnln(...any) {}

func (l *chatTestLogger) Errorf(string, ...any) {}

func (l *chatTestLogger) Errorln(...any) {}

func (l *chatTestLogger) Printf(format string, values ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.prints = append(l.prints, fmt.Sprintf(format, values...))
}

func (l *chatTestLogger) chatPrints() []string {
	l.mu.Lock()
	defer l.mu.Unlock()

	prints := make([]string, 0, len(l.prints))

	for _, line := range l.prints {
		if len(line) >= 7 && line[:7] == "[chat] " {
			prints = append(prints, line)
		}
	}

	return prints
}

func TestMinecraftCertificateVerifierBindsPlayerAndExpiry(t *testing.T) {
	trustedKey := generateRSAKey(t)
	chatKey := generateRSAKey(t)

	verifier, err := NewMinecraftCertificateVerifier([]*rsa.PublicKey{&trustedKey.PublicKey})
	if err != nil {
		t.Fatalf("create certificate verifier: %v", err)
	}

	now := time.UnixMilli(1_800_000_000_000)

	playerUUID := "00010203-0405-0607-0809-0a0b0c0d0e0f"

	expiresAt := now.Add(time.Hour).UnixMilli()

	publicKey, certificateSignature := signTestCertificate(t, trustedKey, playerUUID, expiresAt, &chatKey.PublicKey)

	verified, err := verifier.Verify(playerUUID, expiresAt, publicKey, certificateSignature, now)
	if err != nil {
		t.Fatalf("verify valid certificate: %v", err)
	}

	if verified.N.Cmp(chatKey.N) != 0 {
		t.Fatal("verified certificate returned the wrong chat key")
	}

	_, err = verifier.Verify("10111213-1415-1617-1819-1a1b1c1d1e1f", expiresAt, publicKey, certificateSignature, now)
	if err == nil {
		t.Fatal("certificate verified for the wrong player UUID")
	}

	_, err = verifier.Verify(playerUUID, expiresAt, publicKey, certificateSignature, now.Add(2*time.Hour))
	if err == nil {
		t.Fatal("expired certificate verified")
	}

	_, err = verifier.Verify(playerUUID, expiresAt, []byte{1, 2, 3}, certificateSignature, now)
	if err == nil {
		t.Fatal("malformed public key verified")
	}
}

func TestSecureChatInitializationSkipsNetworkOfflineAndFailsOnline(t *testing.T) {
	transport := &chatTestRoundTripper{}

	client := &http.Client{Transport: transport}

	runtime := NewRuntime(&game.World{})

	err := runtime.InitializeSecureChat(t.Context(), false, client)
	if err != nil {
		t.Fatalf("initialize offline secure chat: %v", err)
	}

	if transport.calls != 0 || runtime.SecureChatReady() {
		t.Fatalf("offline initialization calls=%d ready=%t", transport.calls, runtime.SecureChatReady())
	}

	runtime.SetChatCertificateVerifier(&MinecraftCertificateVerifier{})

	err = runtime.InitializeSecureChat(t.Context(), true, client)
	if err == nil {
		t.Fatal("online initialization succeeded without certificate keys")
	}

	if transport.calls != 1 || runtime.SecureChatReady() {
		t.Fatalf("online failure calls=%d ready=%t", transport.calls, runtime.SecureChatReady())
	}
}

func TestLoadMinecraftCertificateVerifierDecodesBase64DER(t *testing.T) {
	trustedKey := generateRSAKey(t)

	publicKeyDER, err := x509.MarshalPKIXPublicKey(&trustedKey.PublicKey)
	if err != nil {
		t.Fatalf("marshal trusted key: %v", err)
	}

	body := fmt.Sprintf(`{"playerCertificateKeys":[{"publicKey":%q}]}`, base64.StdEncoding.EncodeToString(publicKeyDER))

	client := &http.Client{Transport: &chatTestResponseRoundTripper{body: body}}

	verifier, err := LoadMinecraftCertificateVerifier(t.Context(), client)
	if err != nil {
		t.Fatalf("load base64 DER certificate key: %v", err)
	}

	if len(verifier.trustedKeys) != 1 || verifier.trustedKeys[0].N.Cmp(trustedKey.N) != 0 {
		t.Fatal("loaded verifier does not contain the response key")
	}
}

func TestSecureChatSessionRotationAndGlobalPropagation(t *testing.T) {
	fixture := newSecureChatTestFixture(t, config.DefaultChatFormat)

	assertPacketPresent(t, fixture.senderConnection, protocol.ClientboundPlayerInfoUpdateID)
	assertPacketPresent(t, fixture.recipientConnection, protocol.ClientboundPlayerInfoUpdateID)

	first := fixture.sender.chatSessionSnapshot()
	if first == nil || first.UUID != fixture.chatSession.UUID {
		t.Fatalf("active chat session = %+v", first)
	}

	trustedKey := generateRSAKey(t)

	verifier, err := NewMinecraftCertificateVerifier([]*rsa.PublicKey{&trustedKey.PublicKey})
	if err != nil {
		t.Fatalf("create replacement verifier: %v", err)
	}

	fixture.runtime.SetChatCertificateVerifier(verifier)

	replacementKey := generateRSAKey(t)

	replacementUUID := "30313233-3435-3637-3839-3a3b3c3d3e3f"

	expiresAt := fixture.now.Add(2 * time.Hour).UnixMilli()

	publicKey, signature := signTestCertificate(t, trustedKey, fixture.sender.Player.UUID, expiresAt, &replacementKey.PublicKey)

	fixture.senderConnection.reset()
	fixture.recipientConnection.reset()

	err = fixture.sender.handleChatSessionUpdate(protocol.ChatSessionUpdate{
		SessionUUID:          replacementUUID,
		ExpiresAt:            expiresAt,
		PublicKey:            publicKey,
		CertificateSignature: signature,
	})

	if err != nil {
		t.Fatalf("rotate chat session: %v", err)
	}

	rotated := fixture.sender.chatSessionSnapshot()
	if rotated == nil || rotated.UUID != replacementUUID {
		t.Fatalf("rotated chat session = %+v", rotated)
	}

	assertPacketPresent(t, fixture.senderConnection, protocol.ClientboundPlayerInfoUpdateID)
	assertPacketPresent(t, fixture.recipientConnection, protocol.ClientboundPlayerInfoUpdateID)
}

func TestSignedPlayerChatAcceptanceReplayAndAcknowledgements(t *testing.T) {
	fixture := newSecureChatTestFixture(t, config.DefaultChatFormat)

	fixture.senderConnection.reset()
	fixture.recipientConnection.reset()

	first := fixture.signedMessage(t, 0, "hello", 11, 0, [3]byte{}, 1, nil)

	firstData := encodeTestChatMessage(t, first)

	err := fixture.sender.handlePlayPacket(&protocol.Packet{ID: protocol.ServerboundChatMessageID, Data: firstData})
	if err != nil {
		t.Fatalf("accept signed chat: %v", err)
	}

	assertOnlyChatPacket(t, fixture.senderConnection, protocol.ClientboundPlayerChatID)
	assertOnlyChatPacket(t, fixture.recipientConnection, protocol.ClientboundPlayerChatID)

	prints := fixture.logger.chatPrints()
	if len(prints) != 1 || prints[0] != "[chat] <Laura> hello\n" {
		t.Fatalf("chat prints = %q", prints)
	}

	fixture.senderConnection.reset()
	fixture.recipientConnection.reset()

	err = fixture.sender.handlePlayPacket(&protocol.Packet{ID: protocol.ServerboundChatMessageID, Data: firstData})
	if err == nil {
		t.Fatal("replayed signed chat was accepted")
	}

	assertOnlyChatPacket(t, fixture.senderConnection, protocol.ClientboundPlayDisconnectID)
	assertOnlyChatPacket(t, fixture.recipientConnection)

	prints = fixture.logger.chatPrints()
	if len(prints) != 1 {
		t.Fatalf("replay produced chat prints %q", prints)
	}

	fixture = newSecureChatTestFixture(t, config.DefaultChatFormat)

	fixture.senderConnection.reset()
	fixture.recipientConnection.reset()

	first = fixture.signedMessage(t, 0, "first", 21, 0, [3]byte{}, 1, nil)

	err = fixture.sender.handlePlayPacket(&protocol.Packet{ID: protocol.ServerboundChatMessageID, Data: encodeTestChatMessage(t, first)})
	if err != nil {
		t.Fatalf("accept first acknowledged chat: %v", err)
	}

	acknowledged := [3]byte{0, 0, 1 << 3}

	tracked := cloneTrackedSignatures(fixture.sender.chatState.tracked)

	err = applyChatOffset(&tracked, 1)
	if err != nil {
		t.Fatalf("prepare acknowledgement: %v", err)
	}

	tracked[19].pending = false

	checksum := chatAcknowledgementChecksum(tracked[:chatAcknowledgementBits])

	second := fixture.signedMessage(t, 1, "second", 22, 1, acknowledged, checksum, [][chatSignatureLength]byte{first.Signature})

	err = fixture.sender.handlePlayPacket(&protocol.Packet{ID: protocol.ServerboundChatMessageID, Data: encodeTestChatMessage(t, second)})
	if err != nil {
		t.Fatalf("accept acknowledged chat: %v", err)
	}

	assertPreviousSignatureCacheID(t, fixture.recipientConnection, 1)

	err = fixture.sender.handleChatAck(protocol.ChatAck{MessageCount: 1})
	if err != nil {
		t.Fatalf("apply standalone chat acknowledgement: %v", err)
	}

	err = fixture.sender.handleChatAck(protocol.ChatAck{MessageCount: 1})
	if err == nil {
		t.Fatal("impossible standalone acknowledgement was accepted")
	}
}

func TestSignedPlayerChatRejectsTamperingTransactionally(t *testing.T) {
	fixture := newSecureChatTestFixture(t, config.DefaultChatFormat)

	valid := fixture.signedMessage(t, 0, "hello", 31, 0, [3]byte{}, 1, nil)

	tests := []tamperedPlayerChatTestCase{
		{name: "message", mutate: func(message *protocol.ChatMessage) {
			message.Message = "tampered"
		}},
		{name: "timestamp", mutate: func(message *protocol.ChatMessage) {
			message.Timestamp--
		}},
		{name: "salt", mutate: func(message *protocol.ChatMessage) {
			message.Salt++
		}},
		{name: "signature", mutate: func(message *protocol.ChatMessage) {
			message.Signature[0] ^= 0xFF
		}},
		{name: "unsigned", mutate: func(message *protocol.ChatMessage) {
			message.HasSignature = false
		}},
		{name: "acknowledgement", mutate: func(message *protocol.ChatMessage) {
			message.Acknowledged[0] = 1
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			message := valid
			test.mutate(&message)

			_, err := fixture.sender.verifyPlayerChat(message)
			if err == nil {
				t.Fatal("tampered chat verified")
			}

			if fixture.sender.chatState.session.nextIndex != 0 {
				t.Fatalf("failed verification advanced index to %d", fixture.sender.chatState.session.nextIndex)
			}
		})
	}
}

func TestSignedCommandAppliesChatAcknowledgementAndExecutes(t *testing.T) {
	fixture := newSecureChatTestFixture(t, config.DefaultChatFormat)

	fixture.runtime.World.Seed = 12345
	fixture.runtime.ChatEnabled = false
	fixture.senderConnection.reset()

	var writer protocol.PacketWriter

	writer.String("seed")
	writer.Long(fixture.now.UnixMilli())
	writer.Long(41)
	writer.VarInt(0)
	writer.VarInt(0)
	writer.Byte(0)
	writer.Byte(0)
	writer.Byte(0)
	writer.Byte(1)

	err := fixture.sender.handlePlayPacket(&protocol.Packet{ID: protocol.ServerboundSignedChatCommandID, Data: writer.Buffer.Bytes()})
	if err != nil {
		t.Fatalf("handle signed command: %v", err)
	}

	assertSystemComponents(t, fixture.senderConnection, game.TranslatableText("commands.seed.success", game.LiteralText("12345").WithColor(game.TextColorGreen).WithClickEvent(game.ClickCopyToClipboard, "12345")))
}

func TestSignedCommandRejectsUnsupportedArgumentSignatures(t *testing.T) {
	fixture := newSecureChatTestFixture(t, config.DefaultChatFormat)

	fixture.senderConnection.reset()

	command := protocol.SignedChatCommand{
		Command:            "seed",
		ArgumentSignatures: []protocol.CommandArgumentSignature{{Name: "target_player"}},
		Checksum:           1,
	}

	err := fixture.sender.handleSignedCommandAcknowledgement(command)
	if err == nil {
		t.Fatal("signed command with argument signature was accepted")
	}

	if fixture.sender.chatState.tracked[0] != nil {
		t.Fatal("rejected signed command changed tracked acknowledgement state")
	}
}

func TestSecureCustomFormatFallsBackToSystemChatAfterVerification(t *testing.T) {
	fixture := newSecureChatTestFixture(t, "[{player}] {message}")

	fixture.senderConnection.reset()
	fixture.recipientConnection.reset()

	message := fixture.signedMessage(t, 0, "hello", 41, 0, [3]byte{}, 1, nil)

	err := fixture.sender.handlePlayPacket(&protocol.Packet{ID: protocol.ServerboundChatMessageID, Data: encodeTestChatMessage(t, message)})
	if err != nil {
		t.Fatalf("accept custom-format signed chat: %v", err)
	}

	assertSystemMessages(t, fixture.senderConnection, "[Laura] hello")
	assertSystemMessages(t, fixture.recipientConnection, "[Laura] hello")

	chatConnections := []*recordingConnection{fixture.senderConnection, fixture.recipientConnection}

	for _, connection := range chatConnections {
		for _, packet := range connection.packets(t) {
			if packet.ID == protocol.ClientboundPlayerChatID {
				t.Fatal("custom formatted content was relayed as signed player chat")
			}
		}
	}

	prints := fixture.logger.chatPrints()
	if len(prints) != 1 || prints[0] != "[chat] <Laura> hello\n" {
		t.Fatalf("chat prints = %q", prints)
	}
}

func TestDisabledChatDoesNotValidateRelayOrLog(t *testing.T) {
	fixture := newSecureChatTestFixture(t, config.DefaultChatFormat)

	fixture.runtime.ChatEnabled = false

	fixture.senderConnection.reset()
	fixture.recipientConnection.reset()

	err := fixture.sender.handlePlayPacket(&protocol.Packet{ID: protocol.ServerboundChatMessageID, Data: serverChatPacketData("ignored")})
	if err != nil {
		t.Fatalf("ignore disabled chat: %v", err)
	}

	assertOnlyChatPacket(t, fixture.senderConnection)
	assertOnlyChatPacket(t, fixture.recipientConnection)

	prints := fixture.logger.chatPrints()
	if len(prints) != 0 {
		t.Fatalf("disabled chat produced prints %q", prints)
	}
}

func newSecureChatTestFixture(t *testing.T, format string) secureChatTestFixture {
	t.Helper()

	now := time.UnixMilli(1_800_000_000_000)

	trustedKey := generateRSAKey(t)
	chatKey := generateRSAKey(t)

	verifier, err := NewMinecraftCertificateVerifier([]*rsa.PublicKey{&trustedKey.PublicKey})
	if err != nil {
		t.Fatalf("create certificate verifier: %v", err)
	}

	runtime := NewRuntime(&game.World{})

	runtime.ChatEnabled = true
	runtime.ChatFormat = format

	runtime.SetChatClock(func() time.Time {
		return now
	})

	runtime.SetChatCertificateVerifier(verifier)

	cfg := &config.Config{Server: config.ServerConfig{OnlineMode: true}}

	logger := &chatTestLogger{}

	sender, senderConnection := newMovementTestSession(runtime, "00010203-0405-0607-0809-0a0b0c0d0e0f", "Laura")
	recipient, recipientConnection := newMovementTestSession(runtime, "10111213-1415-1617-1819-1a1b1c1d1e1f", "Bob")

	sender.Config = cfg
	recipient.Config = cfg

	sender.Log = logger
	recipient.Log = &chatTestLogger{}

	recipient.Player.Position.X = float64((config.DefaultRenderDistance + 2) * ChunkWidth)

	joinTestSession(t, runtime, sender)
	joinTestSession(t, runtime, recipient)

	expiresAt := now.Add(time.Hour).UnixMilli()

	publicKey, certificateSignature := signTestCertificate(t, trustedKey, sender.Player.UUID, expiresAt, &chatKey.PublicKey)

	chatSession := protocol.ChatSession{
		UUID:                 "20212223-2425-2627-2829-2a2b2c2d2e2f",
		ExpiresAt:            expiresAt,
		PublicKey:            publicKey,
		CertificateSignature: certificateSignature,
	}

	err = sender.handleChatSessionUpdate(protocol.ChatSessionUpdate{
		SessionUUID:          chatSession.UUID,
		ExpiresAt:            chatSession.ExpiresAt,
		PublicKey:            chatSession.PublicKey,
		CertificateSignature: chatSession.CertificateSignature,
	})

	if err != nil {
		t.Fatalf("initialize chat session: %v", err)
	}

	return secureChatTestFixture{
		runtime:             runtime,
		sender:              sender,
		recipient:           recipient,
		senderConnection:    senderConnection,
		recipientConnection: recipientConnection,
		logger:              logger,
		now:                 now,
		chatKey:             chatKey,
		chatSession:         chatSession,
	}
}

func (f secureChatTestFixture) signedMessage(t *testing.T, index int32, text string, salt int64, offset int32, acknowledged [3]byte, checksum byte, previous [][chatSignatureLength]byte) protocol.ChatMessage {
	t.Helper()

	message := protocol.ChatMessage{
		Message:      text,
		Timestamp:    f.now.UnixMilli(),
		Salt:         salt,
		HasSignature: true,
		Offset:       offset,
		Acknowledged: acknowledged,
		Checksum:     checksum,
	}

	payload, err := signedChatPayload(f.sender.Player.UUID, f.chatSession.UUID, index, message, previous)
	if err != nil {
		t.Fatalf("build signed chat payload: %v", err)
	}

	digest := sha256.Sum256(payload)

	signature, err := rsa.SignPKCS1v15(rand.Reader, f.chatKey, crypto.SHA256, digest[:])
	if err != nil {
		t.Fatalf("sign chat message: %v", err)
	}

	copy(message.Signature[:], signature)

	return message
}

func signTestCertificate(t *testing.T, trustedKey *rsa.PrivateKey, playerUUID string, expiresAt int64, chatKey *rsa.PublicKey) ([]byte, []byte) {
	t.Helper()

	publicKey, err := x509.MarshalPKIXPublicKey(chatKey)
	if err != nil {
		t.Fatalf("marshal chat public key: %v", err)
	}

	uuid, err := decodeUUID(playerUUID)
	if err != nil {
		t.Fatalf("decode player UUID: %v", err)
	}

	payload := append([]byte(nil), uuid...)
	payload = binary.BigEndian.AppendUint64(payload, uint64(expiresAt))
	payload = append(payload, publicKey...)

	digest := sha1.Sum(payload)

	signature, err := rsa.SignPKCS1v15(rand.Reader, trustedKey, crypto.SHA1, digest[:])
	if err != nil {
		t.Fatalf("sign chat certificate: %v", err)
	}

	return publicKey, signature
}

func generateRSAKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, chatSignatureLength*8)
	if err != nil {
		t.Fatalf("generate RSA key: %v", err)
	}

	return key
}

func encodeTestChatMessage(t *testing.T, message protocol.ChatMessage) []byte {
	t.Helper()

	var writer protocol.PacketWriter

	writer.String(message.Message)
	writer.Long(message.Timestamp)
	writer.Long(message.Salt)
	writer.Bool(message.HasSignature)

	if message.HasSignature {
		for _, value := range message.Signature {
			writer.Byte(value)
		}
	}

	writer.VarInt(message.Offset)

	for _, value := range message.Acknowledged {
		writer.Byte(value)
	}

	writer.Byte(message.Checksum)

	err := writer.Err()
	if err != nil {
		t.Fatalf("encode chat message: %v", err)
	}

	return writer.Buffer.Bytes()
}

func assertPacketPresent(t *testing.T, connection *recordingConnection, packetID int32) {
	t.Helper()

	for _, packet := range connection.packets(t) {
		if packet.ID == packetID {
			return
		}
	}

	t.Fatalf("packet %#x not found in %v", packetID, connection.packetIDs(t))
}

func assertOnlyChatPacket(t *testing.T, connection *recordingConnection, expected ...int32) {
	t.Helper()

	var actual []int32

	for _, packet := range connection.packets(t) {
		switch packet.ID {
		case protocol.ClientboundPlayerChatID, protocol.ClientboundSystemChatID, protocol.ClientboundPlayDisconnectID:
			actual = append(actual, packet.ID)
		}
	}

	assertPacketIDs(t, actual, expected)
}

func assertPreviousSignatureCacheID(t *testing.T, connection *recordingConnection, expected int32) {
	t.Helper()

	packets := connection.packets(t)

	for _, packet := range slices.Backward(packets) {
		if packet.ID != protocol.ClientboundPlayerChatID {
			continue
		}

		reader := protocol.NewPacketReader(packet.Data)

		reader.VarInt()
		reader.UUID()
		reader.VarInt()

		if reader.Bool() {
			for range chatSignatureLength {
				reader.Byte()
			}
		}

		reader.String(256)
		reader.Long()
		reader.Long()

		count := reader.VarInt()
		if count != 1 {
			t.Fatalf("previous signature count = %d, want 1", count)
		}

		actual := reader.VarInt()
		if actual != expected {
			t.Fatalf("previous signature cache ID = %d, want %d", actual, expected)
		}

		err := reader.Err()
		if err != nil {
			t.Fatalf("decode player chat cache ID: %v", err)
		}

		return
	}

	t.Fatal("player chat packet not found")
}
