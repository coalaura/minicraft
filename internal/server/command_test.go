package server

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/coalaura/minicraft/internal/config"
	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

const (
	commandTestBobUUID   = "00010203-0405-0607-0809-0a0b0c0d0e0f"
	commandTestAliceUUID = "10111213-1415-1617-1819-1a1b1c1d1e1f"
	commandTestZedUUID   = "20212223-2425-2627-2829-2a2b2c2d2e2f"
)

type commandTimeQueryTestCase struct {
	input string
	value int64
}

type commandTimeMutationTestCase struct {
	input string
	want  int64
}

type commandTargetErrorTestCase struct {
	input   string
	message game.TextComponent
}

func TestCommandDeclarationDescribesBuiltinCommands(t *testing.T) {
	registry := NewRuntime(&game.World{}).commands

	declaration := registry.declaration()

	if declaration.RootIndex != 0 {
		t.Fatalf("declaration root index = %d, want 0", declaration.RootIndex)
	}

	root := &declaration.Nodes[0]
	if root.Type != protocol.CommandNodeRoot {
		t.Fatalf("root node type = %d, want %d", root.Type, protocol.CommandNodeRoot)
	}

	var commandNames []string

	for _, childIndex := range root.Children {
		child := declaration.Nodes[childIndex]
		if child.Type != protocol.CommandNodeLiteral {
			t.Fatalf("root child %q has type %d, want literal", child.Name, child.Type)
		}

		commandNames = append(commandNames, child.Name)
	}

	expectedCommands := []string{"help", "seed", "time", "gamemode", "give", "clear", "teleport", "tp", "setblock", "fill"}
	if strings.Join(commandNames, ",") != strings.Join(expectedCommands, ",") {
		t.Fatalf("root command literals = %v, want %v", commandNames, expectedCommands)
	}

	seed := commandNodeChild(declaration.Nodes, root, protocol.CommandNodeLiteral, "seed")
	if seed == nil || !seed.Executable || len(seed.Children) != 0 {
		t.Fatalf("seed node = %+v, want executable leaf", seed)
	}

	help := commandNodeChild(declaration.Nodes, root, protocol.CommandNodeLiteral, "help")
	if help == nil || !help.Executable {
		t.Fatalf("help node = %+v, want executable", help)
	}

	helpCommand := commandNodeChild(declaration.Nodes, help, protocol.CommandNodeArgument, "command")
	if helpCommand == nil || !helpCommand.Executable || helpCommand.Parser != protocol.CommandParserString || helpCommand.Properties != (protocol.CommandStringProperties{Type: 2}) {
		t.Fatalf("help command argument = %+v", helpCommand)
	}

	if helpCommand.SuggestionType != commandSuggestionProvider {
		t.Fatalf("help command suggestion type = %q, want %q", helpCommand.SuggestionType, commandSuggestionProvider)
	}

	timeLiteral := commandNodeChild(declaration.Nodes, root, protocol.CommandNodeLiteral, "time")
	query := commandNodeChild(declaration.Nodes, timeLiteral, protocol.CommandNodeLiteral, "query")

	if query == nil || query.Executable || len(query.Children) != 3 {
		t.Fatalf("time query node = %+v, want daytime, gametime, and day children", query)
	}

	setLiteral := commandNodeChild(declaration.Nodes, timeLiteral, protocol.CommandNodeLiteral, "set")
	timeValue := commandNodeChild(declaration.Nodes, setLiteral, protocol.CommandNodeArgument, "time")

	if timeValue == nil || !timeValue.Executable || timeValue.Parser != protocol.CommandParserTime || timeValue.SuggestionType != "" {
		t.Fatalf("time value argument = %+v", timeValue)
	}

	gamemodeLiteral := commandNodeChild(declaration.Nodes, root, protocol.CommandNodeLiteral, "gamemode")
	mode := commandNodeChild(declaration.Nodes, gamemodeLiteral, protocol.CommandNodeArgument, "gamemode")

	if mode == nil || !mode.Executable || mode.Parser != protocol.CommandParserGameMode || mode.SuggestionType != "" {
		t.Fatalf("gamemode mode argument = %+v", mode)
	}

	gamemodeTargets := commandNodeChild(declaration.Nodes, mode, protocol.CommandNodeArgument, "targets")
	if gamemodeTargets == nil || !gamemodeTargets.Executable || gamemodeTargets.Parser != protocol.CommandParserEntity || gamemodeTargets.SuggestionType != "" {
		t.Fatalf("gamemode targets argument = %+v", gamemodeTargets)
	}

	entityProperties, validProperties := gamemodeTargets.Properties.(protocol.CommandEntityProperties)
	if !validProperties || entityProperties.SingleTarget || !entityProperties.OnlyPlayers {
		t.Fatalf("gamemode targets properties = %+v", gamemodeTargets.Properties)
	}

	giveLiteral := commandNodeChild(declaration.Nodes, root, protocol.CommandNodeLiteral, "give")
	giveTargets := commandNodeChild(declaration.Nodes, giveLiteral, protocol.CommandNodeArgument, "targets")
	item := commandNodeChild(declaration.Nodes, giveTargets, protocol.CommandNodeArgument, "item")
	count := commandNodeChild(declaration.Nodes, item, protocol.CommandNodeArgument, "count")

	if giveTargets == nil || giveTargets.Executable || giveTargets.Parser != protocol.CommandParserEntity || giveTargets.SuggestionType != "" {
		t.Fatalf("give targets argument = %+v", giveTargets)
	}

	if item == nil || !item.Executable || item.Parser != protocol.CommandParserResource || item.Properties != (protocol.CommandResourceProperties{Registry: "minecraft:item"}) || item.SuggestionType != "" {
		t.Fatalf("give item argument = %+v", item)
	}

	if count == nil || !count.Executable || count.Parser != protocol.CommandParserInteger {
		t.Fatalf("give count argument = %+v", count)
	}

	countProperties, validProperties := count.Properties.(protocol.CommandIntegerProperties)
	if !validProperties || !countProperties.HasMin || countProperties.HasMax || countProperties.Min != 1 {
		t.Fatalf("give count properties = %+v", count.Properties)
	}

	clearLiteral := commandNodeChild(declaration.Nodes, root, protocol.CommandNodeLiteral, "clear")
	clearTargets := commandNodeChild(declaration.Nodes, clearLiteral, protocol.CommandNodeArgument, "targets")
	clearItem := commandNodeChild(declaration.Nodes, clearTargets, protocol.CommandNodeArgument, "item")
	maxCount := commandNodeChild(declaration.Nodes, clearItem, protocol.CommandNodeArgument, "maxCount")

	if maxCount == nil {
		t.Fatal("clear maxCount argument is not declared")
	}

	maxCountProperties, validProperties := maxCount.Properties.(protocol.CommandIntegerProperties)
	if !maxCount.Executable || !validProperties || !maxCountProperties.HasMin || maxCountProperties.HasMax || maxCountProperties.Min != 0 {
		t.Fatalf("clear maxCount argument = %+v", maxCount)
	}

	teleportLiteral := commandNodeChild(declaration.Nodes, root, protocol.CommandNodeLiteral, "teleport")
	tpLiteral := commandNodeChild(declaration.Nodes, root, protocol.CommandNodeLiteral, "tp")

	if teleportLiteral == nil || tpLiteral == nil || !tpLiteral.HasRedirect || tpLiteral.Redirect != int32(indexCommandNode(declaration.Nodes, teleportLiteral)) {
		t.Fatalf("teleport redirect = %+v, want tp redirected to teleport", tpLiteral)
	}

	location := commandNodeChild(declaration.Nodes, teleportLiteral, protocol.CommandNodeArgument, "location")
	destination := commandNodeChild(declaration.Nodes, teleportLiteral, protocol.CommandNodeArgument, "destination")
	tpTargets := commandNodeChild(declaration.Nodes, teleportLiteral, protocol.CommandNodeArgument, "targets")

	if location == nil || !location.Executable || location.Parser != protocol.CommandParserVec3 {
		t.Fatalf("tp location argument = %+v", location)
	}

	if destination == nil || !destination.Executable || destination.Parser != protocol.CommandParserEntity {
		t.Fatalf("tp destination argument = %+v", destination)
	}

	if tpTargets == nil || tpTargets.Executable || tpTargets.Parser != protocol.CommandParserEntity {
		t.Fatalf("tp targets argument = %+v", tpTargets)
	}

	destinationProperties, validProperties := destination.Properties.(protocol.CommandEntityProperties)
	if !validProperties || !destinationProperties.SingleTarget || !destinationProperties.OnlyPlayers {
		t.Fatalf("tp destination properties = %+v, want one player", destination.Properties)
	}

	targetProperties, validProperties := tpTargets.Properties.(protocol.CommandEntityProperties)
	if !validProperties || targetProperties.SingleTarget || !targetProperties.OnlyPlayers {
		t.Fatalf("tp targets properties = %+v, want multiple players", tpTargets.Properties)
	}

	tpTargetDestination := commandNodeChild(declaration.Nodes, tpTargets, protocol.CommandNodeArgument, "destination")
	if tpTargetDestination == nil || !tpTargetDestination.Executable {
		t.Fatalf("tp targets destination argument = %+v, want executable", tpTargetDestination)
	}

	tpTargetDestinationProperties, validProperties := tpTargetDestination.Properties.(protocol.CommandEntityProperties)
	if !validProperties || !tpTargetDestinationProperties.SingleTarget || !tpTargetDestinationProperties.OnlyPlayers {
		t.Fatalf("tp target destination properties = %+v, want one player", tpTargetDestination.Properties)
	}

	tpTargetLocation := commandNodeChild(declaration.Nodes, tpTargets, protocol.CommandNodeArgument, "location")
	if tpTargetLocation == nil || !tpTargetLocation.Executable {
		t.Fatalf("tp targets location argument = %+v, want executable", tpTargetLocation)
	}

	setblockLiteral := commandNodeChild(declaration.Nodes, root, protocol.CommandNodeLiteral, "setblock")
	setblockPosition := commandNodeChild(declaration.Nodes, setblockLiteral, protocol.CommandNodeArgument, "position")
	setblockBlock := commandNodeChild(declaration.Nodes, setblockPosition, protocol.CommandNodeArgument, "block")

	if setblockPosition == nil || setblockPosition.Executable || setblockPosition.Parser != protocol.CommandParserBlockPosition {
		t.Fatalf("setblock position argument = %+v", setblockPosition)
	}

	if setblockBlock == nil || !setblockBlock.Executable || setblockBlock.Parser != protocol.CommandParserResource || setblockBlock.Properties != (protocol.CommandResourceProperties{Registry: "minecraft:block"}) || setblockBlock.SuggestionType != "" {
		t.Fatalf("setblock block argument = %+v", setblockBlock)
	}

	setblockModes := []string{"replace", "keep", "strict"}

	for _, modeName := range setblockModes {
		if commandNodeChild(declaration.Nodes, setblockBlock, protocol.CommandNodeLiteral, modeName) == nil {
			t.Fatalf("setblock mode %q is not declared", modeName)
		}
	}

	fillLiteral := commandNodeChild(declaration.Nodes, root, protocol.CommandNodeLiteral, "fill")
	fillFrom := commandNodeChild(declaration.Nodes, fillLiteral, protocol.CommandNodeArgument, "from")
	fillTo := commandNodeChild(declaration.Nodes, fillFrom, protocol.CommandNodeArgument, "to")
	fillBlock := commandNodeChild(declaration.Nodes, fillTo, protocol.CommandNodeArgument, "block")

	if fillFrom == nil || fillFrom.Executable || fillFrom.Parser != protocol.CommandParserBlockPosition {
		t.Fatalf("fill from argument = %+v", fillFrom)
	}

	if fillTo == nil || fillTo.Executable || fillTo.Parser != protocol.CommandParserBlockPosition {
		t.Fatalf("fill to argument = %+v", fillTo)
	}

	if fillBlock == nil || !fillBlock.Executable || fillBlock.Parser != protocol.CommandParserResource || fillBlock.Properties != (protocol.CommandResourceProperties{Registry: "minecraft:block"}) || fillBlock.SuggestionType != "" {
		t.Fatalf("fill block argument = %+v", fillBlock)
	}

	fillModes := []string{"replace", "keep", "outline", "hollow", "strict"}

	for _, modeName := range fillModes {
		if commandNodeChild(declaration.Nodes, fillBlock, protocol.CommandNodeLiteral, modeName) == nil {
			t.Fatalf("fill mode %q is not declared", modeName)
		}
	}

	var writer protocol.PacketWriter

	declaration.Encode(&writer)

	err := writer.Err()
	if err != nil {
		t.Fatalf("encode command declaration: %v", err)
	}
}

