package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"go/format"
	"os"
	"strings"
	"unicode"
)

type ItemDefinition struct {
	ID            uint16 `json:"id"`
	Name          string `json:"name"`
	StackSize     int32  `json:"stackSize"`
	MaxDurability int32  `json:"maxDurability"`
}

type ConsumableManifest struct {
	Version     string                   `json:"version"`
	Sources     map[string]string        `json:"sources"`
	Consumables []ConsumableItemMetadata `json:"consumables"`
}

type AttackAttributesManifest struct {
	Version    string                         `json:"version"`
	Sources    map[string]string              `json:"sources"`
	Attributes []ItemAttackAttributesMetadata `json:"attributes"`
}

type ItemAttackAttributesMetadata struct {
	Name            string  `json:"name"`
	DamageModifier  float32 `json:"damageModifier"`
	SpeedModifier   float32 `json:"speedModifier"`
	DamagePerAttack int32   `json:"damagePerAttack"`
}

type ArmorAttributesManifest struct {
	Version    string              `json:"version"`
	Sources    map[string]string   `json:"sources"`
	Attributes []ItemArmorMetadata `json:"attributes"`
}

type ItemArmorMetadata struct {
	Name                string  `json:"name"`
	Slot                string  `json:"slot"`
	Defense             int     `json:"defense"`
	Toughness           float32 `json:"toughness"`
	KnockbackResistance float32 `json:"knockbackResistance"`
}

type ConsumableItemMetadata struct {
	Name               string                  `json:"name"`
	Nutrition          int8                    `json:"nutrition"`
	Saturation         float32                 `json:"saturation"`
	AlwaysEdible       bool                    `json:"alwaysEdible"`
	Duration           uint16                  `json:"duration"`
	Animation          string                  `json:"animation"`
	Sound              string                  `json:"sound"`
	Particles          bool                    `json:"particles"`
	CanSprint          bool                    `json:"canSprint"`
	InteractVibrations bool                    `json:"interactVibrations"`
	SpeedMultiplier    float32                 `json:"speedMultiplier"`
	Remainder          string                  `json:"remainder"`
	Effects            []ConsumeEffectMetadata `json:"effects"`
	DynamicEffects     []ConsumeEffectMetadata `json:"dynamicEffects"`
}

type ConsumeEffectMetadata struct {
	Type        string              `json:"type"`
	Effects     []MobEffectMetadata `json:"effects"`
	Probability float32             `json:"probability"`
	Remove      []string            `json:"remove"`
	Diameter    float32             `json:"diameter"`
}

type MobEffectMetadata struct {
	Effect    string `json:"effect"`
	Duration  uint16 `json:"duration"`
	Amplifier int8   `json:"amplifier"`
}

type BlockProperty struct {
	Name   string   `json:"name"`
	Values []string `json:"values"`
}

type BlockDefinition struct {
	Name         string          `json:"name"`
	DefaultState uint16          `json:"defaultState"`
	Properties   []BlockProperty `json:"states"`
	BoundingBox  string          `json:"boundingBox"`
}

type PlaceableBlock struct {
	Definition BlockDefinition
	Rule       string
}

func main() {
	itemsPath := flag.String("items", "", "path to items.json")
	blocksPath := flag.String("blocks", "", "path to blocks.json")
	consumablesPath := flag.String("consumables", "", "path to item consumables manifest")
	attackAttributesPath := flag.String("attack-attributes", "", "path to item attack attributes manifest")
	armorAttributesPath := flag.String("armor-attributes", "", "path to item armor attributes manifest")
	outputPath := flag.String("output", "", "generated Go output path")

	flag.Parse()

	if *itemsPath == "" || *blocksPath == "" || *consumablesPath == "" || *attackAttributesPath == "" || *armorAttributesPath == "" || *outputPath == "" {
		fmt.Fprintln(os.Stderr, "items, blocks, consumables, attack-attributes, armor-attributes and output are required")

		os.Exit(2)
	}

	items, err := readItems(*itemsPath)
	if err != nil {
		fail(err)
	}

	blocks, err := readBlocks(*blocksPath)
	if err != nil {
		fail(err)
	}

	consumables, err := readConsumables(*consumablesPath)
	if err != nil {
		fail(err)
	}

	attackAttributes, err := readAttackAttributes(*attackAttributesPath)
	if err != nil {
		fail(err)
	}

	armorAttributes, err := readArmorAttributes(*armorAttributesPath)
	if err != nil {
		fail(err)
	}

	generated, err := generate(items, blocks, consumables, attackAttributes, armorAttributes)
	if err != nil {
		fail(err)
	}

	err = os.WriteFile(*outputPath, generated, 0o644)
	if err != nil {
		fail(err)
	}
}

