package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const expectedMinecraftVersion = "1.21.11"

type armorMaterial struct {
	Defense             map[string]int `json:"defense"`
	EquipSound          string         `json:"equipSound"`
	Toughness           float32        `json:"toughness"`
	KnockbackResistance float32        `json:"knockbackResistance"`
}

type armorRegistration struct {
	Material  string `json:"material"`
	ArmorType string `json:"armorType"`
}

type equipmentSourceManifest struct {
	Version            string                            `json:"version"`
	Materials          map[string]armorMaterial          `json:"materials"`
	ArmorRegistrations map[string]armorRegistration      `json:"armorRegistrations"`
	DirectEquippables  map[string]itemEquippableMetadata `json:"directEquippables"`
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

func main() {
	dataPath := flag.String("output", "data", "committed data directory containing source inputs and derived manifests")

	flag.Parse()

	err := regenerateManifests(*dataPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func regenerateManifests(dataPath string) error {
	err := validateEnchantmentOrder(dataPath)
	if err != nil {
		return err
	}

	return writeEquipmentManifests(dataPath)
}

func validateEnchantmentOrder(dataPath string) error {
	var order []string

	err := readJSON(filepath.Join(dataPath, "enchantment_order.json"), &order)
	if err != nil {
		return err
	}

	if len(order) != 43 {
		return fmt.Errorf("enchantment order contains %d entries, want 43", len(order))
	}

	seen := make(map[string]bool, len(order))

	for _, name := range order {
		if seen[name] {
			return fmt.Errorf("enchantment order repeats %q", name)
		}

		seen[name] = true

		_, err = os.Stat(filepath.Join(dataPath, "enchantments", name+".json"))
		if err != nil {
			return fmt.Errorf("enchantment order references %q: %w", name, err)
		}
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

func writeEquipmentManifests(dataPath string) error {
	equipment, armor, err := equipmentManifests(dataPath)
	if err != nil {
		return err
	}

	err = writeJSON(filepath.Join(dataPath, "item_armor_attributes.json"), armor)
	if err != nil {
		return err
	}

	return writeJSON(filepath.Join(dataPath, "item_equippables.json"), equipment)
}

func equipmentManifests(dataPath string) (equippableManifest, armorAttributesManifest, error) {
	var source equipmentSourceManifest

	err := readJSON(filepath.Join(dataPath, "equipment_source.json"), &source)
	if err != nil {
		return equippableManifest{}, armorAttributesManifest{}, err
	}

	if source.Version != expectedMinecraftVersion || len(source.Materials) != 9 || len(source.ArmorRegistrations) != 29 || len(source.DirectEquippables) != 9 {
		return equippableManifest{}, armorAttributesManifest{}, errors.New("equipment source manifest does not match the pinned version or registration counts")
	}

	names, err := resolveItemTag(filepath.Join(dataPath, "item_tags"), "enchantable/equippable", nil)
	if err != nil {
		return equippableManifest{}, armorAttributesManifest{}, err
	}

	equipment := equippableManifest{Version: expectedMinecraftVersion}
	armor := armorAttributesManifest{Version: expectedMinecraftVersion}

	for _, name := range names {
		registration, isArmor := source.ArmorRegistrations[name]
		if isArmor {
			material, exists := source.Materials[registration.Material]
			if !exists {
				return equippableManifest{}, armorAttributesManifest{}, fmt.Errorf("armor item %q uses unknown material %s", name, registration.Material)
			}

			slot, exists := armorTypeSlot(registration.ArmorType)
			if !exists {
				return equippableManifest{}, armorAttributesManifest{}, fmt.Errorf("armor item %q uses unsupported type %s", name, registration.ArmorType)
			}

			armor.Attributes = append(armor.Attributes, itemArmorMetadata{
				Name:                name,
				Defense:             material.Defense[registration.ArmorType],
				Toughness:           material.Toughness,
				KnockbackResistance: material.KnockbackResistance,
			})

			equipment.Equippables = append(equipment.Equippables, itemEquippableMetadata{
				Name:         name,
				Slot:         slot,
				EquipSound:   material.EquipSound,
				Swappable:    true,
				DamageOnHurt: true,
			})

			continue
		}

		direct, exists := source.DirectEquippables[name]
		if !exists || direct.Name != name {
			return equippableManifest{}, armorAttributesManifest{}, fmt.Errorf("equippable item %q has no matching source registration", name)
		}

		equipment.Equippables = append(equipment.Equippables, direct)
	}

	if len(armor.Attributes) != 29 || len(equipment.Equippables) != 38 {
		return equippableManifest{}, armorAttributesManifest{}, fmt.Errorf("derived %d armor and %d equippable items, want 29 and 38", len(armor.Attributes), len(equipment.Equippables))
	}

	return equipment, armor, nil
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

func writeJSON(path string, value any) error {
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}

	encoded = append(encoded, '\n')

	return os.WriteFile(path, encoded, 0o644)
}
