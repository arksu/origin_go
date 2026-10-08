# Action animation definitions

Canonical files are strict JSON: `{ "v": 1, "bindings": [...] }`. The server loads
all `.json` files at startup as one registry. An empty bindings list disables
bindings; unknown fields, null values, conflicting keys or selectors fail startup.

Each binding declares:

| Field | Meaning |
| --- | --- |
| `key` | Unique opaque portable name, 1–128 characters |
| `source` | Exact `kind` (`context`, `menu`, `craft`, `build`), optional `namespace`, and `id` |
| `actor` | `character/<asset id>` |
| `variants` | Nonempty ordered list of `{clip, equipment}`; first ready match wins |
| `equipment` | Array of `{slot, visual_key}` predicates; all must match, empty means unrestricted |
| `eligibility` | Array of `stationary`, `not_carrying`, `not_knocked_out`; all must hold |
| `facing` | `preserve` or `target` (requires a cycle target position or fixed direction) |
| `blend_ms` | Finite nonnegative blending time; default zero |
| `frame` | Integer width/height in 1–1024, pixel `origin_x`/`origin_y` inside bounds (origins default zero) |
| `unbind_equipment_slots` | Optional array of distinct equipment slots to visually detach while this action pose is displayed; absent/empty leaves attachments unchanged |
| `preview` | Optional `{label, duration_ms, equipment}` for local preview |
| `sound_cues` | Optional ordered array of `{id, phase, sound_key, source}`; unique IDs, increasing phases in `(0,1]`, `source` = `actor` or `target` |

Source fields are whitespace-trimmed before exact lookup. A context source uses
the behavior key as its namespace; synthetic context actions use an empty
namespace. Menu, craft and build sources use the existing definition key with an
empty namespace. No wildcard matching or implicit ordering is supported.

Equipment slots use the character visual contract. Equipment keys are visual
asset keys, not inventory item IDs. Variant ordering declares preference. The
frame origin is the ground anchor in native output pixels; a larger output does
not scale the character. Clips must exist in the actor manifest and use its rig.

Defs contain presentation choices and sound markers. They do not set gameplay
duration, stamina or successful effects. The full loaded clip is sampled
at `phase * clip.duration`; the action duration is `total_ticks * tick_duration_ms`.
The network controller clamps phase at one until the next confirmed cycle.

Directed menu actions publish the accepted world angle through the existing
`facing_angle` field, without a synthetic target position. `axe_sweep` and
`axe_strike` both use the `chop_r`/`chop_l` clips in `axe.json`; their full clips
are sampled over the actual six-tick action, facing its fixed attack direction.

Publish with `tools/assets publish-action-animations` using existing manifests.
Deploy server defs with the matching generated client catalog. Adding a binding
for a supported source requires def changes and publication only. Tests share
the positive and negative fixtures under `tests/fixtures/action_animations/`.

## Sound markers

`tree_chop` declares one `chop` cue at phase `0.4`, from the target tree. Its hand
variants share that marker. The server resolves the profile from `data/sounds/`
and emits a world cue once per validated cycle at the first tick at or beyond
the phase (tick 8 of 20). Repeated cycles reset their marker cursor; cancellation
before the marker prevents the sound. Gameplay effects remain at completion.
Successful `tree_fall` feedback remains a completion event with a target point
captured before a terminal effect can remove the tree.

World cues are executed by the server and never duplicated by client animation
sampling. Local cues are presentation-only and skip markers already passed on
late entry/loading; backward corrections cannot re-arm a consumed marker in the
same cycle. An explicitly enabled preview can inspect cues locally. Actor sources
are available for all execution kinds; craft bindings cannot select a target
source because crafting has no guaranteed target. Menu targets must also be
available in their concrete execution definition.

`tools/assets publish-action-animations` publishes the sound, locomotion and
action catalogs together, checking profile references and existing samples.
See [sound profiles](../sounds/README.md) and [locomotion audio](../locomotion_audio/README.md).

## Temporary visual unbind

Set `unbind_equipment_slots` to the slots whose models should leave the actor's
scene during this animation (for example, `left_hand` and `right_hand`). Any
supported equipment slot can be listed. Unknown slots, duplicates, non-array
values and null are invalid. Variant predicates still use actual equipped items;
unbinding never changes inventory, equipment revisions, arm poses or gameplay.

The client retains loaded objects, attachment transforms and resource leases.
Detachment starts before the first action pose and remains active during blend
out, terminal-sample holds and repeated cycles. Overlapping action layers combine
their slot lists. Once a slot is no longer claimed, its current model returns to
its authored attachment; replaced equipment is never restored from an old
snapshot. Late loads obey the current rule. Carry/knockout eligibility and
ordinary equipment visibility continue to apply.

This field is included in the published client projection. Update both strict
def readers (server and client/tooling) before publishing defs that use it. The
server only validates the field; no new network messages or game-state changes
are involved. Existing defs can omit it.
