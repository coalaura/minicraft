package berlinheightprep

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/coalaura/minicraft/internal/berlinheight"
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

func TestPrepareXYZAcceptsInclusiveBerlinTileBoundary(t *testing.T) {
	output := t.TempDir()
	source := strings.NewReader(strings.Join([]string{
		"370000 5808000 99.0",
		"368000 5810000 98.0",
		"370000 5810000 97.0",
		"368000 5808000 41.2",
		"369999 5809999 52.4",
	}, "\n"))

	filename, err := prepareXYZ(source, "dom1_33_368_5808_2_be_2026.xyz", output)
	if err != nil {
		t.Fatalf("prepare XYZ with inclusive boundary: %v", err)
	}

	if filename != "368000_5808000.mch" {
		t.Fatalf("prepared filename = %q, want %q", filename, "368000_5808000.mch")
	}

	store, err := berlinheight.Open(output)
	if err != nil {
		t.Fatalf("open prepared heights: %v", err)
	}

	height, valid := store.HeightAt(368000, 5808000)
	if !valid || height != 41 {
		t.Fatalf("south-west height = %d, valid %t, want 41, true", height, valid)
	}

	height, valid = store.HeightAt(369999, 5809999)
	if !valid || height != 52 {
		t.Fatalf("north-east interior height = %d, valid %t, want 52, true", height, valid)
	}

	key := berlinheight.TileKey{Easting: 368000, Northing: 5808000}
	header, valid := store.TileBounds(key)
	if !valid {
		t.Fatal("prepared tile header is missing")
	}

	if header.ValidCells != 2 {
		t.Fatalf("valid cell count = %d, want 2", header.ValidCells)
	}

	if header.MinimumY != 41 || header.MaximumY != 52 {
		t.Fatalf("prepared height range = %d..%d, want 41..52", header.MinimumY, header.MaximumY)
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
