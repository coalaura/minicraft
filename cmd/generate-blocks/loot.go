package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// BlockLootPrograms is the generation-time representation of the runtime loot VM.
// Index zero is reserved for tables that intentionally produce no loot.
type BlockLootPrograms struct {
	Indexes  map[string]uint16
	Programs []string
	Deferred map[string]string
}

type lootCompiler struct {
	block      BlockDefinition
	itemIDs    map[string]uint16
	properties map[string]map[string]bool
}

type deferredLootError struct {
	reason string
}

func (err deferredLootError) Error() string {
	return err.reason
}

func readBlockLootPrograms(root string, blocks []BlockDefinition, itemIDs map[string]uint16) (BlockLootPrograms, error) {
	result := BlockLootPrograms{Indexes: make(map[string]uint16, len(blocks)), Programs: []string{"{}"}, Deferred: make(map[string]string)}

	for _, block := range blocks {
		if !block.Diggable {
			continue
		}

		path := filepath.Join(root, block.Name+".json")

		raw, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				if len(block.Drops) != 0 {
					result.Deferred[block.Name] = "missing canonical loot table"
				}

				continue
			}

			return BlockLootPrograms{}, fmt.Errorf("read loot table for %s: %w", block.Name, err)
		}

		compiler := newLootCompiler(block, itemIDs)

		program, err := compiler.compileTable(raw)
		if err != nil {
			var deferred deferredLootError

			if !asDeferred(err, &deferred) {
				return BlockLootPrograms{}, fmt.Errorf("compile loot table for %s: %w", block.Name, err)
			}

			result.Deferred[block.Name] = deferred.reason

			continue
		}

		if program == "{}" {
			continue
		}

		if len(result.Programs) > int(^uint16(0)) {
			return BlockLootPrograms{}, fmt.Errorf("too many block loot programs")
		}

		result.Indexes[block.Name] = uint16(len(result.Programs))

		result.Programs = append(result.Programs, program)
	}

	return result, nil
}

func asDeferred(err error, target *deferredLootError) bool {
	return errors.As(err, target)
}

func newLootCompiler(block BlockDefinition, itemIDs map[string]uint16) lootCompiler {
	properties := make(map[string]map[string]bool, len(block.Properties))

	for _, property := range block.Properties {
		values := property.Values

		if property.Type == "bool" {
			values = []string{"true", "false"}
		}

		validValues := make(map[string]bool, len(values))

		for _, value := range values {
			validValues[value] = true
		}

		properties[property.Name] = validValues
	}

	return lootCompiler{block: block, itemIDs: itemIDs, properties: properties}
}

func (compiler lootCompiler) compileTable(raw []byte) (string, error) {
	object, err := lootObject(raw)
	if err != nil {
		return "", err
	}

	tableType := lootString(object, "type")
	if tableType != "minecraft:block" {
		return "", deferredLootError{reason: "non-block loot table"}
	}

	pools, err := lootArray(object, "pools")
	if err != nil {
		return "", err
	}

	functions, err := compiler.compileFunctions(object["functions"])
	if err != nil {
		return "", err
	}

	compiledPools := make([]string, 0, len(pools))

	for _, pool := range pools {
		compiled, compileErr := compiler.compilePool(pool)
		if compileErr != nil {
			return "", compileErr
		}

		compiledPools = append(compiledPools, compiled)
	}

	if len(compiledPools) == 0 && len(functions) == 0 {
		return "{}", nil
	}

	return "{Pools: []blockLootPool{" + strings.Join(compiledPools, ",") + "}, Functions: []blockLootFunction{" + strings.Join(functions, ",") + "}}", nil
}