func TestCommandTreeRejectsIncompatibleNodeMerges(t *testing.T) {
	existing := protocol.CommandNode{
		Type:       protocol.CommandNodeArgument,
		Name:       "target",
		Parser:     protocol.CommandParserEntity,
		Properties: protocol.CommandEntityProperties{SingleTarget: true, OnlyPlayers: true},
	}

	incoming := existing

	incoming.Properties = protocol.CommandEntityProperties{OnlyPlayers: true}

	defer func() {
		if recover() == nil {
			t.Fatal("incompatible command node merge did not panic")
		}
	}()

	mergeCommandTreeNode(&existing, incoming)
}

func TestCommandHelpUsageAndSeedFeedback(t *testing.T) {
	runtime := NewRuntime(&game.World{Seed: 12345})

	session, connection := newCommandTestSession(runtime, commandTestBobUUID, "Bob")

	executeCommand(t, session, "help")

	assertSystemMessages(t, connection,
		"/help", "/help <command>", "/seed",
		"/time query daytime", "/time query gametime", "/time query day", "/time set day", "/time set noon", "/time set night", "/time set midnight", "/time set <time>", "/time add <time>",
		"/gamemode <gamemode>", "/gamemode <gamemode> <targets>",
		"/give <targets> <item>", "/give <targets> <item> <count>",
		"/clear", "/clear <targets>", "/clear <targets> <item>", "/clear <targets> <item> <maxCount>",
		"/teleport <location>", "/teleport <destination>", "/teleport <targets> <destination>", "/teleport <targets> <location>",
		"/tp <location>", "/tp <destination>", "/tp <targets> <destination>", "/tp <targets> <location>",
		"/setblock <position> <block>", "/setblock <position> <block> replace", "/setblock <position> <block> keep", "/setblock <position> <block> strict",
		"/fill <from> <to> <block>", "/fill <from> <to> <block> replace", "/fill <from> <to> <block> keep", "/fill <from> <to> <block> outline", "/fill <from> <to> <block> hollow", "/fill <from> <to> <block> strict")

	connection.reset()

	executeCommand(t, session, "help time set")

	assertSystemMessages(t, connection, "/time set day", "/time set noon", "/time set night", "/time set midnight", "/time set <time>")

	connection.reset()

	executeCommand(t, session, "seed")

	assertSystemComponents(t, connection, game.TranslatableText("commands.seed.success", game.LiteralText("12345").WithColor(game.TextColorGreen).WithClickEvent(game.ClickCopyToClipboard, "12345")))

	connection.reset()

	executeCommand(t, session, "SEED")

	assertSyntaxError(t, connection, "SEED", 0, game.TranslatableText("command.unknown.command"))

	connection.reset()

	executeCommand(t, session, "help bogus")

	assertSystemComponents(t, connection, game.TranslatableText("commands.help.failed").WithColor(game.TextColorRed))

	connection.reset()

	executeCommand(t, session, "seed extra")

	assertSyntaxError(t, connection, "seed extra", 5, game.TranslatableText("command.unknown.argument"))

	connection.reset()

	executeCommand(t, session, "bogus")

	assertSyntaxError(t, connection, "bogus", 0, game.TranslatableText("command.unknown.command"))

	connection.reset()

	executeCommand(t, session, "/")

	assertSyntaxError(t, connection, "", 0, game.TranslatableText("command.unknown.command"))
}

func TestCommandTimeQueryAndSet(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	session, connection := newCommandTestSession(runtime, commandTestBobUUID, "Bob")

	runtime.World.SetTime(48001, false)

	timeQueries := []commandTimeQueryTestCase{
		{"time query daytime", 1},
		{"time query gametime", 0},
		{"time query day", 2},
	}

	for _, query := range timeQueries {
		connection.reset()

		executeCommand(t, session, query.input)

		assertSystemComponents(t, connection, game.TranslatableText("commands.time.query", game.LiteralText(fmt.Sprint(query.value))))
	}

	presets := map[string]int64{"day": 1000, "noon": 6000, "night": 13000, "midnight": 18000}
	for preset, ticks := range presets {
		connection.reset()

		executeCommand(t, session, "time set "+preset)

		assertSystemComponents(t, connection, game.TranslatableText("commands.time.set", game.LiteralText(fmt.Sprint(ticks))))

		dayTime := runtime.World.Time().DayTime
		if dayTime != ticks {
			t.Fatalf("world day time after %s = %d, want %d", preset, dayTime, ticks)
		}
	}

	timeMutations := []commandTimeMutationTestCase{{"time set 4242", 4242}, {"time set 2t", 2}, {"time set 1.5s", 30}, {"time set 0.5d", 12000}, {"time add 0.5s", 12010}}

	for _, test := range timeMutations {
		connection.reset()

		executeCommand(t, session, test.input)

		dayTime := runtime.World.Time().DayTime
		if dayTime != test.want {
			t.Fatalf("day time after %q = %d, want %d", test.input, dayTime, test.want)
		}

		assertSystemComponents(t, connection, game.TranslatableText("commands.time.set", game.LiteralText(fmt.Sprint(test.want))))
	}

	invalidTimeInputs := []string{"time set -1", "time set 1x", "time set nope", "time set DAY", "time query DAY"}

	for _, input := range invalidTimeInputs {
		connection.reset()
		executeCommand(t, session, input)

		if len(packetsByID(t, connection, protocol.ClientboundSystemChatID)) != 2 {
			t.Fatalf("%q did not produce a syntax failure and context", input)
		}
	}
}

func TestCommandTimeSetSynchronizesWorldTime(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	runtime.World.SetTime(500, false)

	for range 37 {
		runtime.World.AdvanceTime()
	}

	session, connection := newCommandTestSession(runtime, commandTestBobUUID, "Bob")

	joinTestSession(t, runtime, session)

	connection.reset()

	executeCommand(t, session, "time set day")

	state := runtime.World.Time()
	if state.Age != 37 || state.DayTime != 1000 || state.DayCycle {
		t.Fatalf("world time state = %+v, want age 37, day time 1000, and day cycle disabled", state)
	}

	timeUpdates := packetsByID(t, connection, protocol.ClientboundUpdateTimeID)
	if len(timeUpdates) != 1 {
		t.Fatalf("time update packets = %d, want 1", len(timeUpdates))
	}

	assertTimeUpdatePacket(t, timeUpdates[0], 37, 1000, false)
	assertSystemComponents(t, connection, game.TranslatableText("commands.time.set", game.LiteralText("1000")))
}

