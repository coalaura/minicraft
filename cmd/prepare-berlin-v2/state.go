package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

type preparationState struct {
	ID    string   `json:"id"`
	Files []string `json:"files"`
}

func preparationComplete(output, id string) (bool, error) {
	contents, err := os.ReadFile(preparationStatePath(output, id))
	if os.IsNotExist(err) {
		return false, nil
	}

	if err != nil {
		return false, fmt.Errorf("read Berlin v2 preparation state: %w", err)
	}

	state := preparationState{}
	err = json.Unmarshal(contents, &state)
	if err != nil || state.ID != id || len(state.Files) == 0 {
		return false, nil
	}

	for _, file := range state.Files {
		_, err := os.Stat(filepath.Join(output, file))
		if err != nil {
			return false, nil
		}
	}

	return true, nil
}

func markPreparationComplete(output, id string, files []string) error {
	stateDirectory := filepath.Join(output, ".state")
	err := os.MkdirAll(stateDirectory, 0o755)
	if err != nil {
		return fmt.Errorf("create Berlin v2 preparation state directory: %w", err)
	}

	sort.Strings(files)
	state := preparationState{ID: id, Files: files}

	contents, err := json.Marshal(state)
	if err != nil {
		return err
	}

	contents = append(contents, '\n')
	err = os.WriteFile(preparationStatePath(output, id), contents, 0o644)
	if err != nil {
		return fmt.Errorf("write Berlin v2 preparation state: %w", err)
	}

	return nil
}

func preparationStatePath(output, id string) string {
	digest := sha256.Sum256([]byte(id))
	name := hex.EncodeToString(digest[:]) + ".json"
	return filepath.Join(output, ".state", name)
}