func readItems(path string) ([]ItemDefinition, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var items []ItemDefinition

	err = json.Unmarshal(raw, &items)
	return items, err
}

func readBlocks(path string) ([]BlockDefinition, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var blocks []BlockDefinition

	err = json.Unmarshal(raw, &blocks)
	return blocks, err
}

func readConsumables(path string) (ConsumableManifest, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return ConsumableManifest{}, err
	}

	var manifest ConsumableManifest

	err = json.Unmarshal(raw, &manifest)
	return manifest, err
}

func readAttackAttributes(path string) (AttackAttributesManifest, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return AttackAttributesManifest{}, err
	}

	var manifest AttackAttributesManifest

	err = json.Unmarshal(raw, &manifest)
	return manifest, err
}

func readArmorAttributes(path string) (ArmorAttributesManifest, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return ArmorAttributesManifest{}, err
	}

	var manifest ArmorAttributesManifest

	err = json.Unmarshal(raw, &manifest)
	return manifest, err
}

func generate(items []ItemDefinition, blocks []BlockDefinition, consumables ConsumableManifest, attackAttributes AttackAttributesManifest, armorAttributes ArmorAttributesManifest) ([]byte, error) {
	if len(items) == 0 {
		return nil, fmt.Errorf("item catalogue is empty")
	}

	for index, item := range items {
		if int(item.ID) != index {
			return nil, fmt.Errorf("item ID %d at index %d is not contiguous", item.ID, index)
		}
	}

	consumablesByName, err := validateConsumables(items, consumables)
	if err != nil {
		return nil, err
	}

	attackAttributesByName, err := validateAttackAttributes(items, attackAttributes)
	if err != nil {
		return nil, err
	}

	armorAttributesByName, err := validateArmorAttributes(items, armorAttributes)
	if err != nil {
		return nil, err
	}

	placeableBlocks := make(map[string]PlaceableBlock)

	for _, block := range blocks {
		rule := blockPlacementRule(block)
		if block.DefaultState == 0 || rule == "" {
			continue
		}

		placeableBlocks[block.Name] = PlaceableBlock{Definition: block, Rule: rule}
	}

	addCropPlacementOverrides(placeableBlocks, blocks)

	var output bytes.Buffer

	fmt.Fprintln(&output, "// Code generated by cmd/generate-items; DO NOT EDIT.")
	fmt.Fprintln(&output)
	fmt.Fprintln(&output, "package game")
	fmt.Fprintln(&output)
	fmt.Fprintf(&output, "const MaxItemID Item = %d\n\n", items[len(items)-1].ID)
	fmt.Fprintln(&output, "const (")

	for _, item := range items {
		fmt.Fprintf(&output, "\tItem%s Item = %d\n", goName(item.Name), item.ID)
	}

	fmt.Fprintln(&output, ")")
	fmt.Fprintln(&output)
	fmt.Fprintln(&output, "var itemDefinitions = [...]ItemDefinition{")

	for _, item := range items {
		food, consumable := itemConsumable(consumableForItem(consumablesByName, item.Name))

		fmt.Fprintf(
			&output,
			"\t{ID: Item%s, Name: %q, StackSize: %d, MaxDurability: %d, AttackDamageModifier: %g, AttackSpeedModifier: %g, DamagePerAttack: %d%s, Mining: %s, Food: %s, Consumable: %s},\n",
			goName(item.Name),
			item.Name,
			item.StackSize,
			item.MaxDurability,
			attackAttributesByName[item.Name].DamageModifier,
			attackAttributesByName[item.Name].SpeedModifier,
			attackAttributesByName[item.Name].DamagePerAttack,
			itemArmor(armorAttributesByName[item.Name]),
			itemMining(item.Name),
			food,
			consumable,
		)
	}

	fmt.Fprintln(&output, "}")
	fmt.Fprintln(&output)
	fmt.Fprintln(&output, "var itemPlacementBlocks = [MaxItemID + 1]Block{")

	for _, item := range items {
		block, ok := placeableBlocks[item.Name]
		if !ok {
			continue
		}

		fmt.Fprintf(&output, "\tItem%s: %s,\n", goName(item.Name), goName(block.Definition.Name))
	}

	fmt.Fprintln(&output, "}")
	fmt.Fprintln(&output)
	fmt.Fprintln(&output, "var itemPlacementRules = [MaxItemID + 1]ItemPlacementRule{")

	for _, item := range items {
		block, ok := placeableBlocks[item.Name]
		if !ok {
			continue
		}

		fmt.Fprintf(&output, "\tItem%s: %s,\n", goName(item.Name), block.Rule)
	}

	fmt.Fprintln(&output, "}")
	fmt.Fprintln(&output)
	fmt.Fprintln(&output, "var itemOnBlockBehaviors = [MaxItemID + 1]ItemOnBlockBehavior{")

	for _, item := range items {
		if strings.HasSuffix(item.Name, "_hoe") {
			fmt.Fprintf(&output, "\tItem%s: ItemOnBlockBehaviorHoe,\n", goName(item.Name))
		}
	}

	fmt.Fprintln(&output, "}")

	formatted, err := format.Source(output.Bytes())
	if err != nil {
		return nil, fmt.Errorf("format generated source: %w", err)
	}

	return formatted, nil
}

