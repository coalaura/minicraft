package server

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

const maxFillVolume int64 = 32768

var (
	supportedSelectors = []string{"@a", "@p", "@r", "@s"}
	gameModeNames      = []string{"adventure", "creative", "spectator", "survival"}
)

type commandPosition struct {
	position game.Position
}

type commandTimeValue struct {
	ticks int64
}

type commandTargetDistance struct {
	session  *Session
	distance float64
}

type commandTimePreset struct {
	name  string
	ticks int64
}

type commandFillMode struct {
	name string
	mode fillMode
}

type fillMode uint8

const (
	fillModeReplace fillMode = iota
	fillModeKeep
	fillModeOutline
	fillModeHollow
	fillModeStrict
)

func registerBuiltinCommands(registry *commandRegistry) {
	registry.registerHelp()
	registry.registerSeed()
	registry.registerTime()
	registry.registerGameMode()
	registry.registerGive()
	registry.registerEnchant()
	registry.registerClear()
	registry.registerTeleport()
	registry.registerSetBlock()
	registry.registerFill()
}

func (registry *commandRegistry) registerEnchant() {
	targets := registry.targetArgument("targets", true)
	enchantment := enchantmentArgument()
	level := integerArgument("level", 0, math.MaxInt32)

	execute := func(source CommandSource, values []any) error {
		resolved := values[0].([]*Session)
		selected := values[1].(game.Enchantment)

		definition, _ := selected.Definition()

		selectedLevel := int32(1)

		if len(values) > 2 {
			selectedLevel = values[2].(int32)
		}

		if selectedLevel > definition.MaximumLevel {
			return commandFailure{message: game.TranslatableText("commands.enchant.failed.level", integerText(int64(selectedLevel)), integerText(int64(definition.MaximumLevel)))}
		}

		affected := 0

		for _, target := range resolved {
			changed, reason, err := registry.runtime.EnchantHeldItem(target, selected, selectedLevel)
			if err != nil {
				return err
			}

			if changed {
				affected++

				continue
			}

			if len(resolved) != 1 {
				continue
			}

			name := game.LiteralText(target.snapshotPlayer().Name)

			if reason == enchantHeldItemEmpty {
				return commandFailure{message: game.TranslatableText("commands.enchant.failed.itemless", name)}
			}

			player := target.snapshotPlayer()
			item := player.Inventory.Hotbar[player.SelectedHotbarSlot].Item

			return commandFailure{message: game.TranslatableText("commands.enchant.failed.incompatible", itemDisplayName(item))}
		}

		if affected == 0 {
			return commandFailure{message: game.TranslatableText("commands.enchant.failed")}
		}

		enchantmentName := game.TranslatableText("enchantment.minecraft." + definition.Name)

		if len(resolved) == 1 {
			return source.Feedback(game.TranslatableText("commands.enchant.success.single", enchantmentName, game.LiteralText(resolved[0].snapshotPlayer().Name)))
		}

		return source.Feedback(game.TranslatableText("commands.enchant.success.multiple", enchantmentName, integerText(int64(affected))))
	}

	registry.register(&registeredCommand{
		Name: "enchant",
		Patterns: []commandPattern{
			{Elements: []commandElement{targets, enchantment}, Execute: execute},
			{Elements: []commandElement{targets, enchantment, level}, Execute: execute},
		},
	})
}

func (registry *commandRegistry) registerHelp() {
	commandPath := commandArgument{
		name:           "command",
		parser:         protocol.CommandParserString,
		properties:     protocol.CommandStringProperties{Type: 2},
		width:          -1,
		parseValue:     parseString,
		suggestValues:  registry.helpSuggestions,
		clientSuggests: true,
	}

	execute := func(source CommandSource, path string) error {
		lines := registry.helpLines(source, path)
		if len(lines) == 0 {
			return commandFailure{message: game.TranslatableText("commands.help.failed")}
		}

		for _, line := range lines {
			err := source.Feedback(game.LiteralText(line))
			if err != nil {
				return err
			}
		}

		return nil
	}

	registry.register(&registeredCommand{
		Name: "help",
		Patterns: []commandPattern{
			{
				Execute: func(source CommandSource, _ []any) error {
					return execute(source, "")
				},
			},
			{
				Elements: []commandElement{commandPath},
				Execute: func(source CommandSource, values []any) error {
					return execute(source, values[0].(string))
				},
			},
		},
	})
}

