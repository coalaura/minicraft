package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type blockLootDeferredTestCase struct {
	raw    string
	reason string
}

func TestCompileBlockLootProgram(t *testing.T) {
	block := BlockDefinition{Name: "test_block", Properties: []BlockProperty{{Name: "age", Values: []string{"0", "1"}}}}
	compiler := newLootCompiler(block, map[string]uint16{"stone": 1, "stick": 2, "pickaxe": 3})
	raw := []byte(`{
"type":"minecraft:block",
"functions":[{"function":"minecraft:limit_count","limit":{"min":0,"max":4}}],
"pools":[
 {"rolls":1,"bonus_rolls":0,"conditions":[{"condition":"minecraft:all_of","terms":[{"condition":"minecraft:block_state_property","block":"minecraft:test_block","properties":{"age":"1"}},{"condition":"minecraft:entity_properties","entity":"this","predicate":{}}]}],"entries":[{"type":"minecraft:alternatives","children":[{"type":"minecraft:item","name":"minecraft:stone","conditions":[{"condition":"minecraft:any_of","terms":[{"condition":"minecraft:match_tool","predicate":{"items":"minecraft:pickaxe"}},{"condition":"minecraft:match_tool","predicate":{"predicates":{"minecraft:enchantments":[{"enchantments":"minecraft:silk_touch","levels":{"min":1}}]}}}]}],"functions":[{"function":"minecraft:set_count","count":{"type":"minecraft:uniform","min":1,"max":2},"add":false},{"function":"minecraft:apply_bonus","enchantment":"minecraft:fortune","formula":"minecraft:uniform_bonus_count","parameters":{"bonusMultiplier":2}}]},{"type":"minecraft:item","name":"minecraft:stick","conditions":[{"condition":"minecraft:inverted","term":{"condition":"minecraft:table_bonus","enchantment":"minecraft:fortune","chances":[0.1,0.2]}},{"condition":"minecraft:random_chance","chance":0.5}],"functions":[{"function":"minecraft:apply_bonus","enchantment":"minecraft:fortune","formula":"minecraft:ore_drops"},{"function":"minecraft:apply_bonus","enchantment":"minecraft:fortune","formula":"minecraft:binomial_with_bonus_count","parameters":{"extra":3,"probability":0.5}},{"function":"minecraft:explosion_decay"}]}]}],"functions":[]},
 {"rolls":1,"entries":[{"type":"minecraft:item","name":"minecraft:stone"}],"functions":[]}
]}`)

	program, err := compiler.compileTable(raw)
	if err != nil {
		t.Fatal(err)
	}

	wants := []string{
		"Pools: []blockLootPool{",
		"blockLootConditionAllOf",
		"blockLootConditionAnyOf",
		"blockLootConditionInverted",
		"blockLootConditionActorRequired",
		"blockLootConditionToolItem",
		"blockLootConditionToolEnchantment",
		"blockLootConditionBlockState",
		"blockLootConditionTableBonus",
		"blockLootConditionRandomChance",
		"blockLootFunctionSetCount",
		"blockLootFunctionLimitCount",
		"blockLootBonusOreDrops",
		"blockLootBonusUniformCount",
		"blockLootBonusBinomialCount",
		"blockLootFunctionNoop",
		"blockLootNumberUniform",
		"Bonus: blockLootBonusUniformCount, Extra: 2",
		"Bonus: blockLootBonusBinomialCount, Extra: 3, Probability: 0.5",
	}

	for _, want := range wants {
		if !strings.Contains(program, want) {
			t.Errorf("program does not contain %q:\n%s", want, program)
		}
	}
}

func TestCompileBlockLootRejectsUnknownNames(t *testing.T) {
	block := BlockDefinition{Name: "test_block", Properties: []BlockProperty{{Name: "age", Values: []string{"0"}}}}

	compiler := newLootCompiler(block, map[string]uint16{"stone": 1})

	rawTables := [][]byte{
		[]byte(`{"type":"minecraft:block","pools":[{"rolls":1,"entries":[{"type":"minecraft:item","name":"minecraft:unknown"}]}]}`),
		[]byte(`{"type":"minecraft:block","pools":[{"rolls":1,"entries":[{"type":"minecraft:item","name":"minecraft:stone","conditions":[{"condition":"minecraft:block_state_property","block":"minecraft:test_block","properties":{"unknown":"0"}}]}]}]}`),
		[]byte(`{"type":"minecraft:block","pools":[{"rolls":1,"entries":[{"type":"minecraft:item","name":"minecraft:stone","conditions":[{"condition":"minecraft:match_tool","predicate":{"predicates":{"minecraft:enchantments":[{"enchantments":"minecraft:unknown","levels":{"min":1}}]}}}]}]}]}`),
	}

	for _, raw := range rawTables {
		_, err := compiler.compileTable(raw)
		if err == nil {
			t.Fatal("compileTable succeeded for an unknown canonical name")
		}
	}
}

func TestCompileBlockLootDefersUnsupportedConstructs(t *testing.T) {
	compiler := newLootCompiler(BlockDefinition{Name: "test_block"}, map[string]uint16{"stone": 1})

	tests := map[string]blockLootDeferredTestCase{
		"location condition": {
			raw:    `{"type":"minecraft:block","pools":[{"rolls":1,"entries":[{"type":"minecraft:item","name":"minecraft:stone","conditions":[{"condition":"minecraft:location_check"}]}]}]}`,
			reason: "location_check",
		},
		"nonzero bonus rolls": {
			raw:    `{"type":"minecraft:block","pools":[{"rolls":1,"bonus_rolls":1,"entries":[{"type":"minecraft:item","name":"minecraft:stone"}]}]}`,
			reason: "nonzero bonus_rolls",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			var deferred deferredLootError

			_, err := compiler.compileTable([]byte(test.raw))
			if !asDeferred(err, &deferred) || deferred.reason != test.reason {
				t.Fatalf("compileTable error = %v, want deferred %s", err, test.reason)
			}
		})
	}
}

func TestGeneratedParity(t *testing.T) {
	root := filepath.Join("..", "..")

	blocksPath := filepath.Join(root, "data", "blocks.json")
	itemsPath := filepath.Join(root, "data", "items.json")
	tagsPath := filepath.Join(root, "data", "block_tags")
	lootPath := filepath.Join(root, "data", "block_loot")

	blocksRaw, err := os.ReadFile(blocksPath)
	if err != nil {
		t.Fatal(err)
	}

	var blocks []BlockDefinition

	err = json.Unmarshal(blocksRaw, &blocks)
	if err != nil {
		t.Fatal(err)
	}

	tags, err := readMiningTags(tagsPath)
	if err != nil {
		t.Fatal(err)
	}

	items, err := readItemIDs(itemsPath)
	if err != nil {
		t.Fatal(err)
	}

	programs, err := readBlockLootPrograms(lootPath, blocks, items)
	if err != nil {
		t.Fatal(err)
	}

	generated, err := generate(blocks, tags, programs)
	if err != nil {
		t.Fatal(err)
	}

	want, err := os.ReadFile(filepath.Join(root, "internal", "game", "blocks_generated.go"))
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(generated, want) {
		t.Fatal("game generated output is stale; run cmd/generate-blocks after adding runtime loot program types")
	}
}
