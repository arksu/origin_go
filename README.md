[![Go](https://github.com/arksu/origin_go/actions/workflows/go.yml/badge.svg)](https://github.com/arksu/origin_go/actions/workflows/go.yml)

# Origin Go

Backend and tooling for a 2D MMO/survival prototype.

This repository contains the server, data definitions, tools, and web clients used to build and experiment with the game. Contributions are welcome, including bug fixes, gameplay improvements, tooling, docs, and tests.

## Welcome

If you want to explore or contribute, a good place to start is:

- `docs/` for design notes, ADRs, and feature specs
- `cmd/gameserver` for server entrypoint
- `internal/` for game/runtime/server systems
- `data/` for item/object/crafting definitions
- `web_new/` for the main web client

Project components:

- Go game server (`cmd/gameserver`)
- map generator (`cmd/mapgen`)
- load-test runner (`cmd/load_test`)
- modern web client (`web_new`)
- terrain editor (`web_editor_terrain`)

## Tech Stack

- **Backend:** Go 1.25.5, PostgreSQL, Redis, Protobuf, sqlc
- **Architecture:** ECS + shard-based world simulation + event bus
- **Frontend:** Vue 3 + TypeScript + PixiJS (in `web_new` and `web_editor_terrain`)

## Repository Layout

```text
.
├── cmd/                    # binaries: gameserver, mapgen, load_test
├── internal/               # game/runtime/server packages
├── data/                   # item/object definitions (JSON/JSONC)
├── migrations/             # DB schema
├── api/proto/              # protobuf contracts
├── docs/                   # spec/ADR/PRD/features
├── web_new/                # main web client
└── web_editor_terrain/     # terrain editor tool
```

## Prerequisites

- Go **1.25.5**
- `protoc` (CI uses `23.x`)
- `protoc-gen-go`
- `sqlc`
- PostgreSQL (or `docker compose` service)
- Redis (or `docker compose` service)

Install codegen tools:

```bash
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest
```

## Quick Start (Backend)

This is the fastest way to get the backend running locally.

1. Start infrastructure:

```bash
docker compose up -d
```

2. Apply schema (first run):

```bash
psql postgresql://db:db@localhost:5430/db -f migrations/schema.sql
```

3. Build and run:

```bash
make build
make run
```

4. If you use docker-compose defaults, set DB env variables:

```bash
export DATABASE_USER=db
export DATABASE_PASSWORD=db
export DATABASE_DATABASE=db
```

## Quick Start (Web Client)

Run the main web client in a separate terminal:

```bash
cd web_new
npm install
npm run dev
```

## Build Workflows

### Local build workflow (Makefile)

Core targets:

```bash
make proto            # generate Go protobuf code
make sqlc             # generate repository code from SQL
make build            # build gameserver (depends on proto + sqlc)
make run              # run gameserver (depends on proto + sqlc)
make test             # run all Go tests (depends on proto + sqlc)
make map-gen          # run map generator
make map-gen-build    # build map generator
make load-test        # run load test
make load-test-build  # build load test binary
```

### CI build workflow (GitHub Actions)

Workflow: `.github/workflows/go.yml`

On push/PR to `main`, CI will:

1. setup Go 1.25.5
2. install `protoc` (23.x)
3. install `protoc-gen-go` and `sqlc`
4. run `sqlc generate`
5. verify generated files are committed
6. generate protobuf code
7. build `gameserver`

## Useful Commands

### Map generation

```bash
go run ./cmd/mapgen -chunks-x 50 -chunks-y 50 -seed 123
```

Common mapgen flags:

- `-river-enabled` (default `true`)
- `-river-layout-draw` (default `true`)
- `-river-major-count` (default `30`)
- `-river-lake-count` (default `220`)
- `-river-lake-border-mix` (default `0.35`)
- `-river-max-lake-degree` (default `2`)
- `-river-shape-long-meander-scale` (default `0.55`)
- `-river-shape-short-meander-scale` (default `2.2`)
- `-river-shape-short-meander-bias` (default `0.0035`)
- `-river-shape-amplitude-scale` (default `1.0`)
- `-river-shape-frequency-scale` (default `1.0`)
- `-river-shape-noise-scale` (default `0.30`)
- `-river-shape-along-scale` (default `0.16`)
- `-river-shape-distance-cap` (default `0.40`)
- `-river-shape-segment-length` (default `70`)
- `-river-source-elevation-min` (default `0.55`)
- `-river-source-chance` (default `0.00015`)
- `-river-meander-strength` (default `0.003`)
- `-river-voronoi-cell-size` (default `96`)
- `-river-voronoi-edge-threshold` (default `0.14`)
- `-river-voronoi-source-boost` (default `0.02`)
- `-river-voronoi-bias` (default `0.01`)
- `-river-sink-lake-chance` (default `0.03`)
- `-river-lake-min-size` (default `48`)
- `-river-lake-connect-chance` (default `0.75`)
- `-river-lake-connection-limit` (default `120`)
- `-river-lake-link-min-distance` (default `120`)
- `-river-lake-link-max-distance` (default `1800`)
- `-river-width-min` (default `5`)
- `-river-width-max` (default `15`)
- `-river-grid-enabled` (default `true`)
- `-river-grid-spacing` (default `760`)
- `-river-grid-jitter` (default `64`)
- `-river-trunk-count` (default `8`)
- `-river-trunk-source-elevation-min` (default `0.62`)
- `-river-trunk-min-length` (default `180`)
- `-river-coast-sample-chance` (default `0.012`)
- `-river-flow-shallow-threshold` (default `6`)
- `-river-flow-deep-threshold` (default `20`)
- `-river-bank-radius` (default `1`)
- `-river-lake-flow-threshold` (default `28`)
- `-png-export` (default `false`)
- `-png-dir` (default `map_png`)
- `-png-scale` (default `1`)
- `-png-highlight-rivers` (default `true`)

Example with PNG export:

```bash
go run ./cmd/mapgen \
  -chunks-x 50 \
  -chunks-y 50 \
  -seed 123 \
  -png-export \
  -png-dir map_png \
  -png-scale 1
```

### Load testing

```bash
make load-test-build
./load_test --clients=20 --duration=60s
```

Detailed scenarios and flags: `cmd/load_test/README.md`.

## Web Projects

### `web_new` (main client)

```bash
cd web_new
npm install
npm run dev
```

Other scripts:

- `npm run build`
- `npm run type-check`
- `npm run lint`
- `npm run proto`

### `web_editor_terrain` (terrain editor)

```bash
cd web_editor_terrain
npm install
npm run dev
```

## Configuration

- Server loads config from `config.yaml` (if present) and environment variables.
- Defaults are defined in `internal/config/config.go`.
- Environment keys follow dot-to-underscore convention (example: `game.tick_rate` -> `GAME_TICK_RATE`).

## Contributing

Contributions of all sizes are useful. If you are not sure where to start, docs updates, small bug fixes, tests, and cleanup PRs are all welcome.

### Before opening a PR

1. Open an issue first for large changes (new systems, protocol changes, major refactors) so the approach can be aligned early.
2. Keep PRs focused on one change when possible.
3. Include a short description of the behavior change and how you tested it.

### Recommended local checks

```bash
make test
```

For frontend changes in `web_new`:

```bash
cd web_new
npm run type-check
npm run lint
```

### Reporting bugs

Please include:

- what you expected to happen
- what actually happened
- reproduction steps
- logs/error messages (if available)
- screenshots/video for UI issues (if relevant)

## Community Guidelines

- Be respectful and constructive in issues and PR reviews.
- Assume good intent and focus feedback on the code and behavior.
- Prefer clear problem statements and reproducible reports.

## Documentation

- Product/game spec: `docs/spec.md`
- Feature docs: `docs/features/`
- ADRs: `docs/adr/`
- PRDs/checklists: `docs/prd/`

## License

MIT (`LICENSE`)
