package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"go/format"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"unicode"
)

type enchantmentDefinition struct {
	MaximumLevel   int32  `json:"max_level"`
	SupportedItems string `json:"supported_items"`
	ExclusiveSet   string `json:"exclusive_set"`
}

type itemDefinition struct {
	ID   uint16 `json:"id"`
	Name string `json:"name"`
}

type tagDefinition struct {
	Values []json.RawMessage `json:"values"`
}

type tagEntry struct {
	ID string `json:"id"`
}

type generatedEnchantment struct {
	Name           string
	ID             int
	MaximumLevel   int32
	SupportedItems []string
	ExclusiveMask  uint64
	Curse          bool
}

type inputPaths struct {
	OrderPath           string
	EnchantmentsPath    string
	EnchantmentTagsPath string
	ItemTagsPath        string
	ItemsPath           string
}

func main() {
	paths := inputPaths{}

	gameOutput := flag.String("game-output", "", "generated game enchantments output path")
	protocolOutput := flag.String("protocol-output", "", "generated protocol enchantment output path")

	flag.StringVar(&paths.OrderPath, "order", "", "canonical bootstrap-order manifest")
	flag.StringVar(&paths.EnchantmentsPath, "enchantments", "", "canonical enchantment definitions directory")
	flag.StringVar(&paths.EnchantmentTagsPath, "enchantment-tags", "", "canonical enchantment tags directory")
	flag.StringVar(&paths.ItemTagsPath, "item-tags", "", "canonical item tags directory")
	flag.StringVar(&paths.ItemsPath, "items", "", "item catalogue")
	flag.Parse()

	if paths.OrderPath == "" || paths.EnchantmentsPath == "" || paths.EnchantmentTagsPath == "" || paths.ItemTagsPath == "" || paths.ItemsPath == "" || *gameOutput == "" || *protocolOutput == "" {
		fail(fmt.Errorf("order, enchantments, enchantment-tags, item-tags, items, game-output and protocol-output are required"))
	}

	game, protocol, err := generate(paths)
	if err != nil {
		fail(err)
	}

	err = os.WriteFile(*gameOutput, game, 0o644)
	if err != nil {
		fail(err)
	}

	err = os.WriteFile(*protocolOutput, protocol, 0o644)
	if err != nil {
		fail(err)
	}
}

func generate(paths inputPaths) ([]byte, []byte, error) {
	order, err := readOrder(paths.OrderPath)
	if err != nil {
		return nil, nil, err
	}

	if len(order) != 43 {
		return nil, nil, fmt.Errorf("bootstrap order has %d entries, want 43", len(order))
	}

	items, err := readItems(paths.ItemsPath)
	if err != nil {
		return nil, nil, err
	}

	itemNames := make(map[string]int, len(items))

	for index, item := range items {
		if int(item.ID) != index {
			return nil, nil, fmt.Errorf("item ID %d at index %d is not contiguous", item.ID, index)
		}

		itemNames[item.Name] = int(item.ID)
	}

	enchantments, err := readEnchantments(paths.EnchantmentsPath)
	if err != nil {
		return nil, nil, err
	}

	if len(enchantments) != len(order) {
		return nil, nil, fmt.Errorf("enchantment catalogue has %d entries, want %d", len(enchantments), len(order))
	}

	orderIDs := make(map[string]int, len(order))

	for id, name := range order {
		_, exists := orderIDs[name]
		if exists {
			return nil, nil, fmt.Errorf("bootstrap order contains duplicate %s", name)
		}

		_, exists = enchantments[name]
		if !exists {
			return nil, nil, fmt.Errorf("bootstrap order references missing enchantment %s", name)
		}

		orderIDs[name] = id
	}

	for name := range enchantments {
		_, exists := orderIDs[name]

		if !exists {
			return nil, nil, fmt.Errorf("enchantment catalogue entry %s is absent from bootstrap order", name)
		}
	}

	itemTags := tagResolver{root: paths.ItemTagsPath}
	enchantmentTags := tagResolver{root: paths.EnchantmentTagsPath}

	curseNames, err := enchantmentTags.expand("curse")
	if err != nil {
		return nil, nil, err
	}

	curses := make(map[string]bool, len(curseNames))

	for _, name := range curseNames {
		curses[name] = true
	}

	definitions := make([]generatedEnchantment, len(order))

	for id, name := range order {
		definition := enchantments[name]

		supportedItems, err := itemTags.expand(tagName(definition.SupportedItems))
		if err != nil {
			return nil, nil, fmt.Errorf("expand supported items for %s: %w", name, err)
		}

		for _, itemName := range supportedItems {
			_, exists := itemNames[itemName]

			if !exists {
				return nil, nil, fmt.Errorf("enchantment %s supports unknown item %s", name, itemName)
			}
		}

		var exclusiveMask uint64

		if definition.ExclusiveSet != "" {
			exclusiveNames, err := enchantmentTags.expand(tagName(definition.ExclusiveSet))

			if err != nil {
				return nil, nil, fmt.Errorf("expand exclusive set for %s: %w", name, err)
			}

			for _, exclusiveName := range exclusiveNames {
				exclusiveID, exists := orderIDs[exclusiveName]
				if !exists {
					return nil, nil, fmt.Errorf("exclusive set for %s references unknown enchantment %s", name, exclusiveName)
				}

				if exclusiveID != id {
					exclusiveMask |= uint64(1) << exclusiveID
				}
			}
		}

		definitions[id] = generatedEnchantment{Name: name, ID: id, MaximumLevel: definition.MaximumLevel, SupportedItems: supportedItems, ExclusiveMask: exclusiveMask, Curse: curses[name]}
	}

	itemTagNames, err := tagNames(filepath.Join(paths.ItemTagsPath, "enchantable"))
	if err != nil {
		return nil, nil, err
	}

	if len(itemTagNames) != 21 {
		return nil, nil, fmt.Errorf("enchantable item tag catalogue has %d entries, want 21", len(itemTagNames))
	}

	enchantmentTagNames, err := tagNames(paths.EnchantmentTagsPath)
	if err != nil {
		return nil, nil, err
	}

	if len(enchantmentTagNames) != 22 {
		return nil, nil, fmt.Errorf("enchantment tag catalogue has %d entries, want 22", len(enchantmentTagNames))
	}

	game, err := generateGame(definitions)
	if err != nil {
		return nil, nil, err
	}

	protocol, err := generateProtocol(order, itemTagNames, enchantmentTagNames, itemTags, enchantmentTags, itemNames, orderIDs)
	if err != nil {
		return nil, nil, err
	}

	return game, protocol, nil
}

