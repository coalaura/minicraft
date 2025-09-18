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
	SB_Handshake = 0x00
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
	SB_LoginStart          = 0x00
	SB_EncryptionResponse  = 0x01
	SB_LoginPluginResponse = 0x02
	SB_LoginAcknowledged   = 0x03
	SB_LoginCookieResponse = 0x04

	// Login clientbound
	CB_DisconnectLogin    = 0x00
	CB_EncryptionRequest  = 0x01
	CB_LoginSuccess       = 0x02
	CB_SetCompression     = 0x03
	CB_LoginPluginRequest = 0x04
	CB_LoginCookieRequest = 0x05
)

// Configuration
const (
	// Clientbound
	CB_PluginMessageConfig = 0x01
	CB_DisconnectConfig    = 0x02
	CB_FinishConfiguration = 0x03

	// Serverbound
	SB_ClientInformation       = 0x00
	SB_AcknowledgeFinishConfig = 0x02
)
