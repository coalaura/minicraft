package server

import (
	"fmt"
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

	expectedCommands := []string{"help", "seed", "time", "gamemode", "give", "clear", "tp", "setblock", "fill"}
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
	if helpCommand == nil || !helpCommand.Executable || helpCommand.Parser != protocol.CommandParserString {
		t.Fatalf("help command argument = %+v", helpCommand)
	}

	if helpCommand.SuggestionType != commandSuggestionProvider {
		t.Fatalf("help command suggestion type = %q, want %q", helpCommand.SuggestionType, commandSuggestionProvider)
	}

	timeLiteral := commandNodeChild(declaration.Nodes, root, protocol.CommandNodeLiteral, "time")
	query := commandNodeChild(declaration.Nodes, timeLiteral, protocol.CommandNodeLiteral, "query")

	if query == nil || !query.Executable || len(query.Children) != 0 {
		t.Fatalf("time query node = %+v, want executable leaf", query)
	}

	setLiteral := commandNodeChild(declaration.Nodes, timeLiteral, protocol.CommandNodeLiteral, "set")
	timeValue := commandNodeChild(declaration.Nodes, setLiteral, protocol.CommandNodeArgument, "time")

	if timeValue == nil || !timeValue.Executable || timeValue.Parser != protocol.CommandParserString {
		t.Fatalf("time value argument = %+v", timeValue)
	}

	gamemodeLiteral := commandNodeChild(declaration.Nodes, root, protocol.CommandNodeLiteral, "gamemode")
	mode := commandNodeChild(declaration.Nodes, gamemodeLiteral, protocol.CommandNodeArgument, "mode")

	if mode == nil || !mode.Executable || mode.Parser != protocol.CommandParserGameMode {
		t.Fatalf("gamemode mode argument = %+v", mode)
	}

	gamemodeTargets := commandNodeChild(declaration.Nodes, mode, protocol.CommandNodeArgument, "targets")
	if gamemodeTargets == nil || !gamemodeTargets.Executable || gamemodeTargets.Parser != protocol.CommandParserEntity {
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

	if giveTargets == nil || giveTargets.Executable || giveTargets.Parser != protocol.CommandParserEntity {
		t.Fatalf("give targets argument = %+v", giveTargets)
	}

	if item == nil || !item.Executable || item.Parser != protocol.CommandParserItemStack {
		t.Fatalf("give item argument = %+v", item)
	}

	if count == nil || !count.Executable || count.Parser != protocol.CommandParserInteger {
		t.Fatalf("give count argument = %+v", count)
	}

	countProperties, validProperties := count.Properties.(protocol.CommandIntegerProperties)
	if !validProperties || !countProperties.HasMin || !countProperties.HasMax || countProperties.Min != 1 || countProperties.Max != 32767 {
		t.Fatalf("give count properties = %+v", count.Properties)
	}

	tpLiteral := commandNodeChild(declaration.Nodes, root, protocol.CommandNodeLiteral, "tp")
	location := commandNodeChild(declaration.Nodes, tpLiteral, protocol.CommandNodeArgument, "location")
	destination := commandNodeChild(declaration.Nodes, tpLiteral, protocol.CommandNodeArgument, "destination")
	tpTargets := commandNodeChild(declaration.Nodes, tpLiteral, protocol.CommandNodeArgument, "targets")

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

	if setblockBlock == nil || !setblockBlock.Executable || setblockBlock.Parser != protocol.CommandParserBlockState {
		t.Fatalf("setblock block argument = %+v", setblockBlock)
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

	if fillBlock == nil || !fillBlock.Executable || fillBlock.Parser != protocol.CommandParserBlockState {
		t.Fatalf("fill block argument = %+v", fillBlock)
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

	expectedHelp := strings.Join([]string{
		"/help [command] - Shows available commands or help for one command.",
		"/seed - Shows the world seed.",
		"/time query | /time set <day|noon|night|midnight|ticks> - Queries or sets the world time.",
		"/gamemode <survival|creative|adventure|spectator> [targets] - Changes game mode for one or more players.",
		"/give <targets> <item> [count] - Adds items when every requested stack fits.",
		"/clear [targets] [item] - Clears all or matching inventory items.",
		"/tp <destination|x y z> | /tp <targets> <destination|x y z> - Teleports players to a player or coordinates.",
		"/setblock <x> <y> <z> <block> - Sets one block through authoritative world mutation.",
		"/fill <x1> <y1> <z1> <x2> <y2> <z2> <block> - Atomically fills up to 32768 blocks.",
	}, "\n")

	assertSystemMessages(t, connection, expectedHelp)

	connection.reset()

	executeCommand(t, session, "help seed")

	assertSystemMessages(t, connection, "/seed - Shows the world seed.")

	connection.reset()

	executeCommand(t, session, "seed")

	assertSystemMessages(t, connection, "Seed: 12345")

	connection.reset()

	executeCommand(t, session, "SEED")

	assertSystemMessages(t, connection, "Seed: 12345")

	connection.reset()

	executeCommand(t, session, "help bogus")

	assertSystemMessages(t, connection, "Unknown command \"bogus\"\nUsage: /help [command]")

	connection.reset()

	executeCommand(t, session, "seed extra")

	assertSystemMessages(t, connection, "Invalid command syntax\nUsage: /seed")

	connection.reset()

	executeCommand(t, session, "bogus")

	assertSystemMessages(t, connection, "Unknown command: bogus")

	connection.reset()

	executeCommand(t, session, "/")

	assertSystemMessages(t, connection, "Unknown command")
}

func TestCommandTimeQueryAndSet(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	session, connection := newCommandTestSession(runtime, commandTestBobUUID, "Bob")

	executeCommand(t, session, "time query")

	assertSystemMessages(t, connection, "Time: 0")

	presets := map[string]int64{
		"day":      1000,
		"noon":     6000,
		"night":    13000,
		"midnight": 18000,
	}

	for _, preset := range []string{"day", "noon", "night", "midnight"} {
		connection.reset()

		executeCommand(t, session, "time set "+preset)

		assertSystemMessages(t, connection, fmt.Sprintf("Set time to %d", presets[preset]))

		dayTime := runtime.World.Time().DayTime
		if dayTime != presets[preset] {
			t.Fatalf("world day time after %s = %d, want %d", preset, dayTime, presets[preset])
		}
	}

	connection.reset()

	executeCommand(t, session, "time set 4242")

	assertSystemMessages(t, connection, "Set time to 4242")

	dayTime := runtime.World.Time().DayTime
	if dayTime != 4242 {
		t.Fatalf("world day time = %d, want 4242", dayTime)
	}

	connection.reset()

	executeCommand(t, session, "time set -1")

	assertSystemMessages(t, connection, "Invalid time \"-1\"\nUsage: /time query | /time set <day|noon|night|midnight|ticks>")

	connection.reset()

	executeCommand(t, session, "time bogus")

	assertSystemMessages(t, connection, "Expected \"query\"\nUsage: /time query | /time set <day|noon|night|midnight|ticks>")
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
	assertSystemMessages(t, connection, "Set time to 1000")
}

func TestCommandTargetsPlayersByName(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	bob, bobConnection := newCommandTestSession(runtime, commandTestBobUUID, "Bob")
	alice, _ := newCommandTestSession(runtime, commandTestAliceUUID, "Alice")

	joinTestSession(t, runtime, bob)
	joinTestSession(t, runtime, alice)

	bobConnection.reset()

	executeCommand(t, bob, "gamemode spectator Alice")

	if mode := alice.snapshotPlayer().GameMode; mode != game.GameModeSpectator {
		t.Fatalf("alice game mode = %d, want spectator", mode)
	}

	if mode := bob.snapshotPlayer().GameMode; mode != game.GameModeSurvival {
		t.Fatalf("bob game mode = %d, want survival", mode)
	}

	bobConnection.reset()

	executeCommand(t, bob, "gamemode creative alice")

	if mode := alice.snapshotPlayer().GameMode; mode != game.GameModeCreative {
		t.Fatalf("alice game mode after case-insensitive target = %d, want creative", mode)
	}

	bobConnection.reset()

	executeCommand(t, bob, "gamemode creative Nobody")

	assertSystemMessages(t, bobConnection, "No player matched \"Nobody\"\nUsage: /gamemode <survival|creative|adventure|spectator> [targets]")

	bobConnection.reset()

	executeCommand(t, bob, "gamemode creative not!valid")

	assertSystemMessages(t, bobConnection, "Invalid player name \"not!valid\"\nUsage: /gamemode <survival|creative|adventure|spectator> [targets]")
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

	if mode := zed.snapshotPlayer().GameMode; mode != game.GameModeAdventure {
		t.Fatalf("zed game mode after @s = %d, want adventure", mode)
	}

	if mode := bob.snapshotPlayer().GameMode; mode != game.GameModeSurvival {
		t.Fatalf("bob game mode after @s = %d, want survival", mode)
	}

	executeCommand(t, zed, "gamemode creative @a")

	for _, session := range []*Session{zed, bob, alice} {
		if mode := session.snapshotPlayer().GameMode; mode != game.GameModeCreative {
			t.Fatalf("%s game mode after @a = %d, want creative", session.Player.Name, mode)
		}
	}

	executeCommand(t, zed, "gamemode spectator @p")

	if mode := bob.snapshotPlayer().GameMode; mode != game.GameModeSpectator {
		t.Fatalf("bob game mode after @p = %d, want spectator", mode)
	}

	if mode := zed.snapshotPlayer().GameMode; mode != game.GameModeCreative {
		t.Fatalf("zed game mode after @p = %d, want creative", mode)
	}

	runtime.commandRandomMu.Lock()

	runtime.commandRandom = func(int) int {
		return 0
	}

	runtime.commandRandomMu.Unlock()

	executeCommand(t, zed, "gamemode adventure @r")

	if mode := alice.snapshotPlayer().GameMode; mode != game.GameModeAdventure {
		t.Fatalf("alice game mode after @r = %d, want adventure", mode)
	}

	if mode := bob.snapshotPlayer().GameMode; mode != game.GameModeSpectator {
		t.Fatalf("bob game mode after @r = %d, want spectator", mode)
	}
}

func TestCommandRejectsUnsupportedSelectors(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	bob, bobConnection := newCommandTestSession(runtime, commandTestBobUUID, "Bob")

	joinTestSession(t, runtime, bob)

	executeCommand(t, bob, "gamemode creative @e")

	assertSystemMessages(t, bobConnection, "Unsupported selector \"@e\"\nUsage: /gamemode <survival|creative|adventure|spectator> [targets]")

	bobConnection.reset()

	executeCommand(t, bob, "gamemode creative @a[gamemode=survival]")

	assertSystemMessages(t, bobConnection, "Selector options are not supported\nUsage: /gamemode <survival|creative|adventure|spectator> [targets]")

	if mode := bob.snapshotPlayer().GameMode; mode != game.GameModeSurvival {
		t.Fatalf("bob game mode after rejected selectors = %d, want survival", mode)
	}
}

func TestCommandSuggestions(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	bob, _ := newCommandTestSession(runtime, commandTestBobUUID, "Bob")
	alice, _ := newCommandTestSession(runtime, commandTestAliceUUID, "Alice")

	joinTestSession(t, runtime, bob)
	joinTestSession(t, runtime, alice)

	source := playerCommandSource{session: bob}

	commandNames := []string{"clear", "fill", "gamemode", "give", "help", "seed", "setblock", "time", "tp"}
	targets := []string{"@a", "@p", "@r", "@s", "Alice", "Bob"}

	assertCommandSuggestions(t, runtime.commands.suggestions(source, "/"), 1, 0, commandNames)
	assertCommandSuggestions(t, runtime.commands.suggestions(source, "/he"), 1, 2, []string{"help"})
	assertCommandSuggestions(t, runtime.commands.suggestions(source, "/help"), 1, 4, []string{"help"})
	assertCommandSuggestions(t, runtime.commands.suggestions(source, "/help "), 6, 0, commandNames)
	assertCommandSuggestions(t, runtime.commands.suggestions(source, "/gamemode "), 10, 0, []string{"adventure", "creative", "spectator", "survival"})
	assertCommandSuggestions(t, runtime.commands.suggestions(source, "/gamemode creative "), 19, 0, targets)
	assertCommandSuggestions(t, runtime.commands.suggestions(source, "/time set "), 10, 0, []string{"day", "midnight", "night", "noon"})
	assertCommandSuggestions(t, runtime.commands.suggestions(source, "/time s"), 6, 1, []string{"set"})
	assertCommandSuggestions(t, runtime.commands.suggestions(source, "/give "), 6, 0, targets)
	assertCommandSuggestions(t, runtime.commands.suggestions(source, "/tp "), 4, 0, targets)
	assertCommandSuggestions(t, runtime.commands.suggestions(source, "/tp @"), 4, 1, []string{"@a", "@p", "@r", "@s"})
	assertCommandSuggestions(t, runtime.commands.suggestions(source, "/give Bob "), 10, 0, game.ItemNames)
	assertCommandSuggestions(t, runtime.commands.suggestions(source, "/give Bob stone_a"), 10, 7, []string{"stone_axe"})
	assertCommandSuggestions(t, runtime.commands.suggestions(source, "/setblock 0 70 0 "), 17, 0, game.BlockNames)
	assertCommandSuggestions(t, runtime.commands.suggestions(source, "/setblock 0 70 0 grass_bl"), 17, 8, []string{"grass_block"})
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

	if mode := bob.snapshotPlayer().GameMode; mode != game.GameModeCreative {
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

	assertSystemMessages(t, bobConnection, "Set game mode for 1 player(s)")

	bobConnection.reset()
	aliceConnection.reset()

	executeCommand(t, bob, "gamemode creative")

	assertPacketIDs(t, bobConnection.packetIDs(t), []int32{protocol.ClientboundSystemChatID})
	assertPacketIDs(t, aliceConnection.packetIDs(t), nil)

	bobConnection.reset()

	executeCommand(t, bob, "gamemode bogus")

	assertSystemMessages(t, bobConnection, "invalid game mode \"bogus\"\nUsage: /gamemode <survival|creative|adventure|spectator> [targets]")

	if mode := bob.snapshotPlayer().GameMode; mode != game.GameModeCreative {
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

	assertSystemMessages(t, bobConnection, "Gave 1 item(s) to 1 player(s)")

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

	stateID, items, _ = decodeInventorySnapshot(t, bobConnection.packets(t)[0])
	if stateID != 4 {
		t.Fatalf("inventory state id after four gives = %d, want 4", stateID)
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

	assertSystemMessages(t, bobConnection, "Unknown item \"air\"\nUsage: /give <targets> <item> [count]")

	bobConnection.reset()

	executeCommand(t, bob, "give Bob bogus")

	assertSystemMessages(t, bobConnection, "Unknown item \"bogus\"\nUsage: /give <targets> <item> [count]")

	bobConnection.reset()

	executeCommand(t, bob, "give Bob stone 0")

	assertSystemMessages(t, bobConnection, "count must be between 1 and 32767\nUsage: /give <targets> <item> [count]")

	bobConnection.reset()

	executeCommand(t, bob, "give Bob stone 32768")

	assertSystemMessages(t, bobConnection, "count must be between 1 and 32767\nUsage: /give <targets> <item> [count]")

	player := bob.snapshotPlayer()
	if player.Inventory.StateID != 0 {
		t.Fatalf("inventory state id after rejected gives = %d, want 0", player.Inventory.StateID)
	}

	for slot := 9; slot < game.PlayerInventorySlots; slot++ {
		if !player.Inventory.Slot(slot).Empty() {
			t.Fatalf("slot %d = %+v after rejected gives, want empty", slot, *player.Inventory.Slot(slot))
		}
	}

	assertPacketIDs(t, bobConnection.packetIDs(t), []int32{protocol.ClientboundSystemChatID})
}

func TestCommandGiveFailsWhenInventoryIsFull(t *testing.T) {
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

	assertSystemMessages(t, bobConnection, "Command failed: give to Bob: inventory does not have enough space")

	player := bob.snapshotPlayer()
	if player.Inventory.StateID != 0 {
		t.Fatalf("inventory state id after failed give = %d, want 0", player.Inventory.StateID)
	}

	for slot := 9; slot <= 44; slot++ {
		expected := game.ItemStack{Item: game.ItemDirt, Count: 64}
		if !player.Inventory.Slot(slot).Equal(expected) {
			t.Fatalf("slot %d = %+v after failed give, want a full dirt stack", slot, *player.Inventory.Slot(slot))
		}
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

func TestCommandGiveMultipleTargetsCommitsAtomically(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		runtime := NewRuntime(&game.World{})

		bob, bobConnection := newCommandTestSession(runtime, commandTestBobUUID, "Bob")
		alice, aliceConnection := newCommandTestSession(runtime, commandTestAliceUUID, "Alice")

		joinTestSession(t, runtime, bob)
		joinTestSession(t, runtime, alice)

		bobConnection.reset()
		aliceConnection.reset()

		executeCommand(t, bob, "give @a stone 3")

		for _, session := range []*Session{bob, alice} {
			inventory := session.snapshotPlayer().Inventory
			if inventory.StateID != 1 || !inventory.Hotbar[0].Equal(game.ItemStack{Item: game.ItemStone, Count: 3}) {
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

	t.Run("rollback", func(t *testing.T) {
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

		if bobInventory.StateID != 0 || !bobInventory.Hotbar[0].Empty() {
			t.Fatalf("bob inventory changed during rollback: %+v", bobInventory)
		}

		if aliceInventory.StateID != 0 || !aliceInventory.Hotbar[0].Equal(game.ItemStack{Item: game.ItemDirt, Count: 64}) {
			t.Fatalf("alice inventory changed during rollback: %+v", aliceInventory)
		}

		if len(packetsByID(t, bobConnection, protocol.ClientboundContainerSetContentID)) != 0 || len(packetsByID(t, aliceConnection, protocol.ClientboundContainerSetContentID)) != 0 {
			t.Fatal("failed multi-target give synchronized an inventory")
		}

		assertSystemMessages(t, bobConnection, "Command failed: give to Alice: inventory does not have enough space")
	})
}

func TestCommandClearRemovesInventoryAndCursor(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	bob, bobConnection := newCommandTestSession(runtime, commandTestBobUUID, "Bob")
	observer, observerConnection := newCommandTestSession(runtime, commandTestAliceUUID, "Alice")

	bob.Player.Inventory.Hotbar[0] = game.ItemStack{Item: game.ItemStone, Count: 3}
	bob.Player.Inventory.Main[0] = game.ItemStack{Item: game.ItemDirt, Count: 2}
	bob.Player.Inventory.Carried = game.ItemStack{Item: game.ItemStone, Count: 4}

	joinTestSession(t, runtime, observer)
	joinTestSession(t, runtime, bob)

	bobConnection.reset()
	observerConnection.reset()

	executeCommand(t, bob, "clear")

	player := bob.snapshotPlayer()
	if !player.Inventory.Hotbar[0].Empty() || !player.Inventory.Main[0].Empty() || !player.Inventory.Carried.Empty() {
		t.Fatalf("inventory after clear = %+v, want every stack removed", player.Inventory)
	}

	if player.Inventory.StateID != 1 {
		t.Fatalf("inventory state id after clear = %d, want 1", player.Inventory.StateID)
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

	assertSystemMessages(t, bobConnection, "Cleared 9 item(s) from 1 player(s)")

	assertPacketIDs(t, observerConnection.packetIDs(t), []int32{protocol.ClientboundEntityEquipmentID})
	assertEmptyEquipmentUpdate(t, observerConnection.packets(t)[0], bob.Player.EntityID, protocol.EquipmentSlotMainHand)
}

func TestCommandClearFiltersByItem(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	bob, bobConnection := newCommandTestSession(runtime, commandTestBobUUID, "Bob")

	bob.Player.Inventory.Hotbar[0] = game.ItemStack{Item: game.ItemStone, Count: 3}
	bob.Player.Inventory.Main[0] = game.ItemStack{Item: game.ItemDirt, Count: 2}
	bob.Player.Inventory.Offhand = game.ItemStack{Item: game.ItemDirt, Count: 1}
	bob.Player.Inventory.Carried = game.ItemStack{Item: game.ItemStone, Count: 5}

	joinTestSession(t, runtime, bob)

	bobConnection.reset()

	executeCommand(t, bob, "clear Bob stone")

	player := bob.snapshotPlayer()
	if !player.Inventory.Hotbar[0].Empty() || !player.Inventory.Carried.Empty() {
		t.Fatalf("stone stacks after filtered clear = hotbar %+v carried %+v, want both empty", player.Inventory.Hotbar[0], player.Inventory.Carried)
	}

	if !player.Inventory.Main[0].Equal(game.ItemStack{Item: game.ItemDirt, Count: 2}) {
		t.Fatalf("dirt stack after filtered clear = %+v, want untouched", player.Inventory.Main[0])
	}

	if !player.Inventory.Offhand.Equal(game.ItemStack{Item: game.ItemDirt, Count: 1}) {
		t.Fatalf("offhand dirt after filtered clear = %+v, want untouched", player.Inventory.Offhand)
	}

	assertSystemMessages(t, bobConnection, "Cleared 8 item(s) from 1 player(s)")

	bobConnection.reset()

	executeCommand(t, bob, "clear Bob oak_log")

	assertSystemMessages(t, bobConnection, "Cleared 0 item(s) from 1 player(s)")

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

	executeCommand(t, bob, "tp ~1 ~2 ~-1")

	expected := game.Position{X: 9.5, Y: 72, Z: 7.5}

	player := bob.snapshotPlayer()
	if player.Position != expected {
		t.Fatalf("position after relative teleport = %+v, want %+v", player.Position, expected)
	}

	if player.Velocity != (game.Velocity{}) || player.OnGround {
		t.Fatalf("movement state after teleport = %+v on ground %v, want reset velocity and airborne", player.Velocity, player.OnGround)
	}

	assertPacketIDs(t, bobConnection.packetIDs(t), []int32{
		protocol.ClientboundPlayerPositionID,
		protocol.ClientboundSystemChatID,
	})

	assertPlayerPositionPacket(t, bobConnection.packets(t)[0], 1, expected)
	assertSystemMessages(t, bobConnection, "Teleported 1 player(s)")

	usage := "Usage: /tp <destination|x y z> | /tp <targets> <destination|x y z>"

	invalidTeleports := map[string]string{
		"tp ^1 ^ ^":        "Local (^) coordinates are not supported\n" + usage,
		"tp abc 64 0":      "Invalid coordinate \"abc\"\n" + usage,
		"tp 30000001 64 0": "Coordinates are outside the valid world range\n" + usage,
	}

	for input, feedback := range invalidTeleports {
		bobConnection.reset()

		executeCommand(t, bob, input)

		position := bob.snapshotPlayer().Position
		if position != expected {
			t.Fatalf("position after %q = %+v, want %+v", input, position, expected)
		}

		assertSystemMessages(t, bobConnection, feedback)
	}

	bobConnection.reset()

	executeCommand(t, bob, "tp 100.5 64 -200.25")

	absolute := game.Position{X: 100.5, Y: 64, Z: -200.25}

	positionPackets := packetsByID(t, bobConnection, protocol.ClientboundPlayerPositionID)
	if len(positionPackets) != 1 {
		t.Fatalf("position packets after absolute teleport = %d, want 1", len(positionPackets))
	}

	assertPlayerPositionPacket(t, positionPackets[0], 2, absolute)

	if position := bob.snapshotPlayer().Position; position != absolute {
		t.Fatalf("position after absolute teleport = %+v, want %+v", position, absolute)
	}
}

func TestCommandTeleportToPlayerAndMultipleTargets(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	bob, bobConnection := newCommandTestSession(runtime, commandTestBobUUID, "Bob")
	alice, aliceConnection := newCommandTestSession(runtime, commandTestAliceUUID, "Alice")

	bob.Player.Position = game.Position{X: 8.5, Y: 70, Z: 8.5}
	alice.Player.Position = game.Position{X: 8.5, Y: 75, Z: 8.5}

	for _, session := range []*Session{bob, alice} {
		session.Config = &config.Config{Server: config.ServerConfig{RenderDistance: new(int32(1))}}

		prepareCommandSessionChunks(session)
	}

	joinTestSession(t, runtime, bob)
	joinTestSession(t, runtime, alice)

	bobConnection.reset()
	aliceConnection.reset()

	executeCommand(t, bob, "tp Alice")

	destination := game.Position{X: 8.5, Y: 75, Z: 8.5}

	if position := bob.snapshotPlayer().Position; position != destination {
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

	for _, session := range []*Session{bob, alice} {
		if position := session.snapshotPlayer().Position; position != shared {
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

	assertSystemMessages(t, bobConnection, "Missing destination\nUsage: /tp <destination|x y z> | /tp <targets> <destination|x y z>")
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

	if block := runtime.World.BlockAt(position); block != game.Stone {
		t.Fatalf("block after setblock = %d, want stone", block)
	}

	assertPacketIDs(t, bobConnection.packetIDs(t), []int32{
		protocol.ClientboundBlockUpdateID,
		protocol.ClientboundSystemChatID,
	})

	assertBlockUpdate(t, bobConnection.packets(t)[0], position, protocol.StoneBlockState)
	assertSystemMessages(t, bobConnection, "Changed 1 block(s)")

	bobConnection.reset()

	executeCommand(t, bob, "setblock 3 70 -2 dirt")

	dirtState, err := protocolBlockState(game.Dirt)
	if err != nil {
		t.Fatalf("encode dirt state: %v", err)
	}

	if block := runtime.World.BlockAt(position); block != game.Dirt {
		t.Fatalf("block after replacing = %d, want dirt", block)
	}

	assertBlockUpdate(t, bobConnection.packets(t)[0], position, dirtState)
	assertSystemMessages(t, bobConnection, "Changed 1 block(s)")

	bobConnection.reset()

	executeCommand(t, bob, "setblock 3 70 -2 dirt")

	assertSystemMessages(t, bobConnection, "Changed 0 block(s)")
	assertPacketIDs(t, bobConnection.packetIDs(t), []int32{protocol.ClientboundSystemChatID})

	bobConnection.reset()

	executeCommand(t, bob, "setblock 3 70 -2 bogus")

	assertSystemMessages(t, bobConnection, "Unknown block \"bogus\"\nUsage: /setblock <x> <y> <z> <block>")

	if block := runtime.World.BlockAt(position); block != game.Dirt {
		t.Fatalf("block after unknown block = %d, want dirt", block)
	}

	bobConnection.reset()

	executeCommand(t, bob, "setblock ~1 ~-1 ~ cobblestone")

	relative := game.BlockPosition{X: 4, Y: 70, Z: -2}

	if block := runtime.World.BlockAt(relative); block != game.Cobblestone {
		t.Fatalf("relative block after setblock = %d, want cobblestone", block)
	}

	cobblestoneState, err := protocolBlockState(game.Cobblestone)
	if err != nil {
		t.Fatalf("encode cobblestone state: %v", err)
	}

	assertBlockUpdate(t, bobConnection.packets(t)[0], relative, cobblestoneState)
	assertSystemMessages(t, bobConnection, "Changed 1 block(s)")
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

	if block := runtime.World.BlockAt(headPosition); block != game.Stone {
		t.Fatalf("block placed inside the player = %d, want stone", block)
	}

	if pose := bob.snapshotPlayer().Pose; pose != game.PlayerPoseCrawling {
		t.Fatalf("pose after placing a block at head height = %d, want crawling", pose)
	}

	assertSystemMessages(t, bobConnection, "Changed 1 block(s)")
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
		if block := runtime.World.BlockAt(position); block != game.Stone {
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

	assertSystemMessages(t, bobConnection, "Changed 4 block(s)")

	bobConnection.reset()

	executeCommand(t, bob, "fill 0 70 0 1 70 1 stone")

	assertSystemMessages(t, bobConnection, "Changed 0 block(s)")
	assertPacketIDs(t, bobConnection.packetIDs(t), []int32{protocol.ClientboundSystemChatID})

	bobConnection.reset()

	executeCommand(t, bob, "fill 1 70 1 0 70 0 dirt")

	for _, position := range region {
		if block := runtime.World.BlockAt(position); block != game.Dirt {
			t.Fatalf("block at %+v after reversed fill = %d, want dirt", position, block)
		}
	}

	assertSystemMessages(t, bobConnection, "Changed 4 block(s)")
}

func TestCommandFillVolumeLimits(t *testing.T) {
	t.Run("boundary volume fills every block", func(t *testing.T) {
		runtime := NewRuntime(&game.World{})

		bob, bobConnection := newCommandTestSession(runtime, commandTestBobUUID, "Bob")

		executeCommand(t, bob, "fill 0 0 0 31 31 31 stone")

		assertSystemMessages(t, bobConnection, "Changed 32768 block(s)")

		samples := []game.BlockPosition{
			{X: 0, Y: 0, Z: 0},
			{X: 31, Y: 31, Z: 31},
			{X: 16, Y: 16, Z: 16},
		}

		for _, position := range samples {
			if block := runtime.World.BlockAt(position); block != game.Stone {
				t.Fatalf("block at %+v = %d, want stone", position, block)
			}
		}
	})

	t.Run("volume overflow is rejected without changes", func(t *testing.T) {
		runtime := NewRuntime(&game.World{})

		bob, bobConnection := newCommandTestSession(runtime, commandTestBobUUID, "Bob")

		executeCommand(t, bob, "fill 0 0 0 32 31 31 stone")

		assertSystemMessages(t, bobConnection, "Fill volume 33792 exceeds 32768 blocks\nUsage: /fill <x1> <y1> <z1> <x2> <y2> <z2> <block>")

		for _, position := range []game.BlockPosition{{X: 0, Y: 0, Z: 0}, {X: 32, Y: 31, Z: 31}} {
			if block := runtime.World.BlockAt(position); block != game.Air {
				t.Fatalf("block at %+v after rejected fill = %d, want air", position, block)
			}
		}
	})

	t.Run("axis overflow is rejected without changes", func(t *testing.T) {
		runtime := NewRuntime(&game.World{})

		bob, bobConnection := newCommandTestSession(runtime, commandTestBobUUID, "Bob")

		executeCommand(t, bob, "fill 0 0 0 32768 0 0 stone")

		assertSystemMessages(t, bobConnection, "Fill volume exceeds 32768 blocks\nUsage: /fill <x1> <y1> <z1> <x2> <y2> <z2> <block>")

		for _, position := range []game.BlockPosition{{X: 0, Y: 0, Z: 0}, {X: 32768, Y: 0, Z: 0}} {
			if block := runtime.World.BlockAt(position); block != game.Air {
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

		assertSystemMessages(t, bobConnection, "Changed 1 block(s)")

		if block := runtime.World.BlockAt(game.BlockPosition{X: 0, Y: 70, Z: 0}); block != game.Dirt {
			t.Fatalf("existing block after partial fill = %d, want dirt", block)
		}

		if block := runtime.World.BlockAt(game.BlockPosition{X: 1, Y: 70, Z: 0}); block != game.Dirt {
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

	assertSystemMessages(t, bobConnection, "Seed: -987")

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
