# Generated data inputs

All inputs needed to regenerate, build, test, and run minicraft are committed in this repository. Regenerate derived manifests and Go sources from a standalone checkout with:

```sh
go run ./cmd/sync-data
go generate ./...
```

Repository tooling reads only committed local inputs, primarily under `data/`; `cmd/sync-data` regenerates derived manifests and has no external-reference import mode. Refreshing pinned Minecraft Java Edition 1.21.11 (protocol 774) inputs is a separate development-workspace task: research the workspace's `reference/` sources there, then copy or derive the required inputs into `minicraft/data/`. External-source readers and import tooling do not belong in this repository.

## Exact copies

These inputs originate from the pinned client data pack's `data/minecraft` tree; retained files are byte-for-byte copies:

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

`biomes.json`, `blocks.json`, and `entities.json` are byte-for-byte copies from the pinned minecraft-data catalogue. That catalogue supplies protocol registry IDs and properties which the client data pack does not expose as equivalent flat JSON files.

## Natural spawning

`cmd/generate-spawns` defaults to `data/worldgen_biome/` and `data/biome_tags/`. It reads the original biome JSON spawners and potential costs, filters to supported entities, and resolves biome tags for sheep colors, tropical-fish placement and reduced water-ambient spawning. Only used biome tags and their recursive dependencies are retained; the full biome JSON spawn catalogue is preserved. `go generate ./internal/server` regenerates `internal/server/natural_spawn_data_generated.go` using these same local inputs. Spawn lists are not hand-transcribed.

## Derived files

`items.json` comes from the pinned minecraft-data catalogue with only its non-vanilla `enchantCategories` metadata removed. Canonical enchantment item applicability comes from `item_tags/enchantable/` instead.

`enchantment_order.json` is a committed input derived from the `register` calls in `Enchantments.bootstrap`. It records registry raw-ID order, which is not stored in the individual enchantment JSON files. Java field declaration order is not the registry order. Normal generation validates and consumes this local manifest.

`equipment_source.json` contains only the armor materials, humanoid armor registrations, and direct Equippable registrations derived from the pinned Java sources. `cmd/sync-data` combines this local input with `item_tags/enchantable/equippable` to regenerate `item_armor_attributes.json` and `item_equippables.json`. The former contains defensive attributes; the latter independently records player equipment slots and server-relevant equip behavior. Item generation remains self-contained within `data/`.

`mob_effects.json` and `potions.json` contain their ordered registry definitions and their generators require no files outside `data/`.
