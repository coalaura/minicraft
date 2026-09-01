package main

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"sort"
	"strings"
)

const maximumAtomBytes = 16 << 20

type atomFeed struct {
	Links   []atomLink  `xml:"link"`
	Entries []atomEntry `xml:"entry"`
}

type atomEntry struct {
	Links []atomLink `xml:"link"`
}

type atomLink struct {
	Href string `xml:"href,attr"`
	Rel  string `xml:"rel,attr"`
	Type string `xml:"type,attr"`
}

func discoverRemoteResources(ctx context.Context, source string) ([]inputResource, error) {
	if supportedResourcePath(source) {
		return []inputResource{{ID: source, Location: source, Remote: true}}, nil
	}

	client := &http.Client{}
	queue := []string{source}
	feeds := make(map[string]struct{})
	resources := make(map[string]struct{})

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		if _, visited := feeds[current]; visited {
			continue
		}
		feeds[current] = struct{}{}

		feed, err := fetchAtom(ctx, client, current)
		if err != nil {
			return nil, err
		}

		links := append([]atomLink{}, feed.Links...)
		for _, entry := range feed.Entries {
			links = append(links, entry.Links...)
		}

		for _, link := range links {
			resolved, err := resolveURL(current, link.Href)
			if err != nil {
				continue
			}

			if resourceLink(link, resolved) {
				resources[resolved] = struct{}{}
				continue
			}

			if atomLinkTarget(link) {
				queue = append(queue, resolved)
			}
		}
	}

	locations := make([]string, 0, len(resources))
	for location := range resources {
		locations = append(locations, location)
	}
	sort.Strings(locations)

	result := make([]inputResource, 0, len(locations))
	for _, location := range locations {
		result = append(result, inputResource{ID: location, Location: location, Remote: true})
	}

	return result, nil
}

func fetchAtom(ctx context.Context, client *http.Client, location string) (atomFeed, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, location, nil)
	if err != nil {
		return atomFeed{}, fmt.Errorf("create Atom request %q: %w", location, err)
	}

	response, err := client.Do(request)
	if err != nil {
		return atomFeed{}, fmt.Errorf("fetch Atom feed %q: %w", location, err)
	}
	defer response.Body.Close()

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return atomFeed{}, fmt.Errorf("fetch Atom feed %q: HTTP %s", location, response.Status)
	}

	decoder := xml.NewDecoder(io.LimitReader(response.Body, maximumAtomBytes))
	feed := atomFeed{}

	err = decoder.Decode(&feed)
	if err != nil {
		return atomFeed{}, fmt.Errorf("decode Atom feed %q: %w", location, err)
	}

	return feed, nil
}

func resolveURL(base, reference string) (string, error) {
	baseURL, err := url.Parse(base)
	if err != nil {
		return "", err
	}

	referenceURL, err := url.Parse(reference)
	if err != nil {
		return "", err
	}

	return baseURL.ResolveReference(referenceURL).String(), nil
}

func resourceLink(link atomLink, resolved string) bool {
	if supportedResourcePath(resolved) {
		return true
	}

	mediaType := strings.ToLower(link.Type)
	if link.Rel == "enclosure" && (strings.Contains(mediaType, "zip") || strings.Contains(mediaType, "octet-stream") || strings.Contains(mediaType, "text/plain")) {
		return true
	}

	return false
}

func atomLinkTarget(link atomLink) bool {
	if strings.EqualFold(link.Rel, "self") || strings.EqualFold(link.Rel, "up") {
		return false
	}

	return strings.Contains(strings.ToLower(link.Type), "application/atom+xml")
}

func supportedResourcePath(location string) bool {
	parsed, err := url.Parse(location)
	if err != nil {
		return false
	}

	extension := strings.ToLower(filepath.Ext(parsed.Path))
	return extension == ".zip" || extension == ".xyz" || extension == ".gz"
}
