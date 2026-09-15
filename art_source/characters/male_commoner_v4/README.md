# Meshy commoner

User-supplied source: `Meshy_AI_Stonebound_Titan_0915094206_texture.glb`, received
2026-09-15. Unmodified copy: `source/meshy_character.glb`. This generated mesh is
not the old MakeHuman asset and must not inherit its CC0 attribution.

`commoner_rigged.blend` contains the fitted 50-bone rig, texture and four retargeted
clips: idle, walk, carry_idle, carry_walk. `commoner_meshy.glb` is the game export:
16,000 triangles at high detail, 5,500 at low detail, one shared 1024² base-color
texture. Full original 2048² PBR maps remain in the source GLB.

Rigging and review were executed through the running Blender MCP add-on. The
pipeline is `tools/blender/rig_meshy_commoner.py`; load its definitions and call
`main()` in a fresh Blender scene/session. It appends animation from the preserved
v3 runtime Blender file, fits the new skeleton, binds, retargets and exports.
`render_review(rig)` writes the five larger deformation-review images.

Walk/carry_walk place ankle centers 0.21 m apart using two-bone IK, keeping donor
foot timing, height and fore/aft motion. Idle uses the same 0.21 m ankle-center width.
Arm weights use connected lower-arm islands and smooth anatomical transitions at
the shoulder, elbow and wrist; hands do not receive torso/hip weights. The model
uses linear skinning in Blender and the client. The old asset's DQ mode remains
supported but causes unsuitable shoulder bulging for this arms-down source.

The wrap and belt are welded into the supplied body surface; they are currently
part of the base, not removable equipment. Old v3 linen assets are incompatible
with the new bind matrices and were removed from the active equipment catalog.
The equipment loader remains available for future Meshy-rig-compatible garments.
Hands retain the supplied closed fists; this does not create individually
articulated fingers or an open-palm mesh.

`validation.json` records all 49 source frames of every clip: normalized weights
(maximum four influences), finite geometry, unit bone scales, exact loop closure,
0.21 m walk stance and absence of torso weights on the lower hands. This does not
substitute for visual review in `/tests/hybrid-character.html` at game resolution.

Carry pose raises the shoulder girdle and brings the upper arms beside the head.
Arm rotations propagate through the parent chain before each joint swing. Computing
independent rest-to-overhead swings caused incompatible forearm/hand twist and
collapsed wrists; the corrected hands follow the forearm axis with their existing
finger shape. This changes the faulty wrist orientation, without opening or turning
the palms into upward-facing support hands. `validation.json` now checks wrist
alignment and limits the relative skinning rotation across each wrist to 30 degrees.
The carry grip now turns inward by sharing a mirrored 90-degree axial turn
between the upper arm and forearm. The wrist remains aligned and inherits the
forearm rotation. Idle bone matrices remain unchanged.

Forward/backward foot travel is 1.4375 times the donor motion (a further 15%
increase over 1.25), with cycle distance 1.3877975879375 tiles. Idle and moving ankle-center width remains 0.21 m.

Photo references used for the wrist/shoulder correction:
- Evan Osar / On Target Publications, left-hand overhead side-view photograph:
  https://www.otpbooks.com/evan-osar-pushing-patterns-forward-shoulder-posture/
  https://cdn.otpbooks.com/2016/08/07113502/Evan-Osar-Shoulder-Pushing-concentric.jpg
- CrossFit Games 2021 movement standard, image 10 (overhead finish):
  https://games.crossfit.com/workouts/onlinequalifiers/2021?division=18
  https://games-assets.crossfit.com/10-AG-Test1-2021.jpg
These guide the forearm/wrist silhouette and arm placement, not the character's
object-dependent grip or a literal exercise animation.

Skin base-color correction: `tools/blender/brighten_meshy_skin.py` applies a 1.15
RGB multiplier through a conservative skin-color mask with protected garment,
buckle and hair UV regions. The unadjusted image remains packed as the source,
so rerunning the correction does not compound it. The rig generator applies this
step before export. `skin-brightness-validation.json` records the protected pixel
comparison; lighting and the runtime palette remain unchanged.
