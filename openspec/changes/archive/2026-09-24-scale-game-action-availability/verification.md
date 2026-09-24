# Verification

Checked on 2026-09-24:

| Check | Result |
| --- | --- |
| `make proto` | Passed; regenerated Go protocol bindings. |
| `npm --prefix web_new run proto` | Passed; regenerated browser bindings. These generated browser files are ignored by Git and are rebuilt by `npm run build`. |
| `go test ./...` | Passed with `GOCACHE=/private/tmp/origin-go-gocache`. The sandbox cannot write to the default Go cache under `~/Library/Caches/go-build`. |
| `go build ./...` | Passed with the same `GOCACHE` override. |
| `npm --prefix web_new run test:actions` | Passed. Covers selectable menu actions, immediate panel close, mini-alert message, drag, hotbar catalog checks, and cursor behavior. |
| `npm --prefix web_new run type-check` | Passed. |
| `npm --prefix web_new run build` | Passed. Vite reported non-failing warnings about large chunks and an existing mixed static/dynamic import. |
| `openspec validate scale-game-action-availability --strict` | Passed. |
| `git diff --check` | Passed. |

Focused Go tests cover a static catalog without requirement lookups or refreshes; activation, target, start, and commit validation; active-only rechecks through selection, approach, and timed execution; cancellation without stamina cost; and a fresh catalog with idle action state after re-entry. A repository search found no runtime reads of the removed protocol fields or availability signature.
