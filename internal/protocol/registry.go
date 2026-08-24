package protocol

type Registry struct {
	ID      string
	Entries []string
}

type RegistryTag struct {
	ID      string
	Entries []int32
}

type RegistryTags struct {
	RegistryID string
	Tags       []RegistryTag
}

func (r Registry) EntryID(id string) (int32, bool) {
	for i, entry := range r.Entries {
		if entry == id {
			return int32(i), true
		}
	}

	return 0, false
}
