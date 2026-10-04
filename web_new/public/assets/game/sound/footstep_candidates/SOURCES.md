# Footstep replacement candidates

Prepared on 2026-10-04. These are six audition options. Soft leather is the
active default; stone, gravel and forest leaves are selected tile overrides.
See [active sample provenance](../steps/SOURCES.md).

Listen at `/footstep-preview.html` on the existing web client dev server.
Each preview contains sixteen footfalls spaced 0.5 seconds apart.
The selected profiles live in `data/sounds/actions.json` and are published
through the existing asset tooling.

## Sources and licenses

The six candidates use the CC0 1.0 license offered by the linked source
pages: https://creativecommons.org/publicdomain/zero/1.0/. HaelDB also offers
OGA-BY; these candidates use its CC0 option.

| Option | Recording source | Prepared samples |
| --- | --- | --- |
| 1. Soft leather | [HaelDB](https://opengameart.org/content/footsteps-leather-cloth-armor) | 4 |
| 2. Grass / sand | [Fantozzi; edited by Iwan Gabovitch (qubodup)](https://opengameart.org/content/fantozzis-footsteps-grasssand-stone) | 6 |
| 3. Gravel | [TinyWorlds; recordings from pdsounds.org](https://opengameart.org/content/different-steps-on-wood-stone-leaves-gravel-and-mud) | 4 |
| 4. Forest leaves | [TinyWorlds; recordings from pdsounds.org](https://opengameart.org/content/different-steps-on-wood-stone-leaves-gravel-and-mud) | 2 |
| 5. Stone | [Fantozzi; edited by Iwan Gabovitch (qubodup)](https://opengameart.org/content/fantozzis-footsteps-grasssand-stone) | 6 |
| 6. Wood | [TinyWorlds; recordings from pdsounds.org](https://opengameart.org/content/different-steps-on-wood-stone-leaves-gravel-and-mud) | 3 |

TinyWorlds describes its source recordings as pdsounds.org recordings
edited for Minetest. Fantozzi recordings were cut into single steps by
Iwan Gabovitch (qubodup). All six options are sourced recordings, not new
AI-generated audio.

Gravel has one original recording; its four prepared samples use playback
rates 0.97, 1.0, 1.03 and 1.015 to reduce identical repetition. The other
options use distinct source footfalls with no pitch changes.

## Preparation

Original Ogg files and decoded source WAVs are retained in
`art_source/audio/footstep_candidates/originals/`. Ogg sources were decoded
with Chromium Web Audio at 44,100 Hz and downmixed by averaging channels.
Decoded WAVs are mono signed 16-bit PCM, so regeneration needs no codecs.
The deterministic stdlib preparer removes DC offset, trims quiet margins,
adds 3 ms edge fades and matches energy per footfall to -29 dB RMS at two
footfalls per second, subject to a 0.85 peak cap. No reverb is added.
The active default reference keeps its original level. Browser volume starts at
100% for audition; the game-volume button uses the current 35% multiplier.

From the repository root:

```sh
python3 art_source/audio/footstep_candidates/prepare.py
```

Recipes, source filenames and descriptions live in `options.json` beside
the preparer. Output hashes, durations, peaks, RMS and per-sample source
mapping are recorded in `manifest.json` beside this file.

## Original recording hashes

| Source file | SHA-256 |
| --- | --- |
| `originals/fantozzi/Fantozzi-SandL1.ogg` | `fe6ec00cbe8790be0f37540cb8db8d245ca062337b121003685b6a96700cd0ae` |
| `originals/fantozzi/Fantozzi-SandL2.ogg` | `68ba66063efa32b3a83979194582b1420f808cd217ccb5839c382d0e08522e18` |
| `originals/fantozzi/Fantozzi-SandL3.ogg` | `0a526053424687c18735bca107b7234e48aceeb6bf12eec1f182d89aaf792b71` |
| `originals/fantozzi/Fantozzi-SandR1.ogg` | `439ffe4d6fdf38194caf87e103897354756d69607bd4e48cef923e1ade68ee70` |
| `originals/fantozzi/Fantozzi-SandR2.ogg` | `5eec48117da8fa433963ba06a93f67705f0bbd86ac3f473ab7b3bd120bf55fce` |
| `originals/fantozzi/Fantozzi-SandR3.ogg` | `7498c4e50282acc4e7e0442a9f2e551fae685898f8261152dbc627f04901ddbe` |
| `originals/fantozzi/Fantozzi-StoneL1.ogg` | `2d85768f04a85b6068f3548b33d6e0b0424c5e62b0941042d593affd3a9555d4` |
| `originals/fantozzi/Fantozzi-StoneL2.ogg` | `1cdd36f02590d2b968b1dd311334e71db8514d7c276e62ad474d01c26b37d4de` |
| `originals/fantozzi/Fantozzi-StoneL3.ogg` | `6e008bba3c24c6adc20151126768eba8c9259ec9f40b7c0b8399d60a9ca67812` |
| `originals/fantozzi/Fantozzi-StoneR1.ogg` | `d6949efd0255d50efe4853cc077b18e3ee96b155707ccf4199fd530aaac82d58` |
| `originals/fantozzi/Fantozzi-StoneR2.ogg` | `075c27afbf29b7771cb01a7bf51530b835e82c56698ca5551318fa0c6b7adb9f` |
| `originals/fantozzi/Fantozzi-StoneR3.ogg` | `73bbdfc3d9e891bc1afbab7050de3bd59adf13cf5145ff3fb2f1fa7b251d6d82` |
| `originals/haeldb/step_lth1.ogg` | `8abf47391e53218a4826a1419e4861ae7dae14df55c51a322f2fa2d4fab2005f` |
| `originals/haeldb/step_lth2.ogg` | `35214722297cda533de2158e3a4754c5639990760b213101fe097ab34bab1b86` |
| `originals/haeldb/step_lth33.ogg` | `a9b08f5e1b9baad2dabaee57a0f33942cee42844805e2f8e5ce84449f0500bda` |
| `originals/haeldb/step_lth4.ogg` | `cef53154dc37d57a4c1d2548f23daa25d5bbdecc5cd21f807ada5a1078e12513` |
| `originals/tinyworlds/gravel.ogg` | `34e6057ec1581b5cb8802e4942952b00b1dac235bfcb0ed8cff0ceb15a3ebbbf` |
| `originals/tinyworlds/leaves01.ogg` | `93b3bbcd06380eec54335d1869924db6ff2cc6c1cb103502133d78484c5ca836` |
| `originals/tinyworlds/leaves02.ogg` | `477c35bfd3f740915b24f93cb66364523217692d2749057f9b8be162eb8209f4` |
| `originals/tinyworlds/wood01.ogg` | `482d97c5e1e6eb7b239be46f25d093ebebb94cefb56eedb04807b6a5dc68e972` |
| `originals/tinyworlds/wood02.ogg` | `588bb121bd3db005b4c2a5b55c1cf39b8ec620f820c2123ac1f62748d132dac2` |
| `originals/tinyworlds/wood03.ogg` | `9b677c284389163f132b94e1323ea713a4c7ac9d5fba849cf7cbf8b53a1627e2` |
