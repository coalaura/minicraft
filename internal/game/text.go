package game

import "strconv"

type TextColor string

const (
	TextColorGray  TextColor = "gray"
	TextColorRed   TextColor = "red"
	TextColorGold  TextColor = "gold"
	TextColorGreen TextColor = "green"
)

type ClickAction string

const (
	ClickSuggestCommand  ClickAction = "suggest_command"
	ClickCopyToClipboard ClickAction = "copy_to_clipboard"
)

type ClickEvent struct {
	Action ClickAction
	Value  string
}

type TextStyle struct {
	Color      TextColor
	Italic     *bool
	Underlined *bool
	ClickEvent *ClickEvent
}

type TextComponent struct {
	Text      string
	Translate string
	Arguments []TextComponent
	Siblings  []TextComponent
	Style     TextStyle
}

func LiteralText(value string) TextComponent {
	return TextComponent{Text: value}
}

func TranslatableText(key string, arguments ...TextComponent) TextComponent {
	return TextComponent{Translate: key, Arguments: arguments}
}

func (component TextComponent) Append(siblings ...TextComponent) TextComponent {
	component.Siblings = append(component.Siblings, siblings...)

	return component
}

func (component TextComponent) WithColor(color TextColor) TextComponent {
	component.Style.Color = color

	return component
}

func (component TextComponent) WithItalic(italic bool) TextComponent {
	component.Style.Italic = &italic

	return component
}

func (component TextComponent) WithUnderline(underlined bool) TextComponent {
	component.Style.Underlined = &underlined

	return component
}

func (component TextComponent) WithClickEvent(action ClickAction, value string) TextComponent {
	component.Style.ClickEvent = &ClickEvent{Action: action, Value: value}

	return component
}

func formatInt32(value int32) string {
	return strconv.FormatInt(int64(value), 10)
}