func (compiler lootCompiler) compilePool(raw json.RawMessage) (string, error) {
	object, err := lootObject(raw)
	if err != nil {
		return "", err
	}

	rolls, err := compiler.compileNumber(object["rolls"])
	if err != nil {
		return "", err
	}

	bonusRolls := float64(0)

	if len(object["bonus_rolls"]) != 0 {
		err = json.Unmarshal(object["bonus_rolls"], &bonusRolls)
		if err != nil || bonusRolls != 0 {
			return "", deferredLootError{reason: "nonzero bonus_rolls"}
		}
	}

	conditions, err := compiler.compileConditions(object["conditions"])
	if err != nil {
		return "", err
	}

	functions, err := compiler.compileFunctions(object["functions"])
	if err != nil {
		return "", err
	}

	entries, err := lootArray(object, "entries")
	if err != nil {
		return "", err
	}

	compiledEntries := make([]string, 0, len(entries))

	for _, entry := range entries {
		compiled, compileErr := compiler.compileEntry(entry)
		if compileErr != nil {
			return "", compileErr
		}

		compiledEntries = append(compiledEntries, compiled)
	}

	return "{Rolls: " + rolls + ", BonusRolls: blockLootNumberProvider{Kind: blockLootNumberConstant}, Conditions: []blockLootCondition{" + strings.Join(conditions, ",") + "}, Entries: []blockLootEntry{" + strings.Join(compiledEntries, ",") + "}, Functions: []blockLootFunction{" + strings.Join(functions, ",") + "}}", nil
}

func (compiler lootCompiler) compileEntry(raw json.RawMessage) (string, error) {
	object, err := lootObject(raw)
	if err != nil {
		return "", err
	}

	var kind string

	entryType := lootString(object, "type")

	switch entryType {
	case "minecraft:item":
		kind = "blockLootEntryItem"
	case "minecraft:alternatives":
		kind = "blockLootEntryAlternatives"
	default:
		return "", deferredLootError{reason: "unsupported entry " + entryType}
	}

	conditions, err := compiler.compileConditions(object["conditions"])
	if err != nil {
		return "", err
	}

	functions, err := compiler.compileFunctions(object["functions"])
	if err != nil {
		return "", err
	}

	children, err := lootArray(object, "children")
	if err != nil {
		return "", err
	}

	compiledChildren := make([]string, 0, len(children))

	for _, child := range children {
		compiled, compileErr := compiler.compileEntry(child)
		if compileErr != nil {
			return "", compileErr
		}

		compiledChildren = append(compiledChildren, compiled)
	}

	item := "0"

	if entryType == "minecraft:item" {
		itemName := strings.TrimPrefix(lootString(object, "name"), "minecraft:")

		itemID, valid := compiler.itemIDs[itemName]
		if !valid {
			return "", fmt.Errorf("unknown item %q", itemName)
		}

		item = fmt.Sprintf("Item(%d)", itemID)
	}

	weight := lootInt(object, "weight")
	quality := lootInt(object, "quality")

	return fmt.Sprintf("{Kind: %s, Item: %s, Weight: %d, Quality: %d, Conditions: []blockLootCondition{%s}, Children: []blockLootEntry{%s}, Functions: []blockLootFunction{%s}}", kind, item, weight, quality, strings.Join(conditions, ","), strings.Join(compiledChildren, ","), strings.Join(functions, ",")), nil
}

func (compiler lootCompiler) compileConditions(raw json.RawMessage) ([]string, error) {
	if len(raw) == 0 {
		return nil, nil
	}

	values, err := lootRawArray(raw)
	if err != nil {
		return nil, err
	}

	result := make([]string, 0, len(values))

	for _, value := range values {
		compiled, compileErr := compiler.compileCondition(value)
		if compileErr != nil {
			return nil, compileErr
		}

		result = append(result, compiled)
	}

	return result, nil
}

