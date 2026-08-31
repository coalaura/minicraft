package game

import "fmt"

const (
	GameModeSurvival GameMode = iota
	GameModeCreative
	GameModeAdventure
	GameModeSpectator
)

type GameMode byte

func ParseGameMode(value string) (GameMode, error) {
	switch value {
	case "survival":
		return GameModeSurvival, nil
	case "creative":
		return GameModeCreative, nil
	case "adventure":
		return GameModeAdventure, nil
	case "spectator":
		return GameModeSpectator, nil
	default:
		return 0, fmt.Errorf("invalid game mode %q", value)
	}
}