func TestCommandTargetsPlayersByName(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	bob, bobConnection := newCommandTestSession(runtime, commandTestBobUUID, "Bob")
	alice, aliceConnection := newCommandTestSession(runtime, commandTestAliceUUID, "Alice")

	joinTestSession(t, runtime, bob)
	joinTestSession(t, runtime, alice)

	bobConnection.reset()

	executeCommand(t, bob, "gamemode spectator Alice")

	assertSystemComponents(t, bobConnection, game.TranslatableText("commands.gamemode.success.other", game.LiteralText("Alice"), game.TranslatableText("gameMode.spectator")))
	assertSystemComponents(t, aliceConnection, game.TranslatableText("gameMode.changed", game.TranslatableText("gameMode.spectator")))

	mode := alice.snapshotPlayer().GameMode
	if mode != game.GameModeSpectator {
		t.Fatalf("alice game mode = %d, want spectator", mode)
	}

	mode = bob.snapshotPlayer().GameMode
	if mode != game.GameModeSurvival {
		t.Fatalf("bob game mode = %d, want survival", mode)
	}

	bobConnection.reset()

	executeCommand(t, bob, "gamemode creative alice")

	mode = alice.snapshotPlayer().GameMode
	if mode != game.GameModeCreative {
		t.Fatalf("alice game mode after case-insensitive target = %d, want creative", mode)
	}

	bobConnection.reset()

	executeCommand(t, bob, "gamemode creative Nobody")

	assertSyntaxError(t, bobConnection, "gamemode creative Nobody", 18, game.TranslatableText("argument.player.unknown"))

	bobConnection.reset()

	executeCommand(t, bob, "gamemode creative not!valid")

	assertSyntaxError(t, bobConnection, "gamemode creative not!valid", 18, game.TranslatableText("argument.entity.invalid"))
}

func TestCommandSelectorResolution(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	zed, _ := newCommandTestSession(runtime, commandTestZedUUID, "Zed")
	bob, _ := newCommandTestSession(runtime, commandTestBobUUID, "Bob")
	alice, _ := newCommandTestSession(runtime, commandTestAliceUUID, "Alice")

	sharedPosition := game.Position{X: 8.5, Y: 70, Z: 8.5}

	zed.Player.Position = sharedPosition
	bob.Player.Position = sharedPosition
	alice.Player.Position = game.Position{X: 1000.5, Y: 70, Z: 1000.5}

	joinTestSession(t, runtime, zed)
	joinTestSession(t, runtime, bob)
	joinTestSession(t, runtime, alice)

	executeCommand(t, zed, "gamemode adventure @s")

	mode := zed.snapshotPlayer().GameMode
	if mode != game.GameModeAdventure {
		t.Fatalf("zed game mode after @s = %d, want adventure", mode)
	}

	mode = bob.snapshotPlayer().GameMode
	if mode != game.GameModeSurvival {
		t.Fatalf("bob game mode after @s = %d, want survival", mode)
	}

	executeCommand(t, zed, "gamemode creative @a")

	selectorSessions := []*Session{zed, bob, alice}

	for _, session := range selectorSessions {
		mode := session.snapshotPlayer().GameMode
		if mode != game.GameModeCreative {
			t.Fatalf("%s game mode after @a = %d, want creative", session.Player.Name, mode)
		}
	}

	executeCommand(t, zed, "gamemode spectator @p")

	mode = bob.snapshotPlayer().GameMode
	if mode != game.GameModeSpectator {
		t.Fatalf("bob game mode after @p = %d, want spectator", mode)
	}

	mode = zed.snapshotPlayer().GameMode
	if mode != game.GameModeCreative {
		t.Fatalf("zed game mode after @p = %d, want creative", mode)
	}

	runtime.commandRandomMu.Lock()

	runtime.commandRandom = func(int) int {
		return 0
	}

	runtime.commandRandomMu.Unlock()

	executeCommand(t, zed, "gamemode adventure @r")

	mode = alice.snapshotPlayer().GameMode
	if mode != game.GameModeAdventure {
		t.Fatalf("alice game mode after @r = %d, want adventure", mode)
	}

	mode = bob.snapshotPlayer().GameMode
	if mode != game.GameModeSpectator {
		t.Fatalf("bob game mode after @r = %d, want spectator", mode)
	}
}

func TestCommandRejectsUnsupportedSelectors(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	bob, bobConnection := newCommandTestSession(runtime, commandTestBobUUID, "Bob")

	joinTestSession(t, runtime, bob)

	executeCommand(t, bob, "gamemode creative @e")

	assertSyntaxError(t, bobConnection, "gamemode creative @e", 18, game.TranslatableText("argument.player.entities"))

	bobConnection.reset()

	executeCommand(t, bob, "gamemode creative @a[gamemode=survival]")

	assertSyntaxError(t, bobConnection, "gamemode creative @a[gamemode=survival]", 18, game.LiteralText("Selector options are not supported"))

	mode := bob.snapshotPlayer().GameMode
	if mode != game.GameModeSurvival {
		t.Fatalf("bob game mode after rejected selectors = %d, want survival", mode)
	}

	alice, _ := newCommandTestSession(runtime, commandTestAliceUUID, "Alice")

	joinTestSession(t, runtime, alice)

	targetErrors := []commandTargetErrorTestCase{
		{"teleport @a", game.TranslatableText("argument.player.toomany")},
		{"teleport Nobody", game.TranslatableText("argument.player.unknown")},
	}

	for _, test := range targetErrors {
		argument := runtime.commands.targetArgument("destination", false)

		token := commandToken{value: strings.TrimPrefix(test.input, "teleport "), start: len("teleport "), end: len(test.input)}

		_, _, err := argument.parse(playerCommandSource{session: bob}, []commandToken{token}, token.start)

		syntax, valid := errors.AsType[commandSyntaxError](err)
		if !valid {
			t.Fatalf("single destination %q error = %v, want syntax error", token.value, err)
		}

		bobConnection.reset()

		err = runtime.commands.sendSyntaxError(playerCommandSource{session: bob}, test.input, syntax)
		if err != nil {
			t.Fatalf("send destination error: %v", err)
		}

		assertSyntaxError(t, bobConnection, test.input, token.start, test.message)
	}
}

func TestCommandSuggestions(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	bob, _ := newCommandTestSession(runtime, commandTestBobUUID, "Bob")
	alice, _ := newCommandTestSession(runtime, commandTestAliceUUID, "Alice")

	joinTestSession(t, runtime, bob)
	joinTestSession(t, runtime, alice)

	source := playerCommandSource{session: bob}

	commandNames := []string{"clear", "fill", "gamemode", "give", "help", "seed", "setblock", "teleport", "time", "tp"}
	targets := []string{"@a", "@p", "@r", "@s", "Alice", "Bob"}

	assertCommandSuggestions(t, runtime.commands.suggestions(source, "/"), 1, 0, commandNames)
	assertCommandSuggestions(t, runtime.commands.suggestions(source, "/he"), 1, 2, []string{"help"})
	assertCommandSuggestions(t, runtime.commands.suggestions(source, "/help"), 1, 4, []string{"help"})
	assertCommandSuggestions(t, runtime.commands.suggestions(source, "/help "), 6, 0, []string{"clear", "fill", "gamemode", "give", "help", "seed", "setblock", "teleport", "time", "time add", "time query", "time query day", "time query daytime", "time query gametime", "time set", "time set day", "time set midnight", "time set night", "time set noon", "tp"})
	assertCommandSuggestions(t, runtime.commands.suggestions(source, "/gamemode "), 10, 0, []string{"adventure", "creative", "spectator", "survival"})
	assertCommandSuggestions(t, runtime.commands.suggestions(source, "/gamemode creative "), 19, 0, targets)
	assertCommandSuggestions(t, runtime.commands.suggestions(source, "/time set "), 10, 0, []string{"day", "midnight", "night", "noon"})
	assertCommandSuggestions(t, runtime.commands.suggestions(source, "/time s"), 6, 1, []string{"set"})
	assertCommandSuggestions(t, runtime.commands.suggestions(source, "/give "), 6, 0, targets)
	assertCommandSuggestions(t, runtime.commands.suggestions(source, "/tp "), 4, 0, targets)
	assertCommandSuggestions(t, runtime.commands.suggestions(source, "/tp @"), 4, 1, []string{"@a", "@p", "@r", "@s"})
	assertCommandSuggestions(t, runtime.commands.suggestions(source, "/give Bob "), 10, 0, nil)
	assertCommandSuggestions(t, runtime.commands.suggestions(source, "/give Bob stone_a"), 10, 7, nil)
	assertCommandSuggestions(t, runtime.commands.suggestions(source, "/setblock 0 70 0 "), 17, 0, nil)
	assertCommandSuggestions(t, runtime.commands.suggestions(source, "/setblock 0 70 0 grass_bl"), 17, 8, nil)
	assertCommandSuggestions(t, runtime.commands.suggestions(source, "/bogus "), 7, 0, nil)
}

func TestCommandSuggestionRangesUseUTF16Offsets(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	bob, _ := newCommandTestSession(runtime, commandTestBobUUID, "Bob")

	source := playerCommandSource{session: bob}

	first := commandArgument{name: "first", parser: protocol.CommandParserString, width: 1, parseValue: parseString}
	second := commandArgument{name: "second", parser: protocol.CommandParserString, width: 1, parseValue: parseString, suggestValues: staticSuggestions([]string{"value"})}

	runtime.commands.register(&registeredCommand{
		Name:     "range",
		Patterns: []commandPattern{{Elements: []commandElement{first, second}}},
	})

	assertCommandSuggestions(t, runtime.commands.suggestions(source, "/help café"), 6, 4, nil)
	assertCommandSuggestions(t, runtime.commands.suggestions(source, "/help 😀"), 6, 2, nil)
	assertCommandSuggestions(t, runtime.commands.suggestions(source, "/range 😀 "), 10, 0, []string{"value"})
}

