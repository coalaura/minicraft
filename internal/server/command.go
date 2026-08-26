package server

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

const commandSuggestionProvider = "minecraft:ask_server"

type CommandSource interface {
	Name() string
	Position() game.Position
	Feedback(string) error
	HasPermission(string) bool
	PlayerSession() (*Session, bool)
}

type playerCommandSource struct {
	session *Session
}

type commandRegistry struct {
	runtime  *Runtime
	commands []*registeredCommand
	byName   map[string]*registeredCommand
}

type registeredCommand struct {
	Name        string
	Usage       string
	Description string
	Permission  string
	Patterns    []commandPattern
}

type commandPattern struct {
	Elements []commandElement
	Execute  commandExecutor
}

type commandExecutor func(CommandSource, []any) error

type commandElement interface {
	parse(CommandSource, []string) (any, int, error)
	suggestions(CommandSource, string) []string
	commandNode() protocol.CommandNode
	key() string
}

type commandLiteral struct {
	value string
}

type commandArgument struct {
	name           string
	declarationKey string
	parser         int32
	properties     protocol.CommandParserProperties
	width          int
	parseValue     func(CommandSource, []string) (any, error)
	suggestValues  func(CommandSource) []string
	clientSuggests bool
}

type commandTreeNode struct {
	node       protocol.CommandNode
	key        string
	children   []*commandTreeNode
	executable bool
}

type commandMatch struct {
	pattern commandPattern
	values  []any
}

type commandSyntaxError struct {
	message string
}

func (err commandSyntaxError) Error() string {
	return err.message
}

func (source playerCommandSource) Name() string {
	return source.session.snapshotPlayer().Name
}

func (source playerCommandSource) Position() game.Position {
	return source.session.snapshotPlayer().Position
}

func (source playerCommandSource) Feedback(message string) error {
	return source.session.sendSystemMessage(message)
}

func (source playerCommandSource) HasPermission(string) bool {
	return true
}

func (source playerCommandSource) PlayerSession() (*Session, bool) {
	return source.session, true
}

func newCommandRegistry(runtime *Runtime) *commandRegistry {
	registry := &commandRegistry{
		runtime: runtime,
		byName:  make(map[string]*registeredCommand),
	}

	registerBuiltinCommands(registry)

	return registry
}

func (registry *commandRegistry) register(command *registeredCommand) {
	registry.commands = append(registry.commands, command)
	registry.byName[command.Name] = command
}

func (registry *commandRegistry) execute(source CommandSource, input string) error {
	input = strings.TrimSpace(strings.TrimPrefix(input, "/"))
	if input == "" {
		return source.Feedback("Unknown command")
	}

	tokens := strings.Fields(input)

	command := registry.byName[strings.ToLower(tokens[0])]
	if command == nil || !source.HasPermission(command.Permission) {
		return source.Feedback(fmt.Sprintf("Unknown command: %s", tokens[0]))
	}

	match, err := registry.match(source, command, tokens[1:])
	if err != nil {
		return source.Feedback(fmt.Sprintf("%s\nUsage: %s", err, command.Usage))
	}

	err = match.pattern.Execute(source, match.values)
	if err == nil {
		return nil
	}

	syntaxError, syntax := errors.AsType[commandSyntaxError](err)
	if syntax {
		return source.Feedback(fmt.Sprintf("%s\nUsage: %s", syntaxError.message, command.Usage))
	}

	return source.Feedback(fmt.Sprintf("Command failed: %v", err))
}

func (registry *commandRegistry) match(source CommandSource, command *registeredCommand, tokens []string) (commandMatch, error) {
	var bestErr error

	bestConsumed := -1

	for _, pattern := range command.Patterns {
		values := make([]any, 0, len(pattern.Elements))

		consumed := 0
		valid := true

		for _, element := range pattern.Elements {
			value, count, err := element.parse(source, tokens[consumed:])
			if err != nil {
				if consumed > bestConsumed {
					bestErr = err
					bestConsumed = consumed
				}

				valid = false
				break
			}

			consumed += count
			values = append(values, value)
		}

		if valid && consumed == len(tokens) {
			return commandMatch{pattern: pattern, values: values}, nil
		}
	}

	if bestErr != nil {
		return commandMatch{}, bestErr
	}

	return commandMatch{}, commandSyntaxError{message: "Invalid command syntax"}
}

