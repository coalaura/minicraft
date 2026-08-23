package protocol

// Version 1.21.11
const ProtocolVersion = 774

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
	CB_RegistryData        = 0x07
	CB_KnownPacks          = 0x0E

	// Serverbound
	SB_ClientInformation       = 0x00
	SB_CustomPayload           = 0x02
	SB_AcknowledgeFinishConfig = 0x03
	SB_KeepAliveConfig         = 0x04
	SB_KnownPacks              = 0x07
)

// Play
const (
	// Clientbound
	CB_GameEvent      = 0x26
	CB_PlayKeepAlive  = 0x2B
	CB_ChunkData      = 0x2C
	CB_ChunkBatchEnd  = 0x0B
	CB_ChunkBatchBeg  = 0x0C
	CB_PlayLogin      = 0x30
	CB_PlayPosition   = 0x46
	CB_SetCenterChunk = 0x5C

	// Serverbound
	SB_ConfirmTeleport    = 0x00
	SB_ChunkBatchReceived = 0x0A
	SB_ClientInfoPlay     = 0x0D
	SB_KeepAlivePlay      = 0x1B
	SB_MovePlayerPos      = 0x1D
	SB_MovePlayerPosRot   = 0x1E
	SB_MovePlayerRot      = 0x1F
	SB_MoveStatusOnly     = 0x20
	SB_PlayerLoaded       = 0x2B
)
