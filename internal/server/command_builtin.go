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
	timePresetNames    = []string{"day", "midnight", "night", "noon"}
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

func registerBuiltinCommands(registry *commandRegistry) {
	registry.registerHelp()
	registry.registerSeed()
	registry.registerTime()
	registry.registerGameMode()
	registry.registerGive()
	registry.registerClear()
	registry.registerTeleport()
	registry.registerSetBlock()
	registry.registerFill()
}

func (registry *commandRegistry) registerHelp() {
	commandName := stringArgument("command", registry.commandNames)

	registry.register(&registeredCommand{
		Name:        "help",
		Usage:       "/help [command]",
		Description: "Shows available commands or help for one command.",
		Patterns: []commandPattern{
			{Execute: func(source CommandSource, _ []any) error {
				lines := make([]string, 0, len(registry.commands))

				for _, command := range registry.commands {
					if source.HasPermission(command.Permission) {
						lines = append(lines, fmt.Sprintf("%s - %s", command.Usage, command.Description))
					}
				}

				return source.Feedback(strings.Join(lines, "\n"))
			}},
			{Elements: []commandElement{commandName}, Execute: func(source CommandSource, values []any) error {
				name := strings.ToLower(values[0].(string))

				command := registry.byName[name]
				if command == nil || !source.HasPermission(command.Permission) {
					return commandSyntaxError{message: fmt.Sprintf("Unknown command %q", name)}
				}

				return source.Feedback(fmt.Sprintf("%s - %s", command.Usage, command.Description))
			}},
		},
	})
}

func (registry *commandRegistry) registerSeed() {
	registry.register(&registeredCommand{
		Name:        "seed",
		Usage:       "/seed",
		Description: "Shows the world seed.",
		Patterns: []commandPattern{{Execute: func(source CommandSource, _ []any) error {
			return source.Feedback(fmt.Sprintf("Seed: %d", registry.runtime.World.Seed))
		}}},
	})
}

func (registry *commandRegistry) registerTime() {
	timeValue := commandArgument{
		name:           "time",
		parser:         protocol.CommandParserString,
		properties:     protocol.CommandStringProperties{Type: 0},
		width:          1,
		parseValue:     parseTimeValue,
		suggestValues:  staticSuggestions(timePresetNames),
		clientSuggests: true,
	}

	registry.register(&registeredCommand{
		Name:        "time",
		Usage:       "/time query | /time set <day|noon|night|midnight|ticks>",
		Description: "Queries or sets the world time.",
		Patterns: []commandPattern{
			{Elements: []commandElement{commandLiteral{value: "query"}}, Execute: func(source CommandSource, _ []any) error {
				return source.Feedback(fmt.Sprintf("Time: %d", registry.runtime.World.Time().DayTime))
			}},
			{Elements: []commandElement{commandLiteral{value: "set"}, timeValue}, Execute: func(source CommandSource, values []any) error {
				state := registry.runtime.World.Time()

				registry.runtime.World.SetTime(values[1].(commandTimeValue).ticks, state.DayCycle)

				state = registry.runtime.World.Time()

				registry.runtime.broadcastTime(state)

				return source.Feedback(fmt.Sprintf("Set time to %d", state.DayTime))
			}},
		},
	})
}

func (registry *commandRegistry) registerGameMode() {
	mode := gameModeArgument()

	targets := registry.targetArgument("targets", true)

	execute := func(source CommandSource, values []any) error {
		gameMode := values[0].(game.GameMode)

		resolved := registry.sourceTargets(source)
		if len(values) > 1 {
			resolved = values[1].([]*Session)
		}

		for _, target := range resolved {
			err := registry.runtime.ChangeGameMode(target, gameMode)
			if err != nil {
				return err
			}
		}

		return source.Feedback(fmt.Sprintf("Set game mode for %d player(s)", len(resolved)))
	}

	registry.register(&registeredCommand{
		Name:        "gamemode",
		Usage:       "/gamemode <survival|creative|adventure|spectator> [targets]",
		Description: "Changes game mode for one or more players.",
		Patterns: []commandPattern{
			{Elements: []commandElement{mode}, Execute: execute},
			{Elements: []commandElement{mode, targets}, Execute: execute},
		},
	})
}

