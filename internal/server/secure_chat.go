package server

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/coalaura/minicraft/internal/protocol"
)

const (
	minecraftCertificateKeysURL = "https://api.minecraftservices.com/publickeys"
	chatSignatureLength         = 256
	chatAcknowledgementBits     = 20
	chatSignatureCacheSize      = 128
	maxTrackedChatSignatures    = 256
	maxCertificateResponseBytes = 1 << 20
	certificateFetchTimeout     = 10 * time.Second
	maxChatMessageAge           = 7 * time.Minute
)

type ChatCertificateVerifier interface {
	Verify(playerUUID string, expiresAt int64, publicKey, certificateSignature []byte, now time.Time) (*rsa.PublicKey, error)
}

type MinecraftCertificateVerifier struct {
	trustedKeys []*rsa.PublicKey
}

type minecraftPublicKeysResponse struct {
	PlayerCertificateKeys []minecraftPublicKey `json:"playerCertificateKeys"`
}

type minecraftPublicKey struct {
	PublicKey string `json:"publicKey"`
}

type playerChatSession struct {
	certificate protocol.ChatSession
	publicKey   *rsa.PublicKey
	nextIndex   int32
	lastTime    int64
}

type trackedChatSignature struct {
	signature [chatSignatureLength]byte
	pending   bool
}

type sessionChatState struct {
	session        *playerChatSession
	tracked        []*trackedChatSignature
	signatureCache [][chatSignatureLength]byte
}

type verifiedPlayerChat struct {
	message            protocol.ChatMessage
	senderIndex        int32
	previousSignatures [][chatSignatureLength]byte
}

func NewMinecraftCertificateVerifier(trustedKeys []*rsa.PublicKey) (*MinecraftCertificateVerifier, error) {
	if len(trustedKeys) == 0 {
		return nil, errors.New("no Minecraft player-certificate keys")
	}

	keys := append([]*rsa.PublicKey(nil), trustedKeys...)

	return &MinecraftCertificateVerifier{trustedKeys: keys}, nil
}

func LoadMinecraftCertificateVerifier(ctx context.Context, client *http.Client) (*MinecraftCertificateVerifier, error) {
	if client == nil {
		client = &http.Client{Timeout: certificateFetchTimeout}
	}

	ctx, cancel := context.WithTimeout(ctx, certificateFetchTimeout)
	defer cancel()

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, minecraftCertificateKeysURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create Minecraft certificate-key request: %w", err)
	}

	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("fetch Minecraft certificate keys: %w", err)
	}

	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch Minecraft certificate keys: unexpected HTTP status %s", response.Status)
	}

	body, err := io.ReadAll(io.LimitReader(response.Body, maxCertificateResponseBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read Minecraft certificate keys: %w", err)
	}

	if len(body) > maxCertificateResponseBytes {
		return nil, fmt.Errorf("Minecraft certificate-key response exceeds %d bytes", maxCertificateResponseBytes)
	}

	var payload minecraftPublicKeysResponse

	err = json.Unmarshal(body, &payload)
	if err != nil {
		return nil, fmt.Errorf("decode Minecraft certificate keys: %w", err)
	}

	trustedKeys := make([]*rsa.PublicKey, 0, len(payload.PlayerCertificateKeys))

	for _, entry := range payload.PlayerCertificateKeys {
		keyDER, decodeErr := base64.StdEncoding.DecodeString(entry.PublicKey)
		if decodeErr != nil {
			return nil, fmt.Errorf("decode Minecraft player-certificate key: %w", decodeErr)
		}

		parsed, parseErr := x509.ParsePKIXPublicKey(keyDER)
		if parseErr != nil {
			return nil, fmt.Errorf("parse Minecraft player-certificate key: %w", parseErr)
		}

		publicKey, ok := parsed.(*rsa.PublicKey)
		if !ok {
			return nil, fmt.Errorf("Minecraft player-certificate key has type %T, want RSA", parsed)
		}

		trustedKeys = append(trustedKeys, publicKey)
	}

	return NewMinecraftCertificateVerifier(trustedKeys)
}

