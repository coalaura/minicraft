package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

const (
	expectedMinecraftVersion = "1.21.11"
	expectedProtocolVersion  = 774
)

type clientVersion struct {
	Name            string `json:"name"`
	ProtocolVersion int    `json:"protocol_version"`
}

type catalogueVersion struct {
	Version          int    `json:"version"`
	MinecraftVersion string `json:"minecraftVersion"`
}

type directoryImport struct {
	source string
	target string
}

type armorMaterial struct {
	defense             map[string]int
	equipSound          string
	toughness           float32
	knockbackResistance float32
}

type armorRegistration struct {
	material  string
	armorType string
}

type armorAttributesManifest struct {
	Version    string              `json:"version"`
	Attributes []itemArmorMetadata `json:"attributes"`
}

type itemArmorMetadata struct {
	Name                string  `json:"name"`
	Defense             int     `json:"defense"`
	Toughness           float32 `json:"toughness"`
	KnockbackResistance float32 `json:"knockbackResistance"`
}

type equippableManifest struct {
	Version     string                   `json:"version"`
	Equippables []itemEquippableMetadata `json:"equippables"`
}

type itemEquippableMetadata struct {
	Name         string `json:"name"`
	Slot         string `json:"slot"`
	EquipSound   string `json:"equipSound"`
	Swappable    bool   `json:"swappable"`
	DamageOnHurt bool   `json:"damageOnHurt"`
}

type itemTag struct {
	Values []string `json:"values"`
}

var (
	enchantmentKeyPattern      = regexp.MustCompile(`public static final ResourceKey<Enchantment> ([A-Z0-9_]+) = key\("([a-z0-9_]+)"\);`)
	enchantmentRegisterPattern = regexp.MustCompile(`(?s)register\(\s*context,\s*([A-Z0-9_]+),`)
	enchantCategoriesPattern   = regexp.MustCompile(`,\r?\n    "enchantCategories": \[\r?\n(?:      "[^"]+"(?:,\r?\n|\r?\n))*    \](,?)`)
	armorMaterialPattern       = regexp.MustCompile(`(?s)ArmorMaterial ([A-Z_]+) = new ArmorMaterial\(\s*\d+,\s*makeDefense\((\d+),\s*(\d+),\s*(\d+),\s*(\d+),\s*\d+\),\s*\d+,\s*SoundEvents\.([A-Z_]+),\s*([0-9.]+)F,\s*([0-9.]+)F,`)
	armorRegistrationPattern   = regexp.MustCompile(`(?s)public static final Item [A-Z0-9_]+ = registerItem\(\s*"([a-z0-9_]+)",[^;]*?humanoidArmor\(ArmorMaterials\.([A-Z_]+), ArmorType\.([A-Z]+)\)`)
	equipmentSlotPattern       = regexp.MustCompile(`EquipmentSlot\.([A-Z]+)`)
	equipSoundPattern          = regexp.MustCompile(`setEquipSound\(SoundEvents\.([A-Z_]+)\)`)
)