func (literal commandLiteral) parse(_ CommandSource, tokens []string) (any, int, error) {
	if len(tokens) == 0 || !strings.EqualFold(tokens[0], literal.value) {
		return nil, 0, commandSyntaxError{message: fmt.Sprintf("Expected %q", literal.value)}
	}

	return literal.value, 1, nil
}

func (literal commandLiteral) suggestions(_ CommandSource, prefix string) []string {
	if strings.HasPrefix(strings.ToLower(literal.value), strings.ToLower(prefix)) {
		return []string{literal.value}
	}

	return nil
}

func (literal commandLiteral) commandNode() protocol.CommandNode {
	return protocol.CommandNode{Type: protocol.CommandNodeLiteral, Name: literal.value}
}

func (literal commandLiteral) key() string {
	return "literal:" + literal.value
}

func (argument commandArgument) parse(source CommandSource, tokens []string) (any, int, error) {
	if len(tokens) < argument.width {
		return nil, 0, commandSyntaxError{message: fmt.Sprintf("Missing %s", argument.name)}
	}

	value, err := argument.parseValue(source, tokens[:argument.width])
	if err != nil {
		return nil, 0, err
	}

	return value, argument.width, nil
}

func (argument commandArgument) suggestions(source CommandSource, prefix string) []string {
	if argument.suggestValues == nil {
		return nil
	}

	return filterSuggestions(argument.suggestValues(source), prefix)
}

func (argument commandArgument) commandNode() protocol.CommandNode {
	suggestionType := ""
	if argument.clientSuggests {
		suggestionType = commandSuggestionProvider
	}

	return protocol.CommandNode{
		Type:           protocol.CommandNodeArgument,
		Name:           argument.name,
		Parser:         argument.parser,
		Properties:     argument.properties,
		SuggestionType: suggestionType,
	}
}

func (argument commandArgument) key() string {
	if argument.declarationKey != "" {
		return "argument:" + argument.declarationKey
	}

	return fmt.Sprintf("argument:%s:%d", argument.name, argument.parser)
}

func (registry *commandRegistry) declaration() protocol.DeclareCommands {
	root := &commandTreeNode{node: protocol.CommandNode{Type: protocol.CommandNodeRoot}, key: "root"}

	for _, command := range registry.commands {
		literal := findOrAppendTreeChild(root, protocol.CommandNode{Type: protocol.CommandNodeLiteral, Name: command.Name}, "command:"+command.Name)

		for _, pattern := range command.Patterns {
			current := literal

			if len(pattern.Elements) == 0 {
				current.executable = true
				continue
			}

			for _, element := range pattern.Elements {
				current = findOrAppendTreeChild(current, element.commandNode(), element.key())
			}

			current.executable = true
		}
	}

	var flat []*commandTreeNode

	flattenCommandTree(root, &flat)

	indices := make(map[*commandTreeNode]int32, len(flat))

	for index, node := range flat {
		indices[node] = int32(index)
	}

	nodes := make([]protocol.CommandNode, len(flat))

	for index, treeNode := range flat {
		node := treeNode.node

		node.Executable = treeNode.executable
		node.Children = make([]int32, len(treeNode.children))

		for childIndex, child := range treeNode.children {
			node.Children[childIndex] = indices[child]
		}

		nodes[index] = node
	}

	return protocol.DeclareCommands{Nodes: nodes, RootIndex: 0}
}

func findOrAppendTreeChild(parent *commandTreeNode, node protocol.CommandNode, key string) *commandTreeNode {
	for _, child := range parent.children {
		if child.key == key {
			mergeCommandTreeNode(&child.node, node)

			return child
		}
	}

	child := &commandTreeNode{node: node, key: key}
	parent.children = append(parent.children, child)

	return child
}