func (registry *commandRegistry) registerGive() {
	targets := registry.targetArgument("targets", true)

	item := itemArgument()

	count := integerArgument("count", 1, 32767)

	execute := func(source CommandSource, values []any) error {
		resolved := values[0].([]*Session)

		selectedItem := values[1].(game.Item)

		itemCount := int32(1)
		if len(values) > 2 {
			itemCount = values[2].(int32)
		}

		for _, target := range resolved {
			err := registry.runtime.GiveItem(target, selectedItem, itemCount)
			if err != nil {
				return fmt.Errorf("give to %s: %w", target.snapshotPlayer().Name, err)
			}
		}

		return source.Feedback(fmt.Sprintf("Gave %d item(s) to %d player(s)", itemCount, len(resolved)))
	}

	registry.register(&registeredCommand{
		Name:        "give",
		Usage:       "/give <targets> <item> [count]",
		Description: "Adds items when every requested stack fits.",
		Patterns: []commandPattern{
			{Elements: []commandElement{targets, item}, Execute: execute},
			{Elements: []commandElement{targets, item, count}, Execute: execute},
		},
	})
}

func (registry *commandRegistry) registerClear() {
	targets := registry.targetArgument("targets", true)

	item := itemArgument()

	execute := func(source CommandSource, values []any) error {
		resolved := registry.sourceTargets(source)

		var selectedItem *game.Item

		if len(values) > 0 {
			resolved = values[0].([]*Session)
		}

		if len(values) > 1 {
			value := values[1].(game.Item)

			selectedItem = &value
		}

		var removed int32

		for _, target := range resolved {
			count, err := registry.runtime.ClearItems(target, selectedItem)
			if err != nil {
				return err
			}

			removed += count
		}

		return source.Feedback(fmt.Sprintf("Cleared %d item(s) from %d player(s)", removed, len(resolved)))
	}

	registry.register(&registeredCommand{
		Name:        "clear",
		Usage:       "/clear [targets] [item]",
		Description: "Clears all or matching inventory items.",
		Patterns: []commandPattern{
			{Execute: execute},
			{Elements: []commandElement{targets}, Execute: execute},
			{Elements: []commandElement{targets, item}, Execute: execute},
		},
	})
}

func (registry *commandRegistry) registerTeleport() {
	targets := registry.targetArgument("targets", true)
	destination := registry.targetArgument("destination", false)

	targets.declarationKey = "teleport-player"
	destination.declarationKey = "teleport-player"

	position := positionArgument("location", protocol.CommandParserVec3)

	registry.register(&registeredCommand{
		Name:        "tp",
		Usage:       "/tp <destination|x y z> | /tp <targets> <destination|x y z>",
		Description: "Teleports players to a player or coordinates.",
		Patterns: []commandPattern{
			{Elements: []commandElement{position}, Execute: func(source CommandSource, values []any) error {
				return registry.teleport(source, registry.sourceTargets(source), values[0].(commandPosition).position)
			}},
			{Elements: []commandElement{destination}, Execute: func(source CommandSource, values []any) error {
				player := values[0].([]*Session)[0].snapshotPlayer()
				return registry.teleport(source, registry.sourceTargets(source), player.Position)
			}},
			{Elements: []commandElement{targets, destination}, Execute: func(source CommandSource, values []any) error {
				player := values[1].([]*Session)[0].snapshotPlayer()
				return registry.teleport(source, values[0].([]*Session), player.Position)
			}},
			{Elements: []commandElement{targets, position}, Execute: func(source CommandSource, values []any) error {
				return registry.teleport(source, values[0].([]*Session), values[1].(commandPosition).position)
			}},
		},
	})
}

func (registry *commandRegistry) registerSetBlock() {
	position := positionArgument("position", protocol.CommandParserBlockPosition)

	block := blockArgument()

	registry.register(&registeredCommand{
		Name:        "setblock",
		Usage:       "/setblock <x> <y> <z> <block>",
		Description: "Sets one block through authoritative world mutation.",
		Patterns: []commandPattern{{Elements: []commandElement{position, block}, Execute: func(source CommandSource, values []any) error {
			blockPosition := toBlockPosition(values[0].(commandPosition).position)
			result, err := registry.runtime.MutateWorldBlocks([]game.BlockChange{{Position: blockPosition, Replacement: values[1].(game.Block)}})
			if err != nil {
				return err
			}

			return source.Feedback(fmt.Sprintf("Changed %d block(s)", len(result.Changes)))
		}}},
	})
}

