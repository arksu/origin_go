# Tasks

## 1. Protocol and server routing

- [x] 1.1 Define `MapClick` in `api/proto/packets.proto`, replace legacy movement actions with `map_click = 5`, and reserve retired names/numbers; regenerate Go and browser bindings using `make proto` and `npm --prefix web_new run proto`, verifying schema generation succeeds and a serialization test preserves coordinates, target ID and modifiers.
- [x] 1.2 Replace legacy queue commands and action dispatch in `internal/network/command_queue.go`, `internal/game/game.go`, and `internal/ecs/systems/network_command.go`; verify routing tests accept MapClick and ignore retired wire actions without executing pending commands.

## 2. Server map-click interpretation

- [x] 2.1 Dispatch pending `/spawn`, `/tp`, `/info`, and `/destroy` before normal map-click processing, with a consumed result and current command availability; verify coordinate commands use click coordinates, object commands use target ID, and consumed clicks cause no movement or pickup.
- [x] 2.2 Preserve fixed-coordinate movement and route ordinary dropped-item clicks through existing pickup behavior; verify non-dropped and stale target IDs still move to x/y, dropped items are picked up once, and movement restrictions remain enforced.
- [x] 2.3 Remove inspection/destruction interception from `Interact`, and remove `handleMoveToEntity` while preserving internal target following; verify right-click interaction does not consume pending inspection and existing pickup/link tests pass.
- [x] 2.4 Verify pending action replacement, empty and missing target errors, player destruction rejection, isolation, death and world-departure cleanup; extend focused tests for any uncovered lifecycle path and confirm `/destroy` still invokes its existing deletion operation.

## 3. Client and maintained senders

- [x] 3.1 Add ordinary `sendMapClick` with hit-test target, rounding and modifiers; migrate normal primary clicks and dropped-item primary clicks in `Render.ts`, preserving placement/hand-drop priority and context interaction; verify focused input tests emit one correct action per click for ground, objects, dropped items and active tools.
- [x] 3.2 Remove client `/info` and `/destroy` parsing, `adminObjectInfoSelectionArmed`, facade arm/cancel APIs, Escape selection handling, and unused movement senders; verify input remains identical before and after arbitrary chat commands and targeted source search finds no remaining administrator selection wiring.
- [x] 3.3 Migrate `cmd/load_test/virtual_client.go` payload construction and mutation plus `cmd/load_test/README.md`; verify the load-test binary builds and its generated action carries MapClick coordinates with target zero.
- [x] 3.4 Update stale production comments referring to the removed wire command without removing internal movement components; verify repository searches contain no active legacy movement action types or dispatch routes outside reserved schema names and historical documentation.

## 4. Integration verification

- [x] 4.1 Run `go test ./...`, `npm --prefix web_new run type-check`, the focused frontend input tests, and `npm --prefix web_new run build`; record results and any environment limitations.
- [x] 4.2 Exercise browser/server flows for ordinary fixed-point movement, pickup, right-click interaction, `/spawn`, `/tp`, `/info`, and `/destroy`, including empty/stale object selections; verify administrator clicks are consumed without a second action and document the coordinated client/server deployment requirement.
