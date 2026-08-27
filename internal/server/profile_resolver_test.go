package server

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coalaura/minicraft/internal/config"
	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

const testMojangProfileID = "0123456789abcdef0123456789abcdef"

func TestOfflineProfileResolution(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/lookup/Bob":
			fmt.Fprintf(writer, `{"id":%q,"name":"Bob"}`, testMojangProfileID)
		case "/session/" + testMojangProfileID:
			if request.URL.Query().Get("unsigned") != "false" {
				http.Error(writer, "unsigned must be false", http.StatusBadRequest)

				return
			}

			fmt.Fprintf(writer, `{"id":%q,"name":"Bob","properties":[{"name":"textures","value":"texture-value","signature":"exact-signature"}]}`, testMojangProfileID)
		default:
			http.NotFound(writer, request)
		}
	}))

	defer server.Close()

	resolver := newTestOfflineProfileResolver(server)

	properties, err := resolver.resolve(t.Context(), "Bob")
	if err != nil {
		t.Fatalf("resolve profile: %v", err)
	}

	want := []game.ProfileProperty{{Name: "textures", Value: "texture-value", Signature: "exact-signature"}}
	if len(properties) != len(want) || properties[0] != want[0] {
		t.Fatalf("properties = %+v, want %+v", properties, want)
	}
}

func TestOfflineLoginKeepsOfflineUUID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/lookup/Bob" {
			fmt.Fprintf(writer, `{"id":%q}`, testMojangProfileID)

			return
		}

		fmt.Fprintf(writer, `{"id":%q,"properties":[{"name":"textures","value":"value","signature":"signature"}]}`, testMojangProfileID)
	}))

	defer server.Close()

	session := &Session{
		Config:          &config.Config{},
		Runtime:         NewRuntime(&game.World{}),
		offlineProfiles: newTestOfflineProfileResolver(server),
	}

	err := session.handleOfflineLogin(t.Context(), protocol.LoginStart{Name: "Bob"})
	if err != nil {
		t.Fatalf("offline login: %v", err)
	}

	if session.Player.UUID != "faa5dca3-c3d4-354b-ae1b-dde9e5a14b3b" {
		t.Fatalf("uuid = %q, want deterministic offline uuid", session.Player.UUID)
	}

	if len(session.Player.Properties) != 1 || session.Player.Properties[0].Signature != "signature" {
		t.Fatalf("properties = %+v, want signed textures", session.Player.Properties)
	}
}

func TestOfflineProfileUnknownUsernameFallsBackAndCaches(t *testing.T) {
	var requests atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests.Add(1)
		http.NotFound(writer, request)
	}))

	defer server.Close()

	resolver := newTestOfflineProfileResolver(server)

	session := &Session{
		Config:          &config.Config{},
		Runtime:         NewRuntime(&game.World{}),
		offlineProfiles: resolver,
	}

	for range 2 {
		err := session.handleOfflineLogin(t.Context(), protocol.LoginStart{Name: "UnknownPlayer"})
		if err != nil {
			t.Fatalf("offline login: %v", err)
		}

		if len(session.Player.Properties) != 0 {
			t.Fatalf("properties = %+v, want default properties", session.Player.Properties)
		}
	}

	if requests.Load() != 1 {
		t.Fatalf("requests = %d, want one cached lookup", requests.Load())
	}
}

func TestOfflineProfileAPIFailureDoesNotRejectLogin(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		http.Error(writer, "unavailable", http.StatusTooManyRequests)
	}))

	defer server.Close()

	session := &Session{
		Config:          &config.Config{},
		Runtime:         NewRuntime(&game.World{}),
		offlineProfiles: newTestOfflineProfileResolver(server),
	}

	err := session.handleOfflineLogin(t.Context(), protocol.LoginStart{Name: "Bob"})
	if err != nil {
		t.Fatalf("offline login: %v", err)
	}

	if len(session.Player.Properties) != 0 {
		t.Fatalf("properties = %+v, want default properties", session.Player.Properties)
	}
}

func TestOfflineProfileMalformedResponseFallsBack(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		fmt.Fprint(writer, `{"id":"not-a-uuid"}`)
	}))

	defer server.Close()

	session := &Session{
		Config:          &config.Config{},
		Runtime:         NewRuntime(&game.World{}),
		offlineProfiles: newTestOfflineProfileResolver(server),
	}

	err := session.handleOfflineLogin(t.Context(), protocol.LoginStart{Name: "Bob"})
	if err != nil {
		t.Fatalf("offline login: %v", err)
	}

	if len(session.Player.Properties) != 0 {
		t.Fatalf("properties = %+v, want default properties", session.Player.Properties)
	}
}

func TestOfflineProfileCachePreventsRepeatRequests(t *testing.T) {
	var requests atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests.Add(1)

		if request.URL.Path == "/lookup/Bob" {
			fmt.Fprintf(writer, `{"id":%q}`, testMojangProfileID)

			return
		}

		fmt.Fprintf(writer, `{"id":%q,"properties":[{"name":"textures","value":"value","signature":"signature"}]}`, testMojangProfileID)
	}))

	defer server.Close()

	resolver := newTestOfflineProfileResolver(server)

	for _, username := range []string{"Bob", "bob"} {
		properties, err := resolver.resolve(t.Context(), username)
		if err != nil {
			t.Fatalf("resolve %q: %v", username, err)
		}

		if len(properties) != 1 {
			t.Fatalf("properties for %q = %+v", username, properties)
		}
	}

	if requests.Load() != 2 {
		t.Fatalf("requests = %d, want one lookup and one session request", requests.Load())
	}
}

func TestOfflineProfileResolutionCanBeDisabled(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests.Add(1)
		http.Error(writer, "unexpected request", http.StatusInternalServerError)
	}))
	defer server.Close()

	session := &Session{
		Config: &config.Config{Server: config.ServerConfig{
			ResolveOfflineSkins: new(false),
		}},
		Runtime:         NewRuntime(&game.World{}),
		offlineProfiles: newTestOfflineProfileResolver(server),
	}

	err := session.handleOfflineLogin(t.Context(), protocol.LoginStart{Name: "Laura"})
	if err != nil {
		t.Fatalf("offline login: %v", err)
	}

	if requests.Load() != 0 {
		t.Fatalf("requests = %d, want none", requests.Load())
	}

	if len(session.Player.Properties) != 0 {
		t.Fatalf("properties = %+v, want default properties", session.Player.Properties)
	}
}

func newTestOfflineProfileResolver(server *httptest.Server) *offlineProfileResolver {
	return newOfflineProfileResolver(server.Client(), server.URL+"/lookup", server.URL+"/session", time.Hour)
}