func (compiler lootCompiler) compileCondition(raw json.RawMessage) (string, error) {
	object, err := lootObject(raw)
	if err != nil {
		return "", err
	}

	condition := lootString(object, "condition")

	switch condition {
	case "minecraft:survives_explosion":
		return "{Kind: blockLootConditionAlways}", nil
	case "minecraft:random_chance":
		return fmt.Sprintf("{Kind: blockLootConditionRandomChance, Chance: %s}", lootFloat(object, "chance")), nil
	case "minecraft:block_state_property":
		blockName := strings.TrimPrefix(lootString(object, "block"), "minecraft:")
		if blockName != compiler.block.Name {
			return "", fmt.Errorf("block state condition references unknown block %q", blockName)
		}

		properties, err := lootStringMap(object["properties"])
		if err != nil {
			return "", err
		}

		parts := make([]string, 0, len(properties))

		for name, value := range properties {
			validValues, valid := compiler.properties[name]
			if !valid || !validValues[value] {
				return "", fmt.Errorf("unknown property %s=%s for %s", name, value, compiler.block.Name)
			}

			parts = append(parts, fmt.Sprintf("{Name: %q, Value: %q}", name, value))
		}

		sort.Strings(parts)

		return "{Kind: blockLootConditionBlockState, Properties: []blockLootProperty{" + strings.Join(parts, ",") + "}}", nil
	case "minecraft:all_of", "minecraft:any_of":
		terms, termsErr := compiler.compileConditions(object["terms"])
		if termsErr != nil {
			return "", termsErr
		}

		kind := "blockLootConditionAllOf"

		if condition == "minecraft:any_of" {
			kind = "blockLootConditionAnyOf"
		}

		return "{Kind: " + kind + ", Terms: []blockLootCondition{" + strings.Join(terms, ",") + "}}", nil
	case "minecraft:inverted":
		term, termErr := compiler.compileCondition(object["term"])
		if termErr != nil {
			return "", termErr
		}

		return "{Kind: blockLootConditionInverted, Terms: []blockLootCondition{" + term + "}}", nil
	case "minecraft:entity_properties":
		if lootString(object, "entity") != "this" || !lootEmptyObject(object["predicate"]) {
			return "", deferredLootError{reason: "non-empty entity_properties predicate"}
		}

		return "{Kind: blockLootConditionActorRequired}", nil
	case "minecraft:table_bonus":
		enchantment, enchantmentErr := lootEnchantment(lootString(object, "enchantment"))
		if enchantmentErr != nil {
			return "", enchantmentErr
		}

		chances, chancesErr := lootFloatArray(object["chances"])
		if chancesErr != nil {
			return "", chancesErr
		}

		return "{Kind: blockLootConditionTableBonus, Enchantment: " + enchantment + ", Chances: []float32{" + strings.Join(chances, ",") + "}}", nil
	case "minecraft:match_tool":
		return compiler.compileMatchTool(object)
	case "minecraft:location_check":
		return "", deferredLootError{reason: "location_check"}
	default:
		return "", deferredLootError{reason: "unsupported condition " + condition}
	}
}

func (compiler lootCompiler) compileMatchTool(object map[string]json.RawMessage) (string, error) {
	item := lootStringFromObject(object["predicate"], "items")
	if item != "" {
		if strings.HasPrefix(item, "#") {
			return "", deferredLootError{reason: "match_tool item tag"}
		}

		itemID, valid := compiler.itemIDs[strings.TrimPrefix(item, "minecraft:")]
		if !valid {
			return "", fmt.Errorf("unknown item %q", item)
		}

		return fmt.Sprintf("{Kind: blockLootConditionToolItem, Item: Item(%d)}", itemID), nil
	}

	predicates, err := lootObjectFromRaw(lootObjectField(object, "predicate"), "predicates")
	if err != nil {
		return "", err
	}

	enchantments, valid := predicates["minecraft:enchantments"]
	if !valid {
		return "", deferredLootError{reason: "unsupported match_tool predicate"}
	}

	values, arrayErr := lootRawArray(enchantments)
	if arrayErr != nil || len(values) != 1 {
		return "", deferredLootError{reason: "unsupported enchantment predicate"}
	}

	requirement, objectErr := lootObject(values[0])
	if objectErr != nil {
		return "", objectErr
	}

	enchantment, enchantmentErr := lootEnchantment(lootString(requirement, "enchantments"))
	if enchantmentErr != nil {
		return "", enchantmentErr
	}

	levels, levelsErr := lootObject(requirement["levels"])
	if levelsErr != nil {
		return "", levelsErr
	}

	return fmt.Sprintf("{Kind: blockLootConditionToolEnchantment, Enchantment: %s, MinLevel: %d, MaxLevel: %d}", enchantment, lootInt(levels, "min"), lootInt(levels, "max")), nil
}

