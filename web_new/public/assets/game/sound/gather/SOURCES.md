# Tree gathering sound sources

Selected on 2026-10-16. These are the standard sounds for every tree using the
corresponding action: `take_branch` and `pick_catkin` share A1; `take_bark` uses B2.
The existing action-animation markers play one local cue at phase `0.5` per cycle.

Both recordings are offered under
[CC0 1.0](https://creativecommons.org/publicdomain/zero/1.0/).
The files are unchanged public high-quality MP3 previews; the original WAV/AIFF
downloads require a Freesound login. No trimming, normalization or pitch edits
were applied. Playback level and distance attenuation live in
`data/sounds/actions.json`.

| File | Recording | Download | SHA-256 |
| --- | --- | --- | --- |
| `branch_snap.mp3` | [Twig Snap 04.wav by Glitchedtones](https://freesound.org/people/Glitchedtones/sounds/322411/) (A1) | [HQ MP3](https://cdn.freesound.org/previews/322/322411_3294528-hq.mp3) | `a39a4c788dfde25de235522b303bc0b57399495a77aa4765e1313e2f655cbcb8` |
| `bark_scrape.mp3` | [Wood_Scraping_Little.aif by RutgerMuller](https://freesound.org/people/RutgerMuller/sounds/104012/) (B2) | [HQ MP3](https://cdn.freesound.org/previews/104/104012_179538-hq.mp3) | `f30cad847e01ed557012b618ed5cd4be919032a827a15a7d48060d0e91411e71` |

Publish the matching sound and action definitions with
`tools/assets publish-action-animations`, then restart the server and reload clients.
