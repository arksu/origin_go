# Tasks

## 1. Baseline and configuration

- [x] 1.1 Capture current fixed-seed river/network and final-terrain golden fixtures plus the current H&H configuration snapshot before runtime edits; verify they include existing lake/island/variable-width behavior and pass on the actual working tree.
- [x] 1.2 Add `junction_chance` and `junction_spacing_tiles` with zero/180 general defaults and focused validation; verify omitted/explicit-zero controls, probability 0/1, NaN/infinity, spacing bounds, active-zero spacing, incompatible layouts and invalid clearance in table tests.
- [x] 1.3 Add checked allocation estimates for active junction metadata, lookup entries and bounded scratch to direct-network and preview preflight; verify overflow/over-budget rejection before allocation and no added Y allocation estimate when probability is zero.

## 2. Parent metadata and selection

- [x] 2.1 Record origin lakes/border endpoints for accepted ordinary routes in an optional junction state, with stable route IDs and world-tile join coordinates; verify source-incident parents, Y parents and inland tributaries are excluded without changing zero-mode geometry.
- [x] 2.2 Implement deterministic spatial lookup and bounded parent-interior sampling with stage-specific distance ranges, full-corridor separation and conservative exclusions for all lake boundaries; verify valid distant parents, tight/short parents, future lake entrance clearance, and deterministic ordering under equal-distance ties.
- [x] 2.3 Implement one seeded probability roll per eligible remaining-major/short-stage opportunity, preferring an unconnected candidate lake as source; verify 0/1 and multi-seed selection counts, selection independent of retry/subdivision counts, and explicit eligibility statistics.

## 3. Confluence shape and atomic commit

- [x] 3.1 Generate an Option B branch with a smooth, oblique terminal approach to a sampled parent tangent and bounded local corridor blend; verify exact endpoint attachment, both approach sides, a curved parent, full configured variable widths/banks, and three arms without a continuation across the parent.
- [x] 3.2 Add parent-specific contact validation over the whole new corridor and its local blend; verify rejection of X crossings, nearby foreign water inside an endpoint allowance, remote arms of the same parent, unrelated lake contacts, and branch self-shortcuts.
- [x] 3.3 Commit only accepted candidates and track source/parent duplicate keys; verify source-only inlet/degree/connected updates, exactly one budget slot, no fictitious target-lake mutation, and complete rollback of rejected masks/routes/index entries before the saved ordinary fallback.

## 4. Network and final terrain integration

- [x] 4.1 Integrate Y attempts into remaining-major and short-link opportunities, including the existing short border fallback; verify regional backbone and explicit major border policy remain intact, original candidate seeds are reused after rejected Y attempts, budgets/degrees remain valid, and zero-mode golden terrain matches.
- [x] 4.2 Extend junction spacing for later inland tributaries using the maximum applicable cross-type spacing and physical corridor clearance; verify sparse nearby candidates are rejected, multiple well-spaced Y branches can share a long ordinary parent, and disabling Y preserves the old tributary result.
- [x] 4.3 Protect and validate connected full-footprint passage through all three arms and the source lake after final bank/biome/Perlin/shoreline processing; verify three-tile and wider clearances, fixed/variable shallow banks, islands and peninsula preservation, and deliberate final corruption produces seed/branch/parent/location diagnostics.
- [x] 4.4 Enforce the finite candidate limits and memory estimator assumptions in the implemented lookup and curve searches; verify dense-network exhaustion terminates cleanly and a bounded stress fixture representative of the supported 2000-major-link configuration stays within checked estimates without increasing the memory limit.

## 5. Preview and documentation

- [x] 5.1 Add both controls to the existing river preview schema with Russian labels, units and probability/off semantics; verify complete schema coverage, request validation, and direct-network versus river-only/full-terrain preview parity for zero and positive probabilities.
- [x] 5.2 Extend preset load/edit/save/reload coverage for partial junction settings and explicit zero; verify comments and unrelated current river/lake/biome values survive round trips and the existing preview browser tests still pass.
- [x] 5.3 Add only the proposed new H&H keys with Russian comments and document the eligible-opportunity denominator, fallback, spacing and separate tributary semantics in README; verify preset loading and compare all unrelated values with the captured snapshot.

## 6. Regression and visual review

- [x] 6.1 Run focused junction, fairway, lake and preset tests, then `go test ./cmd/mapgen/... -count=1`, `node --test cmd/mapgen/preview/index.test.mjs`, `go vet ./cmd/mapgen/...`, a temporary-output mapgen build and `git diff --check`; record results, including repeated seeds and differing worker counts, and distinguish unrelated failures.
- [x] 6.2 Extend opt-in preview-only review tooling and deliver 0/0.25/1 comparisons for seeds 12345, 67890, 314159 and the current H&H seed plus forced oblique/curved/dense-water fixtures; verify unchanged lake placement/backbone and report eligible/selected/placed/fallback counts, rejection reasons, connected lakes, network components and final clearance results.
- [x] 6.3 Run one current-size H&H preview-only review without database writes and deliver the overview and junction close-ups with actual generation time and clearly qualified memory measurements; verify the accepted junctions and lake navigation against final tiles.
- [x] 6.4 Obtain user visual acceptance of the delivered junction shapes and initial density, update only the new tuning if requested, and record the approved result separately from automated passage checks.
