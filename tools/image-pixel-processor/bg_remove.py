from __future__ import annotations

import io
import logging

import cv2
import numpy as np
from PIL import Image

logger = logging.getLogger(__name__)


def remove_background(image: Image.Image) -> Image.Image:
    try:
        from rembg import remove
    except ImportError as exc:
        raise RuntimeError(
            "rembg is required. Activate tool venv and reinstall dependencies: "
            "source tools/image-pixel-processor/.venv/bin/activate && "
            "python3 -m pip install -r tools/image-pixel-processor/requirements.txt"
        ) from exc

    try:
        import onnxruntime  # noqa: F401
    except ModuleNotFoundError as exc:
        raise RuntimeError(
            "onnxruntime backend is missing for rembg. "
            "Install CPU backend in the tool venv: "
            "source tools/image-pixel-processor/.venv/bin/activate && "
            "python3 -m pip install 'rembg[cpu]'"
        ) from exc

    source = image.convert("RGBA")
    input_buffer = io.BytesIO()
    source.save(input_buffer, format="PNG")

    try:
        output_bytes = remove(input_buffer.getvalue())
    except Exception as exc:
        if "onnxruntime backend" in str(exc).lower():
            raise RuntimeError(
                "rembg failed because no onnxruntime backend is available. "
                "Activate venv and run: python3 -m pip install 'rembg[cpu]'"
            ) from exc
        raise

    rembg_result = Image.open(io.BytesIO(output_bytes)).convert("RGBA")
    soft_mask, strong_ratio, median_alpha, p90_alpha = _is_soft_mask(rembg_result)
    if not soft_mask:
        return rembg_result

    fallback_result = _remove_edge_connected_background(source)
    logger.info(
        "event=bg_fallback method=edge_connected reason=soft_rembg_mask "
        "strong_ratio=%.4f median_alpha=%.1f p90_alpha=%.1f",
        strong_ratio,
        median_alpha,
        p90_alpha,
    )
    return fallback_result


def _is_soft_mask(image: Image.Image) -> tuple[bool, float, float, float]:
    alpha_channel = np.asarray(image.getchannel("A"), dtype=np.uint8)
    non_zero_alpha = alpha_channel[alpha_channel > 0]
    if non_zero_alpha.size == 0:
        return True, 0.0, 0.0, 0.0

    strong_ratio = float(np.count_nonzero(non_zero_alpha >= 128)) / float(non_zero_alpha.size)
    median_alpha = float(np.percentile(non_zero_alpha, 50))
    p90_alpha = float(np.percentile(non_zero_alpha, 90))

    is_soft = strong_ratio < 0.12 and median_alpha < 24.0 and p90_alpha < 160.0
    return is_soft, strong_ratio, median_alpha, p90_alpha


def _remove_edge_connected_background(image: Image.Image) -> Image.Image:
    rgba_image = image.convert("RGBA")
    rgb = np.asarray(rgba_image.convert("RGB"), dtype=np.int16)
    alpha_channel = np.asarray(rgba_image.getchannel("A"), dtype=np.uint8)
    height, width = rgb.shape[:2]
    if height == 0 or width == 0:
        return rgba_image

    edge_samples = np.array(
        [
            rgb[0, 0],
            rgb[0, width - 1],
            rgb[height - 1, 0],
            rgb[height - 1, width - 1],
            rgb[0, width // 2],
            rgb[height - 1, width // 2],
            rgb[height // 2, 0],
            rgb[height // 2, width - 1],
        ],
        dtype=np.int16,
    )
    background_color = np.median(edge_samples, axis=0)

    distance = np.sqrt(np.sum((rgb.astype(np.float32) - background_color.astype(np.float32)) ** 2, axis=2))
    edge_distance = np.concatenate(
        [distance[0, :], distance[height - 1, :], distance[:, 0], distance[:, width - 1]]
    )
    threshold = max(8.0, float(np.percentile(edge_distance, 75)) + 6.0)

    candidate_background = distance <= threshold
    edge_background = _border_connected_components(candidate_background)

    foreground = (~edge_background) & (alpha_channel > 0)
    foreground = _remove_small_components(
        foreground,
        min_component_area=max(16, int(round((height * width) * 0.0002))),
    )
    if not bool(foreground.any()):
        return rgba_image

    rgba_array = np.asarray(rgba_image, dtype=np.uint8).copy()
    rgba_array[:, :, 3] = np.where(foreground, 255, 0).astype(np.uint8)
    return Image.fromarray(rgba_array, mode="RGBA")


def _border_connected_components(mask: np.ndarray) -> np.ndarray:
    if not bool(mask.any()):
        return np.zeros_like(mask, dtype=bool)

    labels_count, labels = cv2.connectedComponents(mask.astype(np.uint8), connectivity=8)
    if labels_count <= 1:
        return mask

    border_labels = np.unique(np.concatenate([labels[0, :], labels[-1, :], labels[:, 0], labels[:, -1]]))
    result = np.zeros_like(mask, dtype=bool)
    for label in border_labels:
        if label <= 0:
            continue
        result[labels == label] = True
    return result


def _remove_small_components(mask: np.ndarray, min_component_area: int) -> np.ndarray:
    if min_component_area <= 1:
        return mask
    if not bool(mask.any()):
        return mask

    labels_count, labels, stats, _ = cv2.connectedComponentsWithStats(mask.astype(np.uint8), connectivity=8)
    if labels_count <= 1:
        return mask

    filtered = np.zeros_like(mask, dtype=bool)
    for label in range(1, labels_count):
        if int(stats[label, cv2.CC_STAT_AREA]) < min_component_area:
            continue
        filtered[labels == label] = True
    return filtered