func main() {
	referencePath := flag.String("reference", "../reference", "path containing client_source and minecraft-data")
	dataPath := flag.String("output", "data", "data directory to replace")

	flag.Parse()

	err := syncData(*referencePath, *dataPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func syncData(referencePath string, dataPath string) error {
	clientPath := filepath.Join(referencePath, "client_source")
	cataloguePath := filepath.Join(referencePath, "minecraft-data")

	err := validateVersions(clientPath, cataloguePath)
	if err != nil {
		return err
	}

	imports := []directoryImport{
		{source: filepath.Join(clientPath, "data", "minecraft", "loot_table", "blocks"), target: filepath.Join(dataPath, "block_loot")},
		{source: filepath.Join(clientPath, "data", "minecraft", "tags", "block"), target: filepath.Join(dataPath, "block_tags")},
		{source: filepath.Join(clientPath, "data", "minecraft", "enchantment"), target: filepath.Join(dataPath, "enchantments")},
		{source: filepath.Join(clientPath, "data", "minecraft", "tags", "enchantment"), target: filepath.Join(dataPath, "enchantment_tags")},
		{source: filepath.Join(clientPath, "data", "minecraft", "tags", "item"), target: filepath.Join(dataPath, "item_tags")},
		{source: filepath.Join(clientPath, "data", "minecraft", "recipe"), target: filepath.Join(dataPath, "recipes")},
	}

	catalogues := []string{"biomes.json", "blocks.json", "entities.json"}

	for _, item := range imports {
		err = replaceDirectory(item.source, item.target)
		if err != nil {
			return err
		}
	}

	for _, name := range catalogues {
		err = copyFile(filepath.Join(cataloguePath, name), filepath.Join(dataPath, name))
		if err != nil {
			return err
		}
	}

	err = copyItems(filepath.Join(cataloguePath, "items.json"), filepath.Join(dataPath, "items.json"))
	if err != nil {
		return err
	}

	javaPath := filepath.Join(clientPath, "net", "minecraft", "world", "item", "enchantment", "Enchantments.java")
	orderPath := filepath.Join(dataPath, "enchantment_order.json")

	err = writeEnchantmentOrder(javaPath, orderPath)
	if err != nil {
		return err
	}

	err = writeEquipmentManifests(clientPath, dataPath)
	if err != nil {
		return err
	}

	return nil
}

func validateVersions(clientPath string, cataloguePath string) error {
	var client clientVersion

	err := readJSON(filepath.Join(clientPath, "version.json"), &client)
	if err != nil {
		return err
	}

	if client.Name != expectedMinecraftVersion+" Unobfuscated" || client.ProtocolVersion != expectedProtocolVersion {
		return fmt.Errorf("client source is %q protocol %d, want %s protocol %d", client.Name, client.ProtocolVersion, expectedMinecraftVersion, expectedProtocolVersion)
	}

	var catalogue catalogueVersion

	err = readJSON(filepath.Join(cataloguePath, "version.json"), &catalogue)
	if err != nil {
		return err
	}

	if catalogue.MinecraftVersion != expectedMinecraftVersion || catalogue.Version != expectedProtocolVersion {
		return fmt.Errorf("minecraft-data is %q protocol %d, want %s protocol %d", catalogue.MinecraftVersion, catalogue.Version, expectedMinecraftVersion, expectedProtocolVersion)
	}

	return nil
}

func readJSON(path string, target any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	err = json.Unmarshal(data, target)
	if err != nil {
		return fmt.Errorf("decode %s: %w", path, err)
	}

	return nil
}

func replaceDirectory(source string, target string) error {
	err := os.RemoveAll(target)
	if err != nil {
		return err
	}

	err = filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		relative, relativeErr := filepath.Rel(source, path)
		if relativeErr != nil {
			return relativeErr
		}

		destination := filepath.Join(target, relative)

		if entry.IsDir() {
			return os.MkdirAll(destination, 0o755)
		}

		return copyFile(path, destination)
	})

	if err != nil {
		return fmt.Errorf("copy %s: %w", source, err)
	}

	return nil
}

func copyFile(source string, target string) error {
	data, err := os.ReadFile(source)
	if err != nil {
		return err
	}

	err = os.MkdirAll(filepath.Dir(target), 0o755)
	if err != nil {
		return err
	}

	err = os.WriteFile(target, data, 0o644)
	if err != nil {
		return err
	}

	return nil
}

func copyItems(source string, target string) error {
	data, err := os.ReadFile(source)
	if err != nil {
		return err
	}

	cleaned, count := stripEnchantCategories(data)
	if count == 0 {
		return errors.New("items catalogue contains no enchantCategories fields")
	}

	err = os.WriteFile(target, cleaned, 0o644)
	if err != nil {
		return err
	}

	return nil
}

func stripEnchantCategories(data []byte) ([]byte, int) {
	count := 0

	cleaned := enchantCategoriesPattern.ReplaceAllFunc(data, func(match []byte) []byte {
		count++

		if match[len(match)-1] == ',' {
			return []byte(",")
		}

		return nil
	})

	return cleaned, count
}

func writeEnchantmentOrder(source string, target string) error {
	data, err := os.ReadFile(source)
	if err != nil {
		return err
	}

	order, err := enchantmentOrder(data)
	if err != nil {
		return err
	}

	encoded, err := json.MarshalIndent(order, "", "  ")
	if err != nil {
		return err
	}

	encoded = append(encoded, '\n')

	err = os.WriteFile(target, encoded, 0o644)
	if err != nil {
		return err
	}

	return nil
}

