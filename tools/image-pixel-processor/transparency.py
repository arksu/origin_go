from __future__ import annotations

import numpy as np
from PIL import Image


def has_meaningful_transparency(
    image: Image.Image,
    alpha_ratio_threshold: float,
    alpha_value_threshold: int,
) -> bool:
    if not 0.0 <= alpha_ratio_threshold <= 1.0:
        raise ValueError("alpha_ratio_threshold must be within [0.0, 1.0]")
    if not 0 <= alpha_value_threshold <= 255:
        raise ValueError("alpha_value_threshold must be within [0, 255]")

    rgba_image = image.convert("RGBA")
    alpha_channel = np.asarray(rgba_image.getchannel("A"), dtype=np.uint8)
    transparent_pixels = int(np.count_nonzero(alpha_channel < alpha_value_threshold))
    total_pixels = int(alpha_channel.size)

    if total_pixels == 0:
        raise ValueError("input image has zero pixels")

    transparency_ratio = transparent_pixels / total_pixels
    return transparency_ratio >= alpha_ratio_threshold
