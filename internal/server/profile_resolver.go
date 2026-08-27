package server

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/coalaura/minicraft/internal/game"
)

const (
	defaultProfileCacheTTL = 45 * time.Minute
	profileRequestTimeout  = 5 * time.Second

	minecraftProfileLookupURL  = "https://api.minecraftservices.com/minecraft/profile/lookup/name"
	minecraftSessionProfileURL = "https://sessionserver.mojang.com/session/minecraft/profile"
)

type profileCacheEntry struct {
	properties []game.ProfileProperty
	expiresAt  time.Time
}

type minecraftProfileLookupResponse struct {
	ID string `json:"id"`
}

type offlineProfileResolver struct {
	client            *http.Client
	profileLookupURL  string
	sessionProfileURL string
	cacheTTL          time.Duration

	cacheMx sync.Mutex
	cache   map[string]profileCacheEntry
}

var defaultOfflineProfileResolver = newOfflineProfileResolver(
	&http.Client{Timeout: profileRequestTimeout},
	minecraftProfileLookupURL,
	minecraftSessionProfileURL,
	defaultProfileCacheTTL,
)

func newOfflineProfileResolver(client *http.Client, profileLookupURL, sessionProfileURL string, cacheTTL time.Duration) *offlineProfileResolver {
	return &offlineProfileResolver{
		client:            client,
		profileLookupURL:  strings.TrimRight(profileLookupURL, "/"),
		sessionProfileURL: strings.TrimRight(sessionProfileURL, "/"),
		cacheTTL:          cacheTTL,
		cache:             make(map[string]profileCacheEntry),
	}
}

func (resolver *offlineProfileResolver) resolve(ctx context.Context, username string) ([]game.ProfileProperty, error) {
	cacheKey := strings.ToLower(username)

	properties, cached := resolver.cached(cacheKey)
	if cached {
		return properties, nil
	}

	profileID, found, err := resolver.lookupProfileID(ctx, username)
	if err != nil {
		return nil, err
	}

	if !found {
		resolver.store(cacheKey, nil)

		return nil, nil
	}

	properties, err = resolver.lookupProperties(ctx, profileID)
	if err != nil {
		return nil, err
	}

	resolver.store(cacheKey, properties)

	return cloneProfileProperties(properties), nil
}

func (resolver *offlineProfileResolver) lookupProfileID(ctx context.Context, username string) (string, bool, error) {
	requestURL := resolver.profileLookupURL + "/" + url.PathEscape(username)

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return "", false, err
	}

	response, err := resolver.client.Do(request)
	if err != nil {
		return "", false, err
	}

	defer response.Body.Close()

	if response.StatusCode == http.StatusNotFound {
		return "", false, nil
	}

	if response.StatusCode != http.StatusOK {
		return "", false, fmt.Errorf("profile lookup status: %d", response.StatusCode)
	}

	var profile minecraftProfileLookupResponse

	err = json.NewDecoder(response.Body).Decode(&profile)
	if err != nil {
		return "", false, fmt.Errorf("decode profile lookup: %w", err)
	}

	rawUUID, err := hex.DecodeString(profile.ID)
	if err != nil || len(rawUUID) != 16 {
		return "", false, errors.New("malformed profile lookup uuid")
	}

	return profile.ID, true, nil
}

func (resolver *offlineProfileResolver) lookupProperties(ctx context.Context, profileID string) ([]game.ProfileProperty, error) {
	requestURL := resolver.sessionProfileURL + "/" + url.PathEscape(profileID) + "?unsigned=false"

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, err
	}

	response, err := resolver.client.Do(request)
	if err != nil {
		return nil, err
	}

	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("session profile status: %d", response.StatusCode)
	}

	var profile SessionProfileResponse

	err = json.NewDecoder(response.Body).Decode(&profile)
	if err != nil {
		return nil, fmt.Errorf("decode session profile: %w", err)
	}

	if !strings.EqualFold(profile.ID, profileID) {
		return nil, errors.New("session profile uuid mismatch")
	}

	for _, property := range profile.Properties {
		if property.Name == "textures" && property.Value != "" && property.Signature != "" {
			return cloneProfileProperties(profile.Properties), nil
		}
	}

	return nil, errors.New("session profile has no signed textures property")
}

func (resolver *offlineProfileResolver) cached(cacheKey string) ([]game.ProfileProperty, bool) {
	resolver.cacheMx.Lock()
	defer resolver.cacheMx.Unlock()

	entry, exists := resolver.cache[cacheKey]
	if !exists {
		return nil, false
	}

	if time.Now().After(entry.expiresAt) {
		delete(resolver.cache, cacheKey)

		return nil, false
	}

	return cloneProfileProperties(entry.properties), true
}

func (resolver *offlineProfileResolver) store(cacheKey string, properties []game.ProfileProperty) {
	resolver.cacheMx.Lock()
	defer resolver.cacheMx.Unlock()

	resolver.cache[cacheKey] = profileCacheEntry{
		properties: cloneProfileProperties(properties),
		expiresAt:  time.Now().Add(resolver.cacheTTL),
	}
}

func cloneProfileProperties(properties []game.ProfileProperty) []game.ProfileProperty {
	return append([]game.ProfileProperty(nil), properties...)
}