func enchantmentOrder(data []byte) ([]string, error) {
	keys := make(map[string]string)

	for _, match := range enchantmentKeyPattern.FindAllSubmatch(data, -1) {
		keys[string(match[1])] = string(match[2])
	}

	bootstrapStart := bytes.Index(data, []byte("public static void bootstrap"))
	bootstrapEnd := bytes.Index(data, []byte("private static void register"))

	if bootstrapStart < 0 || bootstrapEnd <= bootstrapStart {
		return nil, errors.New("cannot locate enchantment bootstrap body")
	}

	matches := enchantmentRegisterPattern.FindAllSubmatch(data[bootstrapStart:bootstrapEnd], -1)

	order := make([]string, 0, len(matches))

	for _, match := range matches {
		constant := string(match[1])
		name, exists := keys[constant]

		if !exists {
			return nil, fmt.Errorf("bootstrap references unknown enchantment key %s", constant)
		}

		order = append(order, name)
	}

	if len(order) != 43 {
		return nil, fmt.Errorf("enchantment bootstrap contains %d registrations, want 43", len(order))
	}

	return order, nil
}

func writeEquipmentManifests(clientPath string, dataPath string) error {
	equipment, armor, err := equipmentManifests(clientPath)
	if err != nil {
		return err
	}

	err = writeJSON(filepath.Join(dataPath, "item_armor_attributes.json"), armor)
	if err != nil {
		return err
	}

	return writeJSON(filepath.Join(dataPath, "item_equippables.json"), equipment)
}

func equipmentManifests(clientPath string) (equippableManifest, armorAttributesManifest, error) {
	materialsPath := filepath.Join(clientPath, "net", "minecraft", "world", "item", "equipment", "ArmorMaterials.java")
	itemsPath := filepath.Join(clientPath, "net", "minecraft", "world", "item", "Items.java")

	materialsSource, err := os.ReadFile(materialsPath)
	if err != nil {
		return equippableManifest{}, armorAttributesManifest{}, err
	}

	itemsSource, err := os.ReadFile(itemsPath)
	if err != nil {
		return equippableManifest{}, armorAttributesManifest{}, err
	}

	materials, err := parseArmorMaterials(materialsSource)
	if err != nil {
		return equippableManifest{}, armorAttributesManifest{}, err
	}

	registrations, err := parseArmorRegistrations(itemsSource)
	if err != nil {
		return equippableManifest{}, armorAttributesManifest{}, err
	}

	tagsPath := filepath.Join(clientPath, "data", "minecraft", "tags", "item")

	names, err := resolveItemTag(tagsPath, "enchantable/equippable", nil)
	if err != nil {
		return equippableManifest{}, armorAttributesManifest{}, err
	}

	equipment := equippableManifest{Version: expectedMinecraftVersion}
	armor := armorAttributesManifest{Version: expectedMinecraftVersion}

	for _, name := range names {
		registration, isArmor := registrations[name]
		if isArmor {
			material, exists := materials[registration.material]
			if !exists {
				return equippableManifest{}, armorAttributesManifest{}, fmt.Errorf("armor item %q uses unknown material %s", name, registration.material)
			}

			slot, exists := armorTypeSlot(registration.armorType)
			if !exists {
				return equippableManifest{}, armorAttributesManifest{}, fmt.Errorf("armor item %q uses unsupported type %s", name, registration.armorType)
			}

			armor.Attributes = append(armor.Attributes, itemArmorMetadata{
				Name:                name,
				Defense:             material.defense[registration.armorType],
				Toughness:           material.toughness,
				KnockbackResistance: material.knockbackResistance,
			})
			equipment.Equippables = append(equipment.Equippables, itemEquippableMetadata{
				Name:         name,
				Slot:         slot,
				EquipSound:   material.equipSound,
				Swappable:    true,
				DamageOnHurt: true,
			})

			continue
		}

		direct, directErr := parseDirectEquippable(itemsSource, name)
		if directErr != nil {
			return equippableManifest{}, armorAttributesManifest{}, directErr
		}

		equipment.Equippables = append(equipment.Equippables, direct)
	}

	if len(armor.Attributes) != 29 || len(equipment.Equippables) != 38 {
		return equippableManifest{}, armorAttributesManifest{}, fmt.Errorf("derived %d armor and %d equippable items, want 29 and 38", len(armor.Attributes), len(equipment.Equippables))
	}

	return equipment, armor, nil
}

