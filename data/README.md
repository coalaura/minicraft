# Generated data inputs

The JSON files in this directory are reproducible from the pinned Minecraft Java Edition 1.21.11 references in `../reference`:

```sh
go run ./cmd/sync-data -reference ../reference
```

The command validates Minecraft version 1.21.11 and protocol 774 before replacing these inputs. Run `go generate ./internal/game` afterward to update the Go sources that consume them.

## Exact copies

These paths are copied byte-for-byte from `reference/client_source/data/minecraft`:

| Local path | Client data path |
| --- | --- |
| `block_loot/` | `loot_table/blocks/` |
| `block_tags/` | `tags/block/` |
| `enchantments/` | `enchantment/` |
| `enchantment_tags/` | `tags/enchantment/` |
| `item_tags/` | `tags/item/` |
| `recipes/` | `recipe/` |

`biomes.json`, `blocks.json`, and `entities.json` are copied byte-for-byte from `reference/minecraft-data`. That catalogue supplies protocol registry IDs and properties which the client data pack does not expose as equivalent flat JSON files.

## Derived files

`items.json` is copied from `reference/minecraft-data/items.json` with only its non-vanilla `enchantCategories` metadata removed. Canonical enchantment item applicability comes from `item_tags/enchantable/` instead.

`enchantment_order.json` is extracted from the `register` calls in `Enchantments.bootstrap`. It records registry raw-ID order, which is not stored in the individual enchantment JSON files. Java field declaration order is not the registry order.

`item_armor_attributes.json` contains the player armor slot, defense, toughness, and knockback-resistance values for Minecraft Java 1.21.11. Item generation is self-contained within `data/`.

`mob_effects.json` and `potions.json` contain their ordered registry definitions and their generators require no files outside `data/`.
