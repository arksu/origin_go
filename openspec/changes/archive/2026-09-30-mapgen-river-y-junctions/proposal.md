# Proposal

## Why

The drawn network connects its principal routes through lakes. Existing tributaries create short inland dead ends, so they do not provide configurable lake-to-river connections. Allowing a lake route to join another river would add recognizable Y confluences and more varied boat travel.

## What Changes

- Add a probabilistic destination choice for supplementary routes: connect the source lake to an accepted river's interior instead of another lake or the existing short-stage border fallback.
- Preserve the regional backbone and explicit major border stage; use the existing remaining-major and short-link budgets for Y connections.
- Add `river.junction_chance` and `river.junction_spacing_tiles` to YAML and the river preview. Propose probability 0 by default, 0.25 initially in H&H, and spacing 180 tiles; these are reviewable initial values.
- Construct asymmetric, oblique joins with smooth approach geometry and variable river width/banks. Keep the configured deep boat footprint through all three arms, including H&H's three-tile clearance.
- Separate Y connections from existing inland tributaries. Check spacing between both kinds of junction and exclude lake entrances, existing confluences, and unrelated water contacts.
- When a selected Y attempt cannot fit, fall back to the ordinary candidate for that same link opportunity. Count selections, placements and fallback reasons separately.
- Preserve feature-off terrain exactly, deterministic seeded generation, current lake/island controls, and preset comment/explicit-zero round trips.

## Capabilities

### New Capabilities

- `mapgen-river-junctions`: Probability-controlled lake-to-river Y connections, navigable confluence geometry, placement limits, preview configuration, compatibility, and reporting.

### Modified Capabilities

None. Existing `mapgen-lakes` navigation and land-preservation requirements continue to apply. The unarchived `mapgen-river-bends` change contains related river requirements but has no main `mapgen-rivers` spec yet; this proposal does not invent or modify that missing main capability.

## Impact

- Generation: `cmd/mapgen/river.go`, new focused junction helpers, and route metadata/bookkeeping in `river_fairway.go`.
- Geometry: shared Option B curve/corridor utilities, parent-specific water-contact validation, fairway validation, and tributary junction spacing in `river_tributaries.go`.
- Configuration/preview: `options.go`, river validation/memory estimates, YAML decoding/saving, `preview_server.go`, README, and only the new H&H keys with Russian comments.
- Verification: focused fixtures, zero-mode golden terrain, preview parity and round trips, and opt-in multi-seed/full-size image reviews.
- Planning only. No runtime implementation, dependency changes, database writes, terrain drainage, or changes to existing preset tuning are authorized by this plan request.
