# Tree gathering sound sources

Selected on 2026-10-16. These are the standard sounds for every tree using the
corresponding action: `take_branch` and `pick_catkin` share the G1/G2 pool;
`take_bark` retains the existing B2 recording.
The existing action-animation markers play one local cue at phase `0.5` per cycle.
The existing sound player cycles through the two gathering samples in order.

## Generated gathering pool

G1 (Soft pluck) and G2 (Dry branch break) are original local procedural synthesis.
No external recordings, services or generative audio models were used. The user
approved their repeated audition previews; runtime files are the corresponding
single one-shots, copied byte-for-byte without the preview's second hit or pause.

| File | Duration | SHA-256 |
| --- | --- | --- |
| `g1.wav` | 0.48 s | `8661b4ea55af4d9e751c04ff54d1e20ad6a1dc8dd1939323eed24c734ac05aef` |
| `g2.wav` | 0.56 s | `4511ac89a211124af74b08c984c70489316efb83b03190db47029f508b49a813` |

The generation recipe, fixed seeds, source WAVs and measurement manifest are
preserved in `art_source/audio/gather_candidates/2026-10-16-synth-v1/`.
Format: mono PCM16 at 44,100 Hz. Event energy is matched at -30 dBFS for one event
per second. Playback level and distance attenuation remain authored in
`data/sounds/actions.json`.

## Recorded sounds

Both recordings are offered under
[CC0 1.0](https://creativecommons.org/publicdomain/zero/1.0/).
The files are unchanged public high-quality MP3 previews; the original WAV/AIFF
downloads require a Freesound login. No trimming, normalization or pitch edits
were applied. Playback level and distance attenuation live in
`data/sounds/actions.json`.
`branch_snap.mp3` is retained as the previous A1 selection and is no longer in
the active pool. `bark_scrape.mp3` remains the active B2 bark sound.

| File | Recording | Download | SHA-256 |
| --- | --- | --- | --- |
| `branch_snap.mp3` | [Twig Snap 04.wav by Glitchedtones](https://freesound.org/people/Glitchedtones/sounds/322411/) (A1) | [HQ MP3](https://cdn.freesound.org/previews/322/322411_3294528-hq.mp3) | `a39a4c788dfde25de235522b303bc0b57399495a77aa4765e1313e2f655cbcb8` |
| `bark_scrape.mp3` | [Wood_Scraping_Little.aif by RutgerMuller](https://freesound.org/people/RutgerMuller/sounds/104012/) (B2) | [HQ MP3](https://cdn.freesound.org/previews/104/104012_179538-hq.mp3) | `f30cad847e01ed557012b618ed5cd4be919032a827a15a7d48060d0e91411e71` |

Publish the matching sound and action definitions with
`tools/assets publish-action-animations`, then restart the server and reload clients.