func addCropPlacementOverrides(placeableBlocks map[string]PlaceableBlock, blocks []BlockDefinition) {
	blocksByName := make(map[string]BlockDefinition, len(blocks))

	for _, block := range blocks {
		blocksByName[block.Name] = block
	}

	overrides := map[string]string{
		"wheat_seeds":    "wheat",
		"carrot":         "carrots",
		"potato":         "potatoes",
		"beetroot_seeds": "beetroots",
	}

	for itemName, blockName := range overrides {
		block, valid := blocksByName[blockName]
		if !valid {
			continue
		}

		placeableBlocks[itemName] = PlaceableBlock{Definition: block, Rule: "ItemPlacementPlant"}
	}
}

func validateConsumables(items []ItemDefinition, manifest ConsumableManifest) (map[string]ConsumableItemMetadata, error) {
	if manifest.Version != "1.21.11" || len(manifest.Sources) != 10 {
		return nil, fmt.Errorf("consumable manifest must identify the 1.21.11 source set")
	}

	itemNames := make(map[string]struct{}, len(items))

	for _, item := range items {
		itemNames[item.Name] = struct{}{}
	}

	consumables := make(map[string]ConsumableItemMetadata, len(manifest.Consumables))
	foodCount := 0

	for _, consumable := range manifest.Consumables {
		if _, exists := itemNames[consumable.Name]; !exists {
			return nil, fmt.Errorf("consumable item %q is absent from item catalogue", consumable.Name)
		}

		if _, exists := consumables[consumable.Name]; exists {
			return nil, fmt.Errorf("consumable item %q is duplicated", consumable.Name)
		}

		if consumable.Nutrition < 0 || consumable.Saturation < 0 || consumable.Duration == 0 {
			return nil, fmt.Errorf("consumable item %q has invalid gameplay values", consumable.Name)
		}

		if consumable.Nutrition == 0 && consumable.Saturation != 0 {
			return nil, fmt.Errorf("non-food consumable item %q has saturation", consumable.Name)
		}

		if consumable.Nutrition > 0 {
			foodCount++
		}

		if consumable.Animation != "eat" && consumable.Animation != "drink" {
			return nil, fmt.Errorf("consumable item %q has invalid animation %q", consumable.Name, consumable.Animation)
		}

		if consumable.Remainder != "" {
			if _, exists := itemNames[consumable.Remainder]; !exists {
				return nil, fmt.Errorf("consumable item %q has unknown remainder %q", consumable.Name, consumable.Remainder)
			}
		}

		for _, effect := range append(consumable.Effects, consumable.DynamicEffects...) {
			if !validConsumeEffect(effect) {
				return nil, fmt.Errorf("consumable item %q has invalid consume effect %q", consumable.Name, effect.Type)
			}
		}

		consumables[consumable.Name] = consumable
	}

	if foodCount != 40 || len(consumables) != 42 {
		return nil, fmt.Errorf("consumable manifest has %d foods and %d consumables, want 40 and 42", foodCount, len(consumables))
	}

	return consumables, nil
}