func (v *MinecraftCertificateVerifier) Verify(playerUUID string, expiresAt int64, publicKeyDER, certificateSignature []byte, now time.Time) (*rsa.PublicKey, error) {
	if expiresAt <= now.UnixMilli() {
		return nil, errors.New("chat certificate has expired")
	}

	playerUUIDBytes, err := decodeUUID(playerUUID)
	if err != nil {
		return nil, fmt.Errorf("decode authenticated player UUID: %w", err)
	}

	parsed, err := x509.ParsePKIXPublicKey(publicKeyDER)
	if err != nil {
		return nil, fmt.Errorf("parse chat public key: %w", err)
	}

	publicKey, ok := parsed.(*rsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("chat public key has type %T, want RSA", parsed)
	}

	if publicKey.Size() != chatSignatureLength {
		return nil, fmt.Errorf("chat RSA key size is %d bytes, want %d", publicKey.Size(), chatSignatureLength)
	}

	payload := make([]byte, 0, len(playerUUIDBytes)+8+len(publicKeyDER))

	payload = append(payload, playerUUIDBytes...)
	payload = binary.BigEndian.AppendUint64(payload, uint64(expiresAt))
	payload = append(payload, publicKeyDER...)

	digest := sha1.Sum(payload)

	for _, trustedKey := range v.trustedKeys {
		err = rsa.VerifyPKCS1v15(trustedKey, crypto.SHA1, digest[:], certificateSignature)
		if err == nil {
			return publicKey, nil
		}
	}

	return nil, errors.New("invalid Minecraft chat certificate signature")
}

func (r *Runtime) InitializeSecureChat(ctx context.Context, onlineMode bool, client *http.Client) error {
	if !onlineMode {
		r.SetChatCertificateVerifier(nil)

		return nil
	}

	r.SetChatCertificateVerifier(nil)

	verifier, err := LoadMinecraftCertificateVerifier(ctx, client)
	if err != nil {
		return err
	}

	r.SetChatCertificateVerifier(verifier)

	return nil
}

func (r *Runtime) SetChatCertificateVerifier(verifier ChatCertificateVerifier) {
	r.chatMu.Lock()
	r.certificateVerifier = verifier
	r.chatMu.Unlock()
}

func (r *Runtime) SecureChatReady() bool {
	r.chatMu.Lock()
	defer r.chatMu.Unlock()

	return r.certificateVerifier != nil
}

func (s *Session) secureChatEnforced() bool {
	return s.Config != nil && s.Config.Server.OnlineMode && s.Runtime != nil && s.Runtime.SecureChatReady()
}

func (s *Session) handleChatSessionUpdate(update protocol.ChatSessionUpdate) error {
	if !s.secureChatEnforced() {
		return nil
	}

	player := s.snapshotPlayer()

	now := s.Runtime.now()

	s.Runtime.chatMu.Lock()
	verifier := s.Runtime.certificateVerifier
	s.Runtime.chatMu.Unlock()

	publicKey, err := verifier.Verify(player.UUID, update.ExpiresAt, update.PublicKey, update.CertificateSignature, now)
	if err != nil {
		return s.disconnectChatViolation(fmt.Sprintf("invalid chat session: %v", err))
	}

	certificate := protocol.ChatSession{
		UUID:                 update.SessionUUID,
		ExpiresAt:            update.ExpiresAt,
		PublicKey:            append([]byte(nil), update.PublicKey...),
		CertificateSignature: append([]byte(nil), update.CertificateSignature...),
	}

	s.chatMx.Lock()
	s.chatState = newSessionChatState()

	s.chatState.session = &playerChatSession{certificate: certificate, publicKey: publicKey}
	s.chatMx.Unlock()

	s.Runtime.BroadcastChatSession(s)

	return nil
}

func (s *Session) verifyPlayerChat(message protocol.ChatMessage) (verifiedPlayerChat, error) {
	now := s.Runtime.now()

	s.chatMx.Lock()
	defer s.chatMx.Unlock()

	if s.chatState == nil || s.chatState.session == nil {
		return verifiedPlayerChat{}, errors.New("chat session is not initialized")
	}

	if !message.HasSignature {
		return verifiedPlayerChat{}, errors.New("unsigned chat message")
	}

	active := s.chatState.session
	if active.certificate.ExpiresAt <= now.UnixMilli() {
		return verifiedPlayerChat{}, errors.New("chat session has expired")
	}

	if message.Timestamp > now.UnixMilli() {
		return verifiedPlayerChat{}, errors.New("chat message timestamp is in the future")
	}

	if now.UnixMilli()-message.Timestamp > maxChatMessageAge.Milliseconds() {
		return verifiedPlayerChat{}, errors.New("chat message timestamp is too old")
	}

	if active.nextIndex > 0 && message.Timestamp < active.lastTime {
		return verifiedPlayerChat{}, errors.New("chat message timestamp moved backwards")
	}

	tracked := cloneTrackedSignatures(s.chatState.tracked)

	previous, err := applyChatAcknowledgement(&tracked, message.Offset, message.Acknowledged, message.Checksum)
	if err != nil {
		return verifiedPlayerChat{}, err
	}

	player := s.snapshotPlayer()

	payload, err := signedChatPayload(player.UUID, active.certificate.UUID, active.nextIndex, message, previous)
	if err != nil {
		return verifiedPlayerChat{}, err
	}

	digest := sha256.Sum256(payload)

	err = rsa.VerifyPKCS1v15(active.publicKey, crypto.SHA256, digest[:], message.Signature[:])
	if err != nil {
		return verifiedPlayerChat{}, errors.New("invalid chat message signature")
	}

	verified := verifiedPlayerChat{
		message:            message,
		senderIndex:        active.nextIndex,
		previousSignatures: previous,
	}

	s.chatState.tracked = tracked

	active.nextIndex++
	active.lastTime = message.Timestamp

	return verified, nil
}