func (registry *commandRegistry) registerSeed() {
	registry.register(&registeredCommand{
		Name: "seed",
		Patterns: []commandPattern{{
			Execute: func(source CommandSource, _ []any) error {
				seed := strconv.FormatInt(registry.runtime.World.Seed, 10)

				seedComponent := game.LiteralText(seed).
					WithColor(game.TextColorGreen).
					WithClickEvent(game.ClickCopyToClipboard, seed)

				return source.Feedback(game.TranslatableText("commands.seed.success", seedComponent))
			},
		}},
	})
}

func (registry *commandRegistry) registerTime() {
	timeValue := timeArgument("time", 0)

	query := func(source CommandSource, value int64) error {
		return source.Feedback(game.TranslatableText("commands.time.query", integerText(value)))
	}

	set := func(source CommandSource, ticks int64) error {
		registry.runtime.World.SetDayTime(ticks)

		registry.runtime.broadcastTime(registry.runtime.World.Time())

		return source.Feedback(game.TranslatableText("commands.time.set", integerText(ticks)))
	}

	add := func(source CommandSource, ticks int64) error {
		dayTime := registry.runtime.World.Time().DayTime + ticks

		return set(source, dayTime)
	}

	patterns := []commandPattern{
		{
			Elements: []commandElement{commandLiteral{value: "query"}, commandLiteral{value: "daytime"}},
			Execute: func(source CommandSource, _ []any) error {
				return query(source, floorMod(registry.runtime.World.Time().DayTime, 24000))
			},
		},
		{
			Elements: []commandElement{commandLiteral{value: "query"}, commandLiteral{value: "gametime"}},
			Execute: func(source CommandSource, _ []any) error {
				return query(source, floorMod(registry.runtime.World.Time().Age, math.MaxInt32))
			},
		},
		{
			Elements: []commandElement{commandLiteral{value: "query"}, commandLiteral{value: "day"}},
			Execute: func(source CommandSource, _ []any) error {
				return query(source, floorMod(registry.runtime.World.Time().DayTime/24000, math.MaxInt32))
			},
		},
	}

	presets := []commandTimePreset{
		{name: "day", ticks: 1000},
		{name: "noon", ticks: 6000},
		{name: "night", ticks: 13000},
		{name: "midnight", ticks: 18000},
	}

	for _, preset := range presets {
		patterns = append(patterns, commandPattern{
			Elements: []commandElement{commandLiteral{value: "set"}, commandLiteral{value: preset.name}},
			Execute: func(source CommandSource, _ []any) error {
				return set(source, preset.ticks)
			},
		})
	}

	patterns = append(patterns,
		commandPattern{
			Elements: []commandElement{commandLiteral{value: "set"}, timeValue},
			Execute: func(source CommandSource, values []any) error {
				return set(source, values[1].(commandTimeValue).ticks)
			},
		},
		commandPattern{
			Elements: []commandElement{commandLiteral{value: "add"}, timeValue},
			Execute: func(source CommandSource, values []any) error {
				return add(source, values[1].(commandTimeValue).ticks)
			},
		},
	)

	registry.register(&registeredCommand{Name: "time", Patterns: patterns})
}

func (registry *commandRegistry) registerGameMode() {
	mode := gameModeArgument()
	targets := registry.targetArgument("targets", true)

	execute := func(source CommandSource, values []any) error {
		selectedMode := values[0].(game.GameMode)

		resolved := registry.sourceTargets(source)

		if len(values) > 1 {
			resolved = values[1].([]*Session)
		}

		sourceSession, sourceIsPlayer := source.PlayerSession()

		modeComponent := game.TranslatableText("gameMode." + gameModeName(selectedMode))

		for _, target := range resolved {
			changed, err := registry.runtime.ChangeGameMode(target, selectedMode)
			if err != nil {
				return err
			}

			if !changed {
				continue
			}

			if sourceIsPlayer && sourceSession == target {
				err = source.Feedback(game.TranslatableText("commands.gamemode.success.self", modeComponent))
			} else {
				name := game.LiteralText(target.snapshotPlayer().Name)

				err = source.Feedback(game.TranslatableText("commands.gamemode.success.other", name, modeComponent))
				if err == nil {
					err = target.sendSystemComponent(game.TranslatableText("gameMode.changed", modeComponent))
				}
			}

			if err != nil {
				return err
			}
		}

		return nil
	}

	registry.register(&registeredCommand{
		Name: "gamemode",
		Patterns: []commandPattern{
			{Elements: []commandElement{mode}, Execute: execute},
			{Elements: []commandElement{mode, targets}, Execute: execute},
		},
	})
}