func validateAttackAttributes(items []ItemDefinition, manifest AttackAttributesManifest) (map[string]ItemAttackAttributesMetadata, error) {
	if manifest.Version != "1.21.11" || len(manifest.Sources) != 2 {
		return nil, fmt.Errorf("attack attributes manifest must identify the 1.21.11 source set")
	}

	itemNames := make(map[string]struct{}, len(items))

	for _, item := range items {
		itemNames[item.Name] = struct{}{}
	}

	attributes := make(map[string]ItemAttackAttributesMetadata, len(manifest.Attributes))

	for _, attribute := range manifest.Attributes {
		if _, exists := itemNames[attribute.Name]; !exists {
			return nil, fmt.Errorf("attack attributes item %q is absent from item catalogue", attribute.Name)
		}

		if _, exists := attributes[attribute.Name]; exists {
			return nil, fmt.Errorf("attack attributes item %q is duplicated", attribute.Name)
		}

		if attribute.DamagePerAttack != 1 && attribute.DamagePerAttack != 2 {
			return nil, fmt.Errorf("attack attributes item %q has invalid durability cost", attribute.Name)
		}

		attributes[attribute.Name] = attribute
	}

	if len(attributes) != 35 {
		return nil, fmt.Errorf("attack attributes manifest has %d items, want 35", len(attributes))
	}

	materials := []string{"wooden", "copper", "stone", "golden", "iron", "diamond", "netherite"}
	tools := []string{"sword", "shovel", "pickaxe", "axe", "hoe"}

	for _, material := range materials {
		for _, tool := range tools {
			name := material + "_" + tool

			if _, exists := attributes[name]; !exists {
				return nil, fmt.Errorf("attack attributes manifest is missing %q", name)
			}
		}
	}

	return attributes, nil
}

func validateArmorAttributes(items []ItemDefinition, manifest ArmorAttributesManifest) (map[string]ItemArmorMetadata, error) {
	if manifest.Version != "1.21.11" || len(manifest.Sources) != 4 {
		return nil, fmt.Errorf("armor attributes manifest must identify the 1.21.11 source set")
	}

	itemNames := make(map[string]struct{}, len(items))

	for _, item := range items {
		itemNames[item.Name] = struct{}{}
	}

	armorAttributes := make(map[string]ItemArmorMetadata, len(manifest.Attributes))

	for _, armor := range manifest.Attributes {
		name := armor.Name

		if _, exists := itemNames[name]; !exists {
			return nil, fmt.Errorf("armor item %q is absent from item catalogue", name)
		}

		if _, exists := armorAttributes[name]; exists {
			return nil, fmt.Errorf("armor item %q is duplicated", name)
		}

		if armor.Defense <= 0 || armor.Slot == "" || armor.Slot == "BODY" || armor.Toughness < 0 || armor.KnockbackResistance < 0 {
			return nil, fmt.Errorf("armor item %q has invalid gameplay values", name)
		}

		armorAttributes[name] = armor
	}

	if len(armorAttributes) != 29 {
		return nil, fmt.Errorf("armor source has %d humanoid items, want 29", len(armorAttributes))
	}

	return armorAttributes, nil
}

func validConsumeEffect(effect ConsumeEffectMetadata) bool {
	switch effect.Type {
	case "apply":
		return len(effect.Effects) > 0 && effect.Probability > 0 && effect.Probability <= 1
	case "remove":
		return len(effect.Remove) > 0
	case "clear", "suspicious_stew", "potion_contents":
		return true
	case "teleport":
		return effect.Diameter > 0
	default:
		return false
	}
}

func consumableForItem(consumables map[string]ConsumableItemMetadata, name string) ConsumableItemMetadata {
	return consumables[name]
}