func (s *Session) handleChatAck(acknowledgement protocol.ChatAck) error {
	if !s.secureChatEnforced() {
		return nil
	}

	s.chatMx.Lock()
	defer s.chatMx.Unlock()

	if s.chatState == nil {
		s.chatState = newSessionChatState()
	}

	tracked := cloneTrackedSignatures(s.chatState.tracked)

	err := applyChatOffset(&tracked, acknowledgement.MessageCount)
	if err != nil {
		return err
	}

	s.chatState.tracked = tracked

	return nil
}

func (s *Session) handleSignedCommandAcknowledgement(command protocol.SignedChatCommand) error {
	if !s.secureChatEnforced() {
		return nil
	}

	if len(command.ArgumentSignatures) != 0 {
		return errors.New("command contains signatures for unsupported signable arguments")
	}

	s.chatMx.Lock()
	defer s.chatMx.Unlock()

	if s.chatState == nil {
		s.chatState = newSessionChatState()
	}

	tracked := cloneTrackedSignatures(s.chatState.tracked)

	_, err := applyChatAcknowledgement(&tracked, command.MessageCount, command.Acknowledged, command.Checksum)
	if err != nil {
		return err
	}

	s.chatState.tracked = tracked

	return nil
}

func (s *Session) chatSessionSnapshot() *protocol.ChatSession {
	s.chatMx.Lock()
	defer s.chatMx.Unlock()

	if s.chatState == nil || s.chatState.session == nil {
		return nil
	}

	certificate := s.chatState.session.certificate

	certificate.PublicKey = append([]byte(nil), certificate.PublicKey...)
	certificate.CertificateSignature = append([]byte(nil), certificate.CertificateSignature...)

	return &certificate
}

func (s *Session) sendVerifiedPlayerChat(globalIndex int32, senderUUID, senderName string, verified verifiedPlayerChat) error {
	s.chatMx.Lock()
	defer s.chatMx.Unlock()

	if s.chatState == nil {
		s.chatState = newSessionChatState()
	}

	if len(s.chatState.tracked) >= maxTrackedChatSignatures {
		return errors.New("too many unacknowledged chat messages")
	}

	packed := make([]protocol.PreviousChatSignature, 0, len(verified.previousSignatures))

	for _, signature := range verified.previousSignatures {
		packedSignature := protocol.PreviousChatSignature{Signature: signature}

		cacheIndex := signatureCacheIndex(s.chatState.signatureCache, signature)
		if cacheIndex >= 0 {
			packedSignature.ID = int32(cacheIndex + 1)
		}

		packed = append(packed, packedSignature)
	}

	packet := protocol.PlayerChat{
		GlobalIndex:        globalIndex,
		SenderUUID:         senderUUID,
		SenderIndex:        verified.senderIndex,
		HasSignature:       true,
		Signature:          verified.message.Signature,
		PlainMessage:       verified.message.Message,
		Timestamp:          verified.message.Timestamp,
		Salt:               verified.message.Salt,
		PreviousSignatures: packed,
		FilterType:         0,
		Type:               protocol.ChatTypesHolder{ID: 1},
		NetworkName:        senderName,
	}

	err := s.writePacket(protocol.ClientboundPlayerChatID, packet)
	if err != nil {
		return err
	}

	s.chatState.tracked = append(s.chatState.tracked, &trackedChatSignature{
		signature: verified.message.Signature,
		pending:   true,
	})

	s.chatState.signatureCache = updateSignatureCache(s.chatState.signatureCache, verified.message.Signature, verified.previousSignatures)

	return nil
}

func (s *Session) disconnectChatViolation(reason string) error {
	err := s.writePacket(protocol.ClientboundPlayDisconnectID, protocol.PlayDisconnect{Reason: reason})
	if err != nil {
		return fmt.Errorf("%s (send disconnect: %w)", reason, err)
	}

	return errors.New(reason)
}

func newSessionChatState() *sessionChatState {
	return &sessionChatState{tracked: make([]*trackedChatSignature, chatAcknowledgementBits)}
}