func (registry *commandRegistry) registerGive() {
	targets := registry.targetArgument("targets", true)
	item := itemArgument()
	count := integerArgument("count", 1, math.MaxInt32)

	execute := func(source CommandSource, values []any) error {
		resolved := values[0].([]*Session)
		selectedItem := values[1].(game.Item)

		itemCount := int32(1)

		if len(values) > 2 {
			itemCount = values[2].(int32)
		}

		definition, _ := selectedItem.Definition()
		maximum := definition.StackSize * 100

		itemName := itemDisplayName(selectedItem)

		if itemCount > maximum {
			return commandFailure{message: game.TranslatableText("commands.give.failed.toomanyitems", integerText(int64(maximum)), itemName)}
		}

		err := registry.runtime.GiveItems(resolved, selectedItem, itemCount)
		if err != nil {
			return err
		}

		if len(resolved) == 1 {
			return source.Feedback(game.TranslatableText(
				"commands.give.success.single",
				integerText(int64(itemCount)),
				itemName,
				game.LiteralText(resolved[0].snapshotPlayer().Name),
			))
		}

		return source.Feedback(game.TranslatableText(
			"commands.give.success.multiple",
			integerText(int64(itemCount)),
			itemName,
			integerText(int64(len(resolved))),
		))
	}

	registry.register(&registeredCommand{
		Name: "give",
		Patterns: []commandPattern{
			{Elements: []commandElement{targets, item}, Execute: execute},
			{Elements: []commandElement{targets, item, count}, Execute: execute},
		},
	})
}

func (registry *commandRegistry) registerClear() {
	targets := registry.targetArgument("targets", true)
	item := itemArgument()
	maximum := integerArgument("maxCount", 0, math.MaxInt32)

	execute := func(source CommandSource, values []any) error {
		resolved := registry.sourceTargets(source)
		selectedItem := (*game.Item)(nil)
		maxCount := int32(-1)

		if len(values) > 0 {
			resolved = values[0].([]*Session)
		}

		if len(values) > 1 {
			value := values[1].(game.Item)
			selectedItem = &value
		}

		if len(values) > 2 {
			maxCount = values[2].(int32)
		}

		var affected int32

		for _, target := range resolved {
			count, err := registry.runtime.ClearItems(target, selectedItem, maxCount)
			if err != nil {
				return err
			}

			affected += count
		}

		if affected == 0 {
			if len(resolved) == 1 {
				return commandFailure{message: game.LiteralText("No items were found on player " + resolved[0].snapshotPlayer().Name)}
			}

			return commandFailure{message: game.LiteralText(fmt.Sprintf("No items were found on %d players", len(resolved)))}
		}

		key := "commands.clear.success.single"

		if maxCount == 0 {
			key = "commands.clear.test.single"
		}

		arguments := []game.TextComponent{integerText(int64(affected)), game.LiteralText(resolved[0].snapshotPlayer().Name)}

		if len(resolved) > 1 {
			key = "commands.clear.success.multiple"

			if maxCount == 0 {
				key = "commands.clear.test.multiple"
			}

			arguments[1] = integerText(int64(len(resolved)))
		}

		return source.Feedback(game.TranslatableText(key, arguments...))
	}

	registry.register(&registeredCommand{
		Name: "clear",
		Patterns: []commandPattern{
			{
				Execute: execute,
			},
			{
				Elements: []commandElement{targets},
				Execute:  execute,
			},
			{
				Elements: []commandElement{targets, item},
				Execute:  execute,
			},
			{
				Elements: []commandElement{targets, item, maximum},
				Execute:  execute,
			},
		},
	})
}

func (registry *commandRegistry) registerTeleport() {
	targets := registry.targetArgument("targets", true)
	destination := registry.targetArgument("destination", false)
	position := positionArgument("location", protocol.CommandParserVec3)

	command := &registeredCommand{
		Name: "teleport",
		Patterns: []commandPattern{
			{
				Elements: []commandElement{position},
				Execute: func(source CommandSource, values []any) error {
					return registry.teleportToLocation(source, registry.sourceTargets(source), values[0].(commandPosition).position)
				},
			},
			{
				Elements: []commandElement{destination},
				Execute: func(source CommandSource, values []any) error {
					return registry.teleportToPlayer(source, registry.sourceTargets(source), values[0].([]*Session)[0])
				},
			},
			{
				Elements: []commandElement{targets, destination},
				Execute: func(source CommandSource, values []any) error {
					return registry.teleportToPlayer(source, values[0].([]*Session), values[1].([]*Session)[0])
				},
			},
			{
				Elements: []commandElement{targets, position},
				Execute: func(source CommandSource, values []any) error {
					return registry.teleportToLocation(source, values[0].([]*Session), values[1].(commandPosition).position)
				},
			},
		},
	}

	registry.register(command)
	registry.registerRedirect("tp", command)
}