func itemConsumable(metadata ConsumableItemMetadata) (string, string) {
	if metadata.Name == "" {
		return "ItemFood{}", "ItemConsumable{}"
	}

	animation := "ItemUseAnimationEat"
	sound := "SoundEntityGenericEat"

	if metadata.Animation == "drink" {
		animation = "ItemUseAnimationDrink"
		sound = "SoundEntityGenericDrink"
	}

	if metadata.Sound != "" {
		name := strings.TrimPrefix(metadata.Sound, "minecraft:")
		sound = "Sound" + goName(strings.ReplaceAll(name, ".", "_"))
	}

	remainder := "ItemAir"

	if metadata.Remainder != "" {
		remainder = "Item" + goName(metadata.Remainder)
	}

	food := fmt.Sprintf("ItemFood{Nutrition: %d, Saturation: %g, AlwaysEdible: %t}", metadata.Nutrition, metadata.Saturation, metadata.AlwaysEdible)

	consumable := fmt.Sprintf(
		"ItemConsumable{UseEffects: ItemUseEffects{CanSprint: %t, InteractVibrations: %t, SpeedMultiplier: %g}, Particles: %t, Sound: %s, Duration: %d, Animation: %s, Remainder: %s, Effects: %s, DynamicEffects: %s}",
		metadata.CanSprint,
		metadata.InteractVibrations,
		metadata.SpeedMultiplier,
		metadata.Particles,
		sound,
		metadata.Duration,
		animation,
		remainder,
		itemConsumeEffects(metadata.Effects),
		itemConsumeEffects(metadata.DynamicEffects),
	)

	return food, consumable
}

func itemArmor(metadata ItemArmorMetadata) string {
	if metadata.Name == "" {
		return ""
	}

	return fmt.Sprintf(
		", Armor: ItemArmor{Slot: ItemEquipmentSlot%s, Defense: %d, Toughness: %g, KnockbackResistance: %g}",
		goName(strings.ToLower(metadata.Slot)),
		metadata.Defense,
		metadata.Toughness,
		metadata.KnockbackResistance,
	)
}

func itemConsumeEffects(effects []ConsumeEffectMetadata) string {
	if len(effects) == 0 {
		return "nil"
	}

	values := make([]string, len(effects))

	for index, effect := range effects {
		values[index] = itemConsumeEffect(effect)
	}

	return "[]ItemConsumeEffect{" + strings.Join(values, ", ") + "}"
}

func itemConsumeEffect(effect ConsumeEffectMetadata) string {
	typeName := "ItemConsumeEffect"

	switch effect.Type {
	case "apply":
		instances := make([]string, len(effect.Effects))

		for index, instance := range effect.Effects {
			instances[index] = fmt.Sprintf("{Effect: MobEffect%s, Duration: %d, Amplifier: %d, Visible: true, ShowIcon: true}", goName(instance.Effect), instance.Duration, instance.Amplifier)
		}

		return fmt.Sprintf("%s{Type: ItemConsumeEffectApplyStatusEffects, Effects: []MobEffectInstance{%s}, Probability: %g}", typeName, strings.Join(instances, ", "), effect.Probability)
	case "remove":
		removed := make([]string, len(effect.Remove))

		for index, name := range effect.Remove {
			removed[index] = "MobEffect" + goName(name)
		}

		return fmt.Sprintf("%s{Type: ItemConsumeEffectRemoveStatusEffects, Remove: []MobEffect{%s}}", typeName, strings.Join(removed, ", "))
	case "clear":
		return typeName + "{Type: ItemConsumeEffectClearAllStatusEffects}"
	case "teleport":
		return fmt.Sprintf("%s{Type: ItemConsumeEffectTeleportRandomly, Diameter: %g}", typeName, effect.Diameter)
	case "potion_contents":
		return typeName + "{Type: ItemConsumeEffectPotionContents}"
	default:
		return typeName + "{Type: ItemConsumeEffectSuspiciousStew}"
	}
}

