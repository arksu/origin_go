# Commoner source and provenance

Open `../source.blend` normally in Blender. The selected `commoner_rig` plays the
approved walk at frames 1–49 / 50 fps. The Action Editor contains all eight runtime
actions. Their muted NLA tracks retain convenient references; leave them muted
when editing one action. The four axe actions intentionally affect only the
three arm bones on their named side. To inspect one as a layer, use its matching
base idle/walk track under that arm action. Low LOD is hidden only in the viewport.

`EXPORT` contains the authored 50-bone rig, two original meshes, and bone-parented
hand/forearm sockets. `PREVIEW` and `ASSET_INFO` are authoring aids. All material
images are packed. The normal build reads the blend and recipe; it never runs the
one-time migration or reads this reference directory.

`meshy-original.glb` preserves the supplied Meshy geometry and original materials.
It does not inherit the old MakeHuman geometry license. `motion-donor/` separately
retains the earlier commoner animation documentation, manifests, and attribution.
The approved runtime animation was retargeted to the Meshy skeleton; migration
imports these approved channels as editable keys instead of rebuilding a gait.

`approved-runtime.glb` and `approved-baseline.json` are immutable comparison
goldens for the migration test, including the approved stride and attachment
transforms. They are diagnostic references, never normal-build inputs.
`candidate-audit.json` compares every legacy commoner action and each user-edit
scene to that approved runtime. Source hashes and copy provenance are recorded
in `provenance.json`. No source is selected by modification time.

Run `npm --prefix tools/asset_pipeline run test:migration` from the repository
root to check source editability, both LODs, textures, bind matrices, animation,
skinned positions, sockets, and equipment alignment against these goldens.
