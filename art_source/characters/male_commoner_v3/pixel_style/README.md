# Male commoner v3 — cartoon source for pixel baking

Editable static Blender model derived from `../male_commoner_v3.blend`.
This variant follows the approved change of direction: a cartoon character whose
silhouette reads in a frame only a few dozen pixels tall. It is an art source and
static bake proof, not a rigged or animated game character.

## Review

- `male_commoner_v3_pixel.blend`: source model in its editable rest pose.
- `bake/male_commoner_v3_pixel_bake.blend`: posed character beneath a fixed camera
  and light; the `Pixel • MODEL ROTATION ONLY` empty turns the whole character.
  Timeline frames 1–8 select the eight directions; these are turntable keys, not
  skeletal character animations.
- `previews/model_turnaround.png`: front, three-quarter, back and face views.
- `previews/concept_comparison.png`: the concept next to the updated model,
  with face and repaired-wrap detail views.
- `previews/model_bake_pose.png`: the compact static pose used for sprite proofs.
- `previews/model_linen_front.png` and `model_linen_side.png`: close inspection of
  the repaired wrap, including the upper panel tucked beneath the belt.
- `previews/pixel_readability.png`: native-size and nearest-neighbour enlarged
  examples at 32×48, 48×64 and 64×96; all eight directions at 48×64.
- `bake/pixel/`: transparent PNG frames and eight-column static turnaround atlases.
- `bake/manifest.json`: frame sizes, atlas order, ground anchors and bake checks.

## Art changes

- Larger, broader head, enlarged eyes and bold solid eyebrows; shorter legs and
  torso, larger hands and feet. The head transform also fits the eyes and hair.
- Stronger deltoids, biceps/triceps and forearm masses, following the muscular
  original concept. `shoulder_scale`, `upper_arm_scale` and `forearm_scale` in
  `style_config.json` control tapered regional volume changes, not uniform arm
  length or a percentage increase in total arm volume.
- Anatomical body remains a continuous UV-mapped mesh with editable sculpt keys.
  Non-destructive smoothing reduces small skin and facial creases.
- The concept refinement adds a separate expression/chest shape key: a subtle
  smile, broader jaw, stronger chest planes and more visible abdominal volumes.
  `concept_refinement` in the style configuration holds the sculpt controls.
- Hair is rebuilt into asymmetric swept forelocks and layered side/nape curls,
  with a continuous fitted underlayer and three restrained chestnut tones.
- The linen wrap is rebuilt with wider panels and flatter hems. The protruding
  side ties are removed. Panel tops sit underneath the continuous waistband,
  avoiding the previous floating loop and cloth/belt surface intersection.
- Matte skin, broad chestnut hair locks and cream cloth. Photographic skin maps,
  pore bump, hair strand shaders, cloth weave, nail plates and stitch objects are
  not used by the visible character. Upstream packed images remain in the file
  for provenance and the eye texture.
- Static bake pose brings the arms closer to the body. This pose is generated in
  memory for proofs; it is not a substitute for a deformation rig.

## Bake contract

Primary test frame: **48×64 px**. Secondary tests: 32×48 and 64×96.
An orthographic camera at 30° elevation renders each exact aspect ratio at 4×
resolution. BOX coverage resolve, binary alpha, a fixed 22-colour palette and a
one-pixel four-connected outer contour produce the final grid without dithering.
Review enlargements use nearest-neighbour filtering.

The **model rotates**, around world Z at the ground origin. The bake camera and
key light are fixed, independent and unanimated throughout all eight directions.
Their world matrices and the model angles are checked for every timeline frame
and recorded in `bake/scene_contract.json`. Large studio review images use their
own review camera; they do not control the sprite bake. Ground anchors are recorded per
frame; these are fractional pixel coordinates and must be used consistently by
the eventual sprite exporter. There is no cast-shadow layer in these sprite PNGs.

Atlas order is S, SE, E, NE, N, NW, W, SW. These labels describe character-facing
directions relative to the bake camera, not a tested mapping to the client.

## Rebuild

From the repository root, with Blender 5.2.1:

```sh
/Applications/Blender.app/Contents/MacOS/Blender --background --python tools/blender/adapt_male_commoner_v3_pixel.py
python3 tools/blender/bake_male_commoner_v3_pixel.py
```

The second command requires NumPy and Pillow. `--preview` on the Blender script
uses smaller model-review renders; the sprite source dimensions remain exact.
`--bake-only` loads the saved cartoon source and re-renders static sprite proofs.
`--verify-saved` reopens the source and checks geometry without rebuilding it.
`--verify-bake` reopens the separate bake scene and verifies model rotation and
the fixed camera/light on all eight frames.

`validation.json` records finite geometry, closed body edges, non-degenerate body
faces, retained shape keys/UVs and source/config hashes. The bake checks enforce
source dimensions, a transparent margin, binary alpha and palette membership.
The saved source check also measures waistband clearance from the evaluated body
and rejects any remaining obsolete side-tie geometry. Construction details are
in `tools/blender/commoner_concept_refinement.py`.
The shared leg-proportion deformation blends continuously across the centreline;
both cloth panels are checked for reversed horizontal vertex order to prevent
the former central self-intersection.
These are technical checks; final style acceptance is visual.

## Remaining production work

No animation rig, skin weights, action cycles, motion stability checks or client
integration are included. At 32×48 most facial and finger details are necessarily
lost; 48×64 carries the intended silhouette better. Animation baking will need
consistent pixel anchors and a review for flickering clusters. The mesh remains a
high-resolution render source, not a retopologized runtime asset.

The original v3 source and its license manifests are retained unchanged. The
MakeHuman anatomical mesh, morphs, eyes and source textures are CC0; see the parent
README and the source manifests for provenance.
