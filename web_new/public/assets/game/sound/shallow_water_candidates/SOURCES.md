# Shallow-water footstep candidates

**04 Wading slosh** was selected on 2026-10-05 and is active for shallow-water
tiles through `footstep_shallow_water`. Its four individual footfalls are copied
unchanged into `sound/steps/shallow_water/`.
All previews contain 16 contacts at 120 steps per minute, mono PCM16 / 44.1 kHz,
with matched walking-preview RMS (-29.08 dBFS). Individual footfalls are retained
for later use in the game; do not use the walking previews as looping game assets.

## 01 — Soft water (CC0)

Water foley by **Peludo**, [Water Splash and sand footsteps](https://opengameart.org/content/water-splash-and-sand-footsteps),
[CC0 1.0](https://creativecommons.org/publicdomain/zero/1.0/).
The author requests a link to their itch.io page when used in a game; the source page records this request.
Quiet footwear layer by **HaelDB**, [Footsteps Leather, Cloth, Armor](https://opengameart.org/content/footsteps-leather-cloth-armor), using the offered CC0 license.
Edits: splash excerpts, a gentle low-pass filter, tail fades, leather layering,
normalization, and ±1.5% pitch variation. Four derived variants from two water recordings.

## 02 — Puddle boots (CC BY 3.0)

Derived from **Water Footsteps by EminYILDIRIM**:
https://freesound.org/people/EminYILDIRIM/sounds/608663/

And **footstep-concrete.wav by swuing**:
https://freesound.org/people/swuing/sounds/38873/

Mastered by **congusbongus**, [Footsteps on different surfaces](https://opengameart.org/content/footsteps-on-different-surfaces).
[Creative Commons Attribution 3.0](https://creativecommons.org/licenses/by/3.0/).
The upstream `license.txt` is preserved beside the five original OGG files.
Edits: Web Audio decoding/downmixing, short tail fades, contact trimming,
edge fades and normalization. Five distinct upstream contacts.

## 03 — Natural splashes (CC0)

**Splashing Footsteps Shallow Water by ChristopherJngs**:
https://freesound.org/people/ChristopherJngs/sounds/861369/

[CC0 1.0](https://creativecommons.org/publicdomain/zero/1.0/).
Uses the publicly available high-quality MP3 preview, not the login-only original WAV.
The MP3 and its decoded PCM are preserved. Edits: four footfall excerpts,
tail fades, contact trimming, edge fades and normalization.

## 04 — Wading slosh (CC BY 3.0)

**Blender Foundation / Yo Frankie!**, [Water Step Splashes](https://opengameart.org/content/water-step-splashes-yo-frankie),
[Creative Commons Attribution 3.0](https://creativecommons.org/licenses/by/3.0/).
Original left/right FLAC files and decoded PCM are preserved.
Edits: 0.70-second excerpts, tail fades, contact trimming, edge fades,
normalization and ±1.5% pitch variation. Four derived variants from two recorded contacts.

## Reproduction

Run from the repository root:

```sh
python3 art_source/audio/shallow_water_candidates/prepare.py
```

Recipes are in `art_source/audio/shallow_water_candidates/options.json`.
Shared stdlib PCM preparation is in `art_source/audio/footstep_audio.py`.
The manifest records individual output hashes and preserved source hashes.

## Preserved source hashes (SHA-256)

- `originals/christopherjngs/shallow-water-hq.mp3`: `f9f67010299d84663bdaca507e1a9696fa2e659311c852db3d5e2b8b155c99e3`
- `originals/christopherjngs/shallow-water.wav`: `00234a1ca3e6875fa8babca12f8ab900dd60d3e779766fb35f10c3cbb5103144`
- `originals/congusbongus/0.ogg`: `7a4a2934576d03d765a640df07ad89b70b5a5b17ed7cc4b80c9de4c51da44b10`
- `originals/congusbongus/0.ogg.wav`: `ddb8fc720759ee907eda195145c4eff0685fceb47ebd296c7045e312b8add3e5`
- `originals/congusbongus/1.ogg`: `ed60614c620953220da1b3109e2bf5a6ff863f5aeeaba802f88d05595a9ec919`
- `originals/congusbongus/1.ogg.wav`: `b6b9ecb7a8017626832247a1c6535be8c5e57cb58469245199278a2e740c24ab`
- `originals/congusbongus/2.ogg`: `b5d523ad6246c89c82a633c5a89c859aec3a8c7ac0fd5f3bcf890583fcd2805c`
- `originals/congusbongus/2.ogg.wav`: `3afafb4d379b9154501a338d39c18668fb4284f95f0cb679c08b14ceef0dc63d`
- `originals/congusbongus/3.ogg`: `32986928b63c6915e010dda99519fbe903e330cc1da060caf2dbd71fae81a738`
- `originals/congusbongus/3.ogg.wav`: `bde6404676e2fb5a1c96dd5f815bbde38ca36a8da553252181ea889f530d03b7`
- `originals/congusbongus/4.ogg`: `8c6af00b87cff844ae364735fbd185c7a1eb44939cefb8d02978f4f24ae854cc`
- `originals/congusbongus/4.ogg.wav`: `b37a6a068a3d540d2d0789412d5986b4572bbf08e58a057c268e1c84f8fcf189`
- `originals/congusbongus/license.txt`: `4ce92212722a6b7c5b6003223b2771974707ae2cd1b45103491ae94c707915a8`
- `originals/peludo/splash1.wav`: `e45d466f7e63e5d566f8a822a7a1a3f7d5131319d17ca0436dcc615b251e9277`
- `originals/peludo/splash2.wav`: `ad9472b711666de2d43c3b102d155a5d1118b1fcd0ecd29cb63177256ab63f31`
- `originals/yofrankie/water-left.flac`: `c0f1d087ec1ef2a30b69dec183967a05533e8659f7f05ad248eba75d1de93765`
- `originals/yofrankie/water-left.wav`: `f89141360f6404302fc2acd544a9743e3f1ee8570b38f6ba993a8a66b99c7cac`
- `originals/yofrankie/water-right.flac`: `7713430ceb95f8c65e4869ecdfe543d5a6c0df1f8b934e43370546b6bff15d0a`
- `originals/yofrankie/water-right.wav`: `1b843b1064477b7d31c3f448d644846a1e927d717a5e1a5aab0797545965a68c`
