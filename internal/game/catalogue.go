package game

import (
	"sort"
	"strings"
)

var (
	ItemNames  []string
	BlockNames []string

	itemsByName  map[string]Item
	blocksByName map[string]BlockID
)

func init() {
	ItemNames = make([]string, 0, len(itemDefinitions))
	itemsByName = make(map[string]Item, len(itemDefinitions))

	for item, definition := range itemDefinitions {
		ItemNames = append(ItemNames, definition.Name)
		itemsByName[definition.Name] = Item(item)
	}

	sort.Strings(ItemNames)

	BlockNames = make([]string, 0, len(blockDefinitions))
	blocksByName = make(map[string]BlockID, len(blockDefinitions))

	for id := range blockDefinitions {
		definition, ok := BlockByID(BlockID(id))
		if !ok {
			continue
		}

		BlockNames = append(BlockNames, definition.Name)
		blocksByName[definition.Name] = BlockID(id)
	}

	sort.Strings(BlockNames)
}

// ItemByName returns the item with a generated bare or minecraft-namespaced name.
func ItemByName(name string) (Item, bool) {
	name, ok := generatedName(name)
	if !ok {
		return 0, false
	}

	item, ok := itemsByName[name]

	return item, ok
}

// BlockByName returns the default state for a generated bare or minecraft-namespaced block name.
func BlockByName(name string) (Block, bool) {
	name, ok := generatedName(name)
	if !ok {
		return 0, false
	}

	id, ok := blocksByName[name]
	if !ok {
		return 0, false
	}

	definition, ok := BlockByID(id)
	if !ok {
		return 0, false
	}

	return definition.DefaultState, true
}

func generatedName(name string) (string, bool) {
	separator := strings.IndexByte(name, ':')
	if separator < 0 {
		return name, name != ""
	}

	if name[:separator] != "minecraft" || separator == len(name)-1 || strings.IndexByte(name[separator+1:], ':') >= 0 {
		return "", false
	}

	return name[separator+1:], true
}
