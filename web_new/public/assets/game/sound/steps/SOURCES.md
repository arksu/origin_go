# Original footstep samples

Created for this project on 2026-10-03 using the checked-in standard-library
synthesizer `tools/asset_pipeline/generate-footsteps.py`. These are original
procedural waveforms: short damped low-frequency impacts, filtered noise and a
brief friction transient. No recording, downloaded sample or third-party audio
was used. They are project-authored audio, distributed under the same terms as
the repository. The generator uses fixed variant seeds and is deterministic.

Format: mono 44,100 Hz, signed 16-bit PCM WAV. Regenerate from the repository root:

```sh
python3 tools/asset_pipeline/generate-footsteps.py
```

| File | SHA-256 | Duration |
| --- | --- | --- |
| `step_01.wav` | `4404881f14b14903bdb2fb491b647d57ef44aa26c63853ae0641b58f04bfdff8` | 0.16 s |
| `step_02.wav` | `d020454452da4b51b47007b11d363e99b0cf7ef40aa1847bc95af893478db472` | 0.16 s |
| `step_03.wav` | `cd660c0ebfa3abfbb8bda73e11ab7fa3d4e0d0141d59d3252e667a3c2ae67ba6` | 0.16 s |
| `step_04.wav` | `1daa2b2d38f2601b02f064016f23eeb48d0753badabfb2943e45ec4064bbd0b2` | 0.16 s |
