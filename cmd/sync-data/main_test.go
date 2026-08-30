package main

import (
	"reflect"
	"testing"
)

func TestStripEnchantCategories(t *testing.T) {
	input := []byte("[\n  {\n    \"id\": 1,\n    \"enchantCategories\": [\n      \"mining\",\n      \"durability\"\n    ],\n    \"maxDurability\": 10\n  },\n  {\n    \"id\": 2,\n    \"enchantCategories\": [\n      \"vanishing\"\n    ]\n  }\n]\n")
	want := "[\n  {\n    \"id\": 1,\n    \"maxDurability\": 10\n  },\n  {\n    \"id\": 2\n  }\n]\n"

	got, count := stripEnchantCategories(input)
	if count != 2 {
		t.Fatalf("removed fields = %d, want 2", count)
	}

	if string(got) != want {
		t.Fatalf("cleaned items:\n%s\nwant:\n%s", got, want)
	}
}

func TestEnchantmentOrder(t *testing.T) {
	input := []byte(`
public static final ResourceKey<Enchantment> FIRST = key("first");
public static final ResourceKey<Enchantment> SECOND = key("second");
public static void bootstrap(final BootstrapContext<Enchantment> context) {
   register(
      context,
      SECOND,
      builder()
   );
   register(context, FIRST, builder());
}
private static void register() {}
`)

	_, err := enchantmentOrder(input)
	if err == nil {
		t.Fatal("short bootstrap order was accepted")
	}

	repeated := make([]byte, 0, len(input)*22)

	repeated = append(repeated, []byte("public static final ResourceKey<Enchantment> ENTRY = key(\"entry\");\npublic static void bootstrap() {\n")...)

	for range 43 {
		repeated = append(repeated, []byte("register(context, ENTRY, builder());\n")...)
	}

	repeated = append(repeated, []byte("}\nprivate static void register() {}")...)

	got, err := enchantmentOrder(repeated)
	if err != nil {
		t.Fatal(err)
	}

	want := make([]string, 43)

	for index := range want {
		want[index] = "entry"
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("order = %q, want %q", got, want)
	}
}