func (registry *commandRegistry) registerSetBlock() {
	position := positionArgument("position", protocol.CommandParserBlockPosition)
	block := blockArgument()

	execute := func(source CommandSource, values []any, keep, strict bool) error {
		blockPosition := toBlockPosition(values[0].(commandPosition).position)
		replacement := values[1].(game.Block)

		change := []game.BlockChange{{Position: blockPosition, Replacement: replacement}}

		var result BlockMutationResult

		var err error

		if keep {
			result, err = registry.runtime.MutateEmptyWorldBlocks(change)
		} else if strict {
			result, err = registry.runtime.MutateWorldBlocksStrict(change)
		} else {
			result, err = registry.runtime.MutateWorldBlocks(change)
		}

		if err != nil {
			return err
		}

		if len(result.Changes) == 0 {
			return commandFailure{message: game.TranslatableText("commands.setblock.failed")}
		}

		return source.Feedback(game.TranslatableText(
			"commands.setblock.success",
			integerText(int64(blockPosition.X)),
			integerText(int64(blockPosition.Y)),
			integerText(int64(blockPosition.Z)),
		))
	}

	registry.register(&registeredCommand{
		Name: "setblock",
		Patterns: []commandPattern{
			{
				Elements: []commandElement{position, block},
				Execute: func(source CommandSource, values []any) error {
					return execute(source, values, false, false)
				},
			},
			{
				Elements: []commandElement{position, block, commandLiteral{value: "replace"}},
				Execute: func(source CommandSource, values []any) error {
					return execute(source, values, false, false)
				},
			},
			{
				Elements: []commandElement{position, block, commandLiteral{value: "keep"}},
				Execute: func(source CommandSource, values []any) error {
					return execute(source, values, true, false)
				},
			},
			{
				Elements: []commandElement{position, block, commandLiteral{value: "strict"}},
				Execute: func(source CommandSource, values []any) error {
					return execute(source, values, false, true)
				},
			},
		},
	})
}

func (registry *commandRegistry) registerFill() {
	first := positionArgument("from", protocol.CommandParserBlockPosition)
	second := positionArgument("to", protocol.CommandParserBlockPosition)
	block := blockArgument()

	execute := func(source CommandSource, values []any, mode fillMode) error {
		from := toBlockPosition(values[0].(commandPosition).position)
		to := toBlockPosition(values[1].(commandPosition).position)

		minimum, maximum := orderedBlockPositions(from, to)

		xSize := int64(maximum.X) - int64(minimum.X) + 1
		ySize := int64(maximum.Y) - int64(minimum.Y) + 1
		zSize := int64(maximum.Z) - int64(minimum.Z) + 1

		volume := xSize * ySize * zSize

		if xSize > maxFillVolume || ySize > maxFillVolume || zSize > maxFillVolume || volume > maxFillVolume {
			return commandFailure{message: game.TranslatableText(
				"commands.fill.toobig",
				integerText(maxFillVolume),
				integerText(volume),
			)}
		}

		replacement := values[2].(game.Block)

		changes := make([]game.BlockChange, 0, int(volume))

		for x := minimum.X; x <= maximum.X; x++ {
			for y := minimum.Y; y <= maximum.Y; y++ {
				for z := minimum.Z; z <= maximum.Z; z++ {
					position := game.BlockPosition{X: x, Y: y, Z: z}

					boundary := x == minimum.X || x == maximum.X || y == minimum.Y || y == maximum.Y || z == minimum.Z || z == maximum.Z

					current := registry.runtime.World.BlockAt(position)

					switch mode {
					case fillModeKeep:
					case fillModeOutline:
						if !boundary {
							continue
						}
					case fillModeHollow:
						if !boundary {
							replacement = game.Air
						} else {
							replacement = values[2].(game.Block)
						}
					}

					if mode == fillModeKeep || current != replacement {
						changes = append(changes, game.BlockChange{Position: position, Replacement: replacement})
					}
				}
			}
		}

		var result BlockMutationResult

		var err error

		switch mode {
		case fillModeKeep:
			result, err = registry.runtime.MutateEmptyWorldBlocks(changes)
		case fillModeStrict:
			result, err = registry.runtime.MutateWorldBlocksStrict(changes)
		default:
			result, err = registry.runtime.MutateWorldBlocks(changes)
		}

		if err != nil {
			return err
		}

		if len(result.Changes) == 0 {
			return commandFailure{message: game.TranslatableText("commands.fill.failed")}
		}

		return source.Feedback(game.TranslatableText("commands.fill.success", integerText(int64(len(result.Changes)))))
	}

	patterns := []commandPattern{{
		Elements: []commandElement{first, second, block},
		Execute: func(source CommandSource, values []any) error {
			return execute(source, values, fillModeReplace)
		},
	}}

	modes := []commandFillMode{
		{name: "replace", mode: fillModeReplace},
		{name: "keep", mode: fillModeKeep},
		{name: "outline", mode: fillModeOutline},
		{name: "hollow", mode: fillModeHollow},
		{name: "strict", mode: fillModeStrict},
	}

	for _, mode := range modes {
		patterns = append(patterns, commandPattern{
			Elements: []commandElement{first, second, block, commandLiteral{value: mode.name}},
			Execute: func(source CommandSource, values []any) error {
				return execute(source, values, mode.mode)
			},
		})
	}

	registry.register(&registeredCommand{Name: "fill", Patterns: patterns})
}

