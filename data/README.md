# Generated data inputs

All inputs needed to regenerate, build, test, and run minicraft are committed in this repository. Regenerate derived manifests and Go sources from a standalone checkout with:

```sh
go run ./cmd/sync-data
go generate ./...
```

To refresh the committed source inputs from the pinned Minecraft Java Edition 1.21.11 development references, explicitly supply their location:

```sh
go run ./cmd/sync-data -reference ../reference
```

The source-import command validates Minecraft version 1.21.11 and protocol 774 before replacing these inputs. It reads Java reference source only to extract compact JSON manifests; no Java files are copied into this repository. The reference tree is only needed for this explicit source refresh, not normal generation. Run `go generate ./...` afterward to update the Go sources that consume them.

## Exact copies

These paths are copied byte-for-byte from `reference/client_source/data/minecraft`:

| Local path | Client data path |
| --- | --- |
| `block_loot/` | `loot_table/blocks/` |
| `block_tags/` | `tags/block/` |
| `biome_tags/` | `tags/worldgen/biome/` |
| `enchantments/` | `enchantment/` |
| `enchantment_tags/` | `tags/enchantment/` |
| `item_tags/` | `tags/item/` |
| `recipes/` | `recipe/` |
| `worldgen_biome/` | `worldgen/biome/` |

`biomes.json`, `blocks.json`, and `entities.json` are copied byte-for-byte from `reference/minecraft-data`. That catalogue supplies protocol registry IDs and properties which the client data pack does not expose as equivalent flat JSON files.

## Natural spawning

`cmd/generate-spawns` defaults to `data/worldgen_biome/` and `data/biome_tags/`. It reads the original biome JSON spawners and potential costs, filters to supported entities, and resolves biome tags for sheep colors and tropical-fish placement. `go generate ./internal/server` regenerates `internal/server/natural_spawn_data_generated.go` using these same local inputs. Spawn lists are not hand-transcribed.

## Derived files

`items.json` is copied from `reference/minecraft-data/items.json` with only its non-vanilla `enchantCategories` metadata removed. Canonical enchantment item applicability comes from `item_tags/enchantable/` instead.

`enchantment_order.json` is a committed input extracted during explicit source refresh from the `register` calls in `Enchantments.bootstrap`. It records registry raw-ID order, which is not stored in the individual enchantment JSON files. Java field declaration order is not the registry order. Normal generation validates and consumes this local manifest.

`equipment_source.json` contains only the armor materials, humanoid armor registrations, and direct Equippable registrations extracted from the pinned Java sources during explicit source refresh. `cmd/sync-data` combines this local input with `item_tags/enchantable/equippable` to regenerate `item_armor_attributes.json` and `item_equippables.json`. The former contains defensive attributes; the latter independently records player equipment slots and server-relevant equip behavior. Item generation remains self-contained within `data/`.

`mob_effects.json` and `potions.json` contain their ordered registry definitions and their generators require no files outside `data/`.