func readOrder(path string) ([]string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var order []string

	err = json.Unmarshal(raw, &order)
	return order, err
}

func readItems(path string) ([]itemDefinition, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var items []itemDefinition

	err = json.Unmarshal(raw, &items)
	return items, err
}

func readEnchantments(root string) (map[string]enchantmentDefinition, error) {
	names, err := tagNames(root)
	if err != nil {
		return nil, err
	}

	definitions := make(map[string]enchantmentDefinition, len(names))

	for _, name := range names {
		raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(name)+".json"))
		if err != nil {
			return nil, err
		}

		var definition enchantmentDefinition

		err = json.Unmarshal(raw, &definition)
		if err != nil {
			return nil, fmt.Errorf("parse enchantment %s: %w", name, err)
		}

		if definition.MaximumLevel < 1 || definition.SupportedItems == "" {
			return nil, fmt.Errorf("enchantment %s has incomplete definition", name)
		}

		definitions[name] = definition
	}

	return definitions, nil
}

type tagResolver struct {
	root  string
	cache map[string][]string
}

func (resolver *tagResolver) expand(name string) ([]string, error) {
	if resolver.cache == nil {
		resolver.cache = make(map[string][]string)
	}

	return resolver.expandPath(name, make(map[string]bool))
}

func (resolver *tagResolver) expandPath(name string, visiting map[string]bool) ([]string, error) {
	name = tagName(name)
	cachedValues, exists := resolver.cache[name]

	if exists {
		return cachedValues, nil
	}

	if visiting[name] {
		return nil, fmt.Errorf("tag cycle at %s", name)
	}

	visiting[name] = true
	defer delete(visiting, name)

	raw, err := os.ReadFile(filepath.Join(resolver.root, filepath.FromSlash(name)+".json"))
	if err != nil {
		return nil, fmt.Errorf("read tag %s: %w", name, err)
	}

	var definition tagDefinition

	err = json.Unmarshal(raw, &definition)
	if err != nil {
		return nil, fmt.Errorf("parse tag %s: %w", name, err)
	}

	values := make([]string, 0, len(definition.Values))
	seen := make(map[string]bool)

	for _, rawValue := range definition.Values {
		value, err := parseTagValue(rawValue)
		if err != nil {
			return nil, fmt.Errorf("parse tag %s: %w", name, err)
		}

		if strings.HasPrefix(value, "#") {
			expanded, err := resolver.expandPath(value, visiting)

			if err != nil {
				return nil, err
			}

			for _, entry := range expanded {
				if !seen[entry] {
					values = append(values, entry)
					seen[entry] = true
				}
			}

			continue
		}

		value = tagName(value)
		if !seen[value] {
			values = append(values, value)
			seen[value] = true
		}
	}

	resolver.cache[name] = values

	return values, nil
}

