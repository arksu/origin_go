# Tasks

## 1. Compatibility and configuration

- [x] 1.1 Capture pre-change deterministic fixtures for drawn river masks and a small final terrain map using fixed legacy settings; verify the fixtures pass against the unchanged generator before implementing new behavior.
- [x] 1.2 Add the six zero/off-compatible river controls from the design, cross-field validation, and memory-budget accounting; verify YAML tests cover omission, explicit zero, valid widths 1/3/5, non-finite input, invalid width/length/spacing, and unsupported layout combinations without changing legacy defaults.

## 2. Bends within bends

- [x] 2.1 Add a focused Option B path helper with tile-distance wavelengths, broad-curve arc-length sampling, smaller-scale normal displacement, bounded adaptive subdivision, and endpoint blending; verify fixed endpoint tests cover short/long routes, retained fine bends on long links, exact endpoints, and repeatable seeds.
- [x] 2.2 Add ordered connected rasterization and bounded rejection of unintended self-contact/crossings; verify diagonal, tight-bend, crowded, and boundary fixtures preserve continuity and terminate without painting rejected candidates, while all-zero legacy fixtures remain identical.

## 3. Protected deep fairways

- [x] 3.1 Retain accepted main-route metadata and sweep the configured deep footprint into a separate protected mask, adding banks only outside it; verify widths 1/3/5 along straight, curved, diagonal, terminal, and edge-cropped routes, including a deliberate centerline-only counterexample.
- [x] 3.2 Apply the same protection to main junctions and lake-inlet routes, connecting accepted entrances through a reachable deep interior with bounded local widening where necessary; verify multiple-inlet, narrow-neck, shallow-seam, and disconnected-water fixtures, plus unchanged legacy inlet tests when protection is zero.

## 4. Sparse tributaries

- [x] 4.1 Add a deterministic post-main-route tributary pass with independent salts, floor(count * ratio) budget, at most one branch per parent, no recursion, junction spacing, length bounds, and bounded candidates; verify zero disables it, 100 routes at 0.08 allow at most eight branches, main routes/lakes remain unchanged, and more geometry samples do not increase the budget.
- [x] 4.2 Route each accepted tributary through Option B and the shared protected carver; verify parent-junction clearance, deep terminal caps, nearby-junction rejection, no unintended extra water contacts, and no partial shallow fragments after rejection.

## 5. Final terrain integration

- [x] 5.1 Add explicit protected-fairway precedence to final tile resolution while keeping existing behavior elsewhere; verify the legacy ocean-precedence test still passes and new regressions keep protected deep water through shallow elevation and the full shoreline pass.
- [x] 5.2 Validate retained route footprints and intended deep connections on final tiles, propagate contextual errors before world output, and report main/tributary/rejection counts; verify intentionally corrupted final tiles fail with seed and route/location details, and accepted main, branch, connector, and lake-interior routes pass.
- [x] 5.3 Verify no terrain-model regression with identical elevation buffers, unchanged generation when rivers are disabled, no imposed coastal frame, and identical output for one versus multiple workers; keep checks focused on unchanged inputs rather than requiring land beside moved river paths to remain identical.

## 6. Preview and preset review

- [x] 6.1 Expose the new controls in the existing river preview schema and label legacy-only controls; verify preview API validation, JSON/YAML key parity, and agreement with production river masks for identical settings without a frontend rewrite.
- [x] 6.2 Record the user's minimum-clearance choice, then enable Option B with explicit sparse settings in H&H only and document control semantics, legacy rollback, and the distinction between generator clearance and boat-runtime testing; verify every shipped preset loads and unaffected presets retain legacy fixtures.
- [x] 6.3 Run `go test ./cmd/mapgen/...`, `go vet ./cmd/mapgen/...`, and `go build ./cmd/mapgen`; record outcomes separately from visual and runtime boat acceptance.
- [x] 6.4 Render full-size H&H seeds 12345, 67890, and 314159 without DB/world writes, plus rivers-only and deep/shallow close-ups; deliver an image/report set with bend detail, branch counts/spacing, main-route coverage/components versus baseline, final clearance results, timing, and memory. Do not mark this complete based only on passing unit tests.
- [x] 6.5 Present the rendered comparisons for user visual acceptance and record any requested tuning; verify acceptance explicitly before marking the selected preset's appearance complete or archiving the change.

Visual acceptance recorded on 2026-09-29: "Yes, the picture is satisfactory overall."
No further visual tuning requested. Acceptance covers appearance, not boat-runtime
testing or a guarantee of global waterway connectivity; those caveats remain in
`docs/plans/river-bends-review.md`.