func (registry *commandRegistry) teleportToPlayer(source CommandSource, targets []*Session, destination *Session) error {
	player := destination.snapshotPlayer()

	for _, target := range targets {
		err := registry.runtime.TeleportPlayer(target, player.Position, &player.Rotation)
		if err != nil {
			return err
		}
	}

	if len(targets) == 1 {
		return source.Feedback(game.TranslatableText(
			"commands.teleport.success.entity.single",
			game.LiteralText(targets[0].snapshotPlayer().Name),
			game.LiteralText(player.Name),
		))
	}

	return source.Feedback(game.TranslatableText(
		"commands.teleport.success.entity.multiple",
		integerText(int64(len(targets))),
		game.LiteralText(player.Name),
	))
}

func (registry *commandRegistry) teleportToLocation(source CommandSource, targets []*Session, position game.Position) error {
	for _, target := range targets {
		err := registry.runtime.TeleportPlayer(target, position, nil)
		if err != nil {
			return err
		}
	}

	arguments := []game.TextComponent{
		game.LiteralText(targets[0].snapshotPlayer().Name),
		game.LiteralText(formatCoordinate(position.X)),
		game.LiteralText(formatCoordinate(position.Y)),
		game.LiteralText(formatCoordinate(position.Z)),
	}

	key := "commands.teleport.success.location.single"

	if len(targets) > 1 {
		key = "commands.teleport.success.location.multiple"
		arguments[0] = integerText(int64(len(targets)))
	}

	return source.Feedback(game.TranslatableText(key, arguments...))
}

func (registry *commandRegistry) helpLines(source CommandSource, path string) []string {
	parts := strings.Fields(path)
	if len(parts) == 0 {
		var lines []string

		for _, command := range registry.commands {
			if !source.HasPermission(command.Permission) {
				continue
			}

			lines = append(lines, commandUsageLines(command.Name, command)...)
		}

		return lines
	}

	command := registry.byName[parts[0]]
	if command == nil || !source.HasPermission(command.Permission) {
		return nil
	}

	displayName := command.Name

	if command.Redirect != nil {
		command = command.Redirect
	}

	var lines []string

	for _, pattern := range command.Patterns {
		if !helpPathMatches(pattern, parts[1:]) {
			continue
		}

		lines = append(lines, patternUsage(displayName, pattern))
	}

	return lines
}

func commandUsageLines(name string, command *registeredCommand) []string {
	if command.Redirect != nil {
		command = command.Redirect
	}

	lines := make([]string, len(command.Patterns))

	for index, pattern := range command.Patterns {
		lines[index] = patternUsage(name, pattern)
	}

	return lines
}

func helpPathMatches(pattern commandPattern, path []string) bool {
	if len(path) > len(pattern.Elements) {
		return false
	}

	for index, value := range path {
		literal, ok := pattern.Elements[index].(commandLiteral)
		if !ok || literal.value != value {
			return false
		}
	}

	return true
}

func patternUsage(name string, pattern commandPattern) string {
	parts := make([]string, 1, len(pattern.Elements)+1)

	parts[0] = "/" + name

	for _, element := range pattern.Elements {
		parts = append(parts, element.usage())
	}

	return strings.Join(parts, " ")
}

