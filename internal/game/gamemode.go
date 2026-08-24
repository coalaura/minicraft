package game

type GameMode byte

const (
	GameModeSurvival GameMode = iota
	GameModeCreative
	GameModeAdventure
	GameModeSpectator
)
