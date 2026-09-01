# minicraft

Minicraft is a small, fast Minecraft Java Edition server built in Go. It speaks the Minecraft protocol directly, without a vanilla server or JVM behind it and provides a focused foundation for experimenting with networking and procedural world generation.

Despite its small size, it implements enough of the protocol and game behavior to explore generated worlds with an unmodified client, including multiplayer, secure chat, chunk streaming, lighting, block interaction and full player inventory handling.

It currently targets **Minecraft Java Edition 1.21.11** (protocol 774).

![The backrooms generator](.github/backrooms.png)

## What works

- Online and offline authentication
- Secure player chat and multiplayer presence
- Player properties, including skins
- Chunk streaming with configurable render distance
- Player movement and synchronization
- Authoritative creative and timed survival block breaking with nearby crack animations
- Generated 1.21.11 block hardness, tool speed, durability, harvest gating and compiled canonical ordinary block loot
- Canonical 1.21.11 enchantment registry, tags and item applicability, with Efficiency, Unbreaking, Silk Touch and Fortune gameplay behavior
- Survival placement with exact main-hand/offhand item consumption and synchronized equipment
- Authoritative player health, repeated-hit timing, air and drowning, fire and lava, movement-time fall resets, block-specific landing damage and void damage
- Configurable Peaceful, Easy, Normal and Hard difficulty with hunger, exhaustion, natural regeneration and starvation
- Server-authoritative timed consumables from either hand, including canonical food, effect, milk and use-remainder behavior
- Generated 1.21.11 mob-effect registry with synchronized active effects, absorption, Regeneration, Poison, Hunger, Fire Resistance, Resistance and Nausea
- Idempotent death with inventory loss, `/kill`, and client-requested respawn at the world spawn
- Stateful block placement for common structures such as slabs, stairs, doors, trapdoors, fences and walls
- Full player inventory and creative inventory interactions
- Manual 2x2 and crafting-table recipes, including shaped, shapeless and ordinary crafting remainders
- Furnaces, smokers and blast furnaces with generated cooking recipes and fuel values, ticking, menus and synchronized progress
- Stateful barrels and single/double normal, trapped and copper chest variants, including container contents drops on removal
- Vanilla-like hoppers with sided container automation, item suction, persisted cooldowns and synchronized five-slot menus
- Generic runtime entities, dropped items and item pickup
- Basic commands, including `/enchant`, with client-side declarations and suggestions
- Biomes, lighting, world time and a day/night cycle
- Vanilla-like derived water and lava flow in active world chunks, with scheduled ticks intentionally paused in inactive chunks, waterlogging, buckets and item fluid motion
- Deterministic, seeded procedural world generation
- In-memory world modifications without world save files
- Transient chunk and player entity lifecycle
- TOML configuration

Minicraft is intentionally lightweight. It runs as a single Go binary and avoids the JVM, vanilla server internals and persistent world storage of a regular Minecraft server, making it especially quick to start and inexpensive to run.

![Multiplayer in a generated world](.github/multiplayer.png)

<p align="center">
  <img src=".github/blocks.png" alt="Block placement in a generated world" width="49%">
  <img src=".github/sponge.png" alt="The Menger sponge generator" width="49%">
</p>

![Player chat](.github/chat.png)

## Getting started

You will need Go 1.27 and a Minecraft Java Edition 1.21.11 client.

```sh
cp example.config.toml config.toml
go run .
```

Then connect to `localhost:25565`.

Edit `config.toml` to change the address, game mode, render distance, chat behavior, world settings and other server options.

To build a standalone binary instead:

```sh
go build -o minicraft .
./minicraft
```

## World generators

Set `world.generator` in `config.toml` to one of:

```text
babel
backrooms
fractal-vaults
maze
menger-sponge
natural
quasicrystal
superflat
test-world
wave-terrain
```

Generators are deterministic for a given seed and produce their worlds procedurally as chunks are requested. `superflat` provides a simple grass-topped building world with sparse vegetation, while `test-world` uses a stone-brick surface with contrasting chunk borders, centers and axes. The others demonstrate more complex and unusual generation techniques.

No persistent world storage is required: generated terrain can always be reconstructed from its generator and seed, while player modifications live only in memory.

## Development

Run the test suite with:

```sh
go test ./...
```

## Scope and limitations

Minicraft is a focused protocol and procedural-world server, not a drop-in replacement for vanilla Minecraft.

- World and player changes are held in memory; persistent storage is not implemented.
- Fluid ticks pause in inactive chunks, and broader fluid parity remains out of scope.
- Recipe-book support, special recipes, furnace XP and component-transforming recipes are deferred.
- Timed item use continues across same-item count, component and backing-stack changes, but cancels when the active hand becomes empty or changes item.
- Suspicious-stew effects, chorus-fruit teleportation, potion swirls and `USE_EFFECTS` movement or vibration behavior remain explicitly deferred.
- Potions, armor, combat, mobs, projectiles, XP, bed placement/sleeping, respawn anchors and gamerule configuration are not implemented.
- Honey-block side sliding is deferred; honey landing damage is implemented.

## Todo

- Broader block placement and interaction support
- Other inventory blocks
- Recipe-book and special crafting recipe parity
- Broader survival mechanics such as potions, armor and combat
- Adventure map features

## License

Minicraft is available under the [Mozilla Public License 2.0](LICENSE).
