package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
)

const (
	expectedMinecraftVersion = "1.21.11"
	expectedProtocolVersion  = 774
)

type clientVersion struct {
	Name            string `json:"name"`
	ProtocolVersion int    `json:"protocol_version"`
}

type catalogueVersion struct {
	Version          int    `json:"version"`
	MinecraftVersion string `json:"minecraftVersion"`
}

type directoryImport struct {
	source string
	target string
}

var (
	enchantmentKeyPattern      = regexp.MustCompile(`public static final ResourceKey<Enchantment> ([A-Z0-9_]+) = key\("([a-z0-9_]+)"\);`)
	enchantmentRegisterPattern = regexp.MustCompile(`(?s)register\(\s*context,\s*([A-Z0-9_]+),`)
	enchantCategoriesPattern   = regexp.MustCompile(`,\r?\n    "enchantCategories": \[\r?\n(?:      "[^"]+"(?:,\r?\n|\r?\n))*    \](,?)`)
)

func main() {
	referencePath := flag.String("reference", "../reference", "path containing client_source and minecraft-data")
	dataPath := flag.String("output", "data", "data directory to replace")

	flag.Parse()

	err := syncData(*referencePath, *dataPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func syncData(referencePath string, dataPath string) error {
	clientPath := filepath.Join(referencePath, "client_source")
	cataloguePath := filepath.Join(referencePath, "minecraft-data")

	err := validateVersions(clientPath, cataloguePath)
	if err != nil {
		return err
	}

	imports := []directoryImport{
		{source: filepath.Join(clientPath, "data", "minecraft", "loot_table", "blocks"), target: filepath.Join(dataPath, "block_loot")},
		{source: filepath.Join(clientPath, "data", "minecraft", "tags", "block"), target: filepath.Join(dataPath, "block_tags")},
		{source: filepath.Join(clientPath, "data", "minecraft", "enchantment"), target: filepath.Join(dataPath, "enchantments")},
		{source: filepath.Join(clientPath, "data", "minecraft", "tags", "enchantment"), target: filepath.Join(dataPath, "enchantment_tags")},
		{source: filepath.Join(clientPath, "data", "minecraft", "tags", "item"), target: filepath.Join(dataPath, "item_tags")},
		{source: filepath.Join(clientPath, "data", "minecraft", "recipe"), target: filepath.Join(dataPath, "recipes")},
	}

	catalogues := []string{"biomes.json", "blocks.json", "entities.json"}

	for _, item := range imports {
		err = replaceDirectory(item.source, item.target)
		if err != nil {
			return err
		}
	}

	for _, name := range catalogues {
		err = copyFile(filepath.Join(cataloguePath, name), filepath.Join(dataPath, name))
		if err != nil {
			return err
		}
	}

	err = copyItems(filepath.Join(cataloguePath, "items.json"), filepath.Join(dataPath, "items.json"))
	if err != nil {
		return err
	}

	javaPath := filepath.Join(clientPath, "net", "minecraft", "world", "item", "enchantment", "Enchantments.java")
	orderPath := filepath.Join(dataPath, "enchantment_order.json")

	err = writeEnchantmentOrder(javaPath, orderPath)
	if err != nil {
		return err
	}

	return nil
}

func validateVersions(clientPath string, cataloguePath string) error {
	var client clientVersion

	err := readJSON(filepath.Join(clientPath, "version.json"), &client)
	if err != nil {
		return err
	}

	if client.Name != expectedMinecraftVersion+" Unobfuscated" || client.ProtocolVersion != expectedProtocolVersion {
		return fmt.Errorf("client source is %q protocol %d, want %s protocol %d", client.Name, client.ProtocolVersion, expectedMinecraftVersion, expectedProtocolVersion)
	}

	var catalogue catalogueVersion

	err = readJSON(filepath.Join(cataloguePath, "version.json"), &catalogue)
	if err != nil {
		return err
	}

	if catalogue.MinecraftVersion != expectedMinecraftVersion || catalogue.Version != expectedProtocolVersion {
		return fmt.Errorf("minecraft-data is %q protocol %d, want %s protocol %d", catalogue.MinecraftVersion, catalogue.Version, expectedMinecraftVersion, expectedProtocolVersion)
	}

	return nil
}

func readJSON(path string, target any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	err = json.Unmarshal(data, target)
	if err != nil {
		return fmt.Errorf("decode %s: %w", path, err)
	}

	return nil
}

func replaceDirectory(source string, target string) error {
	err := os.RemoveAll(target)
	if err != nil {
		return err
	}

	err = filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		relative, relativeErr := filepath.Rel(source, path)
		if relativeErr != nil {
			return relativeErr
		}

		destination := filepath.Join(target, relative)

		if entry.IsDir() {
			return os.MkdirAll(destination, 0o755)
		}

		return copyFile(path, destination)
	})

	if err != nil {
		return fmt.Errorf("copy %s: %w", source, err)
	}

	return nil
}

func copyFile(source string, target string) error {
	data, err := os.ReadFile(source)
	if err != nil {
		return err
	}

	err = os.MkdirAll(filepath.Dir(target), 0o755)
	if err != nil {
		return err
	}

	err = os.WriteFile(target, data, 0o644)
	if err != nil {
		return err
	}

	return nil
}

func copyItems(source string, target string) error {
	data, err := os.ReadFile(source)
	if err != nil {
		return err
	}

	cleaned, count := stripEnchantCategories(data)
	if count == 0 {
		return errors.New("items catalogue contains no enchantCategories fields")
	}

	err = os.WriteFile(target, cleaned, 0o644)
	if err != nil {
		return err
	}

	return nil
}

func stripEnchantCategories(data []byte) ([]byte, int) {
	count := 0

	cleaned := enchantCategoriesPattern.ReplaceAllFunc(data, func(match []byte) []byte {
		count++

		if match[len(match)-1] == ',' {
			return []byte(",")
		}

		return nil
	})

	return cleaned, count
}

func writeEnchantmentOrder(source string, target string) error {
	data, err := os.ReadFile(source)
	if err != nil {
		return err
	}

	order, err := enchantmentOrder(data)
	if err != nil {
		return err
	}

	encoded, err := json.MarshalIndent(order, "", "  ")
	if err != nil {
		return err
	}

	encoded = append(encoded, '\n')

	err = os.WriteFile(target, encoded, 0o644)
	if err != nil {
		return err
	}

	return nil
}

func enchantmentOrder(data []byte) ([]string, error) {
	keys := make(map[string]string)

	for _, match := range enchantmentKeyPattern.FindAllSubmatch(data, -1) {
		keys[string(match[1])] = string(match[2])
	}

	bootstrapStart := bytes.Index(data, []byte("public static void bootstrap"))
	bootstrapEnd := bytes.Index(data, []byte("private static void register"))

	if bootstrapStart < 0 || bootstrapEnd <= bootstrapStart {
		return nil, errors.New("cannot locate enchantment bootstrap body")
	}

	matches := enchantmentRegisterPattern.FindAllSubmatch(data[bootstrapStart:bootstrapEnd], -1)

	order := make([]string, 0, len(matches))

	for _, match := range matches {
		constant := string(match[1])
		name, exists := keys[constant]

		if !exists {
			return nil, fmt.Errorf("bootstrap references unknown enchantment key %s", constant)
		}

		order = append(order, name)
	}

	if len(order) != 43 {
		return nil, fmt.Errorf("enchantment bootstrap contains %d registrations, want 43", len(order))
	}

	return order, nil
}