func itemMining(name string) string {
	parts := strings.Split(name, "_")

	if name == "shears" {
		return "ItemMining{Rules: []ItemMiningRule{{BlockID: CobwebID, Speed: 15, HasSpeed: true, Correct: true, HasCorrectness: true}, {Trait: BlockTraitLeaves, Speed: 15, HasSpeed: true}, {Trait: BlockTraitWool, Speed: 5, HasSpeed: true}, {BlockID: VineID, Speed: 2, HasSpeed: true}, {BlockID: GlowLichenID, Speed: 2, HasSpeed: true}}, DefaultSpeed: 1, DamagePerBlock: 1}"
	}

	if len(parts) != 2 {
		return "ItemMining{}"
	}

	if parts[1] == "sword" {
		return "ItemMining{Rules: []ItemMiningRule{{BlockID: CobwebID, Speed: 15, HasSpeed: true, Correct: true, HasCorrectness: true}, {Trait: BlockTraitSwordInstantlyMines, Speed: 3.4028235e38, HasSpeed: true}, {Trait: BlockTraitSwordEfficient, Speed: 1.5, HasSpeed: true}}, DefaultSpeed: 1, DamagePerBlock: 2}"
	}

	var mineableTrait string

	switch parts[1] {
	case "pickaxe":
		mineableTrait = "BlockTraitMineablePickaxe"
	case "shovel":
		mineableTrait = "BlockTraitMineableShovel"
	case "axe":
		mineableTrait = "BlockTraitMineableAxe"
	case "hoe":
		mineableTrait = "BlockTraitMineableHoe"
	default:
		return "ItemMining{}"
	}

	incorrectTrait := ""
	var speed float32

	switch parts[0] {
	case "wooden":
		incorrectTrait = "BlockTraitIncorrectWoodenTool"
		speed = 2
	case "golden":
		incorrectTrait = "BlockTraitIncorrectGoldTool"
		speed = 12
	case "copper":
		incorrectTrait = "BlockTraitIncorrectCopperTool"
		speed = 5
	case "stone":
		incorrectTrait = "BlockTraitIncorrectStoneTool"
		speed = 4
	case "iron":
		incorrectTrait = "BlockTraitIncorrectIronTool"
		speed = 6
	case "diamond":
		incorrectTrait = "BlockTraitIncorrectDiamondTool"
		speed = 8
	case "netherite":
		incorrectTrait = "BlockTraitIncorrectNetheriteTool"
		speed = 9
	default:
		return "ItemMining{}"
	}

	return fmt.Sprintf("ItemMining{Rules: []ItemMiningRule{{Trait: %s, Correct: false, HasCorrectness: true}, {Trait: %s, Speed: %g, HasSpeed: true, Correct: true, HasCorrectness: true}}, DefaultSpeed: 1, DamagePerBlock: 1}", incorrectTrait, mineableTrait, speed)
}

func blockPlacementRule(block BlockDefinition) string {
	switch {
	case block.Name == "barrel":
		return "ItemPlacementDirectionalFacing"
	case block.Name == "chest" || block.Name == "trapped_chest" || isCopperChest(block.Name):
		return "ItemPlacementChest"
	case isExcludedPlacementBlock(block.Name):
		return ""
	case strings.HasSuffix(block.Name, "_slab"):
		return "ItemPlacementSlab"
	case strings.HasSuffix(block.Name, "_stairs"):
		return "ItemPlacementStairs"
	case strings.HasSuffix(block.Name, "_trapdoor"):
		return "ItemPlacementTrapdoor"
	case strings.HasSuffix(block.Name, "_door"):
		return "ItemPlacementDoor"
	case strings.HasSuffix(block.Name, "_fence_gate"):
		return "ItemPlacementFenceGate"
	case strings.HasSuffix(block.Name, "_fence"):
		return "ItemPlacementFence"
	case block.Name == "iron_bars" || strings.HasSuffix(block.Name, "copper_bars") || strings.HasSuffix(block.Name, "_glass_pane") || block.Name == "glass_pane":
		return "ItemPlacementPane"
	case strings.HasSuffix(block.Name, "_wall"):
		return "ItemPlacementWall"
	case strings.HasSuffix(block.Name, "_leaves"):
		return "ItemPlacementLeaves"
	case strings.HasSuffix(block.Name, "_chain"):
		return "ItemPlacementChain"
	case isSimpleCarpet(block.Name):
		return "ItemPlacementSupported"
	case strings.HasSuffix(block.Name, "_button"):
		return "ItemPlacementButton"
	case strings.HasSuffix(block.Name, "_pressure_plate"):
		if hasProperty(block.Properties, "power") {
			return "ItemPlacementWeightedPressurePlate"
		}

		return "ItemPlacementPressurePlate"
	case block.Name == "snow":
		return "ItemPlacementSnow"
	case block.Name == "candle" || strings.HasSuffix(block.Name, "_candle"):
		return "ItemPlacementCandle"
	case block.Name == "pointed_dripstone":
		return "ItemPlacementPointedDripstone"
	case block.Name == "furnace" || block.Name == "smoker" || block.Name == "blast_furnace":
		return "ItemPlacementFurnace"
	case block.Name == "hopper":
		return "ItemPlacementHopper"
	case isSimplePlant(block.Name):
		return "ItemPlacementPlant"
	case block.Name == "cobweb":
		return "ItemPlacementDefault"
	case isMushroomBlock(block.Name) && hasOnlyProperties(block.Properties, "down", "east", "north", "south", "up", "west"):
		return "ItemPlacementDefault"
	case hasOnlyProperties(block.Properties, "snowy"):
		return "ItemPlacementDefault"
	case isCopperGrate(block.Name) && hasOnlyProperties(block.Properties, "waterlogged"):
		return "ItemPlacementDefault"
	}

	if len(block.Properties) == 0 && block.BoundingBox == "block" {
		return "ItemPlacementDefault"
	}

	if horizontalFacingProperties(block.Properties) {
		return "ItemPlacementHorizontalFacing"
	}

	if len(block.Properties) != 1 {
		return ""
	}

	property := block.Properties[0]
	if property.Name != "axis" || len(property.Values) != 3 {
		return ""
	}

	if property.Values[0] != "x" || property.Values[1] != "y" || property.Values[2] != "z" {
		return ""
	}

	return "ItemPlacementAxis"
}

