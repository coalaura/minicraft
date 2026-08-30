package main

import "testing"

func TestHopperPlacementRuleUsesItsDedicatedIdentity(t *testing.T) {
	hopper := BlockDefinition{
		Name:         "hopper",
		DefaultState: 1,
		BoundingBox:  "block",
		Properties:   properties("enabled", "facing"),
	}

	rule := blockPlacementRule(hopper)
	if rule != "ItemPlacementHopper" {
		t.Fatalf("hopper placement rule = %q, want %q", rule, "ItemPlacementHopper")
	}
}