func TestCommandGameModeSynchronization(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	bob, bobConnection := newCommandTestSession(runtime, commandTestBobUUID, "Bob")
	alice, aliceConnection := newCommandTestSession(runtime, commandTestAliceUUID, "Alice")

	joinTestSession(t, runtime, bob)
	joinTestSession(t, runtime, alice)

	bobConnection.reset()
	aliceConnection.reset()

	executeCommand(t, bob, "gamemode creative")

	mode := bob.snapshotPlayer().GameMode
	if mode != game.GameModeCreative {
		t.Fatalf("bob game mode = %d, want creative", mode)
	}

	assertPacketIDs(t, bobConnection.packetIDs(t), []int32{
		protocol.ClientboundGameEventID,
		protocol.ClientboundPlayerInfoUpdateID,
		protocol.ClientboundSystemChatID,
	})

	assertPacketIDs(t, aliceConnection.packetIDs(t), []int32{protocol.ClientboundPlayerInfoUpdateID})

	bobPackets := bobConnection.packets(t)

	assertGameModeEventPacket(t, bobPackets[0], game.GameModeCreative)
	assertGameModeInfoPacket(t, bobPackets[1], commandTestBobUUID, game.GameModeCreative)
	assertGameModeInfoPacket(t, aliceConnection.packets(t)[0], commandTestBobUUID, game.GameModeCreative)

	assertSystemComponents(t, bobConnection, game.TranslatableText("commands.gamemode.success.self", game.TranslatableText("gameMode.creative")))

	bobConnection.reset()
	aliceConnection.reset()

	executeCommand(t, bob, "gamemode creative")

	assertPacketIDs(t, bobConnection.packetIDs(t), nil)
	assertPacketIDs(t, aliceConnection.packetIDs(t), nil)

	bobConnection.reset()

	executeCommand(t, bob, "gamemode bogus")

	assertSyntaxError(t, bobConnection, "gamemode bogus", 9, game.TranslatableText("argument.gamemode.invalid", game.LiteralText("bogus")))

	mode = bob.snapshotPlayer().GameMode
	if mode != game.GameModeCreative {
		t.Fatalf("bob game mode after invalid mode = %d, want creative", mode)
	}
}

func TestCommandGiveUpdatesTargetInventory(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	bob, bobConnection := newCommandTestSession(runtime, commandTestBobUUID, "Bob")

	joinTestSession(t, runtime, bob)

	bobConnection.reset()

	executeCommand(t, bob, "give Bob stone")

	stateID, items, carried := decodeInventorySnapshot(t, bobConnection.packets(t)[0])
	if stateID != 1 {
		t.Fatalf("inventory state id = %d, want 1", stateID)
	}

	if !items[36].Equal(game.ItemStack{Item: game.ItemStone, Count: 1}) {
		t.Fatalf("first given stack = %+v, want one stone in the hotbar", items[36])
	}

	if !carried.Empty() {
		t.Fatalf("carried stack after give = %+v, want empty", carried)
	}

	assertSystemComponents(t, bobConnection, game.TranslatableText("commands.give.success.single", game.LiteralText("1"), game.TranslatableText("block.minecraft.stone"), game.LiteralText("Bob")))

	bobConnection.reset()

	executeCommand(t, bob, "give Bob stone 5")

	_, items, _ = decodeInventorySnapshot(t, bobConnection.packets(t)[0])
	if !items[36].Equal(game.ItemStack{Item: game.ItemStone, Count: 6}) {
		t.Fatalf("merged stack = %+v, want six stone", items[36])
	}

	bobConnection.reset()

	executeCommand(t, bob, "give Bob dirt")

	_, items, _ = decodeInventorySnapshot(t, bobConnection.packets(t)[0])
	if !items[37].Equal(game.ItemStack{Item: game.ItemDirt, Count: 1}) {
		t.Fatalf("dirt stack = %+v, want one dirt in the second hotbar slot", items[37])
	}

	bobConnection.reset()

	executeCommand(t, bob, "give Bob stone 100")

	inventoryPackets := packetsByID(t, bobConnection, protocol.ClientboundContainerSetContentID)

	stateID, items, _ = decodeInventorySnapshot(t, inventoryPackets[len(inventoryPackets)-1])
	if stateID != 5 {
		t.Fatalf("inventory state id after four gives = %d, want 5", stateID)
	}

	if !items[36].Equal(game.ItemStack{Item: game.ItemStone, Count: 64}) {
		t.Fatalf("overflowed stack = %+v, want a full stone stack", items[36])
	}

	if !items[38].Equal(game.ItemStack{Item: game.ItemStone, Count: 42}) {
		t.Fatalf("split stack = %+v, want the remaining 42 stone", items[38])
	}
}

func TestCommandGiveFallsBackToMainInventoryWhenHotbarIsFull(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	bob, bobConnection := newCommandTestSession(runtime, commandTestBobUUID, "Bob")

	for slot := range bob.Player.Inventory.Hotbar {
		bob.Player.Inventory.Hotbar[slot] = game.ItemStack{Item: game.ItemDirt, Count: 64}
	}

	joinTestSession(t, runtime, bob)

	bobConnection.reset()

	executeCommand(t, bob, "give Bob stone")

	player := bob.snapshotPlayer()
	if !player.Inventory.Main[0].Equal(game.ItemStack{Item: game.ItemStone, Count: 1}) {
		t.Fatalf("first main inventory stack = %+v, want one stone", player.Inventory.Main[0])
	}

	for slot, stack := range player.Inventory.Hotbar {
		if !stack.Equal(game.ItemStack{Item: game.ItemDirt, Count: 64}) {
			t.Fatalf("hotbar slot %d = %+v after give, want a full dirt stack", slot, stack)
		}
	}
}

func TestCommandGiveRejectsInvalidArguments(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	bob, bobConnection := newCommandTestSession(runtime, commandTestBobUUID, "Bob")

	joinTestSession(t, runtime, bob)

	executeCommand(t, bob, "give Bob air")

	assertSyntaxError(t, bobConnection, "give Bob air", 9, game.LiteralText("Unknown item 'air'"))

	bobConnection.reset()

	executeCommand(t, bob, "give Bob bogus")

	assertSyntaxError(t, bobConnection, "give Bob bogus", 9, game.LiteralText("Unknown item 'bogus'"))

	bobConnection.reset()

	executeCommand(t, bob, "give Bob stone 0")

	assertSyntaxError(t, bobConnection, "give Bob stone 0", 15, game.TranslatableText("argument.integer.low", game.LiteralText("0"), game.LiteralText("1")))

	bobConnection.reset()

	executeCommand(t, bob, "give Bob stone 6401")

	assertSystemComponents(t, bobConnection, game.TranslatableText("commands.give.failed.toomanyitems", game.LiteralText("6400"), game.TranslatableText("block.minecraft.stone")).WithColor(game.TextColorRed))

	player := bob.snapshotPlayer()

	if bob.activeMenu().stateID != 0 {
		t.Fatalf("inventory state id after rejected gives = %d, want 0", bob.activeMenu().stateID)
	}

	for slot := 9; slot < game.PlayerInventorySlots; slot++ {
		if !player.Inventory.Slot(slot).Empty() {
			t.Fatalf("slot %d = %+v after rejected gives, want empty", slot, *player.Inventory.Slot(slot))
		}
	}

	assertPacketIDs(t, bobConnection.packetIDs(t), []int32{protocol.ClientboundSystemChatID})
}

func TestCommandGiveDropsOverflowWhenInventoryIsFull(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	bob, bobConnection := newCommandTestSession(runtime, commandTestBobUUID, "Bob")

	for slot := range bob.Player.Inventory.Main {
		bob.Player.Inventory.Main[slot] = game.ItemStack{Item: game.ItemDirt, Count: 64}
	}

	for slot := range bob.Player.Inventory.Hotbar {
		bob.Player.Inventory.Hotbar[slot] = game.ItemStack{Item: game.ItemDirt, Count: 64}
	}

	joinTestSession(t, runtime, bob)

	bobConnection.reset()

	executeCommand(t, bob, "give Bob stone")

	assertSystemComponents(t, bobConnection, game.TranslatableText("commands.give.success.single", game.LiteralText("1"), game.TranslatableText("block.minecraft.stone"), game.LiteralText("Bob")))

	player := bob.snapshotPlayer()

	if bob.activeMenu().stateID != 0 {
		t.Fatalf("inventory state id after failed give = %d, want 0", bob.activeMenu().stateID)
	}

	for slot := 9; slot <= 44; slot++ {
		expected := game.ItemStack{Item: game.ItemDirt, Count: 64}
		if !player.Inventory.Slot(slot).Equal(expected) {
			t.Fatalf("slot %d = %+v after failed give, want a full dirt stack", slot, *player.Inventory.Slot(slot))
		}
	}

	entities := runtime.snapshotRuntimeEntities()
	if len(entities) != 1 {
		t.Fatalf("overflow entities = %d, want 1", len(entities))
	}

	overflow := entities[0].(*runtimeItemEntity)
	if !overflow.Stack.Equal(game.ItemStack{Item: game.ItemStone, Count: 1}) || overflow.PickupDelay != 0 || overflow.TargetUUID != bob.Player.UUID {
		t.Fatalf("overflow entity = %+v", overflow)
	}

	assertPacketIDs(t, bobConnection.packetIDs(t), []int32{protocol.ClientboundSystemChatID})
}

