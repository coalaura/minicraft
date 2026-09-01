package main

import (
	"archive/zip"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type inputResource struct {
	ID       string
	Location string
	Remote   bool
}

type resourceState struct {
	ID    string   `json:"id"`
	Tiles []string `json:"tiles"`
}

func discoverResources(ctx context.Context, source string) ([]inputResource, error) {
	if strings.HasPrefix(source, "https://") || strings.HasPrefix(source, "http://") {
		return discoverRemoteResources(ctx, source)
	}

	info, err := os.Stat(source)
	if err != nil {
		return nil, fmt.Errorf("inspect Berlin height source %q: %w", source, err)
	}

	if !info.IsDir() {
		absolute, err := filepath.Abs(source)
		if err != nil {
			return nil, err
		}

		return []inputResource{{ID: absolute, Location: absolute}}, nil
	}

	resources := make([]inputResource, 0)
	err = filepath.WalkDir(source, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		if entry.IsDir() || !supportedLocalPath(path) {
			return nil
		}

		absolute, err := filepath.Abs(path)
		if err != nil {
			return err
		}

		resources = append(resources, inputResource{ID: absolute, Location: absolute})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan Berlin height source directory: %w", err)
	}

	sort.Slice(resources, func(left, right int) bool {
		return resources[left].ID < resources[right].ID
	})

	return resources, nil
}

func prepareResource(ctx context.Context, output string, resource inputResource) ([]string, error) {
	path := resource.Location
	cleanup := func() {}

	if resource.Remote {
		downloaded, remove, err := downloadResource(ctx, output, resource.Location)
		if err != nil {
			return nil, err
		}

		path = downloaded
		cleanup = remove
	}
	defer cleanup()

	return preparePath(path, output)
}

func downloadResource(ctx context.Context, output, location string) (string, func(), error) {
	temporaryDirectory := filepath.Join(output, ".downloads")
	err := os.MkdirAll(temporaryDirectory, 0o755)
	if err != nil {
		return "", nil, fmt.Errorf("create Berlin download directory: %w", err)
	}

	extension := remoteExtension(location)
	temporary, err := os.CreateTemp(temporaryDirectory, "dgm1-*"+extension)
	if err != nil {
		return "", nil, fmt.Errorf("create Berlin download file: %w", err)
	}

	path := temporary.Name()
	cleanup := func() {
		_ = os.Remove(path)
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, location, nil)
	if err != nil {
		_ = temporary.Close()
		cleanup()
		return "", nil, err
	}

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		_ = temporary.Close()
		cleanup()
		return "", nil, fmt.Errorf("download %q: %w", location, err)
	}
	defer response.Body.Close()

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		_ = temporary.Close()
		cleanup()
		return "", nil, fmt.Errorf("download %q: HTTP %s", location, response.Status)
	}

	_, err = io.Copy(temporary, response.Body)
	if err != nil {
		_ = temporary.Close()
		cleanup()
		return "", nil, fmt.Errorf("download %q: %w", location, err)
	}

	err = temporary.Close()
	if err != nil {
		cleanup()
		return "", nil, fmt.Errorf("close downloaded resource: %w", err)
	}

	return path, cleanup, nil
}

func preparePath(path, output string) ([]string, error) {
	extension := strings.ToLower(filepath.Ext(path))

	switch extension {
	case ".zip":
		return prepareZip(path, output)
	case ".gz":
		return prepareGzip(path, output)
	case ".xyz":
		return prepareXYZFile(path, output)
	default:
		zipArchive, err := fileIsZip(path)
		if err != nil {
			return nil, err
		}

		if zipArchive {
			return prepareZip(path, output)
		}

		return prepareXYZFile(path, output)
	}
}

