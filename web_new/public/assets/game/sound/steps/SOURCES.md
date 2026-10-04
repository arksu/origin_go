# Active footstep recordings

The default and land profiles were selected on 2026-10-04 from the six audition options.
These four profiles use CC0 1.0: https://creativecommons.org/publicdomain/zero/1.0/.
The original source pages and original-file hashes are preserved in
[candidate provenance](../footstep_candidates/SOURCES.md).

| Sound profile | Surface assignment | Recording source |
| --- | --- | --- |
| `footstep` | Default for every unassigned or unavailable tile | [HaelDB](https://opengameart.org/content/footsteps-leather-cloth-armor) |
| `footstep_stone` | Mountain | [Fantozzi; edited by Iwan Gabovitch (qubodup)](https://opengameart.org/content/fantozzis-footsteps-grasssand-stone) |
| `footstep_gravel` | Dirt, clay, plowed | [TinyWorlds; recordings from pdsounds.org](https://opengameart.org/content/different-steps-on-wood-stone-leaves-gravel-and-mud) |
| `footstep_forest_leaves` | Coniferous forest, broadleaf forest | [TinyWorlds; recordings from pdsounds.org](https://opengameart.org/content/different-steps-on-wood-stone-leaves-gravel-and-mud) |
| `footstep_shallow_water` | Shallow water | [Blender Foundation / Yo Frankie! — Wading slosh](https://opengameart.org/content/water-step-splashes-yo-frankie) · CC BY 3.0 |

## Shallow-water attribution

**Water Step Splashes (Yo Frankie!) by Blender Foundation**, selected as
**04 Wading slosh** on 2026-10-05. Source:
https://opengameart.org/content/water-step-splashes-yo-frankie

Licensed under **Creative Commons Attribution 3.0**:
https://creativecommons.org/licenses/by/3.0/.
Original left/right FLAC recordings and decoded PCM are preserved in
`art_source/audio/shallow_water_candidates/originals/yofrankie/`.
Modifications: mono PCM conversion, 0.70-second excerpts, 150 ms tail fades,
contact trimming, 3 ms edge fades, normalization, and ±1.5% pitch variation.
Four derived footfalls from two original recordings, copied unchanged from
the audition candidate. Full source hashes and processing details:
[water candidate provenance](../shallow_water_candidates/SOURCES.md).

Regenerate with `python3 art_source/audio/shallow_water_candidates/prepare.py`,
copy `04-wading-slosh/step_01.wav` through `step_04.wav` into `shallow_water/`,
then publish with `tools/assets publish-action-animations`.

## Playback and land recordings

Each profile keeps the established local hearing, attenuation, volume and
voice limits. The samples are mono 44,100 Hz signed 16-bit PCM WAVs with
quiet margins trimmed, 3 ms edge fades and matched energy per footfall.
Gravel uses one original recording with four small playback-rate variations.
No added reverb or new synthetic layer is used.

The retained flat `step_01.wav` through `step_04.wav` files are the previous
project-authored procedural sounds. They are no longer referenced by the
active profiles. `tools/asset_pipeline/generate-footsteps.py` only regenerates
those legacy files, not the selected recordings.

Regenerate audition samples with
`python3 art_source/audio/footstep_candidates/prepare.py`, copy the selected
single-step WAVs into the corresponding directories here, then publish
definitions with `tools/assets publish-action-animations`.

## Active sample hashes

| File | SHA-256 | Duration |
| --- | --- | --- |
| `leather/step_01.wav` | `f66c08b50468eb8edcb01827d3f815c82300d4c2be352f35f010b5ee3571734f` | 0.25 s |
| `leather/step_02.wav` | `a7a97aef97926aa4f307758b2b151018155ec09d47c64ad59c3ec971c62e5a14` | 0.1875 s |
| `leather/step_03.wav` | `8d9064d57e6c4e6eacffd28b5dc965685d9d58abb7821ac2aacb1107d0ab8262` | 0.25 s |
| `leather/step_04.wav` | `dd01a30f1bb9ed81c65bcd0651e3c78dc6ce8784741467733373c2af51422a67` | 0.25 s |
| `stone/step_01.wav` | `4781932f2f78f6bd5cb16a662a0be5ed6ef87f482805e7f66bc8b6911c3c2735` | 0.3485 s |
| `stone/step_02.wav` | `8df1155aba7bed7104f0042bd564d1405b505305721701250dc79a087df402ca` | 0.3445 s |
| `stone/step_03.wav` | `2539dcd1de07b2d7d7ebb097978ace986298ac4311d2c9e42e8816f391037def` | 0.3163 s |
| `stone/step_04.wav` | `ca06d7acdf55bd18043d4516fcc2a5adf7157f5c2017e1c080c50126993df206` | 0.3161 s |
| `stone/step_05.wav` | `e35c7eaad28280b7278dd2d13558025d34fcf8ba2199c008794f1999048ea432` | 0.3238 s |
| `stone/step_06.wav` | `7afdb25ec391b106e90c07b31c2ff9b68715203e90d52e45241769f8ba13c7f3` | 0.3392 s |
| `gravel/step_01.wav` | `b7ffbb6c6112d1bb0171a3fa080fa01cbffe3f911f9a6d7c243ca12f18086992` | 0.6874 s |
| `gravel/step_02.wav` | `bd8b97a2c5cbf57015608ceced58be719bc85e1b81b4ce5e4e01f97c7c8dd4ce` | 0.6668 s |
| `gravel/step_03.wav` | `f34abd4376b8432f94be232c223548da5f172df4811e4039c4304f4fc33b1577` | 0.6473 s |
| `gravel/step_04.wav` | `45235ee8d6a63096f948636d15b1de3c55dfaee70797a099ef6a90f7d864a3a2` | 0.6569 s |
| `forest_leaves/step_01.wav` | `a907651bad881ebc03cc9f734e1c1c80a39ec1272bfcc8a90381651d9dc56f9b` | 0.3841 s |
| `forest_leaves/step_02.wav` | `e142601e34afcbc270d42a33f3b68ce37b0cfd6d83a300488c8baa591583b9e7` | 0.4702 s |
| `shallow_water/step_01.wav` | `50b9b613435a54ef059a1dcc26588d7e6200c074569118f83a56b6ea3658e19a` | 0.7036 s |
| `shallow_water/step_02.wav` | `43c27fdd7b1baebde902b07155dc2b01956aa968c1a900f5a7bcdf53a1cdab5d` | 0.6828 s |
| `shallow_water/step_03.wav` | `3f13027f0edd8703a027af697bbfdba4bb667aeef3712edf232b7a62409c3776` | 0.7106 s |
| `shallow_water/step_04.wav` | `927d07a7b51b43a0d999d8a43113145f1ce6b1b2def60a166e8c9967766f0da4` | 0.6896 s |