func signedChatPayload(playerUUID, sessionUUID string, index int32, message protocol.ChatMessage, previous [][chatSignatureLength]byte) ([]byte, error) {
	playerUUIDBytes, err := decodeUUID(playerUUID)
	if err != nil {
		return nil, err
	}

	sessionUUIDBytes, err := decodeUUID(sessionUUID)
	if err != nil {
		return nil, err
	}

	messageBytes := []byte(message.Message)
	payload := make([]byte, 0, 4+16+16+4+8+8+4+len(messageBytes)+4+len(previous)*chatSignatureLength)

	payload = binary.BigEndian.AppendUint32(payload, 1)
	payload = append(payload, playerUUIDBytes...)
	payload = append(payload, sessionUUIDBytes...)
	payload = binary.BigEndian.AppendUint32(payload, uint32(index))
	payload = binary.BigEndian.AppendUint64(payload, uint64(message.Salt))
	payload = binary.BigEndian.AppendUint64(payload, uint64(message.Timestamp/1000))
	payload = binary.BigEndian.AppendUint32(payload, uint32(len(messageBytes)))
	payload = append(payload, messageBytes...)
	payload = binary.BigEndian.AppendUint32(payload, uint32(len(previous)))

	for _, signature := range previous {
		payload = append(payload, signature[:]...)
	}

	return payload, nil
}

func applyChatAcknowledgement(tracked *[]*trackedChatSignature, offset int32, acknowledged [3]byte, checksum byte) ([][chatSignatureLength]byte, error) {
	err := applyChatOffset(tracked, offset)
	if err != nil {
		return nil, err
	}

	window := *tracked

	if acknowledged[2]&0xF0 != 0 {
		return nil, errors.New("chat acknowledgement has bits outside its 20-message window")
	}

	previous := make([][chatSignatureLength]byte, 0, chatAcknowledgementBits)

	for index := 0; index < chatAcknowledgementBits; index++ {
		entry := window[index]
		set := acknowledged[index/8]&(1<<uint(index%8)) != 0

		if set {
			if entry == nil {
				return nil, fmt.Errorf("chat acknowledgement references missing message %d", index)
			}

			entry.pending = false

			previous = append(previous, entry.signature)

			continue
		}

		if entry != nil && !entry.pending {
			return nil, fmt.Errorf("chat acknowledgement removes previously acknowledged message %d", index)
		}

		window[index] = nil
	}

	expected := chatAcknowledgementChecksum(window[:chatAcknowledgementBits])
	if checksum != expected {
		return nil, fmt.Errorf("chat acknowledgement checksum is %d, want %d", checksum, expected)
	}

	return previous, nil
}

func applyChatOffset(tracked *[]*trackedChatSignature, offset int32) error {
	if offset < 0 {
		return fmt.Errorf("invalid chat acknowledgement offset %d", offset)
	}

	available := len(*tracked) - chatAcknowledgementBits
	if int(offset) > available {
		return fmt.Errorf("chat acknowledgement offset %d exceeds %d", offset, available)
	}

	*tracked = (*tracked)[offset:]

	return nil
}

func chatAcknowledgementChecksum(tracked []*trackedChatSignature) byte {
	hash := int32(1)

	for _, entry := range tracked {
		if entry == nil {
			continue
		}

		signatureHash := int32(1)

		for _, value := range entry.signature {
			signatureHash = 31*signatureHash + int32(value)
		}

		hash = 31*hash + signatureHash
	}

	checksum := byte(hash)
	if checksum == 0 {
		return 1
	}

	return checksum
}

func cloneTrackedSignatures(tracked []*trackedChatSignature) []*trackedChatSignature {
	clone := make([]*trackedChatSignature, len(tracked))

	for index, entry := range tracked {
		if entry == nil {
			continue
		}

		copied := *entry
		clone[index] = &copied
	}

	return clone
}

func signatureCacheIndex(cache [][chatSignatureLength]byte, signature [chatSignatureLength]byte) int {
	for index, cached := range cache {
		if cached == signature {
			return index
		}
	}

	return -1
}

func updateSignatureCache(cache [][chatSignatureLength]byte, signature [chatSignatureLength]byte, previous [][chatSignatureLength]byte) [][chatSignatureLength]byte {
	toAdd := make([][chatSignatureLength]byte, 0, len(previous)+1)

	toAdd = append(toAdd, signature)
	toAdd = append(toAdd, previous...)

	for index := len(toAdd) - 1; index >= 0; index-- {
		value := toAdd[index]

		existing := signatureCacheIndex(cache, value)
		if existing >= 0 {
			cache = append(cache[:existing], cache[existing+1:]...)
		}

		cache = append([][chatSignatureLength]byte{value}, cache...)
	}

	if len(cache) > chatSignatureCacheSize {
		cache = cache[:chatSignatureCacheSize]
	}

	return cache
}

func decodeUUID(value string) ([]byte, error) {
	compact := strings.ReplaceAll(value, "-", "")
	if len(compact) != 32 {
		return nil, fmt.Errorf("invalid UUID %q", value)
	}

	decoded := make([]byte, 16)

	_, err := hex.Decode(decoded, []byte(compact))
	if err != nil {
		return nil, fmt.Errorf("invalid UUID %q: %w", value, err)
	}

	return decoded, nil
}