func prepareZip(path, output string) ([]string, error) {
	archive, err := zip.OpenReader(path)
	if err != nil {
		return nil, fmt.Errorf("open Berlin height ZIP %q: %w", path, err)
	}
	defer archive.Close()

	tiles := make([]string, 0)

	for _, entry := range archive.File {
		if entry.FileInfo().IsDir() || strings.ToLower(filepath.Ext(entry.Name)) != ".xyz" {
			continue
		}

		reader, err := entry.Open()
		if err != nil {
			return nil, fmt.Errorf("open %q in %q: %w", entry.Name, path, err)
		}

		tile, prepareErr := prepareXYZ(reader, entry.Name, output)
		closeErr := reader.Close()

		if prepareErr != nil {
			return nil, prepareErr
		}

		if closeErr != nil {
			return nil, fmt.Errorf("close %q in %q: %w", entry.Name, path, closeErr)
		}

		tiles = append(tiles, tile)
	}

	if len(tiles) == 0 {
		return nil, fmt.Errorf("Berlin height ZIP %q contains no .xyz files", path)
	}

	return tiles, nil
}

func prepareGzip(path, output string) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open Berlin height gzip %q: %w", path, err)
	}
	defer file.Close()

	reader, err := gzip.NewReader(file)
	if err != nil {
		return nil, fmt.Errorf("open Berlin height gzip stream %q: %w", path, err)
	}
	defer reader.Close()

	name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	tile, err := prepareXYZ(reader, name, output)
	if err != nil {
		return nil, err
	}

	return []string{tile}, nil
}

func prepareXYZFile(path, output string) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open Berlin height XYZ %q: %w", path, err)
	}
	defer file.Close()

	tile, err := prepareXYZ(file, filepath.Base(path), output)
	if err != nil {
		return nil, err
	}

	return []string{tile}, nil
}

func fileIsZip(path string) (bool, error) {
	file, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer file.Close()

	magic := make([]byte, 4)
	_, err = io.ReadFull(file, magic)
	if err != nil {
		return false, err
	}

	return magic[0] == 'P' && magic[1] == 'K' && magic[2] == 3 && magic[3] == 4, nil
}

func resourceComplete(output string, resource inputResource) (bool, error) {
	path := resourceStatePath(output, resource.ID)
	contents, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return false, nil
	}

	if err != nil {
		return false, fmt.Errorf("read Berlin preparation state: %w", err)
	}

	state := resourceState{}
	err = json.Unmarshal(contents, &state)
	if err != nil || state.ID != resource.ID || len(state.Tiles) == 0 {
		return false, nil
	}

	for _, tile := range state.Tiles {
		_, err := os.Stat(filepath.Join(output, tile))
		if err != nil {
			return false, nil
		}
	}

	return true, nil
}

func markResourceComplete(output string, resource inputResource, tiles []string) error {
	stateDirectory := filepath.Join(output, ".state")
	err := os.MkdirAll(stateDirectory, 0o755)
	if err != nil {
		return fmt.Errorf("create Berlin preparation state directory: %w", err)
	}

	sort.Strings(tiles)
	state := resourceState{ID: resource.ID, Tiles: tiles}

	contents, err := json.Marshal(state)
	if err != nil {
		return err
	}

	contents = append(contents, '\n')
	path := resourceStatePath(output, resource.ID)

	err = os.WriteFile(path, contents, 0o644)
	if err != nil {
		return fmt.Errorf("write Berlin preparation state: %w", err)
	}

	return nil
}

func resourceStatePath(output, id string) string {
	digest := sha256.Sum256([]byte(id))
	name := hex.EncodeToString(digest[:]) + ".json"
	return filepath.Join(output, ".state", name)
}

func remoteExtension(location string) string {
	lower := strings.ToLower(location)

	for _, extension := range []string{".zip", ".xyz", ".gz"} {
		if strings.HasSuffix(lower, extension) {
			return extension
		}
	}

	return ".download"
}

func supportedLocalPath(path string) bool {
	extension := strings.ToLower(filepath.Ext(path))
	return extension == ".zip" || extension == ".xyz" || extension == ".gz"
}