func TestCommandGiveSynchronizesHeldEquipment(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	bob, bobConnection := newCommandTestSession(runtime, commandTestBobUUID, "Bob")
	observer, observerConnection := newCommandTestSession(runtime, commandTestAliceUUID, "Alice")

	for slot := range bob.Player.Inventory.Main {
		bob.Player.Inventory.Main[slot] = game.ItemStack{Item: game.ItemDirt, Count: 64}
	}

	joinTestSession(t, runtime, observer)
	joinTestSession(t, runtime, bob)

	bobConnection.reset()
	observerConnection.reset()

	executeCommand(t, bob, "give Bob stone")

	player := bob.snapshotPlayer()
	if !player.Inventory.Hotbar[0].Equal(game.ItemStack{Item: game.ItemStone, Count: 1}) {
		t.Fatalf("held stack after give = %+v, want one stone in the selected hotbar slot", player.Inventory.Hotbar[0])
	}

	assertPacketIDs(t, bobConnection.packetIDs(t), []int32{
		protocol.ClientboundContainerSetContentID,
		protocol.ClientboundSystemChatID,
	})

	assertPacketIDs(t, observerConnection.packetIDs(t), []int32{protocol.ClientboundEntityEquipmentID})
	assertEquipmentUpdate(t, observerConnection.packets(t)[0], bob.Player.EntityID, protocol.EquipmentSlotMainHand, game.ItemStone, 1)
}

func TestCommandGiveProcessesMultipleTargetsIndependently(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		runtime := NewRuntime(&game.World{})

		bob, bobConnection := newCommandTestSession(runtime, commandTestBobUUID, "Bob")
		alice, aliceConnection := newCommandTestSession(runtime, commandTestAliceUUID, "Alice")

		joinTestSession(t, runtime, bob)
		joinTestSession(t, runtime, alice)

		bobConnection.reset()
		aliceConnection.reset()

		executeCommand(t, bob, "give @a stone 3")

		giveSessions := []*Session{bob, alice}

		for _, session := range giveSessions {
			inventory := session.snapshotPlayer().Inventory
			if session.activeMenu().stateID != 1 || !inventory.Hotbar[0].Equal(game.ItemStack{Item: game.ItemStone, Count: 3}) {
				t.Fatalf("%s inventory after give = %+v", session.snapshotPlayer().Name, inventory)
			}
		}

		if len(packetsByID(t, bobConnection, protocol.ClientboundContainerSetContentID)) != 1 || len(packetsByID(t, aliceConnection, protocol.ClientboundContainerSetContentID)) != 1 {
			t.Fatal("successful multi-target give did not synchronize both inventories")
		}

		bobEquipment := packetsByID(t, aliceConnection, protocol.ClientboundEntityEquipmentID)
		aliceEquipment := packetsByID(t, bobConnection, protocol.ClientboundEntityEquipmentID)

		if len(bobEquipment) != 1 || len(aliceEquipment) != 1 {
			t.Fatalf("equipment packets = bob %d alice %d, want one each", len(bobEquipment), len(aliceEquipment))
		}

		assertEquipmentUpdate(t, bobEquipment[0], bob.Player.EntityID, protocol.EquipmentSlotMainHand, game.ItemStone, 3)
		assertEquipmentUpdate(t, aliceEquipment[0], alice.Player.EntityID, protocol.EquipmentSlotMainHand, game.ItemStone, 3)
	})

	t.Run("overflow", func(t *testing.T) {
		runtime := NewRuntime(&game.World{})

		bob, bobConnection := newCommandTestSession(runtime, commandTestBobUUID, "Bob")
		alice, aliceConnection := newCommandTestSession(runtime, commandTestAliceUUID, "Alice")

		for slot := range alice.Player.Inventory.Main {
			alice.Player.Inventory.Main[slot] = game.ItemStack{Item: game.ItemDirt, Count: 64}
		}

		for slot := range alice.Player.Inventory.Hotbar {
			alice.Player.Inventory.Hotbar[slot] = game.ItemStack{Item: game.ItemDirt, Count: 64}
		}

		joinTestSession(t, runtime, bob)
		joinTestSession(t, runtime, alice)

		bobConnection.reset()
		aliceConnection.reset()

		executeCommand(t, bob, "give @a stone")

		bobInventory := bob.snapshotPlayer().Inventory
		aliceInventory := alice.snapshotPlayer().Inventory

		if bob.activeMenu().stateID != 1 || !bobInventory.Hotbar[0].Equal(game.ItemStack{Item: game.ItemStone, Count: 1}) {
			t.Fatalf("bob inventory after give: %+v", bobInventory)
		}

		if alice.activeMenu().stateID != 0 || !aliceInventory.Hotbar[0].Equal(game.ItemStack{Item: game.ItemDirt, Count: 64}) {
			t.Fatalf("alice inventory changed during rollback: %+v", aliceInventory)
		}

		if len(packetsByID(t, bobConnection, protocol.ClientboundContainerSetContentID)) != 1 || len(packetsByID(t, aliceConnection, protocol.ClientboundContainerSetContentID)) != 0 {
			t.Fatal("multi-target give did not synchronize each changed inventory")
		}

		var aliceOverflow *runtimeItemEntity

		for _, entity := range runtime.snapshotRuntimeEntities() {
			itemEntity := entity.(*runtimeItemEntity)
			if itemEntity.TargetUUID == alice.Player.UUID {
				aliceOverflow = itemEntity
			}
		}

		if aliceOverflow == nil || !aliceOverflow.Stack.Equal(game.ItemStack{Item: game.ItemStone, Count: 1}) || aliceOverflow.PickupDelay != 0 {
			t.Fatalf("alice overflow entity = %+v", aliceOverflow)
		}

		assertSystemComponents(t, bobConnection, game.TranslatableText("commands.give.success.multiple", game.LiteralText("1"), game.TranslatableText("block.minecraft.stone"), game.LiteralText("2")))
	})
}

func TestCommandClearRemovesInventoryAndCursor(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	bob, bobConnection := newCommandTestSession(runtime, commandTestBobUUID, "Bob")
	observer, observerConnection := newCommandTestSession(runtime, commandTestAliceUUID, "Alice")

	bob.Player.Inventory.Hotbar[0] = game.ItemStack{Item: game.ItemStone, Count: 3}
	bob.Player.Inventory.Main[0] = game.ItemStack{Item: game.ItemDirt, Count: 2}
	bob.activeMenu().carried = game.ItemStack{Item: game.ItemStone, Count: 4}

	joinTestSession(t, runtime, observer)
	joinTestSession(t, runtime, bob)

	bobConnection.reset()
	observerConnection.reset()

	executeCommand(t, bob, "clear")

	player := bob.snapshotPlayer()
	if !player.Inventory.Hotbar[0].Empty() || !player.Inventory.Main[0].Empty() || !bob.activeMenu().carried.Empty() {
		t.Fatalf("inventory after clear = %+v, want every stack removed", player.Inventory)
	}

	if bob.activeMenu().stateID != 1 {
		t.Fatalf("inventory state id after clear = %d, want 1", bob.activeMenu().stateID)
	}

	stateID, items, carried := decodeInventorySnapshot(t, bobConnection.packets(t)[0])
	if stateID != 1 || !carried.Empty() {
		t.Fatalf("synchronized inventory = state %d carried %+v, want state 1 with empty cursor", stateID, carried)
	}

	for slot := range items {
		if !items[slot].Empty() {
			t.Fatalf("synchronized slot %d = %+v after clear, want empty", slot, items[slot])
		}
	}

	assertSystemComponents(t, bobConnection, game.TranslatableText("commands.clear.success.single", game.LiteralText("9"), game.LiteralText("Bob")))

	assertPacketIDs(t, observerConnection.packetIDs(t), []int32{protocol.ClientboundEntityEquipmentID})
	assertEmptyEquipmentUpdate(t, observerConnection.packets(t)[0], bob.Player.EntityID, protocol.EquipmentSlotMainHand)
}

func TestCommandClearFiltersByItem(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	bob, bobConnection := newCommandTestSession(runtime, commandTestBobUUID, "Bob")

	bob.Player.Inventory.Hotbar[0] = game.ItemStack{Item: game.ItemStone, Count: 3}
	bob.Player.Inventory.Main[0] = game.ItemStack{Item: game.ItemDirt, Count: 2}
	bob.Player.Inventory.Offhand = game.ItemStack{Item: game.ItemDirt, Count: 1}
	bob.activeMenu().carried = game.ItemStack{Item: game.ItemStone, Count: 5}

	joinTestSession(t, runtime, bob)

	bobConnection.reset()

	executeCommand(t, bob, "clear Bob stone")

	player := bob.snapshotPlayer()
	if !player.Inventory.Hotbar[0].Empty() || !bob.activeMenu().carried.Empty() {
		t.Fatalf("stone stacks after filtered clear = hotbar %+v carried %+v, want both empty", player.Inventory.Hotbar[0], bob.activeMenu().carried)
	}

	if !player.Inventory.Main[0].Equal(game.ItemStack{Item: game.ItemDirt, Count: 2}) {
		t.Fatalf("dirt stack after filtered clear = %+v, want untouched", player.Inventory.Main[0])
	}

	if !player.Inventory.Offhand.Equal(game.ItemStack{Item: game.ItemDirt, Count: 1}) {
		t.Fatalf("offhand dirt after filtered clear = %+v, want untouched", player.Inventory.Offhand)
	}

	assertSystemComponents(t, bobConnection, game.TranslatableText("commands.clear.success.single", game.LiteralText("8"), game.LiteralText("Bob")))

	bobConnection.reset()

	executeCommand(t, bob, "clear Bob dirt 0")

	assertSystemComponents(t, bobConnection, game.TranslatableText("commands.clear.test.single", game.LiteralText("3"), game.LiteralText("Bob")))

	if !bob.snapshotPlayer().Inventory.Main[0].Equal(game.ItemStack{Item: game.ItemDirt, Count: 2}) {
		t.Fatal("clear maxCount 0 mutated the inventory")
	}

	bobConnection.reset()

	executeCommand(t, bob, "clear Bob dirt 2")

	assertSystemComponents(t, bobConnection, game.TranslatableText("commands.clear.success.single", game.LiteralText("2"), game.LiteralText("Bob")))

	if !bob.snapshotPlayer().Inventory.Offhand.Equal(game.ItemStack{Item: game.ItemDirt, Count: 1}) {
		t.Fatal("clear maxCount 2 removed more than two items")
	}

	bobConnection.reset()

	executeCommand(t, bob, "clear Bob oak_log")

	assertSystemMessages(t, bobConnection, "No items were found on player Bob")

	assertPacketIDs(t, bobConnection.packetIDs(t), []int32{protocol.ClientboundSystemChatID})
}