func (compiler lootCompiler) compileFunctions(raw json.RawMessage) ([]string, error) {
	if len(raw) == 0 {
		return nil, nil
	}

	values, err := lootRawArray(raw)
	if err != nil {
		return nil, err
	}

	result := make([]string, 0, len(values))

	for _, value := range values {
		object, objectErr := lootObject(value)
		if objectErr != nil {
			return nil, objectErr
		}

		conditions, conditionsErr := compiler.compileConditions(object["conditions"])
		if conditionsErr != nil {
			return nil, conditionsErr
		}

		function := lootString(object, "function")
		compiled := ""

		switch function {
		case "minecraft:explosion_decay":
			compiled = "{Kind: blockLootFunctionNoop}"
		case "minecraft:set_count":
			count, countErr := compiler.compileNumber(object["count"])
			if countErr != nil {
				return nil, countErr
			}

			compiled = fmt.Sprintf("{Kind: blockLootFunctionSetCount, Count: %s, Add: %t}", count, lootBool(object, "add"))
		case "minecraft:limit_count":
			limit, limitErr := compiler.compileNumber(object["limit"])
			if limitErr != nil {
				return nil, limitErr
			}

			compiled = "{Kind: blockLootFunctionLimitCount, Count: " + limit + "}"
		case "minecraft:apply_bonus":
			enchantment, enchantmentErr := lootEnchantment(lootString(object, "enchantment"))
			if enchantmentErr != nil {
				return nil, enchantmentErr
			}

			var formulaKind string

			formula := lootString(object, "formula")

			switch formula {
			case "minecraft:ore_drops":
				formulaKind = "blockLootBonusOreDrops"
			case "minecraft:uniform_bonus_count":
				formulaKind = "blockLootBonusUniformCount"
			case "minecraft:binomial_with_bonus_count":
				formulaKind = "blockLootBonusBinomialCount"
			default:
				return nil, deferredLootError{reason: "unsupported apply_bonus formula " + formula}
			}

			parameters, parametersErr := lootOptionalObject(object["parameters"])
			if parametersErr != nil {
				return nil, parametersErr
			}

			extra := lootFloat(parameters, "extra")

			if formula == "minecraft:uniform_bonus_count" {
				extra = lootFloat(parameters, "bonusMultiplier")
			}

			compiled = "{Kind: blockLootFunctionApplyBonus, Enchantment: " + enchantment + ", Bonus: " + formulaKind + ", Extra: " + extra + ", Probability: " + lootFloat(parameters, "probability") + "}"
		case "minecraft:copy_components":
			include, includeErr := lootStringArray(object["include"])
			if includeErr != nil {
				return nil, includeErr
			}

			if lootString(object, "source") != "block_entity" || len(include) != 1 || include[0] != "minecraft:custom_name" || len(object["exclude"]) != 0 {
				return nil, deferredLootError{reason: "copy_components"}
			}

			// Minicraft block entities cannot currently carry custom names, so the
			// canonical copy is an exact no-op for every representable block entity.
			compiled = "{Kind: blockLootFunctionNoop}"
		case "minecraft:copy_state":
			return nil, deferredLootError{reason: "copy_state"}
		default:
			return nil, deferredLootError{reason: "unsupported function " + function}
		}

		compiled = strings.TrimSuffix(compiled, "}") + ", Conditions: []blockLootCondition{" + strings.Join(conditions, ",") + "}}"
		result = append(result, compiled)
	}

	return result, nil
}

func (compiler lootCompiler) compileNumber(raw json.RawMessage) (string, error) {
	if len(raw) == 0 {
		return "blockLootNumberProvider{Kind: blockLootNumberConstant, Min: 0}", nil
	}

	var number float64

	err := json.Unmarshal(raw, &number)
	if err == nil {
		return "blockLootNumberProvider{Kind: blockLootNumberConstant, Min: " + strconv.FormatFloat(number, 'g', -1, 32) + "}", nil
	}

	object, objectErr := lootObject(raw)
	if objectErr != nil {
		return "", objectErr
	}

	providerType := lootString(object, "type")
	if providerType == "" {
		min := lootFloat(object, "min")
		max := lootFloat(object, "max")

		return "blockLootNumberProvider{Kind: blockLootNumberLimit, Min: " + min + ", Max: " + max + "}", nil
	}

	if providerType != "minecraft:uniform" {
		return "", deferredLootError{reason: "unsupported number provider " + providerType}
	}

	min, minErr := compiler.compileNumber(object["min"])
	if minErr != nil {
		return "", minErr
	}

	max, maxErr := compiler.compileNumber(object["max"])
	if maxErr != nil {
		return "", maxErr
	}

	return "blockLootNumberProvider{Kind: blockLootNumberUniform, MinProvider: &" + min + ", MaxProvider: &" + max + "}", nil
}