func (registry *commandRegistry) helpSuggestions(CommandSource) []string {
	var values []string

	for _, command := range registry.commands {
		values = append(values, command.Name)

		if command.Redirect != nil {
			command = command.Redirect
		}

		for _, pattern := range command.Patterns {
			parts := []string{command.Name}

			for _, element := range pattern.Elements {
				literal, ok := element.(commandLiteral)
				if !ok {
					break
				}

				parts = append(parts, literal.value)
				values = append(values, strings.Join(parts, " "))
			}
		}
	}

	return values
}

func (registry *commandRegistry) sourceTargets(source CommandSource) []*Session {
	session, player := source.PlayerSession()
	if !player {
		return nil
	}

	return []*Session{session}
}

func (registry *commandRegistry) targetArgument(name string, multiple bool) commandArgument {
	return commandArgument{
		name:       name,
		parser:     protocol.CommandParserEntity,
		properties: protocol.CommandEntityProperties{SingleTarget: !multiple, OnlyPlayers: true},
		width:      1,
		parseValue: func(source CommandSource, tokens []commandToken) (any, error) {
			return registry.resolveTargets(source, tokens[0], multiple)
		},
		suggestValues: registry.targetSuggestions,
	}
}

func (registry *commandRegistry) resolveTargets(source CommandSource, token commandToken, multiple bool) ([]*Session, error) {
	value := token.value

	sessions := registry.sortedSessions()

	var targets []*Session

	if strings.HasPrefix(value, "@") {
		if strings.ContainsAny(value, "[]") {
			return nil, commandSyntaxError{message: game.LiteralText("Selector options are not supported"), cursor: token.start}
		}

		switch value {
		case "@a":
			targets = sessions
		case "@s":
			targets = registry.sourceTargets(source)
		case "@p":
			if len(sessions) != 0 {
				sourcePosition := source.Position()
				distances := make([]commandTargetDistance, len(sessions))

				for index, session := range sessions {
					distances[index] = commandTargetDistance{
						session:  session,
						distance: positionDistanceSquared(sourcePosition, session.snapshotPlayer().Position),
					}
				}

				sort.SliceStable(distances, func(first, second int) bool {
					return distances[first].distance < distances[second].distance
				})

				targets = []*Session{distances[0].session}
			}
		case "@r":
			if len(sessions) != 0 {
				registry.runtime.commandRandomMu.Lock()
				index := registry.runtime.commandRandom(len(sessions))
				registry.runtime.commandRandomMu.Unlock()

				targets = sessions[index : index+1]
			}
		case "@e":
			return nil, commandSyntaxError{message: game.TranslatableText("argument.player.entities"), cursor: token.start}
		default:
			return nil, commandSyntaxError{message: game.TranslatableText("argument.entity.invalid"), cursor: token.start}
		}

		if len(targets) == 0 {
			return nil, commandSyntaxError{message: game.TranslatableText("argument.entity.notfound.player"), cursor: token.start}
		}
	} else {
		if !validPlayerName(value) {
			return nil, commandSyntaxError{message: game.TranslatableText("argument.entity.invalid"), cursor: token.start}
		}

		for _, session := range sessions {
			if strings.EqualFold(session.snapshotPlayer().Name, value) {
				targets = []*Session{session}

				break
			}
		}

		if len(targets) == 0 {
			return nil, commandSyntaxError{message: game.TranslatableText("argument.player.unknown"), cursor: token.start}
		}
	}

	if !multiple && len(targets) != 1 {
		return nil, commandSyntaxError{message: game.TranslatableText("argument.player.toomany"), cursor: token.start}
	}

	return targets, nil
}

func (registry *commandRegistry) sortedSessions() []*Session {
	sessions := registry.runtime.snapshotSessions()

	sort.Slice(sessions, func(first, second int) bool {
		firstPlayer := sessions[first].snapshotPlayer()
		secondPlayer := sessions[second].snapshotPlayer()

		firstName := strings.ToLower(firstPlayer.Name)
		secondName := strings.ToLower(secondPlayer.Name)

		if firstName == secondName {
			return firstPlayer.UUID < secondPlayer.UUID
		}

		return firstName < secondName
	})

	return sessions
}

func (registry *commandRegistry) targetSuggestions(CommandSource) []string {
	values := append([]string(nil), supportedSelectors...)

	for _, session := range registry.sortedSessions() {
		values = append(values, session.snapshotPlayer().Name)
	}

	return values
}