func parseArmorMaterials(source []byte) (map[string]armorMaterial, error) {
	matches := armorMaterialPattern.FindAllSubmatch(source, -1)
	materials := make(map[string]armorMaterial, len(matches))

	for _, match := range matches {
		boots, err := strconv.Atoi(string(match[2]))
		if err != nil {
			return nil, err
		}

		legs, err := strconv.Atoi(string(match[3]))
		if err != nil {
			return nil, err
		}

		chest, err := strconv.Atoi(string(match[4]))
		if err != nil {
			return nil, err
		}

		helm, err := strconv.Atoi(string(match[5]))
		if err != nil {
			return nil, err
		}

		toughness, err := strconv.ParseFloat(string(match[7]), 32)
		if err != nil {
			return nil, err
		}

		knockback, err := strconv.ParseFloat(string(match[8]), 32)
		if err != nil {
			return nil, err
		}

		materials[string(match[1])] = armorMaterial{
			defense:             map[string]int{"BOOTS": boots, "LEGGINGS": legs, "CHESTPLATE": chest, "HELMET": helm},
			equipSound:          armorEquipSound(string(match[6])),
			toughness:           float32(toughness),
			knockbackResistance: float32(knockback),
		}
	}

	if len(materials) != 9 {
		return nil, fmt.Errorf("parsed %d armor materials, want 9", len(materials))
	}

	return materials, nil
}

func parseArmorRegistrations(source []byte) (map[string]armorRegistration, error) {
	matches := armorRegistrationPattern.FindAllSubmatch(source, -1)
	registrations := make(map[string]armorRegistration, len(matches))

	for _, match := range matches {
		registrations[string(match[1])] = armorRegistration{material: string(match[2]), armorType: string(match[3])}
	}

	if len(registrations) != 29 {
		return nil, fmt.Errorf("parsed %d humanoid armor registrations, want 29", len(registrations))
	}

	return registrations, nil
}

func parseDirectEquippable(source []byte, name string) (itemEquippableMetadata, error) {
	constant := strings.ToUpper(name)
	startMarker := []byte("public static final Item " + constant + " =")

	_, rest, found := bytes.Cut(source, startMarker)
	if !found {
		return itemEquippableMetadata{}, fmt.Errorf("cannot locate equippable item %q", name)
	}

	rest, _, _ = bytes.Cut(rest, []byte("public static final Item "))

	slotMatch := equipmentSlotPattern.FindSubmatch(rest)

	if slotMatch == nil {
		return itemEquippableMetadata{}, fmt.Errorf("equippable item %q has no equipment slot", name)
	}

	sound := "minecraft:item.armor.equip_generic"

	soundMatch := equipSoundPattern.FindSubmatch(rest)
	if soundMatch != nil {
		sound = armorEquipSound(string(soundMatch[1]))
	}

	return itemEquippableMetadata{
		Name:         name,
		Slot:         string(slotMatch[1]),
		EquipSound:   sound,
		Swappable:    !bytes.Contains(rest, []byte("setSwappable(false)")) && !bytes.Contains(rest, []byte("equippableUnswappable")),
		DamageOnHurt: !bytes.Contains(rest, []byte("setDamageOnHurt(false)")),
	}, nil
}

func resolveItemTag(tagsPath string, name string, active map[string]bool) ([]string, error) {
	if active == nil {
		active = make(map[string]bool)
	}

	if active[name] {
		return nil, fmt.Errorf("item tag cycle at %q", name)
	}

	active[name] = true
	defer delete(active, name)

	var tag itemTag

	err := readJSON(filepath.Join(tagsPath, filepath.FromSlash(name)+".json"), &tag)
	if err != nil {
		return nil, err
	}

	var names []string

	for _, value := range tag.Values {
		isTag := strings.HasPrefix(value, "#")
		value = strings.TrimPrefix(value, "#")
		value = strings.TrimPrefix(value, "minecraft:")

		if !isTag {
			names = append(names, value)

			continue
		}

		nested, nestedErr := resolveItemTag(tagsPath, value, active)
		if nestedErr != nil {
			return nil, nestedErr
		}

		names = append(names, nested...)
	}

	return names, nil
}

func armorTypeSlot(armorType string) (string, bool) {
	switch armorType {
	case "HELMET":
		return "HEAD", true
	case "CHESTPLATE":
		return "CHEST", true
	case "LEGGINGS":
		return "LEGS", true
	case "BOOTS":
		return "FEET", true
	default:
		return "", false
	}
}

func armorEquipSound(constant string) string {
	name := strings.ToLower(strings.TrimPrefix(constant, "ARMOR_"))

	return "minecraft:item.armor." + name
}

func writeJSON(path string, value any) error {
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}

	encoded = append(encoded, '\n')

	return os.WriteFile(path, encoded, 0o644)
}
