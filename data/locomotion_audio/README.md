# Locomotion audio

Author `{ "v": 1, "bindings": [...] }`. Each binding contains `actor`, `clip` and
nonempty ordered `contacts`: `{id, phase, sound_key}`. Actor/clip selectors and
contact IDs are unique. Phases increase strictly within `(0,1)`, avoiding duplicate
loop-boundary representations. Referenced profiles must use `local` delivery.

Locomotion is existing client movement presentation, not a server cyclic action.
Publication requires a compatible looping distance-driven actor clip and derives
`cycle_distance_tiles` from its manifest's `cycleDistanceTiles`. Do not author
stride here. The client uses unwrapped interpolated distance and rebases on
stops, snaps, late visibility/loading, hearing-range entry, locomotion changes,
long pauses and reset without replaying missed contacts.

The authored `footstep` contact is the default sound. The client resolves its
tile override at playback using `web_new/src/game/footstepConfig.ts`; tiles can
share a profile, and unmapped or unavailable tiles retain the default. Surface
changes preserve contact timing for both walk and carry-walk.

The commoner `walk` and `carry_walk` contacts were reviewed against their current
published clips by decoding Meshopt animation channels and evaluating hierarchical
foot transforms at their 49 source samples. Each cycle is 48 intervals / 0.96 s;
the manifest stride is `1.677975879375` tiles. Both clips retain identical lower
body timing. A contact marks the forwardmost landing/stance transition where the
foot reverses its travel and lowers into support, rather than an arbitrary half-cycle.

| Clip | Contact | Source frame | Phase | Ankle world Y / Z at marker |
| --- | --- | --- | --- | --- |
| `walk` | right | 23 | `22/48 = 0.4583333333333333` | `0.1337 / 0.3493` m |
| `walk` | left | 47 | `46/48 = 0.9583333333333334` | `0.1337 / 0.3493` m |
| `carry_walk` | right | 23 | `22/48 = 0.4583333333333333` | `0.1337 / 0.3493` m |
| `carry_walk` | left | 47 | `46/48 = 0.9583333333333334` | `0.1337 / 0.3493` m |

Reviewed clip SHA-256 values: walk
`d575bcb52d0f524696b5550b3b8195f58eb073bf8223b19f48cd4453eae8b2d4`,
carry-walk `703ec01512b7ff548d7c8d2dbe2c4f25cc929be62cdfc63f81892a7fb3f64629`.
At the next sample each landing foot moves back from Z `0.3493` to `0.3436` m
and lowers from Y `0.1337` to `0.1237` m. Review these authored contacts again
after changing source gait timing; stride publication is automatic.

Publish with `tools/assets publish-action-animations`. Output is an immutable
projection referenced by `asset-catalog.json.locomotionAudio`, switched together
with sound and action definitions.
