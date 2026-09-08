# Male commoner v3

Editable static character source based on the existing male-commoner
turnaround. The body uses anatomical quad topology, rather than the v2
assembly of ellipsoids. The asset contains no rig or animation.

## Open and inspect

Open `male_commoner_v3.blend` in Blender 5.2. The default camera shows the
three-quarter view. All image dependencies and the concept reference are
packed in the blend file. `previews/` contains front, side, back,
three-quarter, face, hand, feet and neutral-clay renders.

Collections:

- **BODY:** one connected high-resolution surface with UVs and an editable
  anatomy sculpt shape key.
- **EYES:** fitted eyes, textured iris and clear corneal shells.
- **HAIR:** independently editable swept locks, scalp and fitted eyebrows.
- **LINEN:** front/back draped panels, fitted waistband, hems and stitching.
- **NAILS:** fitted fingernail plates.
- **EDITABLE CONTROL CAGE:** hidden anatomical control mesh before surface
  subdivision and detail sculpting. Unhide it to inspect the working topology.
- **CONCEPT REFERENCE:** hidden original reference image.
- **STUDIO:** cameras, lights and ground.

The high-resolution surface is intended as an offline-render source.
The preserved control cage can be used for further modelling or a later
deformation workflow. Skinning, deformation tests, runtime optimisation and
sprite-atlas integration are not part of this delivery.

## Rebuild

From the repository root:

```sh
/Applications/Blender.app/Contents/MacOS/Blender --background \
  --python tools/blender/generate_male_commoner_v3.py -- --preview
```

Omit `--preview` for 1800x2200 renders at 96 Cycles samples. Use
`--views hero,portrait` to limit renders. The script recreates the blend file;
save manual Blender edits under another filename before rebuilding.

Verify the saved blend independently:

```sh
/Applications/Blender.app/Contents/MacOS/Blender --background \
  --python tools/blender/generate_male_commoner_v3.py -- --verify-saved
```

`model_config.json` contains body morph weights. The generator contains the
surface sculpt, swept hair geometry, garment pattern, fitting and materials.
`validation.json` records mesh and packed-image checks. These checks do not
establish visual quality or animation readiness.

## Asset provenance

The anatomical base, morph targets, male-muscle control topology, eye mesh
and skin/eye texture sources are MakeHuman Community assets released under
CC0. This is an adaptation of those assets, not a claim that their original
topology or textures were authored for this project.

- [MakeHuman asset license](https://static.makehumancommunity.org/about/license.html)
- [Source revision a8bc2d54](https://github.com/makehumancommunity/makehuman/tree/a8bc2d54ff0ac92e78ff71431b1023eda42bf482)
- [System asset pack and per-asset licenses](https://static.makehumancommunity.org/assets/assetpacks/makehuman_system_assets.html)

`source/makehuman/manifest.json` pins the repository revision and individual
checksums. `source/system_assets/manifest.json` records the official archive
URL, archive checksum and extracted-file checksums. The upstream CC0 text is
included at `source/makehuman/LICENSE.ASSETS.md`. The build uses only local
assets and does not require MakeHuman, an addon or a network connection.

## Visual review status

This is a substantially rebuilt v3 source for review, not an approved final
production character. The rendered face and hair retain more generic 3D
forms than the expressive hand-drawn concept; their exact likeness and the
final game art style still need visual approval and possible further sculpting.
The current neutral A-pose is retained for inspecting anatomy and separation
of the fingers. No animation quality is implied by this static model.
