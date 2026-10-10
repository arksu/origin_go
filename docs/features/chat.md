# Local Chat

Local chat is processed in the owning shard's tick and delivered to connected
characters within an inclusive distance of `game.chat_local_radius` (default
1000 world units). It does not depend on visibility or hearing distance.

## Runtime flow

1. `Game.handlePacket` dispatches `C2S_ChatMessage` to `handleChatMessage` in
   `internal/game/game.go`. Dead observer sessions may send only pings.
2. Ingress validates the character ID, LOCAL channel, nonempty trimmed text,
   and `game.chat_max_len` (default 256 **bytes**). It removes carriage returns
   and trims the result again; embedded newlines are preserved. The length
   check happens before carriage-return removal.
3. Ingress enqueues a `CmdChat` in the layer's `PlayerCommandInbox`. It does
   not read or mutate ECS state. The inbox applies the shared per-client
   command rate limit, duplicate detection, bounded queue, and tick fairness.
4. `NetworkCommandSystem` (priority 0) drains player commands before server
   jobs under `Shard.Update`'s world lock. Execution resolves the sender's
   EntityID and requires a live Handle, Transform, and Appearance; recognized
   slash commands are handled before local delivery. Current chat execution
   does not add a connection-ID or stream-epoch ownership check.
5. `Shard.AppendLocalChatRecipients` queries the existing connected-listener
   grid owned by `SoundEventService`, then checks the current Transform with
   `distanceSquared <= radiusSquared`. Each candidate must have a live Handle,
   matching external EntityID, and the same Handle in `CharacterEntities`.
6. `Shard.BroadcastChatMessage` synchronously consumes the selected EntityIDs,
   marshals one `ServerMessage` containing `S2C_ChatMessage`, and passes the same
   immutable serialized bytes to each currently bound client's gameplay queue.

There is no chat event-bus subscriber or asynchronous ECS chat handler.

## Recipient membership and ordering

Each shard queries only its own World and listener grid. Attachment and
reattachment add listener membership; disconnect and transfer detachment remove
it. Collision position commits and immediate relocation update that grid before
subsequent queries. This includes movement that produces no visibility update.

The sender receives the same local echo as any eligible character at distance
zero. Knocked-out characters remain eligible recipients. Detached living bodies
are excluded by listener membership. Permanent-death observers may retain audio
membership, but `CharacterEntities` removal excludes them from local chat.

Selection uses the position committed when the command executes. Since chat
runs at priority 0, normal movement later in that tick affects the next query.
Recipient order is unspecified; every eligible character is selected once.

The grid is shared with audio, while chat selection does not consume audio
events, pending entries, per-tick budgets, or audio statistics. Audio hearing
scaling, strict hearing boundaries, and truncation limits do not apply to chat.

## Buffer ownership and delivery

Each `NetworkCommandSystem` reuses a recipient `[]types.EntityID` buffer. The
selector appends to the caller's slice; the chat handler resets its length for
each message. Delivery borrows that slice only until `BroadcastChatMessage`
returns. It must not retain it or pass it to an asynchronous event handler.

Only immutable marshaled bytes enter client send queues. Chat uses the existing
bounded gameplay queue and its overflow behavior; it has no separate sound
queue, stream epoch, retry queue, or recipient cap. A dense local population
still requires one send attempt per recipient.

## Limits and configuration

- `game.chat_local_radius`: default 1000; the command system uses its absolute
  value and caches the square. There is no new chat-specific config bound.
- `game.chat_max_len`: default 256 bytes, enforced at ingress.
- `game.chat_min_interval_ms`: default 400 is present in configuration but is
  **not enforced** by current chat handling.
- `game.max_packets_per_second`: shared inbox command limit, default 40 per
  client in a one-second window. This is not a separate chat throttle.
- The player inbox also has a bounded queue (default 2000 commands) and per-client
  tick fairness (default 20 commands). Queue overflow returns an input warning;
  chat rate-limit rejection returns an invalid-request error.

Regression coverage is in `internal/ecs/systems/network_command_chat_test.go`,
`internal/game/chat_recipient_selector_test.go`, and
`internal/game/chat_recipient_lifecycle_test.go`. The lifecycle fixture exercises
real movement, relocation, attach/detach, and death-system membership hooks;
transfer destination/rollback choice is driven explicitly without the
DB-backed asynchronous transfer orchestrator.

## Performance validation

Reproduce selector-only checks with:

```sh
make test
go test -race ./internal/game ./internal/ecs/systems -run 'LocalChat|NetworkCommandChat|WorldSound|SoundListener|SoundDetach' -count=1
go test ./internal/game -run '^$' -bench '^BenchmarkLocalChatRecipientSelection$' -benchmem -benchtime=200ms -count=3
```

Measured on 2026-10-10 with Go 1.27.1, darwin/arm64, Apple M3 Max and
GOMAXPROCS 16. Values below are medians of three warmed runs. The full-scan
baseline follows the previous character traversal and allocates its original
32-ID buffer; neither workload includes protobuf encoding or network delivery.

| Population | Listener grid | Full scan | Grid candidates |
| --- | ---: | ---: | ---: |
| 1 nearby | 0.747 us | 0.196 us | 1 |
| 8 nearby | 0.887 us | 0.567 us | 8 |
| 8 nearby + 1,000 remote | 1.001 us | 62.188 us | 8 |
| 8 nearby + 30,000 remote | 1.012 us | 2,412.260 us | 8 |
| 30,000 remote removed, 1 nearby remains | 0.947 us | 21.592 us | 1 |
| 30,000 remote removed, 8 nearby remain | 1.104 us | 22.114 us | 8 |

Every listed grid workload examines 81 cells and allocates 0 B/op with
0 allocs/op; the baseline allocates 256 B/op with 1 alloc/op. Small shards pay
the fixed cell-probing cost even when a full scan would be faster. Adding remote
players leaves local candidates unchanged, although map size and cache locality
can still affect timings.

For unusually small configured cells, large rectangles use occupied-cell
fallback instead of enumerating empty cells. The cell-size-1 workload with 8
nearby and 1,000 remote players visits 1,008 occupied keys, checks 8 candidates,
and measures 7.335 us with no allocations. Fallback iteration can depend on the
map's historical allocation; ordinary small queries always use direct probes.
These measurements validate selection locality, not production server capacity.
