package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"go/format"
	"os"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

type sourceManifest struct {
	Source string `json:"source"`
	SHA256 string `json:"sha256"`
}

type potionEffectDefinition struct {
	Name      string
	Duration  int32
	Amplifier int32
}

type potionDefinition struct {
	Name    string
	Effects []potionEffectDefinition
}

var (
	registrationPattern = regexp.MustCompile(`=\s*register\s*\(`)
	effectPattern       = regexp.MustCompile(`new\s+MobEffectInstance\s*\(\s*MobEffects\.([A-Z_]+)\s*,\s*(\d+)\s*(?:,\s*(\d+)\s*)?\)`)
)

func main() {
	manifestPath := flag.String("manifest", "", "source manifest path")
	sourcePath := flag.String("source", "", "Potions.java source path")
	outputPath := flag.String("output", "", "generated Go output path")

	flag.Parse()

	if *manifestPath == "" || *sourcePath == "" || *outputPath == "" {
		fail(fmt.Errorf("manifest, source and output are required"))
	}

	generated, err := generate(*manifestPath, *sourcePath)
	if err != nil {
		fail(err)
	}

	err = os.WriteFile(*outputPath, generated, 0o644)
	if err != nil {
		fail(err)
	}
}

func generate(manifestPath, sourcePath string) ([]byte, error) {
	manifest, err := readManifest(manifestPath)
	if err != nil {
		return nil, err
	}

	source, err := os.ReadFile(sourcePath)
	if err != nil {
		return nil, err
	}

	digest := sha256.Sum256(source)
	actualHash := hex.EncodeToString(digest[:])

	if actualHash != manifest.SHA256 {
		return nil, fmt.Errorf("potions.java SHA-256 = %s, want %s", actualHash, manifest.SHA256)
	}

	definitions, err := parseDefinitions(source)
	if err != nil {
		return nil, err
	}

	if len(definitions) != 46 {
		return nil, fmt.Errorf("potions.java has %d registered potions, want 46", len(definitions))
	}

	return generateGame(definitions, actualHash)
}

func readManifest(path string) (sourceManifest, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return sourceManifest{}, err
	}

	var manifest sourceManifest

	err = json.Unmarshal(raw, &manifest)
	if err != nil {
		return sourceManifest{}, err
	}

	if manifest.Source == "" || len(manifest.SHA256) != sha256.Size*2 {
		return sourceManifest{}, fmt.Errorf("invalid Potions source manifest")
	}

	return manifest, nil
}

func parseDefinitions(source []byte) ([]potionDefinition, error) {
	matches := registrationPattern.FindAllIndex(source, -1)
	definitions := make([]potionDefinition, 0, len(matches))

	for _, match := range matches {
		open := bytes.IndexByte(source[match[0]:match[1]], '(')
		if open < 0 {
			return nil, fmt.Errorf("registration has no opening parenthesis")
		}

		open += match[0]

		close, err := closingParenthesis(source, open)
		if err != nil {
			return nil, err
		}

		definition, err := parseDefinition(source[open+1 : close])
		if err != nil {
			return nil, err
		}

		definitions = append(definitions, definition)
	}

	return definitions, nil
}

func parseDefinition(registration []byte) (potionDefinition, error) {
	_, remaining, found := strings.Cut(string(registration), `"`)
	if !found {
		return potionDefinition{}, fmt.Errorf("potion registration has no name")
	}

	name, _, found := strings.Cut(remaining, `"`)
	if !found || name == "" {
		return potionDefinition{}, fmt.Errorf("potion registration has invalid name")
	}

	matches := effectPattern.FindAllStringSubmatch(string(registration), -1)
	effectCount := strings.Count(string(registration), "new MobEffectInstance")

	if len(matches) != effectCount {
		return potionDefinition{}, fmt.Errorf("potion %s has an unrecognized effect", name)
	}

	effects := make([]potionEffectDefinition, 0, len(matches))

	for _, match := range matches {
		var duration, amplifier int32

		parsedDuration, err := strconv.ParseInt(match[2], 10, 32)
		if err != nil {
			return potionDefinition{}, fmt.Errorf("parse duration for potion %s: %w", name, err)
		}

		duration = int32(parsedDuration)

		if match[3] != "" {
			parsedAmplifier, err := strconv.ParseInt(match[3], 10, 32)
			if err != nil {
				return potionDefinition{}, fmt.Errorf("parse amplifier for potion %s: %w", name, err)
			}

			amplifier = int32(parsedAmplifier)
		}

		effects = append(effects, potionEffectDefinition{Name: strings.ToLower(match[1]), Duration: duration, Amplifier: amplifier})
	}

	return potionDefinition{Name: name, Effects: effects}, nil
}

func closingParenthesis(source []byte, open int) (int, error) {
	depth := 0
	inString := false
	escaped := false

	for index := open; index < len(source); index++ {
		character := source[index]

		if inString {
			if escaped {
				escaped = false

				continue
			}

			if character == '\\' {
				escaped = true

				continue
			}

			if character == '"' {
				inString = false
			}

			continue
		}

		switch character {
		case '"':
			inString = true
		case '(':
			depth++
		case ')':
			depth--

			if depth == 0 {
				return index, nil
			}
		}
	}

	return 0, fmt.Errorf("unterminated potion registration")
}

func generateGame(definitions []potionDefinition, sourceHash string) ([]byte, error) {
	var output bytes.Buffer

	fmt.Fprintln(&output, "// Code generated by cmd/generate-potions; DO NOT EDIT.")
	fmt.Fprintf(&output, "// Source SHA-256: %s\n", sourceHash)
	fmt.Fprintln(&output)
	fmt.Fprintln(&output, "package game")
	fmt.Fprintln(&output)
	fmt.Fprintln(&output, "const (")

	for id, definition := range definitions {
		fmt.Fprintf(&output, "\tPotion%s Potion = %d\n", goName(definition.Name), id)
	}

	fmt.Fprintln(&output, ")")
	fmt.Fprintln(&output)
	fmt.Fprintln(&output, "var generatedPotionDefinitions = [...]PotionDefinition{")

	for _, definition := range definitions {
		fmt.Fprintf(&output, "\t{Name: %q, Effects: %s},\n", definition.Name, effectSlice(definition.Effects))
	}

	fmt.Fprintln(&output, "}")

	return format.Source(output.Bytes())
}

func effectSlice(effects []potionEffectDefinition) string {
	if len(effects) == 0 {
		return "nil"
	}

	values := make([]string, len(effects))

	for index, effect := range effects {
		values[index] = fmt.Sprintf("{Effect: MobEffect%s, Duration: %d, Amplifier: %d, Visible: true, ShowIcon: true}", goName(effect.Name), effect.Duration, effect.Amplifier)
	}

	return "[]MobEffectInstance{" + strings.Join(values, ", ") + "}"
}

func goName(name string) string {
	parts := strings.Split(name, "_")

	for index, part := range parts {
		runes := []rune(part)
		if len(runes) > 0 {
			runes[0] = unicode.ToUpper(runes[0])
			parts[index] = string(runes)
		}
	}

	return strings.Join(parts, "")
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
