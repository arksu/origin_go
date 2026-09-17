# Reviewed optimizer profile and production budgets

Measured 2026-09-17 with Blender 5.2.1 build `9e2066aef7ef`, locked Node 25.8.0,
glTF Transform 4.5.0, meshoptimizer 1.2.0, sharp 0.35.4 and local toktx 4.4.2.
No package installation or network connection is part of optimization.

The model retains node names, hierarchy, authored transforms, sockets, materials,
region/LOD/skinning extras, skin joint order and inverse bind matrices. Meshopt
uses the low-level extension writer without a quantize transform or lossy filters.
Decoded attributes, skin matrices and animation times/values compare exactly.
Triangle index comparison permits only meshopt's legal cyclic corner rotation;
triangle order, membership and winding remain unchanged. Constant animation
tracks retain all authored sampled keys and their interpolation mode.

Textures are RGBA8 sRGB canonical PNG pixels, resized only to fit the declared
dimensions without enlargement. Neither production asset required resizing.
The declared profile is UASTC quality 2, Zstd 18, full mipmaps, one encoder thread:

```
--t2 --encode uastc --uastc_quality 2 --zcmp 18 --genmipmap
--filter lanczos4 --fscale 1 --threads 1 --assign_oetf srgb
--assign_primaries bt709 --upper_left_maps_to_s0t0
```

Pinned toktx 4.4.2 documents `--wmode clamp` but rejects it (also rejects wrap and
reflect) with exit 1/usage. The explicit profile records the locked binary's
documented default clamp. `TOKTX_OPTIONS` is stripped; a regression supplies
conflicting ambient codec, orientation and mipmap options and verifies identical
model and metadata hashes. There is no automatic ETC1S fallback.

GLB images use `textures/<content-sha256>.ktx2` external URIs fixed before hashing
the model. The packer embeds only the actual primary buffer, leaves meshopt's
virtual fallback buffer virtual, and preserves raw node TRS values that Core
otherwise rounds to identity. Clips contain no images, meshes or materials.

| Published byte bucket | Character | Stone axe |
| --- | ---: | ---: |
| Model GLB (geometry and GLB structure) | 732,036 | 87,428 |
| Animation GLBs | 336,624 | 0 |
| Metadata JSON | 139,858 | 4,176 |
| Unique external KTX2 | 1,169,239 | 324,777 |
| Total | 2,377,757 | 416,381 |
| Recipe byte budget | 2,853,888 | 499,712 |

Each byte budget is `ceil(measured total * 1.20 / 1024) * 1024`. The catalog total
is 2,794,138 bytes. Catalog accounting deduplicates textures by content hash across
assets; each model, clip and metadata file still counts separately.

Character: LOD 0 = 16,000 triangles; LOD 1 = 5,500; 50 bones; maximum 4 weights
per vertex; texture 1024 x 1024. Axe: LOD 0 = 1,200 triangles; no skin or weights;
texture 512 x 512. Required numeric LOD budget keys match numeric `extras.lod`.
Both recipes declare an explicit bone ceiling of 50 and influence ceiling of 4.
Missing budgets fail, including missing limits for encountered LODs.

| Full-resolution RGBA error (0..255) | Character | Stone axe |
| --- | ---: | ---: |
| Mean absolute error | 0.3806307316 | 0.7295494080 |
| RMS error | 0.7409727093 | 1.3672223768 |
| Maximum channel error | 20 | 37 |
| Reference PNG bytes | 1,775,866 | 479,962 |

Measurements decode the actual KTX2 using the locked Three Basis WASM transcoder.
The texture quality gate is RMS < 8/255. Both full-resolution reference sampling
and same-resolution compression sampling are reported. The game's palette/GPU
visual acceptance is intentionally scheduled with Task 8/9 runtime integration
by the controller; file-level color measurements do not claim that visual check.

glTF Validator reports zero errors. Known warnings are its unsupported KTX2 MIME
and image format plus the preserved character's two non-root skinned meshes.
Its unsupported meshopt/Basis extension notices are covered by real decoded
geometry/animation comparisons and real Basis transcoding, not ignored payloads.

Metadata includes profile, profile hash, package-lock hash, full locked toolchain
hash and rig hash including the raw bone hierarchy/rest matrices plus the skin's
joint/inverse-bind contract. Profile hashes:

- Character: `307951c1a3b664c7080cd2a84c84f7d8eb8917731771c472bf388b29c0eae900`
- Axe: `9e861d42c623a1780c26168918981ce3c3b84aedae60df7e183d01535b742442`

Client decoders are copied from installed locked Three 0.186.0 by
`npm run install-decoders`, with package integrity, lock hash, source paths,
per-file hashes and license provenance in
`web_new/public/assets/game/decoders/provenance.json`. This is an offline copy.

Reproduce with `npm run test:optimize` and `npm run test:optimize-production` in
`tools/asset_pipeline`. Production tests print `OPTIMIZATION_REVIEW=<report path>`;
the report includes local raw/optimized paths, byte measurements and color error.
