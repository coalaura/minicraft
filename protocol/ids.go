package protocol

// Version 1.21.8
const ProtocolVersion = 772

// States
const (
	StateHandshake     = 0
	StateStatus        = 1
	StateLogin         = 2
	StateConfiguration = 3
	StatePlay          = 4
)

// Serverbound packet IDs by state
const (
	SB_Handshake = 0x00 // Handshake
)

// Status
const (
	SB_StatusRequest  = 0x00
	SB_StatusPing     = 0x01
	CB_StatusResponse = 0x00
	CB_StatusPong     = 0x01
)

// Login status
const (
	// Login serverbound
	SB_LoginStart         = 0x00
	SB_EncryptionResponse = 0x01
	SB_LoginAcknowledged  = 0x03

	// Login clientbound
	CB_DisconnectLogin   = 0x00
	CB_EncryptionRequest = 0x01
	CB_LoginSuccess      = 0x02
	CB_SetCompression    = 0x03
)