func TestCommandTeleportRelativeCoordinates(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	bob, bobConnection := newCommandTestSession(runtime, commandTestBobUUID, "Bob")

	bob.Config = &config.Config{Server: config.ServerConfig{RenderDistance: new(int32(1))}}
	bob.Player.Position = game.Position{X: 8.5, Y: 70, Z: 8.5}
	bob.Player.Velocity = game.Velocity{X: 3, Y: 4, Z: 5}
	bob.Player.OnGround = true

	prepareCommandSessionChunks(bob)

	joinTestSession(t, runtime, bob)

	bobConnection.reset()

	executeCommand(t, bob, "teleport ~1 ~2 ~-1")

	expected := game.Position{X: 9.5, Y: 72, Z: 7.5}

	player := bob.snapshotPlayer()
	if player.Position != expected {
		t.Fatalf("position after relative teleport = %+v, want %+v", player.Position, expected)
	}

	if player.Velocity != (game.Velocity{X: 3, Z: 5}) || !player.OnGround {
		t.Fatalf("movement state after teleport = %+v on ground %v, want X/Z velocity retained, Y zero, and grounded", player.Velocity, player.OnGround)
	}

	assertPacketIDs(t, bobConnection.packetIDs(t), []int32{
		protocol.ClientboundPlayerPositionID,
		protocol.ClientboundSystemChatID,
	})

	assertPlayerPositionPacket(t, bobConnection.packets(t)[0], 1, expected)
	assertSystemComponents(t, bobConnection, game.TranslatableText("commands.teleport.success.location.single", game.LiteralText("Bob"), game.LiteralText("9.500000"), game.LiteralText("72.000000"), game.LiteralText("7.500000")))

	invalidTeleports := map[string]game.TextComponent{
		"teleport ^1 ^ ^":        game.TranslatableText("argument.entity.invalid"),
		"teleport abc 64 0":      game.TranslatableText("argument.player.unknown"),
		"teleport 30000001 64 0": game.TranslatableText("argument.player.unknown"),
	}

	for input, feedback := range invalidTeleports {
		bobConnection.reset()

		executeCommand(t, bob, input)

		position := bob.snapshotPlayer().Position
		if position != expected {
			t.Fatalf("position after %q = %+v, want %+v", input, position, expected)
		}

		assertSyntaxError(t, bobConnection, input, len("teleport "), feedback)
	}

	bobConnection.reset()

	executeCommand(t, bob, "tp 100.5 64 -200.25")

	absolute := game.Position{X: 100.5, Y: 64, Z: -200.25}

	positionPackets := packetsByID(t, bobConnection, protocol.ClientboundPlayerPositionID)
	if len(positionPackets) != 1 {
		t.Fatalf("position packets after absolute teleport = %d, want 1", len(positionPackets))
	}

	assertPlayerPositionPacket(t, positionPackets[0], 2, absolute)

	position := bob.snapshotPlayer().Position
	if position != absolute {
		t.Fatalf("position after absolute teleport = %+v, want %+v", position, absolute)
	}
}

func TestCommandTeleportToPlayerAndMultipleTargets(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	bob, bobConnection := newCommandTestSession(runtime, commandTestBobUUID, "Bob")
	alice, aliceConnection := newCommandTestSession(runtime, commandTestAliceUUID, "Alice")

	bob.Player.Position = game.Position{X: 8.5, Y: 70, Z: 8.5}
	alice.Player.Position = game.Position{X: 8.5, Y: 75, Z: 8.5}

	tpSessions := []*Session{bob, alice}

	for _, session := range tpSessions {
		session.Config = &config.Config{Server: config.ServerConfig{RenderDistance: new(int32(1))}}

		prepareCommandSessionChunks(session)
	}

	joinTestSession(t, runtime, bob)
	joinTestSession(t, runtime, alice)

	bobConnection.reset()
	aliceConnection.reset()

	executeCommand(t, bob, "tp Alice")

	destination := game.Position{X: 8.5, Y: 75, Z: 8.5}

	position := bob.snapshotPlayer().Position
	if position != destination {
		t.Fatalf("bob position after teleport to alice = %+v, want %+v", position, destination)
	}

	positionPackets := packetsByID(t, bobConnection, protocol.ClientboundPlayerPositionID)
	if len(positionPackets) != 1 {
		t.Fatalf("bob position packets = %d, want 1", len(positionPackets))
	}

	assertPlayerPositionPacket(t, positionPackets[0], 1, destination)

	bobConnection.reset()
	aliceConnection.reset()

	shared := game.Position{X: 5.5, Y: 69, Z: 5.5}

	executeCommand(t, bob, "tp @a 5.5 69 5.5")

	for _, session := range tpSessions {
		position := session.snapshotPlayer().Position
		if position != shared {
			t.Fatalf("%s position after @a teleport = %+v, want %+v", session.Player.Name, position, shared)
		}
	}

	bobPositionPackets := packetsByID(t, bobConnection, protocol.ClientboundPlayerPositionID)
	alicePositionPackets := packetsByID(t, aliceConnection, protocol.ClientboundPlayerPositionID)

	if len(bobPositionPackets) != 1 || len(alicePositionPackets) != 1 {
		t.Fatalf("position packets after @a teleport = bob %d alice %d, want one each", len(bobPositionPackets), len(alicePositionPackets))
	}

	assertPlayerPositionPacket(t, bobPositionPackets[0], 2, shared)
	assertPlayerPositionPacket(t, alicePositionPackets[0], 1, shared)

	bobConnection.reset()

	executeCommand(t, bob, "tp @a")

	assertSyntaxError(t, bobConnection, "tp @a", 5, game.TranslatableText("command.unknown.argument"))
}

func TestCommandSetBlockMutatesWorldAndNotifiesSessions(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	bob, bobConnection := newCommandTestSession(runtime, commandTestBobUUID, "Bob")

	position := game.BlockPosition{X: 3, Y: 70, Z: -2}

	bob.Player.Position = blockMutationTestPlayerPosition(position)

	markChunkLoaded(bob, position)

	joinTestSession(t, runtime, bob)

	bobConnection.reset()

	executeCommand(t, bob, "setblock 3 70 -2 stone")

	block := runtime.World.BlockAt(position)
	if block != game.Stone {
		t.Fatalf("block after setblock = %d, want stone", block)
	}

	assertPacketIDs(t, bobConnection.packetIDs(t), []int32{
		protocol.ClientboundBlockUpdateID,
		protocol.ClientboundSystemChatID,
	})

	assertBlockUpdate(t, bobConnection.packets(t)[0], position, protocol.StoneBlockState)
	assertSystemComponents(t, bobConnection, game.TranslatableText("commands.setblock.success", game.LiteralText("3"), game.LiteralText("70"), game.LiteralText("-2")))

	bobConnection.reset()

	executeCommand(t, bob, "setblock 3 70 -2 dirt")

	dirtState, err := protocolBlockState(game.Dirt)
	if err != nil {
		t.Fatalf("encode dirt state: %v", err)
	}

	block = runtime.World.BlockAt(position)
	if block != game.Dirt {
		t.Fatalf("block after replacing = %d, want dirt", block)
	}

	assertBlockUpdate(t, bobConnection.packets(t)[0], position, dirtState)
	assertSystemComponents(t, bobConnection, game.TranslatableText("commands.setblock.success", game.LiteralText("3"), game.LiteralText("70"), game.LiteralText("-2")))

	bobConnection.reset()

	executeCommand(t, bob, "setblock 3 70 -2 dirt")

	assertSystemComponents(t, bobConnection, game.TranslatableText("commands.setblock.failed").WithColor(game.TextColorRed))
	assertPacketIDs(t, bobConnection.packetIDs(t), []int32{protocol.ClientboundSystemChatID})

	bobConnection.reset()

	executeCommand(t, bob, "setblock 3 70 -2 bogus")

	assertSyntaxError(t, bobConnection, "setblock 3 70 -2 bogus", 17, game.LiteralText("Unknown block 'bogus'"))

	block = runtime.World.BlockAt(position)
	if block != game.Dirt {
		t.Fatalf("block after unknown block = %d, want dirt", block)
	}

	bobConnection.reset()

	executeCommand(t, bob, "setblock ~1 ~-1 ~ cobblestone")

	relative := game.BlockPosition{X: 4, Y: 70, Z: -2}

	block = runtime.World.BlockAt(relative)
	if block != game.Cobblestone {
		t.Fatalf("relative block after setblock = %d, want cobblestone", block)
	}

	cobblestoneState, err := protocolBlockState(game.Cobblestone)
	if err != nil {
		t.Fatalf("encode cobblestone state: %v", err)
	}

	assertBlockUpdate(t, bobConnection.packets(t)[0], relative, cobblestoneState)
	assertSystemComponents(t, bobConnection, game.TranslatableText("commands.setblock.success", game.LiteralText("4"), game.LiteralText("70"), game.LiteralText("-2")))
}