func validPlayerName(name string) bool {
	if len(name) < 1 || len(name) > 16 {
		return false
	}

	for _, character := range name {
		if character != '_' && (character < 'a' || character > 'z') && (character < 'A' || character > 'Z') && (character < '0' || character > '9') {
			return false
		}
	}

	return true
}

func positionDistanceSquared(first, second game.Position) float64 {
	x := first.X - second.X
	y := first.Y - second.Y
	z := first.Z - second.Z

	return x*x + y*y + z*z
}

func gameModeArgument() commandArgument {
	return commandArgument{
		name:          "gamemode",
		parser:        protocol.CommandParserGameMode,
		width:         1,
		suggestValues: staticSuggestions(gameModeNames),
		parseValue: func(_ CommandSource, tokens []commandToken) (any, error) {
			mode, err := game.ParseGameMode(tokens[0].value)
			if err != nil {
				return nil, commandSyntaxError{
					message: game.TranslatableText("argument.gamemode.invalid", game.LiteralText(tokens[0].value)),
					cursor:  tokens[0].start,
				}
			}

			return mode, nil
		},
	}
}

func itemArgument() commandArgument {
	return commandArgument{
		name:       "item",
		parser:     protocol.CommandParserResource,
		properties: protocol.CommandResourceProperties{Registry: "minecraft:item"},
		width:      1,
		parseValue: func(_ CommandSource, tokens []commandToken) (any, error) {
			item, valid := game.ItemByName(tokens[0].value)
			if !valid || item == game.ItemAir {
				return nil, commandSyntaxError{
					message: game.LiteralText("Unknown item '" + tokens[0].value + "'"),
					cursor:  tokens[0].start,
				}
			}

			return item, nil
		},
	}
}

func enchantmentArgument() commandArgument {
	return commandArgument{
		name:       "enchantment",
		parser:     protocol.CommandParserResource,
		properties: protocol.CommandResourceProperties{Registry: "minecraft:enchantment"},
		width:      1,
		parseValue: func(_ CommandSource, tokens []commandToken) (any, error) {
			enchantment, valid := game.EnchantmentByName(tokens[0].value)
			if !valid {
				return nil, commandSyntaxError{
					message: game.LiteralText("Unknown enchantment '" + tokens[0].value + "'"),
					cursor:  tokens[0].start,
				}
			}

			return enchantment, nil
		},
	}
}

func blockArgument() commandArgument {
	return commandArgument{
		name:       "block",
		parser:     protocol.CommandParserResource,
		properties: protocol.CommandResourceProperties{Registry: "minecraft:block"},
		width:      1,
		parseValue: func(_ CommandSource, tokens []commandToken) (any, error) {
			block, valid := game.BlockByName(tokens[0].value)
			if !valid {
				return nil, commandSyntaxError{
					message: game.LiteralText("Unknown block '" + tokens[0].value + "'"),
					cursor:  tokens[0].start,
				}
			}

			return block, nil
		},
	}
}

func integerArgument(name string, minimum, maximum int32) commandArgument {
	return commandArgument{
		name:       name,
		parser:     protocol.CommandParserInteger,
		properties: protocol.CommandIntegerProperties{HasMin: true, Min: minimum, HasMax: maximum < math.MaxInt32, Max: maximum},
		width:      1,
		parseValue: func(source CommandSource, tokens []commandToken) (any, error) {
			value, err := parseInteger(source, tokens)
			if err != nil {
				return nil, err
			}

			integer := value.(int32)
			if integer < minimum {
				return nil, commandSyntaxError{
					message: game.TranslatableText("argument.integer.low", integerText(int64(integer)), integerText(int64(minimum))),
					cursor:  tokens[0].start,
				}
			}

			if integer > maximum {
				return nil, commandSyntaxError{
					message: game.TranslatableText("argument.integer.big", integerText(int64(integer)), integerText(int64(maximum))),
					cursor:  tokens[0].start,
				}
			}

			return integer, nil
		},
	}
}

func timeArgument(name string, minimum int32) commandArgument {
	return commandArgument{
		name:       name,
		parser:     protocol.CommandParserTime,
		properties: protocol.CommandTimeProperties{Min: minimum},
		width:      1,
		parseValue: func(_ CommandSource, tokens []commandToken) (any, error) {
			return parseTimeValue(tokens[0], minimum)
		},
	}
}

