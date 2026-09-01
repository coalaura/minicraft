package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

type decimalTestCase struct {
	input string
	want  int
}

func TestParseRoundedDecimal(t *testing.T) {
	tests := []decimalTestCase{
		{input: "389918", want: 389918},
		{input: "35.49", want: 35},
		{input: "35.50", want: 36},
		{input: "-12.50", want: -13},
		{input: "+42.00", want: 42},
	}

	for _, test := range tests {
		got, err := parseRoundedDecimal([]byte(test.input))
		if err != nil {
			t.Fatalf("parse %q: %v", test.input, err)
		}

		if got != test.want {
			t.Fatalf("parse %q = %d, want %d", test.input, got, test.want)
		}
	}
}

func TestDiscoverRemoteResourcesFollowsAtomDatasetFeed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/atom/":
			writer.Header().Set("Content-Type", "application/atom+xml")
			_, _ = fmt.Fprint(writer, `<?xml version="1.0"?><feed xmlns="http://www.w3.org/2005/Atom"><entry><link rel="alternate" type="application/atom+xml" href="/dataset.xml"/></entry></feed>`)
		case "/dataset.xml":
			writer.Header().Set("Content-Type", "application/atom+xml")
			_, _ = fmt.Fprint(writer, `<?xml version="1.0"?><feed xmlns="http://www.w3.org/2005/Atom"><entry><link rel="enclosure" type="application/zip" href="/tiles/dgm1.zip"/></entry></feed>`)
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	resources, err := discoverRemoteResources(context.Background(), server.URL+"/atom/")
	if err != nil {
		t.Fatalf("discover resources: %v", err)
	}

	if len(resources) != 1 {
		t.Fatalf("resource count = %d, want 1", len(resources))
	}

	want := server.URL + "/tiles/dgm1.zip"
	if resources[0].Location != want {
		t.Fatalf("resource = %q, want %q", resources[0].Location, want)
	}
}
