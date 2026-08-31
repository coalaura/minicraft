package game

type blockLootEntryKind uint8
type blockLootConditionKind uint8
type blockLootFunctionKind uint8
type blockLootNumberKind uint8
type blockLootBonusKind uint8

const (
	blockLootEntryItem blockLootEntryKind = iota
	blockLootEntryAlternatives
)

const (
	blockLootConditionAlways blockLootConditionKind = iota
	blockLootConditionRandomChance
	blockLootConditionBlockState
	blockLootConditionAllOf
	blockLootConditionAnyOf
	blockLootConditionInverted
	blockLootConditionActorRequired
	blockLootConditionTableBonus
	blockLootConditionToolItem
	blockLootConditionToolEnchantment
)

const (
	blockLootFunctionNoop blockLootFunctionKind = iota
	blockLootFunctionSetCount
	blockLootFunctionLimitCount
	blockLootFunctionApplyBonus
)

const (
	blockLootNumberConstant blockLootNumberKind = iota
	blockLootNumberUniform
	blockLootNumberLimit
)

const (
	blockLootBonusOreDrops blockLootBonusKind = iota
	blockLootBonusUniformCount
	blockLootBonusBinomialCount
)

type BlockLootContext struct {
	Block    Block
	Tool     ItemStack
	HasActor bool
}

type BlockLootRandom interface {
	IntN(int) int
	Float32() float32
}

type blockLootProgram struct {
	Pools     []blockLootPool
	Functions []blockLootFunction
}

type blockLootPool struct {
	Rolls      blockLootNumberProvider
	BonusRolls blockLootNumberProvider
	Conditions []blockLootCondition
	Entries    []blockLootEntry
	Functions  []blockLootFunction
}

type blockLootEntry struct {
	Kind       blockLootEntryKind
	Item       Item
	Weight     int
	Quality    int
	Conditions []blockLootCondition
	Children   []blockLootEntry
	Functions  []blockLootFunction
}

type blockLootProperty struct {
	Name  string
	Value string
}

type blockLootCondition struct {
	Kind        blockLootConditionKind
	Chance      float32
	Enchantment Enchantment
	Chances     []float32
	Properties  []blockLootProperty
	Terms       []blockLootCondition
	Item        Item
	MinLevel    int
	MaxLevel    int
}

type blockLootFunction struct {
	Kind        blockLootFunctionKind
	Count       blockLootNumberProvider
	Add         bool
	Enchantment Enchantment
	Bonus       blockLootBonusKind
	Extra       float32
	Probability float32
	Conditions  []blockLootCondition
}

type blockLootNumberProvider struct {
	Kind        blockLootNumberKind
	Min         float32
	Max         float32
	MinProvider *blockLootNumberProvider
	MaxProvider *blockLootNumberProvider
}

type expandedBlockLootEntry struct {
	entry  blockLootEntry
	weight int
}

func EvaluateBlockLoot(context BlockLootContext, random BlockLootRandom) []ItemStack {
	mining := context.Block.MiningProperties()

	index := int(mining.LootProgram)
	if index <= 0 || index >= len(generatedBlockLootPrograms) {
		return nil
	}

	program := generatedBlockLootPrograms[index]

	drops := make([]ItemStack, 0)

	for _, pool := range program.Pools {
		if !blockLootConditionsMatch(pool.Conditions, context, random) {
			continue
		}

		rolls := blockLootNumber(pool.Rolls, random)

		for range rolls {
			entry, valid := selectBlockLootEntry(pool.Entries, context, random)
			if !valid {
				continue
			}

			stack := ItemStack{Item: entry.Item, Count: 1}

			stack = applyBlockLootFunctions(stack, entry.Functions, context, random)
			stack = applyBlockLootFunctions(stack, pool.Functions, context, random)
			stack = applyBlockLootFunctions(stack, program.Functions, context, random)

			if stack.Item != ItemAir && stack.Count > 0 {
				drops = append(drops, stack)
			}
		}
	}

	return drops
}

func selectBlockLootEntry(entries []blockLootEntry, context BlockLootContext, random BlockLootRandom) (blockLootEntry, bool) {
	expanded := make([]expandedBlockLootEntry, 0, len(entries))

	totalWeight := 0

	for _, entry := range entries {
		candidate, valid := expandBlockLootEntry(entry, context, random)
		if !valid {
			continue
		}

		weight := candidate.Weight
		if weight <= 0 {
			weight = 1
		}

		totalWeight += weight

		expanded = append(expanded, expandedBlockLootEntry{entry: candidate, weight: weight})
	}

	if len(expanded) == 0 {
		return blockLootEntry{}, false
	}

	if len(expanded) == 1 {
		return expanded[0].entry, true
	}

	selection := random.IntN(totalWeight)

	for _, candidate := range expanded {
		selection -= candidate.weight
		if selection < 0 {
			return candidate.entry, true
		}
	}

	return blockLootEntry{}, false
}

