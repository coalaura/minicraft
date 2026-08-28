package protocol

const ProtocolVersion = 774

const (
	StateHandshake     = 0
	StateStatus        = 1
	StateLogin         = 2
	StateConfiguration = 3
	StatePlay          = 4
)

// Handshake
const (
	ServerboundHandshakeID = 0x00
)

// Status
const (
	ServerboundStatusRequestID = 0x00
	ServerboundStatusPingID    = 0x01

	ClientboundStatusResponseID = 0x00
	ClientboundStatusPongID     = 0x01
)

// Login
const (
	ServerboundLoginStartID          = 0x00
	ServerboundEncryptionResponseID  = 0x01
	ServerboundLoginPluginResponseID = 0x02
	ServerboundLoginAcknowledgedID   = 0x03
	ServerboundLoginCookieResponseID = 0x04

	ClientboundLoginDisconnectID    = 0x00
	ClientboundEncryptionRequestID  = 0x01
	ClientboundLoginSuccessID       = 0x02
	ClientboundSetCompressionID     = 0x03
	ClientboundLoginPluginRequestID = 0x04
	ClientboundLoginCookieRequestID = 0x05
)

// Configuration
const (
	ServerboundConfigurationClientInformationID  = 0x00
	ServerboundConfigurationCustomPayloadID      = 0x02
	ServerboundConfigurationFinishAcknowledgedID = 0x03
	ServerboundConfigurationKeepAliveID          = 0x04
	ServerboundConfigurationKnownPacksID         = 0x07

	ClientboundConfigurationPluginMessageID = 0x01
	ClientboundConfigurationDisconnectID    = 0x02
	ClientboundConfigurationFinishID        = 0x03
	ClientboundConfigurationRegistryDataID  = 0x07
	ClientboundConfigurationUpdateTagsID    = 0x0D
	ClientboundConfigurationKnownPacksID    = 0x0E
)

// Play
const (
	ServerboundConfirmTeleportID            = 0x00
	ServerboundChatAckID                    = 0x05
	ServerboundChatCommandID                = 0x06
	ServerboundSignedChatCommandID          = 0x07
	ServerboundChatMessageID                = 0x08
	ServerboundChatSessionUpdateID          = 0x09
	ServerboundChunkBatchReceivedID         = 0x0A
	ServerboundClientTickEndID              = 0x0C
	ServerboundPlayClientInformationID      = 0x0D
	ServerboundCommandSuggestionsID         = 0x0E
	ServerboundContainerClickID             = 0x11
	ServerboundCloseContainerID             = 0x12
	ServerboundPlayKeepAliveID              = 0x1B
	ServerboundMovePlayerPositionID         = 0x1D
	ServerboundMovePlayerPositionRotationID = 0x1E
	ServerboundMovePlayerRotationID         = 0x1F
	ServerboundMovePlayerStatusID           = 0x20
	ServerboundPickItemFromBlockID          = 0x23
	ServerboundPlayerActionID               = 0x28
	ServerboundPlayerCommandID              = 0x29
	ServerboundPlayerInputID                = 0x2A
	ServerboundPlayerLoadedID               = 0x2B
	ServerboundSetHeldItemID                = 0x34
	ServerboundSetCreativeModeSlotID        = 0x37
	ServerboundSwingArmID                   = 0x3C
	ServerboundUseItemOnID                  = 0x3F

	ClientboundAddEntityID                    = 0x01
	ClientboundEntityAnimationID              = 0x02
	ClientboundBlockChangedAckID              = 0x04
	ClientboundBlockEventID                   = 0x07
	ClientboundBlockUpdateID                  = 0x08
	ClientboundChunkBatchEndID                = 0x0B
	ClientboundChunkBatchBeginID              = 0x0C
	ClientboundContainerSetContentID          = 0x12
	ClientboundContainerSetDataID             = 0x13
	ClientboundContainerSetSlotID             = 0x14
	ClientboundCloseContainerID               = 0x11
	ClientboundCommandSuggestionsID           = 0x0F
	ClientboundDeclareCommandsID              = 0x10
	ClientboundPlayDisconnectID               = 0x20
	ClientboundEntityEventID                  = 0x22
	ClientboundSynchronizeEntityPositionID    = 0x23
	ClientboundForgetLevelChunkID             = 0x25
	ClientboundGameEventID                    = 0x26
	ClientboundPlayKeepAliveID                = 0x2B
	ClientboundLevelChunkWithLightID          = 0x2C
	ClientboundLevelEventID                   = 0x2D
	ClientboundUpdateLightID                  = 0x2F
	ClientboundPlayLoginID                    = 0x30
	ClientboundUpdateEntityPositionID         = 0x33
	ClientboundUpdateEntityPositionRotationID = 0x34
	ClientboundUpdateEntityRotationID         = 0x36
	ClientboundOpenScreenID                   = 0x39
	ClientboundPlayerChatID                   = 0x3F
	ClientboundPlayerInfoRemoveID             = 0x43
	ClientboundPlayerInfoUpdateID             = 0x44
	ClientboundPlayerPositionID               = 0x46
	ClientboundRemoveEntitiesID               = 0x4B
	ClientboundSetHeadRotationID              = 0x51
	ClientboundSectionBlocksUpdateID          = 0x52
	ClientboundSetCenterChunkID               = 0x5C
	ClientboundEntityMetadataID               = 0x61
	ClientboundSetEntityMotionID              = 0x63
	ClientboundEntityEquipmentID              = 0x64
	ClientboundSetHeldSlotID                  = 0x67
	ClientboundUpdateTimeID                   = 0x6F
	ClientboundSoundID                        = 0x73
	ClientboundSystemChatID                   = 0x77
	ClientboundTakeItemEntityID               = 0x7A
	ClientboundUpdateRecipesID                = 0x83
)
