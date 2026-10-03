#!/usr/bin/env python3
"""Generate original short dry footstep samples without third-party recordings."""
import argparse
import math
from pathlib import Path
import random
import struct
import wave

SAMPLE_RATE = 44100
DURATION_SECONDS = 0.16


def write_step(filename: Path, seed: int) -> None:
    generator = random.Random(seed)
    low_noise = 0.0
    samples = []
    frequency = 92 + generator.uniform(-12, 12)
    for index in range(round(SAMPLE_RATE * DURATION_SECONDS)):
        seconds = index / SAMPLE_RATE
        low_noise = 0.72 * low_noise + 0.28 * generator.uniform(-1, 1)
        attack = min(1.0, seconds / 0.0015)
        thump = math.sin(2 * math.pi * frequency * seconds) * math.exp(-seconds * 65)
        friction = low_noise * math.exp(-seconds * 38)
        transient = generator.uniform(-1, 1) * math.exp(-seconds * 280)
        amplitude = attack * (0.36 * thump + 0.32 * friction + 0.14 * transient)
        # The tail reaches zero to avoid a discontinuity at the sample boundary.
        amplitude *= min(1.0, (DURATION_SECONDS - seconds) / 0.015)
        samples.append(round(max(-1, min(1, amplitude)) * 32767))
    with wave.open(str(filename), 'wb') as output:
        output.setnchannels(1)
        output.setsampwidth(2)
        output.setframerate(SAMPLE_RATE)
        output.writeframes(struct.pack('<' + 'h' * len(samples), *samples))


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--out-dir', type=Path, default=Path(__file__).resolve().parents[2] / 'web_new/public/assets/game/sound/steps')
    arguments = parser.parse_args()
    arguments.out_dir.mkdir(parents=True, exist_ok=True)
    for variant in range(1, 5):
        write_step(arguments.out_dir / f'step_{variant:02}.wav', 2026100200 + variant)


if __name__ == '__main__':
    main()
