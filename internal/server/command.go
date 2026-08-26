package server

import (
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

const commandSuggestionProvider = "minecraft:ask_server"

type CommandSource interface {
	Name() string
	Position() game.Position
	Feedback(game.TextComponent) error
	Failure(game.TextComponent) error
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
	Name       string
	Permission string
	Patterns   []commandPattern
	Redirect   *registeredCommand
}

type commandPattern struct {
	Elements []commandElement
	Execute  commandExecutor
}

type commandExecutor func(CommandSource, []any) error

type commandElement interface {
	parse(CommandSource, []commandToken, int) (any, int, error)
	suggestions(CommandSource, string) []string
	commandNode() protocol.CommandNode
	key() string
	usage() string
}

type commandLiteral struct {
	value string
}

type commandArgument struct {
	name           string
	parser         int32
	properties     protocol.CommandParserProperties
	width          int
	parseValue     func(CommandSource, []commandToken) (any, error)
	suggestValues  func(CommandSource) []string
	clientSuggests bool
}

type commandToken struct {
	value string
	start int
	end   int
}

type commandTreeNode struct {
	node       protocol.CommandNode
	key        string
	children   []*commandTreeNode
	redirect   *commandTreeNode
	executable bool
}

type commandMatch struct {
	pattern commandPattern
	values  []any
}

type commandSyntaxError struct {
	message game.TextComponent
	cursor  int
}

type commandFailure struct {
	message game.TextComponent
}

func (err commandSyntaxError) Error() string {
	return "invalid command syntax"
}

func (err commandFailure) Error() string {
	return "command failed"
}

func (source playerCommandSource) Name() string {
	return source.session.snapshotPlayer().Name
}

func (source playerCommandSource) Position() game.Position {
	return source.session.snapshotPlayer().Position
}

func (source playerCommandSource) Feedback(message game.TextComponent) error {
	return source.session.sendSystemComponent(message)
}

func (source playerCommandSource) Failure(message game.TextComponent) error {
	return source.session.sendSystemComponent(message.WithColor(game.TextColorRed))
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

func (registry *commandRegistry) registerRedirect(name string, target *registeredCommand) {
	registry.register(&registeredCommand{Name: name, Permission: target.Permission, Redirect: target})
}

func (registry *commandRegistry) execute(source CommandSource, input string) error {
	input = strings.TrimPrefix(input, "/")
	tokens := tokenizeCommand(input)

	if len(tokens) == 0 {
		return registry.sendSyntaxError(source, input, commandSyntaxError{
			message: game.TranslatableText("command.unknown.command"),
			cursor:  0,
		})
	}

	command := registry.byName[tokens[0].value]
	if command == nil || !source.HasPermission(command.Permission) {
		return registry.sendSyntaxError(source, input, commandSyntaxError{
			message: game.TranslatableText("command.unknown.command"),
			cursor:  tokens[0].start,
		})
	}

	if command.Redirect != nil {
		command = command.Redirect
	}

	match, err := registry.match(source, command, tokens[1:], len(input))
	if err != nil {
		syntaxError, syntax := errors.AsType[commandSyntaxError](err)
		if syntax {
			return registry.sendSyntaxError(source, input, syntaxError)
		}

		return source.Failure(game.LiteralText(err.Error()))
	}

	err = match.pattern.Execute(source, match.values)
	if err == nil {
		return nil
	}

	failure, failed := errors.AsType[commandFailure](err)
	if failed {
		return source.Failure(failure.message)
	}

	syntaxError, syntax := errors.AsType[commandSyntaxError](err)
	if syntax {
		return registry.sendSyntaxError(source, input, syntaxError)
	}

	return source.Failure(game.LiteralText(err.Error()))
}

func (registry *commandRegistry) sendSyntaxError(source CommandSource, input string, syntaxError commandSyntaxError) error {
	err := source.Failure(syntaxError.message)
	if err != nil {
		return err
	}

	cursor := max(0, min(syntaxError.cursor, len(input)))
	contextStart := max(0, cursor-10)
	prefix := input[contextStart:cursor]

	if contextStart > 0 {
		prefix = "..." + prefix
	}

	context := game.LiteralText(prefix).WithColor(game.TextColorGray)

	invalid := game.LiteralText(input[cursor:]).
		WithColor(game.TextColorRed).
		WithUnderline(true)

	if invalid.Text != "" {
		context = context.Append(invalid)
	}

	context = context.Append(game.TranslatableText("command.context.here").
		WithColor(game.TextColorRed).
		WithItalic(true))

	context = context.WithClickEvent(game.ClickSuggestCommand, "/"+input)

	return source.Feedback(context)
}

func (registry *commandRegistry) match(source CommandSource, command *registeredCommand, tokens []commandToken, end int) (commandMatch, error) {
	bestConsumed := -1
	bestCursor := -1

	var bestErr error

	for _, pattern := range command.Patterns {
		values := make([]any, 0, len(pattern.Elements))

		consumed := 0
		valid := true

		for _, element := range pattern.Elements {
			cursor := end
			if consumed < len(tokens) {
				cursor = tokens[consumed].start
			}

			value, count, err := element.parse(source, tokens[consumed:], cursor)
			if err != nil {
				syntaxError, syntax := errors.AsType[commandSyntaxError](err)
				errorCursor := cursor

				if syntax {
					errorCursor = syntaxError.cursor
				}

				if consumed > bestConsumed || consumed == bestConsumed && errorCursor >= bestCursor {
					bestErr = err
					bestConsumed = consumed
					bestCursor = errorCursor
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

		if valid && consumed < len(tokens) && consumed >= bestConsumed {
			bestConsumed = consumed
			bestCursor = tokens[consumed].start
			bestErr = commandSyntaxError{
				message: game.TranslatableText("command.unknown.argument"),
				cursor:  tokens[consumed].start,
			}
		}
	}

	if bestErr != nil {
		return commandMatch{}, bestErr
	}

	return commandMatch{}, commandSyntaxError{message: game.TranslatableText("command.unknown.argument"), cursor: end}
}

func (literal commandLiteral) parse(_ CommandSource, tokens []commandToken, cursor int) (any, int, error) {
	if len(tokens) == 0 || tokens[0].value != literal.value {
		return nil, 0, commandSyntaxError{message: game.TranslatableText("command.unknown.argument"), cursor: cursor}
	}

	return literal.value, 1, nil
}

func (literal commandLiteral) suggestions(_ CommandSource, prefix string) []string {
	if commandSuggestionMatches(literal.value, prefix) {
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

func (literal commandLiteral) usage() string {
	return literal.value
}

func (argument commandArgument) parse(source CommandSource, tokens []commandToken, cursor int) (any, int, error) {
	width := argument.width
	if width < 0 {
		width = len(tokens)
	}

	if width == 0 || len(tokens) < width {
		return nil, 0, commandSyntaxError{message: game.TranslatableText("command.unknown.argument"), cursor: cursor}
	}

	value, err := argument.parseValue(source, tokens[:width])
	if err != nil {
		return nil, 0, err
	}

	return value, width, nil
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
	return fmt.Sprintf("argument:%s:%d:%#v:%t", argument.name, argument.parser, argument.properties, argument.clientSuggests)
}

func (argument commandArgument) usage() string {
	return "<" + argument.name + ">"
}

func (registry *commandRegistry) declaration() protocol.DeclareCommands {
	root := &commandTreeNode{node: protocol.CommandNode{Type: protocol.CommandNodeRoot}, key: "root"}

	literals := make(map[*registeredCommand]*commandTreeNode, len(registry.commands))

	for _, command := range registry.commands {
		literal := findOrAppendTreeChild(root, protocol.CommandNode{Type: protocol.CommandNodeLiteral, Name: command.Name}, "command:"+command.Name)
		literals[command] = literal
	}

	for _, command := range registry.commands {
		literal := literals[command]

		if command.Redirect != nil {
			literal.redirect = literals[command.Redirect]

			continue
		}

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

		if treeNode.redirect != nil {
			node.HasRedirect = true
			node.Redirect = indices[treeNode.redirect]
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
	compatible := existing.Type == incoming.Type &&
		existing.Name == incoming.Name &&
		existing.Parser == incoming.Parser &&
		reflect.DeepEqual(existing.Properties, incoming.Properties) &&
		existing.SuggestionType == incoming.SuggestionType

	if !compatible {
		panic(fmt.Sprintf("incompatible command nodes share a declaration key: existing=%+v incoming=%+v", *existing, incoming))
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

	completedTokens := tokenizeCommand(completedText)
	completed := make([]string, len(completedTokens))

	for index, token := range completedTokens {
		completed[index] = token.value
	}

	var values []string

	if len(completed) == 0 {
		for _, command := range registry.commands {
			if source.HasPermission(command.Permission) {
				values = append(values, command.Name)
			}
		}
	} else {
		command := registry.byName[completed[0]]
		if command != nil && source.HasPermission(command.Permission) {
			if command.Redirect != nil {
				command = command.Redirect
			}

			values = registry.argumentSuggestions(source, command, completed[1:], prefix)
		}
	}

	values = filterSuggestions(values, prefix)
	matches := make([]protocol.CommandSuggestion, len(values))

	for index, value := range values {
		matches[index] = protocol.CommandSuggestion{Text: value}
	}

	return protocol.CommandSuggestions{
		Start:   utf16Length(text[:offset+prefixStart]),
		Length:  utf16Length(prefix),
		Matches: matches,
	}
}

func utf16Length(value string) int32 {
	return int32(len(utf16.Encode([]rune(value))))
}

func (registry *commandRegistry) argumentSuggestions(source CommandSource, command *registeredCommand, completed []string, prefix string) []string {
	tokens := make([]commandToken, len(completed))

	for index, value := range completed {
		tokens[index] = commandToken{value: value}
	}

	var suggestions []string

	for _, pattern := range command.Patterns {
		consumed := 0
		valid := true

		for _, element := range pattern.Elements {
			if consumed == len(tokens) {
				suggestions = append(suggestions, element.suggestions(source, prefix)...)
				valid = false

				break
			}

			_, count, err := element.parse(source, tokens[consumed:], 0)
			if err != nil || consumed+count > len(tokens) {
				valid = false

				break
			}

			consumed += count
		}

		if valid && consumed != len(tokens) {
			continue
		}
	}

	return suggestions
}

func filterSuggestions(values []string, prefix string) []string {
	seen := make(map[string]struct{}, len(values))
	filtered := make([]string, 0, len(values))

	for _, value := range values {
		if !commandSuggestionMatches(value, prefix) {
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

func commandSuggestionMatches(value, prefix string) bool {
	return strings.HasPrefix(strings.ToLower(value), strings.ToLower(prefix))
}

func tokenizeCommand(input string) []commandToken {
	var tokens []commandToken

	for cursor := 0; cursor < len(input); {
		for cursor < len(input) && input[cursor] == ' ' {
			cursor++
		}

		if cursor == len(input) {
			break
		}

		start := cursor

		for cursor < len(input) && input[cursor] != ' ' {
			cursor++
		}

		tokens = append(tokens, commandToken{value: input[start:cursor], start: start, end: cursor})
	}

	return tokens
}

func parseInteger(_ CommandSource, tokens []commandToken) (any, error) {
	value, err := strconv.ParseInt(tokens[0].value, 10, 32)
	if err != nil {
		return nil, commandSyntaxError{
			message: game.TranslatableText("argument.integer.invalid", game.LiteralText(tokens[0].value)),
			cursor:  tokens[0].start,
		}
	}

	return int32(value), nil
}

func parseString(_ CommandSource, tokens []commandToken) (any, error) {
	values := make([]string, len(tokens))

	for index, token := range tokens {
		if !utf8.ValidString(token.value) {
			return nil, commandSyntaxError{message: game.LiteralText("Invalid text"), cursor: token.start}
		}

		values[index] = token.value
	}

	return strings.Join(values, " "), nil
}
