package main

import (
	"strings"
	"testing"

	"github.com/coalaura/minicraft/internal/berlinvoxel"
)

func TestPrepareCityGMLCreatesHollowBuildingShell(t *testing.T) {
	directory := t.TempDir()
	tiles := newVoxelTiles(directory)

	cityGML := `<?xml version="1.0" encoding="UTF-8"?>
<core:CityModel xmlns:core="http://www.opengis.net/citygml/2.0" xmlns:bldg="http://www.opengis.net/citygml/building/2.0" xmlns:gml="http://www.opengis.net/gml">
  <bldg:Building>
    <bldg:boundedBy><bldg:GroundSurface><gml:Polygon><gml:exterior><gml:LinearRing><gml:posList srsDimension="3">389918 5819699 40 389922 5819699 40 389922 5819695 40 389918 5819695 40 389918 5819699 40</gml:posList></gml:LinearRing></gml:exterior></gml:Polygon></bldg:GroundSurface></bldg:boundedBy>
    <bldg:boundedBy><bldg:RoofSurface><gml:Polygon><gml:exterior><gml:LinearRing><gml:posList srsDimension="3">389918 5819699 50 389918 5819695 50 389922 5819695 50 389922 5819699 50 389918 5819699 50</gml:posList></gml:LinearRing></gml:exterior></gml:Polygon></bldg:RoofSurface></bldg:boundedBy>
    <bldg:boundedBy><bldg:WallSurface><gml:Polygon><gml:exterior><gml:LinearRing><gml:posList srsDimension="3">389918 5819699 40 389918 5819695 40 389918 5819695 50 389918 5819699 50 389918 5819699 40</gml:posList></gml:LinearRing></gml:exterior></gml:Polygon></bldg:WallSurface></bldg:boundedBy>
  </bldg:Building>
</core:CityModel>`

	err := prepareCityGML(strings.NewReader(cityGML), tiles)
	if err != nil {
		t.Fatalf("prepare CityGML: %v", err)
	}

	_, err = tiles.Flush()
	if err != nil {
		t.Fatalf("flush CityGML tiles: %v", err)
	}

	store, err := berlinvoxel.Open(directory)
	if err != nil {
		t.Fatalf("open prepared CityGML store: %v", err)
	}

	marked, spans, valid := store.ColumnAt(389920, 5819697)
	if !valid || !marked {
		t.Fatalf("interior footprint = valid %v marked %v, want true true", valid, marked)
	}

	if !spansContainRaw(spans, 50) {
		t.Fatalf("interior roof spans = %+v, want height 50", spans)
	}

	if spansContainRaw(spans, 45) {
		t.Fatalf("interior spans = %+v, unexpectedly filled building interior at height 45", spans)
	}

	_, wallSpans, valid := store.ColumnAt(389918, 5819697)
	if !valid || !spansContainRaw(wallSpans, 45) {
		t.Fatalf("wall spans = %+v valid=%v, want wall at height 45", wallSpans, valid)
	}
}

func spansContainRaw(spans []berlinvoxel.Span, height uint16) bool {
	for _, span := range spans {
		if height >= span.MinimumY && height <= span.MaximumY {
			return true
		}
	}

	return false
}

func TestPrepareCityGMLClipsGeometryOutsideMinecraftHeight(t *testing.T) {
	directory := t.TempDir()
	tiles := newVoxelTiles(directory)

	cityGML := `<?xml version="1.0" encoding="UTF-8"?>
<core:CityModel xmlns:core="http://www.opengis.net/citygml/2.0" xmlns:bldg="http://www.opengis.net/citygml/building/2.0" xmlns:gml="http://www.opengis.net/gml">
  <bldg:Building>
    <bldg:boundedBy><bldg:GroundSurface><gml:Polygon><gml:exterior><gml:LinearRing><gml:posList srsDimension="3">389020 5817942 -30 389024 5817942 -30 389024 5817938 -30 389020 5817938 -30 389020 5817942 -30</gml:posList></gml:LinearRing></gml:exterior></gml:Polygon></bldg:GroundSurface></bldg:boundedBy>
    <bldg:boundedBy><bldg:RoofSurface><gml:Polygon><gml:exterior><gml:LinearRing><gml:posList srsDimension="3">389020 5817942 40 389020 5817938 40 389024 5817938 40 389024 5817942 40 389020 5817942 40</gml:posList></gml:LinearRing></gml:exterior></gml:Polygon></bldg:RoofSurface></bldg:boundedBy>
    <bldg:boundedBy><bldg:WallSurface><gml:Polygon><gml:exterior><gml:LinearRing><gml:posList srsDimension="3">389020 5817942 -30 389020 5817938 -30 389020 5817938 40 389020 5817942 40 389020 5817942 -30</gml:posList></gml:LinearRing></gml:exterior></gml:Polygon></bldg:WallSurface></bldg:boundedBy>
  </bldg:Building>
</core:CityModel>`

	err := prepareCityGML(strings.NewReader(cityGML), tiles)
	if err != nil {
		t.Fatalf("prepare CityGML with below-world geometry: %v", err)
	}

	_, err = tiles.Flush()
	if err != nil {
		t.Fatalf("flush clipped CityGML tiles: %v", err)
	}

	store, err := berlinvoxel.Open(directory)
	if err != nil {
		t.Fatalf("open clipped CityGML store: %v", err)
	}

	_, wallSpans, valid := store.ColumnAt(389020, 5817940)
	if !valid {
		t.Fatal("clipped wall column is missing")
	}

	if !spansContainRaw(wallSpans, uint16(minimumPreparedHeight)) {
		t.Fatalf("wall spans = %+v, want visible lower boundary %d", wallSpans, minimumPreparedHeight)
	}

	if !spansContainRaw(wallSpans, 40) {
		t.Fatalf("wall spans = %+v, want visible height 40", wallSpans)
	}

	for _, span := range wallSpans {
		if span.MinimumY < uint16(minimumPreparedHeight) {
			t.Fatalf("wall span %+v extends below prepared minimum %d", span, minimumPreparedHeight)
		}
	}
}
