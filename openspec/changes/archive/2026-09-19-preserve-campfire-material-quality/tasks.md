# Tasks

## 1. Campfire construction quality

- [x] 1.1 Implement campfire-specific completion quality calculation from all consumed `branch` stacks using count-weighted arithmetic mean rounded down, and verify multi-stack/multi-slot quality tests.
- [x] 1.2 Apply the calculated quality during campfire build transformation while retaining quality zero when branch provenance is absent, and verify completed and legacy campfire persistence/restore behavior.

## 2. Ash outcome preservation

- [x] 2.1 Verify burner exhaustion creates dropped ash with the constructed campfire quality, including a quality-zero legacy campfire fallback.

## 3. Regression verification

- [x] 3.1 Run focused build, world-object persistence, burner exhaustion, and item-drop tests; fix regressions found.
- [x] 3.2 Run `go test ./...` and `go build ./...` and record unrelated failures separately.
