package protocol

import "github.com/coalaura/minicraft/internal/game"

type LoginStart struct {
	Name string
	UUID string
}

type EncryptionRequest struct {
	ServerID    string
	PublicKey   []byte
	VerifyToken []byte
}

type EncryptionResponse struct {
	SharedSecret []byte
	VerifyToken  []byte
}

type LoginSuccess struct {
	UUID       string
	Username   string
	Properties []game.ProfileProperty
}

type SetCompression struct {
	Threshold int32
}

func DecodeLoginStart(data []byte) (LoginStart, error) {
	rd := NewPacketReader(data)

	start := LoginStart{
		Name: rd.String(16),
		UUID: rd.UUID(),
	}

	err := rd.Done("login start")
	if err != nil {
		return LoginStart{}, err
	}

	return start, nil
}

func DecodeEncryptionResponse(data []byte) (EncryptionResponse, error) {
	rd := NewPacketReader(data)

	response := EncryptionResponse{
		SharedSecret: rd.Bytes(),
		VerifyToken:  rd.Bytes(),
	}

	err := rd.Done("encryption response")
	if err != nil {
		return EncryptionResponse{}, err
	}

	return response, nil
}

func (p EncryptionRequest) Encode(wr *PacketWriter) {
	wr.String(p.ServerID)
	wr.Bytes(p.PublicKey)
	wr.Bytes(p.VerifyToken)
	wr.Bool(true)
}

func (p LoginSuccess) Encode(wr *PacketWriter) {
	wr.UUID(p.UUID)
	wr.String(p.Username)
	encodeProfileProperties(wr, p.Properties)
}

func (p SetCompression) Encode(wr *PacketWriter) {
	wr.VarInt(p.Threshold)
}

func encodeProfileProperties(wr *PacketWriter, properties []game.ProfileProperty) {
	wr.VarInt(int32(len(properties)))

	for _, property := range properties {
		wr.String(property.Name)
		wr.String(property.Value)

		wr.Bool(property.Signature != "")

		if property.Signature != "" {
			wr.String(property.Signature)
		}
	}
}