func (registry *commandRegistry) registerFill() {
	first := positionArgument("from", protocol.CommandParserBlockPosition)
	second := positionArgument("to", protocol.CommandParserBlockPosition)

	block := blockArgument()

	registry.register(&registeredCommand{
		Name:        "fill",
		Usage:       "/fill <x1> <y1> <z1> <x2> <y2> <z2> <block>",
		Description: "Atomically fills up to 32768 blocks.",
		Patterns: []commandPattern{{Elements: []commandElement{first, second, block}, Execute: func(source CommandSource, values []any) error {
			from := toBlockPosition(values[0].(commandPosition).position)
			to := toBlockPosition(values[1].(commandPosition).position)

			minimum, maximum := orderedBlockPositions(from, to)

			xSize := int64(maximum.X) - int64(minimum.X) + 1
			ySize := int64(maximum.Y) - int64(minimum.Y) + 1
			zSize := int64(maximum.Z) - int64(minimum.Z) + 1

			if xSize > maxFillVolume || ySize > maxFillVolume || zSize > maxFillVolume {
				return commandSyntaxError{message: fmt.Sprintf("Fill volume exceeds %d blocks", maxFillVolume)}
			}

			volume := xSize * ySize * zSize
			if volume > maxFillVolume {
				return commandSyntaxError{message: fmt.Sprintf("Fill volume %d exceeds %d blocks", volume, maxFillVolume)}
			}

			changes := make([]game.BlockChange, 0, int(volume))

			for x := minimum.X; x <= maximum.X; x++ {
				for y := minimum.Y; y <= maximum.Y; y++ {
					for z := minimum.Z; z <= maximum.Z; z++ {
						changes = append(changes, game.BlockChange{Position: game.BlockPosition{X: x, Y: y, Z: z}, Replacement: values[2].(game.Block)})
					}
				}
			}

			result, err := registry.runtime.MutateWorldBlocks(changes)
			if err != nil {
				return err
			}

			return source.Feedback(fmt.Sprintf("Changed %d block(s)", len(result.Changes)))
		}}},
	})
}

func (registry *commandRegistry) teleport(source CommandSource, targets []*Session, position game.Position) error {
	for _, target := range targets {
		if err := registry.runtime.TeleportPlayer(target, position); err != nil {
			return err
		}
	}

	return source.Feedback(fmt.Sprintf("Teleported %d player(s)", len(targets)))
}

func (registry *commandRegistry) commandNames(CommandSource) []string {
	names := make([]string, 0, len(registry.commands))

	for _, command := range registry.commands {
		names = append(names, command.Name)
	}

	return names
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
		name:           name,
		parser:         protocol.CommandParserEntity,
		properties:     protocol.CommandEntityProperties{OnlyEntities: !multiple, OnlyPlayers: true},
		width:          1,
		clientSuggests: true,
		parseValue: func(source CommandSource, tokens []string) (any, error) {
			return registry.resolveTargets(source, tokens[0], multiple)
		},
		suggestValues: registry.targetSuggestions,
	}
}

func (registry *commandRegistry) resolveTargets(source CommandSource, value string, multiple bool) ([]*Session, error) {
	sessions := registry.sortedSessions()

	var targets []*Session

	if strings.HasPrefix(value, "@") {
		if strings.ContainsAny(value, "[]") {
			return nil, commandSyntaxError{message: "Selector options are not supported"}
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
		default:
			return nil, commandSyntaxError{message: fmt.Sprintf("Unsupported selector %q", value)}
		}
	} else {
		if !validPlayerName(value) {
			return nil, commandSyntaxError{message: fmt.Sprintf("Invalid player name %q", value)}
		}

		for _, session := range sessions {
			if strings.EqualFold(session.snapshotPlayer().Name, value) {
				targets = []*Session{session}

				break
			}
		}
	}

	if len(targets) == 0 {
		return nil, commandSyntaxError{message: fmt.Sprintf("No player matched %q", value)}
	}

	if !multiple && len(targets) != 1 {
		return nil, commandSyntaxError{message: "Destination must resolve to exactly one player"}
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
		name:           "mode",
		parser:         protocol.CommandParserGameMode,
		width:          1,
		clientSuggests: true,
		suggestValues:  staticSuggestions(gameModeNames),
		parseValue: func(_ CommandSource, tokens []string) (any, error) {
			mode, err := game.ParseGameMode(strings.ToLower(tokens[0]))
			if err != nil {
				return nil, commandSyntaxError{message: err.Error()}
			}

			return mode, nil
		},
	}
}

