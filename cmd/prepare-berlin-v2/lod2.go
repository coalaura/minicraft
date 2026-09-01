package main

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type lod2Resource struct {
	ID       string
	Location string
	Remote   bool
}

func prepareLoD2(ctx context.Context, source, output string, logger *log.Logger) (int, error) {
	err := os.MkdirAll(output, 0o755)
	if err != nil {
		return 0, fmt.Errorf("create Berlin LoD2 output directory: %w", err)
	}

	resources, err := discoverLoD2Resources(ctx, source)
	if err != nil {
		return 0, err
	}

	if len(resources) == 0 {
		return 0, fmt.Errorf("no Berlin LoD2 resources found at %q", source)
	}

	logger.Printf("LoD2: found %d source resource(s)", len(resources))

	for index, resource := range resources {
		complete, err := preparationComplete(output, resource.ID)
		if err != nil {
			return 0, err
		}

		if complete {
			logger.Printf("LoD2 [%d/%d] already prepared: %s", index+1, len(resources), resource.ID)
			continue
		}

		logger.Printf("LoD2 [%d/%d] preparing: %s", index+1, len(resources), resource.ID)

		files, err := prepareLoD2Resource(ctx, resource, output)
		if err != nil {
			return 0, fmt.Errorf("prepare LoD2 %s: %w", resource.ID, err)
		}

		err = markPreparationComplete(output, resource.ID, files)
		if err != nil {
			return 0, err
		}
	}

	files, err := voxelTileFiles(output)
	if err != nil {
		return 0, fmt.Errorf("list Berlin LoD2 tiles: %w", err)
	}

	return len(files), nil
}

func discoverLoD2Resources(ctx context.Context, source string) ([]lod2Resource, error) {
	if strings.HasPrefix(source, "https://") || strings.HasPrefix(source, "http://") {
		resources, err := discoverAtomResources(ctx, source)
		if err != nil {
			return nil, err
		}

		result := make([]lod2Resource, 0, len(resources))
		for _, resource := range resources {
			result = append(result, lod2Resource{ID: resource.URL, Location: resource.URL, Remote: true})
		}

		return result, nil
	}

	info, err := os.Stat(source)
	if err != nil {
		return nil, fmt.Errorf("inspect Berlin LoD2 source %q: %w", source, err)
	}

	if !info.IsDir() {
		absolute, err := filepath.Abs(source)
		if err != nil {
			return nil, err
		}

		return []lod2Resource{{ID: absolute, Location: absolute}}, nil
	}

	resources := make([]lod2Resource, 0)
	err = filepath.WalkDir(source, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		if entry.IsDir() || !supportedLoD2LocalPath(path) {
			return nil
		}

		absolute, err := filepath.Abs(path)
		if err != nil {
			return err
		}

		resources = append(resources, lod2Resource{ID: absolute, Location: absolute})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan Berlin LoD2 source directory: %w", err)
	}

	sort.Slice(resources, func(left, right int) bool {
		return resources[left].ID < resources[right].ID
	})

	return resources, nil
}

func prepareLoD2Resource(ctx context.Context, resource lod2Resource, output string) ([]string, error) {
	path := resource.Location
	cleanup := func() {}

	if resource.Remote {
		downloaded, remove, err := downloadTemporary(ctx, output, resource.Location, "lod2-*")
		if err != nil {
			return nil, err
		}

		path = downloaded
		cleanup = remove
	}
	defer cleanup()

	extension := strings.ToLower(filepath.Ext(path))
	if extension == ".zip" || extension == ".download" {
		zipArchive, err := fileIsZip(path)
		if err != nil {
			return nil, err
		}

		if zipArchive {
			return prepareLoD2Zip(path, output)
		}
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open Berlin LoD2 file %q: %w", path, err)
	}
	defer file.Close()

	tiles := newVoxelTiles(output)
	err = prepareCityGML(file, tiles)
	if err != nil {
		return nil, err
	}

	return tiles.Flush()
}

func prepareLoD2Zip(path, output string) ([]string, error) {
	archive, err := zip.OpenReader(path)
	if err != nil {
		return nil, fmt.Errorf("open Berlin LoD2 ZIP %q: %w", path, err)
	}
	defer archive.Close()

	tiles := newVoxelTiles(output)
	found := false

	for _, entry := range archive.File {
		if entry.FileInfo().IsDir() || !supportedLoD2LocalPath(entry.Name) {
			continue
		}

		reader, err := entry.Open()
		if err != nil {
			return nil, fmt.Errorf("open %q in %q: %w", entry.Name, path, err)
		}

		prepareErr := prepareCityGML(reader, tiles)
		closeErr := reader.Close()

		if prepareErr != nil {
			return nil, fmt.Errorf("parse %q in %q: %w", entry.Name, path, prepareErr)
		}

		if closeErr != nil {
			return nil, fmt.Errorf("close %q in %q: %w", entry.Name, path, closeErr)
		}

		found = true
	}

	if !found {
		return nil, fmt.Errorf("Berlin LoD2 ZIP %q contains no CityGML .xml/.gml files", path)
	}

	return tiles.Flush()
}

func supportedLoD2LocalPath(path string) bool {
	extension := strings.ToLower(filepath.Ext(path))
	return extension == ".zip" || extension == ".xml" || extension == ".gml"
}

func downloadTemporary(ctx context.Context, output, location, pattern string) (string, func(), error) {
	temporaryDirectory := filepath.Join(output, ".downloads")
	err := os.MkdirAll(temporaryDirectory, 0o755)
	if err != nil {
		return "", nil, fmt.Errorf("create Berlin v2 download directory: %w", err)
	}

	extension := strings.ToLower(filepath.Ext(location))
	if extension == "" || len(extension) > 8 {
		extension = ".download"
	}

	temporary, err := os.CreateTemp(temporaryDirectory, pattern+extension)
	if err != nil {
		return "", nil, fmt.Errorf("create Berlin v2 download file: %w", err)
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
