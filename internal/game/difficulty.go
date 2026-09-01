package game

import "fmt"

const (
	DifficultyPeaceful Difficulty = iota
	DifficultyEasy
	DifficultyNormal
	DifficultyHard
)

type Difficulty byte

func ParseDifficulty(value string) (Difficulty, error) {
	switch value {
	case "peaceful":
		return DifficultyPeaceful, nil
	case "easy":
		return DifficultyEasy, nil
	case "normal":
		return DifficultyNormal, nil
	case "hard":
		return DifficultyHard, nil
	default:
		return 0, fmt.Errorf("invalid difficulty %q", value)
	}
}