func itemArgument() commandArgument {
	return commandArgument{
		name:           "item",
		parser:         protocol.CommandParserItemStack,
		width:          1,
		clientSuggests: true,
		suggestValues:  staticSuggestions(game.ItemNames),
		parseValue: func(_ CommandSource, tokens []string) (any, error) {
			item, valid := game.ItemByName(tokens[0])
			if !valid || item == game.ItemAir {
				return nil, commandSyntaxError{message: fmt.Sprintf("Unknown item %q", tokens[0])}
			}

			return item, nil
		},
	}
}

func blockArgument() commandArgument {
	return commandArgument{
		name:           "block",
		parser:         protocol.CommandParserBlockState,
		width:          1,
		clientSuggests: true,
		suggestValues:  staticSuggestions(game.BlockNames),
		parseValue: func(_ CommandSource, tokens []string) (any, error) {
			block, valid := game.BlockByName(tokens[0])
			if !valid {
				return nil, commandSyntaxError{message: fmt.Sprintf("Unknown block %q", tokens[0])}
			}

			return block, nil
		},
	}
}

func integerArgument(name string, minimum, maximum int32) commandArgument {
	return commandArgument{
		name:       name,
		parser:     protocol.CommandParserInteger,
		properties: protocol.CommandIntegerProperties{HasMin: true, Min: minimum, HasMax: true, Max: maximum},
		width:      1,
		parseValue: func(source CommandSource, tokens []string) (any, error) {
			value, err := parseInteger(source, tokens)
			if err != nil {
				return nil, err
			}

			integer := value.(int32)
			if integer < minimum || integer > maximum {
				return nil, commandSyntaxError{message: fmt.Sprintf("%s must be between %d and %d", name, minimum, maximum)}
			}

			return integer, nil
		},
	}
}

func stringArgument(name string, suggestions func(CommandSource) []string) commandArgument {
	return commandArgument{
		name:           name,
		parser:         protocol.CommandParserString,
		properties:     protocol.CommandStringProperties{Type: 0},
		width:          1,
		parseValue:     parseString,
		suggestValues:  suggestions,
		clientSuggests: suggestions != nil,
	}
}

func positionArgument(name string, parser int32) commandArgument {
	return commandArgument{
		name:   name,
		parser: parser,
		width:  3,
		parseValue: func(source CommandSource, tokens []string) (any, error) {
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
				return nil, commandSyntaxError{message: "Coordinates are outside the valid world range"}
			}

			return commandPosition{position: game.Position{X: x, Y: y, Z: z}}, nil
		},
	}
}

func parseCoordinate(value string, base float64) (float64, error) {
	if strings.HasPrefix(value, "^") {
		return 0, commandSyntaxError{message: "Local (^) coordinates are not supported"}
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
		return 0, commandSyntaxError{message: fmt.Sprintf("Invalid coordinate %q", value)}
	}

	if relative {
		coordinate += base
	}

	return coordinate, nil
}

func parseTimeValue(_ CommandSource, tokens []string) (any, error) {
	presets := map[string]int64{"day": 1000, "noon": 6000, "night": 13000, "midnight": 18000}
	if value, valid := presets[strings.ToLower(tokens[0])]; valid {
		return commandTimeValue{ticks: value}, nil
	}

	value, err := strconv.ParseInt(tokens[0], 10, 64)
	if err != nil || value < 0 {
		return nil, commandSyntaxError{message: fmt.Sprintf("Invalid time %q", tokens[0])}
	}

	return commandTimeValue{ticks: value}, nil
}

func staticSuggestions(values []string) func(CommandSource) []string {
	return func(CommandSource) []string {
		return values
	}
}

func toBlockPosition(position game.Position) game.BlockPosition {
	return game.BlockPosition{X: int32(math.Floor(position.X)), Y: int32(math.Floor(position.Y)), Z: int32(math.Floor(position.Z))}
}

func orderedBlockPositions(first, second game.BlockPosition) (game.BlockPosition, game.BlockPosition) {
	minimum := game.BlockPosition{X: min(first.X, second.X), Y: min(first.Y, second.Y), Z: min(first.Z, second.Z)}
	maximum := game.BlockPosition{X: max(first.X, second.X), Y: max(first.Y, second.Y), Z: max(first.Z, second.Z)}

	return minimum, maximum
}
