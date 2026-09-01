package game

import "testing"

func TestParseDifficulty(t *testing.T) {
	cases := map[string]Difficulty{
		"peaceful": DifficultyPeaceful,
		"easy":     DifficultyEasy,
		"normal":   DifficultyNormal,
		"hard":     DifficultyHard,
	}

	for value, expected := range cases {
		difficulty, err := ParseDifficulty(value)
		if err != nil {
			t.Fatalf("parse %q: %v", value, err)
		}

		if difficulty != expected {
			t.Errorf("difficulty %q = %d, want %d", value, difficulty, expected)
		}
	}

	_, err := ParseDifficulty("peace")
	if err == nil {
		t.Fatal("invalid difficulty parsed without an error")
	}
}
