# minicraft

Minicraft is a small Minecraft Java Edition server built in Go. It speaks the protocol directly, without a vanilla server behind it and provides a focused place to experiment with networking, procedural generation and the details that make a Minecraft world feel alive.

It currently targets **Minecraft Java Edition 1.21.11** (protocol 774). The project is still intentionally small and incomplete, but it is playable with an unmodified client.

![The backrooms generator](.github/backrooms.png)

## What works

- Online and offline authentication
- Secure player chat and multiplayer presence
- Player properties like skins
- Chunk streaming with configurable render distance
- Player movement, block breaking, block placement and block modification
- Lighting, world time and a day/night cycle
- Seeded procedural world generators
- Basic hotbar inventory support
- TOML configuration

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

Then connect to `localhost:25565`. Edit `config.toml` to change the address, game mode, render distance, chat behavior, world settings and other server options.

To create a standalone binary instead:

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
menger-sponge
natural
quasicrystal
spawn-platform
wave-terrain
```

Each generator is deterministic for a given seed. `spawn-platform` is the simplest place to start; the others explore more unusual worlds.

## Development

Run the test suite with:

```sh
go test ./...
```

Minicraft is a learning project rather than a complete replacement for a production Minecraft server. Expect missing gameplay systems and ongoing protocol work.

## License

Minicraft is available under the [Mozilla Public License 2.0](LICENSE).
