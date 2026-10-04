# Sound profiles

Author strict JSON `{ "v": 1, "sounds": [...] }` in this directory. Profiles are
loaded together at server startup and projected into the immutable client asset
catalog. The server does not need audio media mounted; publication verifies it.

| Field | Meaning |
| --- | --- |
| `key` | Unique name, 1–128 letters, digits, underscores or hyphens |
| `mode` | `world` for server propagation; `local` for client presentation |
| `loudness` | Finite positive reference hearing distance in world units |
| `volume` | Playback multiplier in `[0,1]`, independent of audibility |
| `files` | Nonempty unique paths below `/assets/game/`, beginning `sound/`, ending `.mp3`, `.wav` or `.ogg` |
| `priority` | Integer `0..255`; larger priorities win bounded audio admission |
| `max_voices` | Integer `1..128` limit for this profile |
| `max_voices_per_source` | Positive integer no greater than `max_voices` |
| `local_attenuation` | Required for local profiles: `{near_distance?, near_gain, far_gain, shape}` with optional absolute `near_distance >= 0` below the effective radius, `0 <= far_gain <= near_gain <= 1`, finite `shape > 0` |
| `feedback_trigger` | Optional unique owner-only FX key for a local feedback profile |

Unknown fields, nulls, duplicate keys/files/triggers and invalid references fail
before activation/publication. All core fields are required, including zero-capable
`volume` and `priority`. World profiles cannot declare local attenuation/feedback.

Hearing is `game.audio.base_hearing`, initially `1.0`. Audibility radius is
`R = loudness * hearing`, and eligibility requires `distance² < R²`. A radius
above `game.audio.max_effective_radius` fails validation; it is not clamped.
For world listeners the server computes `1 - 3t² + 2t³`, `t=distance/R`. The client
applies that gain once together with sample volume and user settings.

For local audio own-source gain is `1`. Others use
`near_gain + (far_gain - near_gain) * ln(1 + shape*t) / ln(1 + shape)` inside
the radius. Footsteps use near `0.9`, far `0`, shape `4` and no `near_distance`:
another character starts at gain `0.9`, reaches roughly `0.286` at half range and
approaches zero continuously at the boundary. Own footsteps retain gain `1`. An
authored `near_distance` keeps gain flat at `near_gain` for the first absolute
world units before the same fade; hearing scales the outer radius, never the
near zone. No new one-shot
starts outside range; there is no audible gain jump at the footstep boundary. Discovery feedback is owner-only, triggered by FX `exp_gain`; ordinary
positive LP deltas do not trigger it.

Profiles: chop/world `1000`, fall/world `1400`, footsteps/local `160`,
discovery/local `80`. Existing MP3 paths/volumes are retained. The five footstep
profiles use selected recordings: four CC0 profiles and shallow water under
CC BY 3.0; see
[provenance](../../web_new/public/assets/game/sound/steps/SOURCES.md).

| Footstep profile | Recording | Tile assignment |
| --- | --- | --- |
| `footstep` | Soft leather | Default for all unassigned or unavailable tiles |
| `footstep_stone` | Stone | Mountain |
| `footstep_gravel` | Gravel | Dirt, clay, plowed |
| `footstep_forest_leaves` | Forest leaves | Coniferous forest, broadleaf forest |
| `footstep_shallow_water` | Wading slosh | Shallow water |

Tile selection is client presentation configuration in
[`footstepConfig.ts`](../../web_new/src/game/footstepConfig.ts). Several tile IDs
can reference the same sound key. Add or remove an entry there to customize a
tile; an absent entry uses `footstep`. Sound media and playback parameters stay
in the profiles here. Selection uses each character's current interpolated
position and does not reset the gait or affect action sound cues.

Publish metadata with `tools/assets publish-action-animations`, or publish it with
a full/partial asset build. The existing publication lock switches `sounds`,
`locomotionAudio` and `actionAnimations` references in one catalog replacement.
Deploy matching server definitions and client/catalog together. Shared fixtures
live in `tests/fixtures/sounds/`.