func TestCommandSetBlockInsidePlayerRecalculatesPose(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	bob, bobConnection := newCommandTestSession(runtime, commandTestBobUUID, "Bob")

	bob.Player.Position = game.Position{X: 8.5, Y: 70, Z: 8.5}

	headPosition := game.BlockPosition{X: 8, Y: 71, Z: 8}

	markChunkLoaded(bob, headPosition)

	joinTestSession(t, runtime, bob)

	bobConnection.reset()

	executeCommand(t, bob, "setblock 8 71 8 stone")

	block := runtime.World.BlockAt(headPosition)
	if block != game.Stone {
		t.Fatalf("block placed inside the player = %d, want stone", block)
	}

	pose := bob.snapshotPlayer().Pose
	if pose != game.PlayerPoseCrawling {
		t.Fatalf("pose after placing a block at head height = %d, want crawling", pose)
	}

	assertSystemComponents(t, bobConnection, game.TranslatableText("commands.setblock.success", game.LiteralText("8"), game.LiteralText("71"), game.LiteralText("8")))
}

func TestCommandBlockMutationModes(t *testing.T) {
	runtime := NewRuntime(&game.World{})
	bob, connection := newCommandTestSession(runtime, commandTestBobUUID, "Bob")

	executeCommand(t, bob, "setblock 0 70 0 stone replace")

	block := runtime.World.BlockAt(game.BlockPosition{X: 0, Y: 70, Z: 0})
	if block != game.Stone {
		t.Fatalf("replace setblock = %d, want stone", block)
	}

	connection.reset()

	executeCommand(t, bob, "setblock 0 70 0 dirt keep")

	assertSystemComponents(t, connection, game.TranslatableText("commands.setblock.failed").WithColor(game.TextColorRed))

	executeCommand(t, bob, "setblock 0 70 0 dirt strict")

	block = runtime.World.BlockAt(game.BlockPosition{X: 0, Y: 70, Z: 0})
	if block != game.Dirt {
		t.Fatalf("strict setblock = %d, want dirt", block)
	}

	executeCommand(t, bob, "fill 0 70 0 2 72 2 stone replace")
	executeCommand(t, bob, "fill 0 70 0 2 72 2 dirt outline")

	block = runtime.World.BlockAt(game.BlockPosition{X: 1, Y: 71, Z: 1})
	if block != game.Stone {
		t.Fatalf("outline fill changed center to %d, want stone", block)
	}

	executeCommand(t, bob, "fill 0 70 0 2 72 2 dirt hollow")

	block = runtime.World.BlockAt(game.BlockPosition{X: 1, Y: 71, Z: 1})
	if block != game.Air {
		t.Fatalf("hollow fill center = %d, want air", block)
	}

	executeCommand(t, bob, "fill 0 70 0 2 72 2 stone keep")

	block = runtime.World.BlockAt(game.BlockPosition{X: 1, Y: 71, Z: 1})
	if block != game.Stone {
		t.Fatalf("keep fill did not fill air center: %d", block)
	}

	executeCommand(t, bob, "fill 0 70 0 0 70 0 cobblestone strict")

	block = runtime.World.BlockAt(game.BlockPosition{X: 0, Y: 70, Z: 0})
	if block != game.Cobblestone {
		t.Fatalf("strict fill = %d, want cobblestone", block)
	}
}

func TestCommandFillMutatesRegion(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	bob, bobConnection := newCommandTestSession(runtime, commandTestBobUUID, "Bob")

	corner := game.BlockPosition{X: 0, Y: 70, Z: 0}
	opposite := game.BlockPosition{X: 1, Y: 70, Z: 1}

	markPlacementChunksLoaded(bob, corner, opposite)

	joinTestSession(t, runtime, bob)

	bobConnection.reset()

	executeCommand(t, bob, "fill 0 70 0 1 70 1 stone")

	region := []game.BlockPosition{
		{X: 0, Y: 70, Z: 0},
		{X: 0, Y: 70, Z: 1},
		{X: 1, Y: 70, Z: 0},
		{X: 1, Y: 70, Z: 1},
	}

	for _, position := range region {
		block := runtime.World.BlockAt(position)
		if block != game.Stone {
			t.Fatalf("block at %+v after fill = %d, want stone", position, block)
		}
	}

	packets := bobConnection.packets(t)
	if len(packets) != len(region)+1 {
		t.Fatalf("fill packets = %d, want one update per change plus feedback", len(packets))
	}

	for index, position := range region {
		assertBlockUpdate(t, packets[index], position, protocol.StoneBlockState)
	}

	assertSystemComponents(t, bobConnection, game.TranslatableText("commands.fill.success", game.LiteralText("4")))

	bobConnection.reset()

	executeCommand(t, bob, "fill 0 70 0 1 70 1 stone")

	assertSystemComponents(t, bobConnection, game.TranslatableText("commands.fill.failed").WithColor(game.TextColorRed))
	assertPacketIDs(t, bobConnection.packetIDs(t), []int32{protocol.ClientboundSystemChatID})

	bobConnection.reset()

	executeCommand(t, bob, "fill 1 70 1 0 70 0 dirt")

	for _, position := range region {
		block := runtime.World.BlockAt(position)
		if block != game.Dirt {
			t.Fatalf("block at %+v after reversed fill = %d, want dirt", position, block)
		}
	}

	assertSystemComponents(t, bobConnection, game.TranslatableText("commands.fill.success", game.LiteralText("4")))
}

func TestCommandFillVolumeLimits(t *testing.T) {
	t.Run("boundary volume fills every block", func(t *testing.T) {
		runtime := NewRuntime(&game.World{})

		bob, bobConnection := newCommandTestSession(runtime, commandTestBobUUID, "Bob")

		executeCommand(t, bob, "fill 0 0 0 31 31 31 stone")

		assertSystemComponents(t, bobConnection, game.TranslatableText("commands.fill.success", game.LiteralText("32768")))

		samples := []game.BlockPosition{
			{X: 0, Y: 0, Z: 0},
			{X: 31, Y: 31, Z: 31},
			{X: 16, Y: 16, Z: 16},
		}

		for _, position := range samples {
			block := runtime.World.BlockAt(position)
			if block != game.Stone {
				t.Fatalf("block at %+v = %d, want stone", position, block)
			}
		}
	})

	t.Run("volume overflow is rejected without changes", func(t *testing.T) {
		runtime := NewRuntime(&game.World{})

		bob, bobConnection := newCommandTestSession(runtime, commandTestBobUUID, "Bob")

		executeCommand(t, bob, "fill 0 0 0 32 31 31 stone")

		assertSystemComponents(t, bobConnection, game.TranslatableText("commands.fill.toobig", game.LiteralText("32768"), game.LiteralText("33792")).WithColor(game.TextColorRed))

		volumeOverflowPositions := []game.BlockPosition{{X: 0, Y: 0, Z: 0}, {X: 32, Y: 31, Z: 31}}

		for _, position := range volumeOverflowPositions {
			block := runtime.World.BlockAt(position)
			if block != game.Air {
				t.Fatalf("block at %+v after rejected fill = %d, want air", position, block)
			}
		}
	})

	t.Run("axis overflow is rejected without changes", func(t *testing.T) {
		runtime := NewRuntime(&game.World{})

		bob, bobConnection := newCommandTestSession(runtime, commandTestBobUUID, "Bob")

		executeCommand(t, bob, "fill 0 0 0 32768 0 0 stone")

		assertSystemComponents(t, bobConnection, game.TranslatableText("commands.fill.toobig", game.LiteralText("32768"), game.LiteralText("32769")).WithColor(game.TextColorRed))

		axisOverflowPositions := []game.BlockPosition{{X: 0, Y: 0, Z: 0}, {X: 32768, Y: 0, Z: 0}}

		for _, position := range axisOverflowPositions {
			block := runtime.World.BlockAt(position)
			if block != game.Air {
				t.Fatalf("block at %+v after rejected fill = %d, want air", position, block)
			}
		}
	})

	t.Run("partially filled region commits the remainder", func(t *testing.T) {
		runtime := NewRuntime(&game.World{})

		bob, bobConnection := newCommandTestSession(runtime, commandTestBobUUID, "Bob")

		executeCommand(t, bob, "setblock 0 70 0 dirt")

		bobConnection.reset()

		executeCommand(t, bob, "fill 0 70 0 1 70 0 dirt")

		assertSystemComponents(t, bobConnection, game.TranslatableText("commands.fill.success", game.LiteralText("1")))

		block := runtime.World.BlockAt(game.BlockPosition{X: 0, Y: 70, Z: 0})
		if block != game.Dirt {
			t.Fatalf("existing block after partial fill = %d, want dirt", block)
		}

		block = runtime.World.BlockAt(game.BlockPosition{X: 1, Y: 70, Z: 0})
		if block != game.Dirt {
			t.Fatalf("missing block after partial fill = %d, want dirt", block)
		}
	})
}