func mergeCommandTreeNode(existing *protocol.CommandNode, incoming protocol.CommandNode) {
	existingEntity, existingIsEntity := existing.Properties.(protocol.CommandEntityProperties)
	incomingEntity, incomingIsEntity := incoming.Properties.(protocol.CommandEntityProperties)

	if existingIsEntity && incomingIsEntity {
		existing.Properties = protocol.CommandEntityProperties{
			OnlyEntities: existingEntity.OnlyEntities && incomingEntity.OnlyEntities,
			OnlyPlayers:  existingEntity.OnlyPlayers && incomingEntity.OnlyPlayers,
		}
	}

	if existing.SuggestionType == "" {
		existing.SuggestionType = incoming.SuggestionType
	}
}

func flattenCommandTree(node *commandTreeNode, nodes *[]*commandTreeNode) {
	*nodes = append(*nodes, node)

	for _, child := range node.children {
		flattenCommandTree(child, nodes)
	}
}

func (registry *commandRegistry) suggestions(source CommandSource, text string) protocol.CommandSuggestions {
	commandText := text
	offset := 0

	if strings.HasPrefix(commandText, "/") {
		commandText = commandText[1:]
		offset = 1
	}

	lastSpace := strings.LastIndex(commandText, " ")
	prefixStart := 0
	prefix := commandText
	completedText := ""

	if lastSpace >= 0 {
		prefixStart = lastSpace + 1
		prefix = commandText[prefixStart:]
		completedText = commandText[:lastSpace]
	}

	completed := strings.Fields(completedText)

	var values []string

	if len(completed) == 0 {
		for _, command := range registry.commands {
			if source.HasPermission(command.Permission) {
				values = append(values, command.Name)
			}
		}
	} else {
		command := registry.byName[strings.ToLower(completed[0])]
		if command != nil && source.HasPermission(command.Permission) {
			values = registry.argumentSuggestions(source, command, completed[1:], prefix)
		}
	}

	values = filterSuggestions(values, prefix)
	matches := make([]protocol.CommandSuggestion, len(values))

	for index, value := range values {
		matches[index] = protocol.CommandSuggestion{Text: value}
	}

	return protocol.CommandSuggestions{
		Start:   int32(offset + prefixStart),
		Length:  int32(len(prefix)),
		Matches: matches,
	}
}

func (registry *commandRegistry) argumentSuggestions(source CommandSource, command *registeredCommand, completed []string, prefix string) []string {
	var suggestions []string

	for _, pattern := range command.Patterns {
		consumed := 0
		valid := true

		for _, element := range pattern.Elements {
			if consumed == len(completed) {
				suggestions = append(suggestions, element.suggestions(source, prefix)...)
				valid = false

				break
			}

			_, count, err := element.parse(source, completed[consumed:])
			if err != nil || consumed+count > len(completed) {
				valid = false

				break
			}

			consumed += count
		}

		if valid && consumed != len(completed) {
			continue
		}
	}

	return suggestions
}

func filterSuggestions(values []string, prefix string) []string {
	prefix = strings.ToLower(prefix)

	seen := make(map[string]struct{}, len(values))
	filtered := make([]string, 0, len(values))

	for _, value := range values {
		if !strings.HasPrefix(strings.ToLower(value), prefix) {
			continue
		}

		if _, exists := seen[value]; exists {
			continue
		}

		seen[value] = struct{}{}
		filtered = append(filtered, value)
	}

	sort.Strings(filtered)

	return filtered
}

func parseInteger(_ CommandSource, tokens []string) (any, error) {
	value, err := strconv.ParseInt(tokens[0], 10, 32)
	if err != nil {
		return nil, commandSyntaxError{message: fmt.Sprintf("Invalid integer %q", tokens[0])}
	}

	return int32(value), nil
}

func parseString(_ CommandSource, tokens []string) (any, error) {
	if !utf8.ValidString(tokens[0]) {
		return nil, commandSyntaxError{message: "Invalid text"}
	}

	return tokens[0], nil
}
