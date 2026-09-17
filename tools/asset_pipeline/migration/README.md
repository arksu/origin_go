# Historical source migration

This directory records the one-time conversion to the canonical saved Blender
sources. Its original input paths refer to assets retired after acceptance on
2026-09-17. It is not a maintained asset generator and is never run by a normal
build. Recover old inputs from the commit recorded in
`docs/assets/legacy-removal.json` only when investigating the migration.

The `--verify-only` source checks and snapshot comparison helpers remain useful
for existing migration tests: they read canonical sources and retained goldens.
Use `tools/assets` and `docs/assets/README.md` for current authoring and export.