func TestCommandDispatchThroughPlayPackets(t *testing.T) {
	runtime := NewRuntime(&game.World{Seed: -987})

	bob, bobConnection := newCommandTestSession(runtime, commandTestBobUUID, "Bob")

	var commandWriter protocol.PacketWriter

	commandWriter.String("/seed")

	err := bob.handlePlayPacket(&protocol.Packet{
		ID:   protocol.ServerboundChatCommandID,
		Data: commandWriter.Buffer.Bytes(),
	})

	if err != nil {
		t.Fatalf("handle chat command packet: %v", err)
	}

	assertSystemComponents(t, bobConnection, game.TranslatableText("commands.seed.success", game.LiteralText("-987").WithColor(game.TextColorGreen).WithClickEvent(game.ClickCopyToClipboard, "-987")))

	var requestWriter protocol.PacketWriter

	requestWriter.VarInt(7)
	requestWriter.String("/he")

	err = bob.handlePlayPacket(&protocol.Packet{
		ID:   protocol.ServerboundCommandSuggestionsID,
		Data: requestWriter.Buffer.Bytes(),
	})

	if err != nil {
		t.Fatalf("handle command suggestion packet: %v", err)
	}

	suggestionPackets := packetsByID(t, bobConnection, protocol.ClientboundCommandSuggestionsID)
	if len(suggestionPackets) != 1 {
		t.Fatalf("command suggestion packets = %d, want 1", len(suggestionPackets))
	}

	suggestions := decodeCommandSuggestionsPacket(t, suggestionPackets[0])
	if suggestions.TransactionID != 7 {
		t.Fatalf("suggestion transaction id = %d, want 7", suggestions.TransactionID)
	}

	assertCommandSuggestions(t, suggestions, 1, 2, []string{"help"})
}

func newCommandTestSession(runtime *Runtime, uuid, name string) (*Session, *recordingConnection) {
	session, connection := newMovementTestSession(runtime, uuid, name)

	session.Log = &chatTestLogger{}

	return session, connection
}

func executeCommand(t *testing.T, session *Session, input string) {
	t.Helper()

	err := session.Runtime.commands.execute(playerCommandSource{session: session}, input)
	if err != nil {
		t.Fatalf("execute command %q: %v", input, err)
	}
}

func prepareCommandSessionChunks(session *Session) {
	center := LoadedChunk{
		X: chunkCoordinate(session.Player.Position.X),
		Z: chunkCoordinate(session.Player.Position.Z),
	}

	loaded := make(map[LoadedChunk]struct{})

	for _, chunk := range chunksInView(center, session.renderDistance()) {
		loaded[chunk] = struct{}{}
	}

	session.chunkMx.Lock()
	session.centerChunk = center
	session.hasChunkCenter = true
	session.loadedChunks = loaded
	session.chunkMx.Unlock()
}

func commandNodeChild(nodes []protocol.CommandNode, parent *protocol.CommandNode, nodeType byte, name string) *protocol.CommandNode {
	for _, childIndex := range parent.Children {
		child := &nodes[childIndex]
		if child.Type == nodeType && child.Name == name {
			return child
		}
	}

	return nil
}

func indexCommandNode(nodes []protocol.CommandNode, target *protocol.CommandNode) int {
	for index := range nodes {
		if &nodes[index] == target {
			return index
		}
	}

	return -1
}

func assertSystemComponents(t *testing.T, connection *recordingConnection, expected ...game.TextComponent) {
	t.Helper()

	var actual []game.TextComponent

	for _, packet := range connection.packets(t) {
		if packet.ID == protocol.ClientboundSystemChatID {
			actual = append(actual, decodeSystemComponent(t, packet.Data))
		}
	}

	if !reflect.DeepEqual(actual, expected) {
		t.Fatalf("system components = %#v, want %#v", actual, expected)
	}
}

func assertSyntaxError(t *testing.T, connection *recordingConnection, input string, cursor int, message game.TextComponent) {
	t.Helper()

	contextStart := max(0, cursor-10)
	prefix := input[contextStart:cursor]

	if contextStart > 0 {
		prefix = "..." + prefix
	}

	context := game.LiteralText(prefix).WithColor(game.TextColorGray)
	invalid := game.LiteralText(input[cursor:]).WithColor(game.TextColorRed).WithUnderline(true)

	if invalid.Text != "" {
		context = context.Append(invalid)
	}

	context = context.Append(game.TranslatableText("command.context.here").WithColor(game.TextColorRed).WithItalic(true))
	context = context.WithClickEvent(game.ClickSuggestCommand, "/"+input)

	assertSystemComponents(t, connection, message.WithColor(game.TextColorRed), context)
}

func packetsByID(t *testing.T, connection *recordingConnection, packetID int32) []protocol.Packet {
	t.Helper()

	var packets []protocol.Packet

	for _, packet := range connection.packets(t) {
		if packet.ID == packetID {
			packets = append(packets, packet)
		}
	}

	return packets
}

func commandSuggestionTexts(suggestions protocol.CommandSuggestions) []string {
	texts := make([]string, 0, len(suggestions.Matches))

	for _, match := range suggestions.Matches {
		texts = append(texts, match.Text)
	}

	return texts
}

func assertCommandSuggestions(t *testing.T, suggestions protocol.CommandSuggestions, start, length int32, matches []string) {
	t.Helper()

	if suggestions.Start != start || suggestions.Length != length {
		t.Fatalf("suggestion window = (%d, %d), want (%d, %d)", suggestions.Start, suggestions.Length, start, length)
	}

	actual := commandSuggestionTexts(suggestions)
	if strings.Join(actual, ",") != strings.Join(matches, ",") {
		t.Fatalf("suggestions = %v, want %v", actual, matches)
	}
}

func decodeCommandSuggestionsPacket(t *testing.T, packet protocol.Packet) protocol.CommandSuggestions {
	t.Helper()

	reader := protocol.NewPacketReader(packet.Data)

	suggestions := protocol.CommandSuggestions{
		TransactionID: reader.VarInt(),
		Start:         reader.VarInt(),
		Length:        reader.VarInt(),
	}

	matchCount := reader.VarInt()

	for range matchCount {
		match := protocol.CommandSuggestion{Text: reader.String(32767)}

		match.HasTooltip = reader.Bool()

		suggestions.Matches = append(suggestions.Matches, match)
	}

	err := reader.Done("command suggestions")
	if err != nil {
		t.Fatalf("decode command suggestions: %v", err)
	}

	return suggestions
}

func decodeInventorySnapshot(t *testing.T, packet protocol.Packet) (int32, []game.ItemStack, game.ItemStack) {
	t.Helper()

	reader := protocol.NewPacketReader(packet.Data)

	windowID := reader.VarInt()
	if windowID != playerInventoryWindowID {
		t.Fatalf("inventory window id = %d, want %d", windowID, playerInventoryWindowID)
	}

	stateID := reader.VarInt()

	itemCount := reader.VarInt()
	if itemCount != game.PlayerInventorySlots {
		t.Fatalf("inventory item count = %d, want %d", itemCount, game.PlayerInventorySlots)
	}

	items := make([]game.ItemStack, game.PlayerInventorySlots)
	for slot := range items {
		items[slot] = readSimpleItemStack(t, reader)
	}

	carried := readSimpleItemStack(t, reader)

	err := reader.Done("inventory snapshot")
	if err != nil {
		t.Fatalf("decode inventory snapshot: %v", err)
	}

	return stateID, items, carried
}

func assertPlayerPositionPacket(t *testing.T, packet protocol.Packet, teleportID int32, position game.Position) {
	t.Helper()

	reader := protocol.NewPacketReader(packet.Data)

	actualTeleportID := reader.VarInt()
	if actualTeleportID != teleportID {
		t.Fatalf("teleport id = %d, want %d", actualTeleportID, teleportID)
	}

	actualPosition := game.Position{X: reader.Double(), Y: reader.Double(), Z: reader.Double()}
	if actualPosition != position {
		t.Fatalf("teleport position = %+v, want %+v", actualPosition, position)
	}

	reader.Double()
	reader.Double()
	reader.Double()
	reader.Float()
	reader.Float()
	reader.Int()

	err := reader.Done("player position")
	if err != nil {
		t.Fatalf("decode player position: %v", err)
	}
}

func assertGameModeEventPacket(t *testing.T, packet protocol.Packet, mode game.GameMode) {
	t.Helper()

	reader := protocol.NewPacketReader(packet.Data)

	event := reader.Byte()
	value := reader.Float()

	if event != gameEventChangeGameMode || value != float32(mode) {
		t.Fatalf("game event = (%d, %v), want game mode change to %d", event, value, mode)
	}

	err := reader.Done("game event")
	if err != nil {
		t.Fatalf("decode game event: %v", err)
	}
}

func assertGameModeInfoPacket(t *testing.T, packet protocol.Packet, uuid string, mode game.GameMode) {
	t.Helper()

	reader := protocol.NewPacketReader(packet.Data)

	actions := reader.Byte()
	playerCount := reader.VarInt()
	actualUUID := reader.UUID()
	actualMode := reader.VarInt()

	if actions != protocol.PlayerInfoActionUpdateGameMode || playerCount != 1 || actualUUID != uuid || actualMode != int32(mode) {
		t.Fatalf("player info update = actions %d count %d uuid %s mode %d, want game mode %d for %s", actions, playerCount, actualUUID, actualMode, mode, uuid)
	}

	err := reader.Done("player info update")
	if err != nil {
		t.Fatalf("decode player info update: %v", err)
	}
}

func assertTimeUpdatePacket(t *testing.T, packet protocol.Packet, age, dayTime int64, dayCycle bool) {
	t.Helper()

	reader := protocol.NewPacketReader(packet.Data)

	actualAge := reader.Long()
	actualDayTime := reader.Long()
	actualDayCycle := reader.Bool()

	if actualAge != age || actualDayTime != dayTime || actualDayCycle != dayCycle {
		t.Fatalf("time update = (%d, %d, %v), want (%d, %d, %v)", actualAge, actualDayTime, actualDayCycle, age, dayTime, dayCycle)
	}

	err := reader.Done("time update")
	if err != nil {
		t.Fatalf("decode time update: %v", err)
	}
}