func isExcludedPlacementBlock(name string) bool {
	switch name {
	case "beacon", "beehive", "bee_nest", "brewing_stand",
		"campfire", "soul_campfire", "suspicious_gravel", "suspicious_sand",
		"chiseled_bookshelf", "crafter", "decorated_pot", "dispenser", "dropper",
		"enchanting_table", "ender_chest", "jukebox", "lectern",
		"spawner", "trial_spawner", "vault":
		return true
	}

	return strings.HasSuffix(name, "_bed") || strings.HasSuffix(name, "_banner") ||
		strings.HasSuffix(name, "_chest") || strings.HasSuffix(name, "_shulker_box") ||
		strings.HasSuffix(name, "_shelf") || strings.HasSuffix(name, "_sign") || strings.HasSuffix(name, "_hanging_sign")
}

func isCopperGrate(name string) bool {
	return name == "copper_grate" || strings.HasSuffix(name, "_copper_grate")
}

func isCopperChest(name string) bool {
	return name == "copper_chest" || strings.HasSuffix(name, "_copper_chest")
}

func isSimplePlant(name string) bool {
	switch name {
	case "short_grass", "fern", "dead_bush", "bush", "short_dry_grass", "tall_dry_grass",
		"dandelion", "poppy", "blue_orchid", "allium", "azure_bluet", "red_tulip",
		"orange_tulip", "white_tulip", "pink_tulip", "oxeye_daisy", "cornflower",
		"wither_rose", "lily_of_the_valley", "open_eyeblossom", "closed_eyeblossom",
		"firefly_bush":
		return true
	default:
		return false
	}
}

func isMushroomBlock(name string) bool {
	return name == "brown_mushroom_block" || name == "red_mushroom_block" || name == "mushroom_stem"
}

func isSimpleCarpet(name string) bool {
	return name != "pale_moss_carpet" && (name == "moss_carpet" || strings.HasSuffix(name, "_carpet"))
}

func hasOnlyProperties(properties []BlockProperty, names ...string) bool {
	if len(properties) != len(names) {
		return false
	}

	for index, name := range names {
		if properties[index].Name != name {
			return false
		}
	}

	return true
}

func hasProperty(properties []BlockProperty, name string) bool {
	for _, property := range properties {
		if property.Name == name {
			return true
		}
	}

	return false
}

func horizontalFacingProperties(properties []BlockProperty) bool {
	if len(properties) != 1 || properties[0].Name != "facing" {
		return false
	}

	values := properties[0].Values
	return len(values) == 4 && values[0] == "north" && values[1] == "south" && values[2] == "west" && values[3] == "east"
}

func goName(name string) string {
	parts := strings.Split(name, "_")

	for index, part := range parts {
		runes := []rune(part)
		if len(runes) == 0 {
			continue
		}

		runes[0] = unicode.ToUpper(runes[0])
		parts[index] = string(runes)
	}

	return strings.Join(parts, "")
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)

	os.Exit(1)
}
