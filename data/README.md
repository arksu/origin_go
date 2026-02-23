# Data Catalog (Content Creator Guide)

This folder contains game content definitions loaded by the server at startup.

Current catalogs:
- `items/` — item definitions (inventory items, stack rules, containers)
- `objects/` — world object definitions (spawned entities, visuals, behaviors)
- `crafts/` — crafting recipes (item inputs -> item outputs)
- `builds/` — build recipes (item inputs -> world object result)

## How Content Loading Works

Server startup loads catalogs in this order:
1. `data/items`
2. `data/objects`
3. `data/crafts`
4. `data/builds`

This matters because:
- `objects` may validate references to items (behavior configs like tree/take)
- `crafts` validate item/object references
- `builds` validate item/object references

If validation fails, startup stops with a clear error message that includes:
- file path
- `defId` and/or `key`
- validation reason

## File Format Rules (All Catalogs)

All content files are JSONC (`.json` or `.jsonc`) with comments supported.

Required wrapper shape (catalog-specific list key):

```json
{
  "v": 1,
  "source": "human-friendly source label",
  "items|objects|crafts|builds": []
}
```

Important rules:
- `v` must be `1`
- Unknown fields fail validation (`DisallowUnknownFields` is enabled)
- Standard JSON rules still apply after comments are stripped
- Do not use trailing commas
- Use unique `defId` values within the whole catalog folder (across all files)
- Use unique `key` values within the whole catalog folder (across all files)

## Recommended Workflow for New Content

1. Pick the target catalog (`items`, `objects`, `crafts`, or `builds`)
2. Copy a similar existing file/entry
3. Change one thing at a time
4. Keep IDs and keys unique
5. Start the server in your local dev environment to validate defs at startup
6. Fix the first reported error and repeat (loaders fail fast on first error)

## Naming / Organization Guidelines

- Use `snake_case` keys (`stone_axe`, `build_crate_from_ore_batch`)
- Keep one theme per file (for example: `tools`, `ores`, `containers`)
- Keep `source` aligned with file purpose (`"tools"`, `"starter crafts"`, etc.)
- Prefer readable names over abbreviations

## Cross-References Cheat Sheet

- `crafts.inputs[].itemKey` -> `items.key`
- `crafts.outputs[].itemKey` -> `items.key`
- `crafts.requiredLinkedObjectKey` -> `objects.key`
- `builds.inputs[].itemKey` -> `items.key`
- `builds.objectKey` -> `objects.key`
- `itemTag` references item tags from `items[].tags`

## Common Mistakes

- Duplicate `defId` or `key` in different files in the same catalog
- Empty `name` where the catalog requires it (`items`, `objects`)
- Using both `itemKey` and `itemTag` in the same input
- Using neither `itemKey` nor `itemTag` in an input
- Referencing an item/object key that does not exist
- Adding an extra field that is not part of the schema

See catalog-specific README files for exact field rules and examples.

