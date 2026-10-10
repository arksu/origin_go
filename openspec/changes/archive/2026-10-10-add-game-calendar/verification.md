# Implementation verification

Date: 2026-10-10. Change: `add-game-calendar` (`spec-driven`).

## Implemented behavior

- Shared pure server calendar conversion, exact boundary arithmetic through
  `MaxInt64`, epoch at runtime zero, and calendar output in administrator `/time`.
- One immutable eight-field `S2C_ServerConstants` after successful authentication,
  before world bootstrap and runtime Pongs; old enter-world fields/names reserved.
- Critical handshake delivery, repeated-auth guard, and existing disconnect cleanup
  coordinated with in-flight authentication, including close before DB commit.
- Coherent response-time projection of runtime seconds into existing Pongs,
  without changing persisted time, gameplay timers, or tick catch-up behavior.
- Received-only client constants, exact calendar arithmetic, monotonic estimation,
  and connection-scoped lifecycle with safe pre-handshake rendering and no stale
  world reactivation after reconnect.
- Related test and browser-preview fixtures migrated to explicit test profiles.
  Server/client deployment and rollback must be coordinated.

## Passing checks

- `make proto` and `npm run proto` in `web_new`: Go and client bindings regenerated.
- `GOCACHE=/private/tmp/origin-go-game-calendar-cache make test`: full server suite,
  including protobuf/sqlc regeneration; passed again after the typed-nil regression
  correction. sqlc outputs had no final changes.
- Focused server calendar, administrator time, protobuf, runtime projection,
  bootstrap, authentication lifecycle, cleanup/retry, and world persistence tests.
- Focused runtime/bootstrap/authentication and calendar suites with `go test -race`.
- `npm run type-check` in `web_new`, including a final run after test corrections.
- Client scripts: `test:game-calendar`, `test:server-constants`, `test:movement-input`,
  `test:chunks`, `test:map-click`, `test:move-marker`, `test:direction-aim`,
  `test:combat-sector`, `test:action-animations`, `test:character-visual`,
  `test:combat-protocol`, `test:sounds`, `test:action-execution`,
  `test:damage-numbers`, `test:knockout`, and `test:minimap`.
- `node scripts/test-character-visual.mjs tests/movement-protocol.test.mjs`:
  both protocol tests passed through the existing esbuild runner.
- `openspec validate add-game-calendar --strict` and `git diff --check`.

Transport tests use actual local WebSockets and decoded protobuf messages to check
ordering, single constants delivery across entries/transfers/Pings, and failure
closure. Authentication failure tests drive the actual generated repository
queries through a test-only `database/sql` driver, verifying online cleanup and
subsequent authentication. Client tests exercise decoded packets, real handlers,
connection replacement, late callbacks, missing constants, false capability,
alternative constants, calendar boundaries, and the pre-auth render update path.

## Known check limitations

- Full `npm run lint` was run with its configured `--fix` and failed. After fixing
  one introduced unused test argument, a non-mutating full lint audit reports 483
  errors: 434 in generated `packets.d.ts`, 30 in the unchanged Basis decoder, and
  19 in existing handwritten code. Every diagnostic in a changed handwritten file
  was compared against linting its `HEAD` version; no new handwritten diagnostic
  remains. Generated and unrelated code was not changed to silence this check.
- Direct `node --test tests/movement-protocol.test.mjs` cannot resolve the generated
  extensionless `protobufjs/minimal` ESM import on the current Node runtime. The
  same tests pass through the project's existing bundling runner, as recorded above.
- Live browser/WebGL initialization and live PostgreSQL authentication were not
  exercised. Automated render-readiness, repository-query, and WebSocket tests
  cover the affected paths; this does not claim a manual browser or DB smoke test.

## Review and repository state

Independent `code-reviewer` review approved the implementation with no findings.
The reviewer reran focused network/game race suites, the nil-database regression,
and client calendar/constants tests. All 19 implementation tasks are complete.
Implementation and planning artifacts are uncommitted; unrelated initial
working-tree files were preserved. Main specifications were synchronized and the
change was archived at `openspec/changes/archive/2026-10-10-add-game-calendar/`.

## Archive verification

- New main specifications `game-calendar` and `server-constants` retain their
  complete delta Purpose, requirements, and scenarios. The changed
  `directional-movement` requirement matches its delta; all content outside that
  block remains unchanged. An independent read-only review confirmed the merge.
- All 19 tasks and all planning artifacts were complete before archiving. The
  archive preserves every change file, including `.openspec.yaml`.
- All three synchronized main specifications passed strict validation. The
  complete `openspec validate --specs` check passed: 25 specifications, no failures.
  Its first run exposed two existing `game-actions` requirements without
  scenarios; minimal scenarios for their already stated behavior were added.
- Documentation-only archive checks were run; runtime suites were not repeated.
  No commit was created by the archive operation, and staging was left intact.