func positionArgument(name string, parser int32) commandArgument {
	return commandArgument{
		name:   name,
		parser: parser,
		width:  3,
		parseValue: func(source CommandSource, tokens []commandToken) (any, error) {
			base := source.Position()

			x, err := parseCoordinate(tokens[0], base.X)
			if err != nil {
				return nil, err
			}

			y, err := parseCoordinate(tokens[1], base.Y)
			if err != nil {
				return nil, err
			}

			z, err := parseCoordinate(tokens[2], base.Z)
			if err != nil {
				return nil, err
			}

			if !validPlayerPosition(x, y, z) {
				return nil, commandSyntaxError{message: game.TranslatableText("argument.pos.outofworld"), cursor: tokens[0].start}
			}

			return commandPosition{position: game.Position{X: x, Y: y, Z: z}}, nil
		},
	}
}

func parseCoordinate(token commandToken, base float64) (float64, error) {
	value := token.value
	if strings.HasPrefix(value, "^") {
		return 0, commandSyntaxError{message: game.TranslatableText("argument.pos.mixed"), cursor: token.start}
	}

	relative := strings.HasPrefix(value, "~")
	if relative {
		value = value[1:]
		if value == "" {
			return base, nil
		}
	}

	coordinate, err := strconv.ParseFloat(value, 64)
	if err != nil || math.IsNaN(coordinate) || math.IsInf(coordinate, 0) {
		return 0, commandSyntaxError{
			message: game.TranslatableText("argument.double.invalid", game.LiteralText(value)),
			cursor:  token.start,
		}
	}

	if relative {
		coordinate += base
	}

	return coordinate, nil
}

func parseTimeValue(token commandToken, minimum int32) (any, error) {
	value := token.value
	unit := byte(0)

	if len(value) != 0 {
		last := value[len(value)-1]
		if last < '0' || last > '9' && last != '.' {
			unit = last
			value = value[:len(value)-1]
		}
	}

	multiplier := float64(1)

	switch unit {
	case 0, 't':
	case 's':
		multiplier = 20
	case 'd':
		multiplier = 24000
	default:
		return nil, commandSyntaxError{message: game.TranslatableText("argument.time.invalid_unit"), cursor: token.end - 1}
	}

	number, err := strconv.ParseFloat(value, 32)
	if err != nil || math.IsNaN(number) || math.IsInf(number, 0) {
		return nil, commandSyntaxError{
			message: game.TranslatableText("argument.float.invalid", game.LiteralText(value)),
			cursor:  token.start,
		}
	}

	ticks := int64(math.Floor(number*multiplier + 0.5))
	if ticks < int64(minimum) {
		return nil, commandSyntaxError{
			message: game.TranslatableText("argument.time.tick_count_too_low", integerText(int64(minimum)), integerText(ticks)),
			cursor:  token.start,
		}
	}

	if ticks > math.MaxInt32 {
		ticks = math.MaxInt32
	}

	return commandTimeValue{ticks: ticks}, nil
}

func staticSuggestions(values []string) func(CommandSource) []string {
	return func(CommandSource) []string {
		return values
	}
}

func integerText(value int64) game.TextComponent {
	return game.LiteralText(strconv.FormatInt(value, 10))
}

func itemDisplayName(item game.Item) game.TextComponent {
	definition, _ := item.Definition()

	translationType := "item"

	_, block := item.PlacementBlock()

	if block {
		translationType = "block"
	}

	return game.TranslatableText(translationType + ".minecraft." + definition.Name)
}

func gameModeName(mode game.GameMode) string {
	switch mode {
	case game.GameModeCreative:
		return "creative"
	case game.GameModeAdventure:
		return "adventure"
	case game.GameModeSpectator:
		return "spectator"
	default:
		return "survival"
	}
}

func floorMod(value, modulus int64) int64 {
	result := value % modulus
	if result < 0 {
		result += modulus
	}

	return result
}

func formatCoordinate(value float64) string {
	return strconv.FormatFloat(value, 'f', 6, 64)
}

func toBlockPosition(position game.Position) game.BlockPosition {
	return game.BlockPosition{X: int32(math.Floor(position.X)), Y: int32(math.Floor(position.Y)), Z: int32(math.Floor(position.Z))}
}

func orderedBlockPositions(first, second game.BlockPosition) (game.BlockPosition, game.BlockPosition) {
	minimum := game.BlockPosition{X: min(first.X, second.X), Y: min(first.Y, second.Y), Z: min(first.Z, second.Z)}
	maximum := game.BlockPosition{X: max(first.X, second.X), Y: max(first.Y, second.Y), Z: max(first.Z, second.Z)}

	return minimum, maximum
}
