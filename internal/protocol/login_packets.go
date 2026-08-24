package protocol

import (
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"
)

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

type LoginProperty struct {
	Name      string
	Value     string
	Signature string
}

type LoginSuccess struct {
	UUID       string
	Username   string
	Properties []LoginProperty
}

type SetCompression struct {
	Threshold int32
}

func DecodeLoginStart(data []byte) (LoginStart, error) {
	rd := NewPacketReader(data)

	name := rd.String(16)

	start := LoginStart{Name: name}

	if rd.Len() >= 16 {
		var uuid [16]byte

		_, err := io.ReadFull(rd, uuid[:])
		if err != nil {
			rd.err = err

			return LoginStart{}, err
		}

		raw := hex.EncodeToString(uuid[:])

		start.UUID = fmt.Sprintf("%s-%s-%s-%s-%s", raw[0:8], raw[8:12], raw[12:16], raw[16:20], raw[20:])
	}

	err := rd.Err()
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

	err := rd.Err()
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
	rawUUID, err := hex.DecodeString(strings.ReplaceAll(p.UUID, "-", ""))
	if err != nil || len(rawUUID) != 16 {
		wr.err = errors.New("malformed uuid")

		return
	}

	_, err = wr.Buffer.Write(rawUUID)
	if err != nil {
		wr.err = err

		return
	}
	wr.String(p.Username)

	wr.VarInt(int32(len(p.Properties)))

	for _, prop := range p.Properties {
		wr.String(prop.Name)
		wr.String(prop.Value)

		if prop.Signature != "" {
			wr.Byte(0x01)
			wr.String(prop.Signature)
		} else {
			wr.Byte(0x00)
		}
	}
}

func (p SetCompression) Encode(wr *PacketWriter) {
	wr.VarInt(p.Threshold)
}
