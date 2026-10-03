# World audio protocol and admission

World audio uses `ServerMessage.sound_batch` (field 51). Its `sounds` entries
retain the existing `S2C_Sound` key, coordinates and radius fields and add
`optional float distance_gain` at field 5. Presence is significant: an absent
gain permits legacy single-message attenuation; an explicit zero means silence.
The batch supplies `stream_epoch` and `server_time_ms` so clients can reject old
world streams and expired events without resolving a source object.

`S2C_PlayerEnterWorld.audio` (field 11) supplies `hearing` and `freshness_ms`.
Refresh these parameters at every world entry. New batch output requires the
matching client; old clients do not support the batch. The new client can still
interpret legacy single world sounds without authoritative gain.

`Client.SendAudio` admits an owned encoded batch nonblockingly to a separate
queue controlled by `game.audio.queue_capacity` (default 1). It returns:

- `AudioSendAccepted`: ownership transferred to the queue.
- `AudioSendFull`: the audio queue was full; count and discard this new batch.
- `AudioSendClosed`: the client is detached, closed, critically failed or the
  server is stopping; discard the batch.

Acceptance means queued, not delivered. Audio has no retry backlog and cannot
occupy gameplay queue slots. The existing writer drains waiting gameplay before
writing at most one audio batch, including when an audio wakeup wins its select.
It rechecks gameplay after taking an audio batch, stops on shutdown, and omits
audio if the client has detached before the write. A stalled shared socket can
still delay both traffic classes, so batch-byte limits and client freshness
checks remain necessary. Critical gameplay overflow retains its existing
connection-close behavior for actual gameplay congestion.

Regenerate bindings with `make proto` and `npm --prefix web_new run proto`.
Run protocol and queue checks with
`go test ./internal/network ./internal/network/proto`; the transport tests use
real framed socket writes to verify ordering and gameplay capacity isolation.
