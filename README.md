[![Go](https://github.com/arksu/origin_go/actions/workflows/go.yml/badge.svg)](https://github.com/arksu/origin_go/actions/workflows/go.yml)

# Origin Go

Backend and tooling for a 2D MMO/survival prototype:

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

## Documentation

- Product/game spec: `docs/spec.md`
- Feature docs: `docs/features/`
- ADRs: `docs/adr/`
- PRDs/checklists: `docs/prd/`

## License

MIT (`LICENSE`)
