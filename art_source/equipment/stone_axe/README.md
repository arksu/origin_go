# Stone axe — travel grip

Source: `/Users/park/Downloads/Meshy_AI_Stone_Axe_0915172926_texture.glb` (unchanged).
Game asset: `web_new/public/assets/game/equipment/stone_axe/stone_axe.glb`.

- 35,424 → 1,200 triangles; one 512×512 base-color texture; rigid, 0.66 m long.
- Grip origin sits inside the lower handle. Both hand bindings live in the catalog.
  The user's confirmed left/right transforms override the generated initial grips.
- `axe_idle_l/r` and `axe_walk_l/r` animate only upper arm, forearm and hand.
  The runtime samples walk by the same traveled-distance phase as the legs.
- `stone_axe_user_idle_pose.blend` preserves the user's right-arm idle pose.
  `idle-pose-overrides.json` contains its exported bone-local transforms plus
  the mirrored left arm. The merger applies these only to `axe_idle_l/r`;
  both equipped walk clips and both grip transforms remain unchanged.
- The runtime character keeps the original mesh, textures, bones and four base
  animations byte-for-byte. Only four new arm clips/buffers are appended.
- `stone_axe_review.blend` is a separate editable scene with the axe bone-parented
  and the right-hand travel layer over the original walk. Play frames 1–49.
- `stone_axe_user_grip.blend` preserves the confirmed right-hand scene;
  `stone_axe_user_grip_l.blend` preserves the confirmed left-hand scene.
  `grip-overrides.json` preserves both grips across rebuilds. Future captures save
  side-specific `_l` / `_r` files so one hand cannot overwrite the other's scene.

## Rebuild with the running Blender MCP add-on

Use `tools/blender/mcp_client.py` with a small script containing:

```python
import runpy
result = runpy.run_path('/Users/park/projects/origin_go/tools/blender/prepare_stone_axe.py')['main']()
```

Then run `python3 tools/blender/merge_axe_animations.py`. If changing the grip,
copy position/quaternion values from `export-report.json` into `equipment.ts`.
The MCP client uses Blender's official local add-on protocol on port 9876; it
does not start a cloud Blender session or replace the user's open file.

To capture another manual adjustment, select the bone-parented axe in Object
Mode and run `capture_stone_axe_grip.py`'s `main()` through MCP. It validates nine
gait frames, restores the current frame, saves the separate user scene and updates
the override/report. Copy the captured binding to `equipment.ts` for runtime use.

Checks: `python3 tools/blender/test_stone_axe_assets.py`,
`npm --prefix web_new run test:character-visual`, and the browser page
`/tests/hybrid-integration.html`. Interactive review: `/tests/axe-review.html`.

Lighting review uses a dark green background and upper-left illumination.
Attacks, sword/shield assets and finger articulation are outside this phase.
