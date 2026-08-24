package protocol

type KnownPack struct {
	Namespace string
	ID        string
	Version   string
}

type KnownPacks struct {
	Packs []KnownPack
}

type RegistryData struct {
	Registry Registry
}

type UpdateTags struct {
	Registries []RegistryTags
}

func (p KnownPacks) Encode(wr *PacketWriter) {
	wr.VarInt(int32(len(p.Packs)))

	for _, pack := range p.Packs {
		wr.String(pack.Namespace)
		wr.String(pack.ID)
		wr.String(pack.Version)
	}
}

func (p RegistryData) Encode(wr *PacketWriter) {
	wr.String(p.Registry.ID)

	wr.VarInt(int32(len(p.Registry.Entries)))

	for _, entryID := range p.Registry.Entries {
		wr.String(entryID)

		// Prefixed Optional NBT: 0 = omitted, client sources data from the
		// selected known pack (minecraft:core).
		wr.Byte(0)
	}
}

func (p UpdateTags) Encode(wr *PacketWriter) {
	wr.VarInt(int32(len(p.Registries)))

	for _, registry := range p.Registries {
		wr.String(registry.RegistryID)

		wr.VarInt(int32(len(registry.Tags)))

		for _, tag := range registry.Tags {
			wr.String(tag.ID)

			wr.VarInt(int32(len(tag.Entries)))

			for _, entry := range tag.Entries {
				wr.VarInt(entry)
			}
		}
	}
}
