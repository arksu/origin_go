# Stone axe source and references

Open `../source.blend` in Blender. The commoner preview is linked using the
relative path `//../../character/male_commoner/source.blend`; no network,
Downloads folder, scripts, or MCP connection is needed. Play frames 1–49.

Select `GRIP_L` or `GRIP_R` to edit that equipment attachment frame. The labeled
left and right preview axes update immediately through native constraints:
socket world transform × inverse grip frame × original axe mesh. The grip frame
is the editable source of truth; the exporter produces its inverse for runtime.
`EXPORT` holds only the axe and grip frames. The original axe is hidden in the
viewport because both equipped copies are visible in `PREVIEW`.

Both bindings deliberately retain ordinary idle/walk motion. Weapon arm clips
remain available in the commoner source but are not automatically enabled by
equipping this axe.

`meshy-original.glb` is the untouched user-supplied Meshy asset. The canonical
axe uses the existing 1,200-triangle authored mesh from the confirmed right-grip
scene, including its original custom normals. `approved-runtime.glb` is the
comparison golden, not a normal-build input.

The four preserved user Blender scenes are historical editable references.
The confirmed idle scene and its mirrored `idle-pose-overrides.json` match the
shipped idle arm channels. The independent walk-edit scene differs from shipped
walk by approximately 0.363208 radians and is retained without applying it.
Both confirmed grip scenes, captured transforms, and validation records remain
available. See the commoner `references/candidate-audit.json` and this directory's
`provenance.json` for measured differences and source hashes.