func parseTagValue(raw json.RawMessage) (string, error) {
	var value string

	err := json.Unmarshal(raw, &value)
	if err == nil {
		return value, nil
	}

	var entry tagEntry

	err = json.Unmarshal(raw, &entry)
	if err != nil || entry.ID == "" {
		return "", fmt.Errorf("invalid tag value %s", raw)
	}

	return entry.ID, nil
}

func tagNames(root string) ([]string, error) {
	var names []string

	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		if entry.IsDir() || filepath.Ext(path) != ".json" {
			return nil
		}

		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}

		names = append(names, strings.TrimSuffix(filepath.ToSlash(relative), ".json"))

		return nil
	})

	if err != nil {
		return nil, err
	}

	slices.Sort(names)

	return names, nil
}

func tagName(value string) string {
	value = strings.TrimPrefix(value, "#")
	return strings.TrimPrefix(value, "minecraft:")
}

func generateGame(definitions []generatedEnchantment) ([]byte, error) {
	var output bytes.Buffer

	fmt.Fprintln(&output, "// Code generated by cmd/generate-enchantments; DO NOT EDIT.")
	fmt.Fprintln(&output)
	fmt.Fprintln(&output, "package game")
	fmt.Fprintln(&output)
	fmt.Fprintf(&output, "const MaxEnchantmentID Enchantment = %d\n\n", len(definitions)-1)
	fmt.Fprintln(&output, "const (")

	for _, definition := range definitions {
		fmt.Fprintf(&output, "\tEnchantment%s Enchantment = %d\n", goName(definition.Name), definition.ID)
	}

	fmt.Fprintln(&output, ")")
	fmt.Fprintln(&output)
	fmt.Fprintln(&output, "var generatedEnchantmentDefinitions = [...]EnchantmentDefinition{")

	for _, definition := range definitions {
		fmt.Fprintf(&output, "\t{ID: Enchantment%s, Name: %q, MaximumLevel: %d, SupportedItems: %s, ExclusiveMask: %#x, Curse: %t},\n", goName(definition.Name), definition.Name, definition.MaximumLevel, itemSlice(definition.SupportedItems), definition.ExclusiveMask, definition.Curse)
	}

	fmt.Fprintln(&output, "}")

	return format.Source(output.Bytes())
}

func generateProtocol(order []string, itemTagNames []string, enchantmentTagNames []string, itemTags tagResolver, enchantmentTags tagResolver, itemNames map[string]int, enchantmentIDs map[string]int) ([]byte, error) {
	var output bytes.Buffer

	fmt.Fprintln(&output, "// Code generated by cmd/generate-enchantments; DO NOT EDIT.")
	fmt.Fprintln(&output)
	fmt.Fprintln(&output, "package protocol")
	fmt.Fprintln(&output)
	fmt.Fprintln(&output, "var generatedEnchantmentRegistryEntries = []string{")

	for _, name := range order {
		fmt.Fprintf(&output, "\t%q,\n", "minecraft:"+name)
	}

	fmt.Fprintln(&output, "}")
	fmt.Fprintln(&output)

	err := writeProtocolTags(&output, "generatedItemEnchantmentTags", "enchantable", itemTagNames, &itemTags, itemNames)
	if err != nil {
		return nil, err
	}

	fmt.Fprintln(&output)

	err = writeProtocolTags(&output, "generatedEnchantmentTags", "", enchantmentTagNames, &enchantmentTags, enchantmentIDs)
	if err != nil {
		return nil, err
	}

	return format.Source(output.Bytes())
}

func writeProtocolTags(output *bytes.Buffer, variable string, prefix string, names []string, resolver *tagResolver, ids map[string]int) error {
	fmt.Fprintf(output, "var %s = []RegistryTag{\n", variable)

	for _, name := range names {
		lookup := name

		if prefix != "" {
			lookup = prefix + "/" + name
		}

		entries, err := resolver.expand(lookup)
		if err != nil {
			return err
		}

		fmt.Fprintf(output, "\t{ID: %q, Entries: []int32{", "minecraft:"+lookup)

		for _, entry := range entries {
			id, exists := ids[entry]

			if !exists {
				return fmt.Errorf("tag %s references unknown entry %s", lookup, entry)
			}

			fmt.Fprintf(output, "%d,", id)
		}

		fmt.Fprintln(output, "}},")
	}

	fmt.Fprintln(output, "}")

	return nil
}

func itemSlice(items []string) string {
	values := make([]string, len(items))

	for index, item := range items {
		values[index] = "Item" + goName(item)
	}

	return "[]Item{" + strings.Join(values, ", ") + "}"
}

func goName(name string) string {
	parts := strings.Split(name, "_")

	for index, part := range parts {
		runes := []rune(part)

		if len(runes) > 0 {
			runes[0] = unicode.ToUpper(runes[0])
			parts[index] = string(runes)
		}
	}

	return strings.Join(parts, "")
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