func emitBlockLootPrograms(output *bytes.Buffer, programs BlockLootPrograms) {
	fmt.Fprintln(output, "var generatedBlockLootPrograms = [...]blockLootProgram{")

	for _, program := range programs.Programs {
		fmt.Fprintln(output, "\t"+program+",")
	}

	fmt.Fprintln(output, "}")
	fmt.Fprintln(output)

	if len(programs.Deferred) == 0 {
		return
	}

	names := make([]string, 0, len(programs.Deferred))

	for name := range programs.Deferred {
		names = append(names, name)
	}

	sort.Strings(names)

	fmt.Fprintln(output, "// Deferred canonical block loot tables:")

	for _, name := range names {
		fmt.Fprintf(output, "// %s: %s\n", name, programs.Deferred[name])
	}

	fmt.Fprintln(output)
}

func lootObject(raw []byte) (map[string]json.RawMessage, error) {
	var value map[string]json.RawMessage

	err := json.Unmarshal(raw, &value)
	if err != nil {
		return nil, fmt.Errorf("expected loot object: %w", err)
	}

	return value, nil
}

func lootOptionalObject(raw []byte) (map[string]json.RawMessage, error) {
	if len(raw) == 0 {
		return nil, nil
	}

	return lootObject(raw)
}

func lootRawArray(raw []byte) ([]json.RawMessage, error) {
	var value []json.RawMessage

	err := json.Unmarshal(raw, &value)
	if err != nil {
		return nil, fmt.Errorf("expected loot array: %w", err)
	}

	return value, nil
}

func lootArray(object map[string]json.RawMessage, key string) ([]json.RawMessage, error) {
	if len(object[key]) == 0 {
		return nil, nil
	}

	return lootRawArray(object[key])
}

func lootString(object map[string]json.RawMessage, key string) string {
	var value string

	_ = json.Unmarshal(object[key], &value)

	return value
}

func lootInt(object map[string]json.RawMessage, key string) int {
	var value int

	_ = json.Unmarshal(object[key], &value)

	return value
}

func lootBool(object map[string]json.RawMessage, key string) bool {
	var value bool

	_ = json.Unmarshal(object[key], &value)

	return value
}

func lootFloat(object map[string]json.RawMessage, key string) string {
	var value float64

	_ = json.Unmarshal(object[key], &value)

	return strconv.FormatFloat(value, 'g', -1, 32)
}

func lootStringMap(raw []byte) (map[string]string, error) {
	var value map[string]string

	err := json.Unmarshal(raw, &value)
	return value, err
}

func lootFloatArray(raw []byte) ([]string, error) {
	var values []float64

	err := json.Unmarshal(raw, &values)
	if err != nil {
		return nil, err
	}

	result := make([]string, 0, len(values))

	for _, value := range values {
		result = append(result, strconv.FormatFloat(value, 'g', -1, 32))
	}

	return result, nil
}

func lootStringArray(raw []byte) ([]string, error) {
	var values []string

	err := json.Unmarshal(raw, &values)
	return values, err
}

func lootEmptyObject(raw []byte) bool {
	object, err := lootObject(raw)
	return err == nil && len(object) == 0
}

func lootStringFromObject(raw []byte, key string) string {
	object, err := lootObject(raw)
	if err != nil {
		return ""
	}

	return lootString(object, key)
}

func lootObjectField(object map[string]json.RawMessage, key string) json.RawMessage {
	return object[key]
}

func lootObjectFromRaw(raw []byte, key string) (map[string]json.RawMessage, error) {
	object, err := lootObject(raw)
	if err != nil {
		return nil, err
	}

	return lootObject(object[key])
}

func lootEnchantment(name string) (string, error) {
	switch name {
	case "minecraft:silk_touch":
		return "EnchantmentSilkTouch", nil
	case "minecraft:fortune":
		return "EnchantmentFortune", nil
	default:
		return "", fmt.Errorf("unknown enchantment %q", name)
	}
}