func expandBlockLootEntry(entry blockLootEntry, context BlockLootContext, random BlockLootRandom) (blockLootEntry, bool) {
	if !blockLootConditionsMatch(entry.Conditions, context, random) {
		return blockLootEntry{}, false
	}

	switch entry.Kind {
	case blockLootEntryItem:
		return entry, true
	case blockLootEntryAlternatives:
		for _, child := range entry.Children {
			candidate, valid := expandBlockLootEntry(child, context, random)
			if valid {
				return candidate, true
			}
		}
	}

	return blockLootEntry{}, false
}

func blockLootConditionsMatch(conditions []blockLootCondition, context BlockLootContext, random BlockLootRandom) bool {
	for _, condition := range conditions {
		if !blockLootConditionMatches(condition, context, random) {
			return false
		}
	}

	return true
}

func blockLootConditionMatches(condition blockLootCondition, context BlockLootContext, random BlockLootRandom) bool {
	switch condition.Kind {
	case blockLootConditionAlways:
		return true
	case blockLootConditionRandomChance:
		return random.Float32() < condition.Chance
	case blockLootConditionBlockState:
		for _, property := range condition.Properties {
			value, valid := context.Block.Property(property.Name)
			if !valid || value != property.Value {
				return false
			}
		}

		return true
	case blockLootConditionAllOf:
		return blockLootConditionsMatch(condition.Terms, context, random)
	case blockLootConditionAnyOf:
		for _, term := range condition.Terms {
			if blockLootConditionMatches(term, context, random) {
				return true
			}
		}

		return false
	case blockLootConditionInverted:
		return len(condition.Terms) == 1 && !blockLootConditionMatches(condition.Terms[0], context, random)
	case blockLootConditionActorRequired:
		return context.HasActor
	case blockLootConditionTableBonus:
		if len(condition.Chances) == 0 {
			return false
		}

		level := int(context.Tool.EnchantmentLevel(condition.Enchantment))
		if level >= len(condition.Chances) {
			level = len(condition.Chances) - 1
		}

		return random.Float32() < condition.Chances[level]
	case blockLootConditionToolItem:
		return context.Tool.Item == condition.Item
	case blockLootConditionToolEnchantment:
		level := int(context.Tool.EnchantmentLevel(condition.Enchantment))
		if level < condition.MinLevel {
			return false
		}

		return condition.MaxLevel == 0 || level <= condition.MaxLevel
	default:
		return false
	}
}

func applyBlockLootFunctions(stack ItemStack, functions []blockLootFunction, context BlockLootContext, random BlockLootRandom) ItemStack {
	for _, function := range functions {
		if !blockLootConditionsMatch(function.Conditions, context, random) {
			continue
		}

		switch function.Kind {
		case blockLootFunctionNoop:
		case blockLootFunctionSetCount:
			count := int32(blockLootNumber(function.Count, random))

			if function.Add {
				stack.Count += count
			} else {
				stack.Count = count
			}
		case blockLootFunctionLimitCount:
			minimum := int32(function.Count.Min)
			maximum := int32(function.Count.Max)

			if stack.Count < minimum {
				stack.Count = minimum
			}

			if maximum > 0 && stack.Count > maximum {
				stack.Count = maximum
			}
		case blockLootFunctionApplyBonus:
			stack.Count = applyBlockLootBonus(stack.Count, function, context, random)
		}
	}

	return stack
}

func applyBlockLootBonus(count int32, function blockLootFunction, context BlockLootContext, random BlockLootRandom) int32 {
	level := int(context.Tool.EnchantmentLevel(function.Enchantment))
	if level <= 0 {
		return count
	}

	switch function.Bonus {
	case blockLootBonusOreDrops:
		bonus := random.IntN(level+2) - 1
		bonus = max(bonus, 0)

		return count * int32(bonus+1)
	case blockLootBonusUniformCount:
		bound := int(function.Extra)*level + 1
		return count + int32(random.IntN(bound))
	case blockLootBonusBinomialCount:
		for range level + int(function.Extra) {
			if random.Float32() < function.Probability {
				count++
			}
		}
	}

	return count
}

func blockLootNumber(provider blockLootNumberProvider, random BlockLootRandom) int {
	switch provider.Kind {
	case blockLootNumberConstant:
		return int(provider.Min)
	case blockLootNumberUniform:
		if provider.MinProvider == nil || provider.MaxProvider == nil {
			return 0
		}

		minimum := blockLootNumber(*provider.MinProvider, random)
		maximum := blockLootNumber(*provider.MaxProvider, random)

		if maximum <= minimum {
			return minimum
		}

		return minimum + random.IntN(maximum-minimum+1)
	case blockLootNumberLimit:
		return int(provider.Min)
	default:
		return 0
	}
}
