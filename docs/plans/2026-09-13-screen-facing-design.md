# Screen-relative character facing and removal of baked characters

Approved by the user on 2026-09-13. Keep eight directions with equal screen-angle
sectors. Project actual visual displacement through the map projection before
quantizing, without camera translation or zoom. Keep the last facing at rest and
use a small angular hysteresis at sector edges. Invert the actor camera projection
to turn the 3D model so its forward vector projects onto the selected screen ray.
Keep server movement/pathfinding, distance-driven gait and the user's updated
0.965424409-tile cycle unchanged.

Remove published baked character atlases and the baked fallback. Keep Blender
sources and reference art required by runtime export. Migrate useful old movement
regressions to the live actor path. Character loading/render failures must remain
explicit; no invisible fallback to missing textures. Test direction round trips,
boundary jitter, all eight screen rays, stopping, context restore and teardown.

Research: DevilutionX engine/point.hpp GetDirection uses tile-coordinate sectors;
Flare src/Avatar.cpp distinguishes mouse map targeting and keyboard directions,
with turn throttling. We choose screen sectors to match the approved UX.

Implementation: add shared facing projection helpers; update ObjectView's facing
from ObjectManager displacement; derive model yaw from camera elevation; remove
baked resources and paths; migrate tests and add a ray comparison; build and run
browser checks. The referenced writing-plans skill is unavailable in this session,
so this concrete plan is included here.
