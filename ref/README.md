# Minecraft reference data

This directory contains version-pinned Minecraft Java Edition protocol and registry data used as a local implementation reference.

The data is sourced from [`PrismarineJS/minecraft-data`](https://github.com/PrismarineJS/minecraft-data).

## 1.21.11

Minecraft Java Edition 1.21.11 uses protocol version **774**.

Available reference data:

* `proto.yml` — packet IDs, packet structures, protocol states and shared wire types
* `blocks.json` — block IDs, states and properties
* `items.json` — item IDs and properties
* `entities.json` — entity IDs and properties
* `biomes.json` — biome IDs and properties
* `version.json` — Minecraft and protocol version information

When implementing or verifying Minecraft protocol behavior, prefer these local, version-specific references before searching externally.

`proto.yml` should be treated as the primary reference for packet encoding and decoding. The JSON files should be used for registry and game data.

These files are reference data only and should not be modified manually.