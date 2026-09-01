package berlinvoxel

import (
	"testing"

	"github.com/coalaura/minicraft/internal/berlinheight"
)

func TestBuilderAndStoreRoundTrip(t *testing.T) {
	directory := t.TempDir()
	key := berlinheight.TileKey{Easting: 388000, Northing: 5818000}
	builder := NewBuilder(key)

	if !builder.Mark(389918, 5819699) {
		t.Fatal("mark Brandenburg Gate cell failed")
	}

	if !builder.AddSpan(389918, 5819699, 35, 48) {
		t.Fatal("add wall span failed")
	}

	if !builder.AddBlock(389918, 5819699, 49) {
		t.Fatal("add adjacent roof block failed")
	}

	if !builder.AddBlock(389919, 5819699, 52) {
		t.Fatal("add neighbouring roof block failed")
	}

	_, err := builder.Write(directory)
	if err != nil {
		t.Fatalf("write builder: %v", err)
	}

	store, err := Open(directory)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}

	marked, spans, valid := store.ColumnAt(389918, 5819699)
	if !valid || !marked {
		t.Fatalf("column valid=%t marked=%t, want true true", valid, marked)
	}

	if len(spans) != 1 || spans[0] != (Span{MinimumY: 35, MaximumY: 49}) {
		t.Fatalf("spans = %+v, want 35..49", spans)
	}

	marked, spans, valid = store.ColumnAt(389919, 5819699)
	if !valid || marked {
		t.Fatalf("neighbour valid=%t marked=%t, want true false", valid, marked)
	}

	if len(spans) != 1 || spans[0] != (Span{MinimumY: 52, MaximumY: 52}) {
		t.Fatalf("neighbour spans = %+v, want 52", spans)
	}
}
