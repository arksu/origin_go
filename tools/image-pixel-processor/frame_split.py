from __future__ import annotations

import logging
import cv2
import numpy as np
from PIL import Image
from dataclasses import dataclass

MIN_AXIS_PIXEL_RATIO = 0.006
EMPTY_BAND_SIGNAL_RATIO = 0.04
EMPTY_BAND_INTERRUPT_MAX = 2
ROW_EMPTY_BAND_MIN_RATIO = 0.04
COL_EMPTY_BAND_MIN_RATIO = 0.015
ROW_EMPTY_BAND_MIN_ABS = 20
COL_EMPTY_BAND_MIN_ABS = 8
SEGMENT_MASS_KEEP_RATIO = 0.2
SEGMENT_LENGTH_KEEP_RATIO = 0.55
MAX_GRID_FRAMES_PER_AXIS = 7
BRIGHT_SPOT_MAX_DOWNSCALE_DIM = 320
BRIGHT_SPOT_MIN_DOWNSCALE_DIM = 96

logger = logging.getLogger(__name__)


@dataclass(frozen=True)
class SplitResult:
    frames: list[Image.Image]
    row_segments: list[tuple[int, int]]
    col_segments: list[tuple[int, int]]


def split_frames(image: Image.Image, min_frame_area: int) -> list[Image.Image]:
    result = split_frames_with_meta(image=image, min_frame_area=min_frame_area)
    return result.frames


def split_frames_with_meta(image: Image.Image, min_frame_area: int) -> SplitResult:
    if min_frame_area <= 0:
        raise ValueError("min_frame_area must be > 0")

    rgba_image = image.convert("RGBA")
    non_empty_mask = _extract_foreground_mask(rgba_image)
    denoise_component_area = max(1, min_frame_area // 2)
    clean_mask = _remove_small_components(non_empty_mask, min_component_area=denoise_component_area)
    if bool(clean_mask.any()):
        non_empty_mask = clean_mask

    if not bool(non_empty_mask.any()):
        raise ValueError("image has no non-transparent pixels after preprocessing")

    min_y, min_x = np.argwhere(non_empty_mask).min(axis=0)
    max_y, max_x = np.argwhere(non_empty_mask).max(axis=0)
    content_mask = non_empty_mask[min_y : max_y + 1, min_x : max_x + 1]

    row_segments, col_segments = _detect_grid_segments(
        content_mask,
        min_frame_area=max(1, min_frame_area),
    )
    if not row_segments or not col_segments:
        fallback_frame = _trim_transparent_edges(rgba_image)
        return SplitResult(frames=[fallback_frame], row_segments=[(int(min_y), int(max_y))], col_segments=[(int(min_x), int(max_x))])

    frames: list[Image.Image] = []
    for row_start, row_end in row_segments:
        for col_start, col_end in col_segments:
            cell_mask = content_mask[row_start : row_end + 1, col_start : col_end + 1]
            if not bool(cell_mask.any()):
                continue

            abs_left = int(min_x + col_start)
            abs_top = int(min_y + row_start)
            abs_right = int(min_x + col_end) + 1
            abs_bottom = int(min_y + row_end) + 1

            cell_image = rgba_image.crop((abs_left, abs_top, abs_right, abs_bottom))
            trimmed = _trim_transparent_edges(cell_image, min_component_area=denoise_component_area)
            if _count_non_transparent_pixels(trimmed) < min_frame_area:
                continue
            frames.append(trimmed)

    if not frames:
        fallback_frame = _trim_transparent_edges(rgba_image)
        return SplitResult(frames=[fallback_frame], row_segments=row_segments, col_segments=col_segments)

    return SplitResult(frames=frames, row_segments=row_segments, col_segments=col_segments)


def _detect_grid_segments(
    mask: np.ndarray,
    min_frame_area: int,
) -> tuple[list[tuple[int, int]], list[tuple[int, int]]]:
    height, width = mask.shape
    row_signal = np.count_nonzero(mask, axis=1)
    col_signal = np.count_nonzero(mask, axis=0)

    row_bright_segments = _segments_from_bright_spots(mask, axis="y")
    col_bright_segments = _segments_from_bright_spots(mask, axis="x")

    row_empty_segments = _segments_from_empty_bands(
        row_signal,
        axis_size=height,
        orthogonal_size=width,
        min_empty_band=max(ROW_EMPTY_BAND_MIN_ABS, int(round(height * ROW_EMPTY_BAND_MIN_RATIO))),
    )
    col_empty_segments = _segments_from_empty_bands(
        col_signal,
        axis_size=width,
        orthogonal_size=height,
        min_empty_band=max(COL_EMPTY_BAND_MIN_ABS, int(round(width * COL_EMPTY_BAND_MIN_RATIO))),
    )

    row_peak_segments = _segments_from_projection_peaks(row_signal, axis_size=height)
    col_peak_segments = _segments_from_projection_peaks(col_signal, axis_size=width)

    component_rows = _segments_from_components(mask, axis="y", min_frame_area=min_frame_area)
    component_cols = _segments_from_components(mask, axis="x", min_frame_area=min_frame_area)

    row_segments = _select_axis_segments(
        primary=row_bright_segments,
        alternatives=[row_empty_segments, row_peak_segments, component_rows],
        axis_size=height,
        signal=row_signal,
    )
    col_segments = _select_axis_segments(
        primary=col_bright_segments,
        alternatives=[col_empty_segments, col_peak_segments, component_cols],
        axis_size=width,
        signal=col_signal,
    )

    logger.debug(
        "grid_detect h=%d w=%d rows(bright=%d empty=%d peak=%d comp=%d final=%d) cols(bright=%d empty=%d peak=%d comp=%d final=%d)",
        height,
        width,
        len(row_bright_segments),
        len(row_empty_segments),
        len(row_peak_segments),
        len(component_rows),
        len(row_segments),
        len(col_bright_segments),
        len(col_empty_segments),
        len(col_peak_segments),
        len(component_cols),
        len(col_segments),
    )
    logger.debug(
        "grid_candidates row_bright=%s row_empty=%s row_peak=%s row_comp=%s col_bright=%s col_empty=%s col_peak=%s col_comp=%s",
        row_bright_segments,
        row_empty_segments,
        row_peak_segments,
        component_rows,
        col_bright_segments,
        col_empty_segments,
        col_peak_segments,
        component_cols,
    )
    logger.debug("grid_segments rows=%s cols=%s", row_segments, col_segments)

    return row_segments, col_segments


def _segments_from_empty_bands(
    signal: np.ndarray,
    axis_size: int,
    orthogonal_size: int,
    min_empty_band: int,
) -> list[tuple[int, int]]:
    non_zero_indices = np.flatnonzero(signal > 0)
    if non_zero_indices.size == 0:
        return []

    start_index = int(non_zero_indices[0])
    end_index = int(non_zero_indices[-1])
    cropped_signal = signal[start_index : end_index + 1]
    non_zero_values = cropped_signal[cropped_signal > 0]
    if non_zero_values.size == 0:
        return [(start_index, end_index)]

    base_signal = float(np.percentile(non_zero_values, 50))
    dynamic_empty_limit = int(round(base_signal * EMPTY_BAND_SIGNAL_RATIO))
    static_empty_limit = int(round(orthogonal_size * MIN_AXIS_PIXEL_RATIO))
    empty_limit = max(1, dynamic_empty_limit, static_empty_limit)

    empty_mask = cropped_signal <= empty_limit
    empty_mask = _fill_short_false_runs(empty_mask, max_false_run=EMPTY_BAND_INTERRUPT_MAX)

    min_empty_band = max(1, min_empty_band)
    empty_bands = [run for run in _extract_runs(empty_mask) if run[1] - run[0] + 1 >= min_empty_band]

    if not empty_bands:
        return [(start_index, end_index)]

    separators = [int((start + end) // 2) for start, end in empty_bands]

    segments: list[tuple[int, int]] = []
    previous = 0
    for separator in separators:
        seg_start = previous
        seg_end = separator - 1
        if seg_end >= seg_start:
            segments.append((start_index + seg_start, start_index + seg_end))
        previous = separator + 1

    if previous <= len(cropped_signal) - 1:
        segments.append((start_index + previous, end_index))

    if not segments:
        return [(start_index, end_index)]

    by_mass = _drop_small_segment_masses(segments, signal)
    return _drop_short_segments(by_mass)


def _segments_from_bright_spots(mask: np.ndarray, axis: str) -> list[tuple[int, int]]:
    if axis not in ("x", "y"):
        raise ValueError("axis must be 'x' or 'y'")
    if not bool(mask.any()):
        return []

    height, width = mask.shape
    max_dim = max(height, width)
    scale = max(1.0, max_dim / BRIGHT_SPOT_MAX_DOWNSCALE_DIM)
    down_w = max(BRIGHT_SPOT_MIN_DOWNSCALE_DIM, int(round(width / scale)))
    down_h = max(BRIGHT_SPOT_MIN_DOWNSCALE_DIM, int(round(height / scale)))
    down_w = min(down_w, width)
    down_h = min(down_h, height)

    reduced = cv2.resize(mask.astype(np.float32), (down_w, down_h), interpolation=cv2.INTER_AREA)
    sigma_x = max(1.0, down_w / (MAX_GRID_FRAMES_PER_AXIS * 3.2))
    sigma_y = max(1.0, down_h / (MAX_GRID_FRAMES_PER_AXIS * 3.2))
    heat = cv2.GaussianBlur(reduced, (0, 0), sigmaX=sigma_x, sigmaY=sigma_y)

    profile = heat.sum(axis=0 if axis == "x" else 1)
    peaks = _detect_profile_peaks(profile, max_frames=MAX_GRID_FRAMES_PER_AXIS)
    if len(peaks) <= 1:
        return []

    down_segments = _segments_from_peak_centers(peaks, profile_length=len(profile))
    if len(down_segments) <= 1:
        return []

    ratio = (width / down_w) if axis == "x" else (height / down_h)
    upscaled_segments = _rescale_segments(down_segments, ratio=ratio, axis_size=(width if axis == "x" else height))
    return _drop_short_segments(upscaled_segments)


def _detect_profile_peaks(profile: np.ndarray, max_frames: int) -> list[int]:
    if profile.size == 0:
        return []

    baseline = float(np.percentile(profile, 55))
    peak_threshold = baseline + (float(profile.max()) - baseline) * 0.22
    candidates: list[int] = []
    for index in range(1, len(profile) - 1):
        value = profile[index]
        if value < peak_threshold:
            continue
        if value >= profile[index - 1] and value >= profile[index + 1] and (
            value > profile[index - 1] or value > profile[index + 1]
        ):
            candidates.append(index)

    if not candidates:
        return [int(np.argmax(profile))]

    min_distance = max(6, len(profile) // (max_frames + 2))
    selected = _merge_peak_indices(candidates, profile, min_distance=min_distance)

    if len(selected) > max_frames:
        selected = sorted(selected, key=lambda idx: profile[idx], reverse=True)[:max_frames]
        selected.sort()

    return selected


def _segments_from_peak_centers(peaks: list[int], profile_length: int) -> list[tuple[int, int]]:
    if not peaks:
        return []
    if len(peaks) == 1:
        return [(0, profile_length - 1)]

    separators: list[int] = []
    for left, right in zip(peaks, peaks[1:]):
        separators.append((left + right) // 2)

    segments: list[tuple[int, int]] = []
    start = 0
    for separator in separators:
        end = separator
        if end >= start:
            segments.append((start, end))
        start = separator + 1
    if start <= profile_length - 1:
        segments.append((start, profile_length - 1))
    return segments


def _rescale_segments(
    segments: list[tuple[int, int]],
    ratio: float,
    axis_size: int,
) -> list[tuple[int, int]]:
    rescaled: list[tuple[int, int]] = []
    for start, end in segments:
        up_start = int(round(start * ratio))
        up_end = int(round((end + 1) * ratio)) - 1
        up_start = max(0, min(axis_size - 1, up_start))
        up_end = max(0, min(axis_size - 1, up_end))
        if up_end >= up_start:
            rescaled.append((up_start, up_end))
    if not rescaled:
        return []
    rescaled.sort(key=lambda item: item[0])
    normalized: list[tuple[int, int]] = [rescaled[0]]
    for start, end in rescaled[1:]:
        prev_start, prev_end = normalized[-1]
        if start <= prev_end:
            start = prev_end + 1
        if end < start:
            continue
        normalized.append((start, end))
    return normalized


def _select_axis_segments(
    primary: list[tuple[int, int]],
    alternatives: list[list[tuple[int, int]]],
    axis_size: int,
    signal: np.ndarray,
) -> list[tuple[int, int]]:
    clipped_primary = _clip_segments(primary, axis_size=axis_size)
    candidate_sets: list[list[tuple[int, int]]] = [clipped_primary]
    for candidate in alternatives:
        candidate_sets.append(_clip_segments(candidate, axis_size=axis_size))

    multi_segment_candidates = [candidate for candidate in candidate_sets if len(candidate) > 1]
    if not multi_segment_candidates:
        return [(0, axis_size - 1)]

    preferred_count = len(clipped_primary) if len(clipped_primary) > 1 else None

    best_candidate = multi_segment_candidates[0]
    best_score = _score_axis_candidate(
        best_candidate,
        signal=signal,
        axis_size=axis_size,
        preferred_count=preferred_count,
    )
    for candidate in multi_segment_candidates[1:]:
        candidate_score = _score_axis_candidate(
            candidate,
            signal=signal,
            axis_size=axis_size,
            preferred_count=preferred_count,
        )
        if candidate_score > best_score:
            best_candidate = candidate
            best_score = candidate_score

    return best_candidate


def _score_axis_candidate(
    segments: list[tuple[int, int]],
    signal: np.ndarray,
    axis_size: int,
    preferred_count: int | None,
) -> float:
    if len(segments) <= 1:
        return float("-inf")

    separator_score = _separator_emptiness_score(segments, signal=signal, axis_size=axis_size)

    lengths = np.asarray([end - start + 1 for start, end in segments], dtype=np.float64)
    mean_length = float(lengths.mean()) if lengths.size else 0.0
    if mean_length <= 0.0:
        balance_score = 0.0
    else:
        variation = float(lengths.std() / mean_length)
        balance_score = max(0.0, 1.0 - variation)

    count_penalty = 0.0
    if preferred_count is not None:
        count_penalty = abs(len(segments) - preferred_count) / float(max(1, preferred_count))

    return (separator_score * 0.78) + (balance_score * 0.22) - (count_penalty * 0.35)


def _separator_emptiness_score(
    segments: list[tuple[int, int]],
    signal: np.ndarray,
    axis_size: int,
) -> float:
    if len(segments) <= 1 or signal.size == 0:
        return 0.0

    max_signal = float(np.max(signal))
    if max_signal <= 0.0:
        return 1.0

    probe_radius = max(2, axis_size // 220)
    separator_scores: list[float] = []
    for left_segment, right_segment in zip(segments, segments[1:]):
        boundary = int((left_segment[1] + right_segment[0]) // 2)
        left = max(0, boundary - probe_radius)
        right = min(len(signal) - 1, boundary + probe_radius)
        local_signal = float(np.mean(signal[left : right + 1]))
        normalized = 1.0 - min(1.0, local_signal / max_signal)
        separator_scores.append(normalized)

    if not separator_scores:
        return 0.0
    return float(np.mean(np.asarray(separator_scores, dtype=np.float64)))


def _clip_segments(segments: list[tuple[int, int]], axis_size: int) -> list[tuple[int, int]]:
    clipped: list[tuple[int, int]] = []
    for start, end in segments:
        start = max(0, min(axis_size - 1, start))
        end = max(0, min(axis_size - 1, end))
        if end >= start:
            clipped.append((start, end))
    return clipped or [(0, axis_size - 1)]


def _segments_from_projection_peaks(signal: np.ndarray, axis_size: int) -> list[tuple[int, int]]:
    non_zero_indices = np.flatnonzero(signal > 0)
    if non_zero_indices.size == 0:
        return []

    start_index = int(non_zero_indices[0])
    end_index = int(non_zero_indices[-1])
    cropped_signal = signal[start_index : end_index + 1].astype(np.float64)
    smoothed_signal = _smooth_1d(cropped_signal, window=max(11, axis_size // 80))

    threshold = float(np.percentile(smoothed_signal, 60))
    raw_peaks: list[int] = []
    for index in range(1, len(smoothed_signal) - 1):
        value = smoothed_signal[index]
        if value < threshold:
            continue
        left = smoothed_signal[index - 1]
        right = smoothed_signal[index + 1]
        if value >= left and value >= right and (value > left or value > right):
            raw_peaks.append(index)

    merged_peaks = _merge_peak_indices(raw_peaks, smoothed_signal, min_distance=max(10, axis_size // 10))
    if len(merged_peaks) <= 1:
        return []

    separators: list[int] = []
    for left_peak, right_peak in zip(merged_peaks, merged_peaks[1:]):
        if right_peak <= left_peak:
            continue
        valley_offset = int(np.argmin(smoothed_signal[left_peak : right_peak + 1]))
        separators.append(left_peak + valley_offset)

    if not separators:
        return []

    segments: list[tuple[int, int]] = []
    previous = 0
    for separator in separators:
        seg_start = previous
        seg_end = separator - 1
        if seg_end >= seg_start:
            segments.append((start_index + seg_start, start_index + seg_end))
        previous = separator + 1

    if previous <= len(cropped_signal) - 1:
        segments.append((start_index + previous, end_index))

    if len(segments) <= 1:
        return []

    by_mass = _drop_small_segment_masses(segments, signal)
    return _drop_short_segments(by_mass)


def _smooth_1d(signal: np.ndarray, window: int) -> np.ndarray:
    if window <= 1:
        return signal
    if window % 2 == 0:
        window += 1
    kernel = np.ones(window, dtype=np.float64) / float(window)
    padding = window // 2
    padded = np.pad(signal, (padding, padding), mode="edge")
    smoothed = np.convolve(padded, kernel, mode="valid")
    return smoothed[: len(signal)]


def _merge_peak_indices(peaks: list[int], signal: np.ndarray, min_distance: int) -> list[int]:
    if not peaks:
        return []

    merged: list[int] = [peaks[0]]
    for peak in peaks[1:]:
        last = merged[-1]
        if peak - last <= min_distance:
            if signal[peak] > signal[last]:
                merged[-1] = peak
            continue
        merged.append(peak)
    return merged


def _drop_small_segment_masses(
    segments: list[tuple[int, int]],
    signal: np.ndarray,
) -> list[tuple[int, int]]:
    if len(segments) <= 1:
        return segments

    masses = [int(signal[start : end + 1].sum()) for start, end in segments]
    max_mass = max(masses) if masses else 0
    if max_mass <= 0:
        return segments

    filtered: list[tuple[int, int]] = []
    for (segment, mass) in zip(segments, masses):
        if mass >= max(1, int(round(max_mass * SEGMENT_MASS_KEEP_RATIO))):
            filtered.append(segment)
    return filtered or segments


def _drop_short_segments(segments: list[tuple[int, int]]) -> list[tuple[int, int]]:
    if len(segments) <= 1:
        return segments

    lengths = [end - start + 1 for start, end in segments]
    median_length = float(np.median(np.asarray(lengths, dtype=np.float64)))
    min_length = max(1, int(round(median_length * SEGMENT_LENGTH_KEEP_RATIO)))

    filtered = [segment for segment, length in zip(segments, lengths) if length >= min_length]
    return filtered or segments


def _fill_short_false_runs(mask: np.ndarray, max_false_run: int) -> np.ndarray:
    if max_false_run <= 0:
        return mask

    result = mask.copy()
    false_start = None
    for index, value in enumerate(result):
        if not value and false_start is None:
            false_start = index
            continue

        if value and false_start is not None:
            false_end = index - 1
            false_len = false_end - false_start + 1
            left_is_true = false_start > 0 and result[false_start - 1]
            right_is_true = index < len(result) and result[index]
            if left_is_true and right_is_true and false_len <= max_false_run:
                result[false_start : false_end + 1] = True
            false_start = None

    return result


def _extract_runs(mask: np.ndarray) -> list[tuple[int, int]]:
    runs: list[tuple[int, int]] = []
    run_start = None
    for index, value in enumerate(mask):
        if value and run_start is None:
            run_start = index
            continue
        if not value and run_start is not None:
            runs.append((run_start, index - 1))
            run_start = None
    if run_start is not None:
        runs.append((run_start, len(mask) - 1))
    return runs


def _segments_from_components(
    mask: np.ndarray,
    axis: str,
    min_frame_area: int,
) -> list[tuple[int, int]]:
    if axis not in ("x", "y"):
        raise ValueError("axis must be 'x' or 'y'")
    if not bool(mask.any()):
        return []

    height, width = mask.shape
    opened = cv2.morphologyEx(mask.astype(np.uint8), cv2.MORPH_OPEN, np.ones((3, 3), dtype=np.uint8))
    close_kernel = _build_close_kernel(height, width, axis)
    merged = cv2.morphologyEx(opened, cv2.MORPH_CLOSE, close_kernel)

    labels_count, labels, stats, _ = cv2.connectedComponentsWithStats(merged, connectivity=8)
    if labels_count <= 1:
        return []

    component_boxes: list[tuple[int, int, int, int, int]] = []
    for label in range(1, labels_count):
        area = int(stats[label, cv2.CC_STAT_AREA])

        left = int(stats[label, cv2.CC_STAT_LEFT])
        top = int(stats[label, cv2.CC_STAT_TOP])
        comp_width = int(stats[label, cv2.CC_STAT_WIDTH])
        comp_height = int(stats[label, cv2.CC_STAT_HEIGHT])
        right = left + comp_width - 1
        bottom = top + comp_height - 1

        component_boxes.append((left, top, right, bottom, area))

    if not component_boxes:
        return []

    max_area = max(box[4] for box in component_boxes)
    area_floor = max(min_frame_area * 4, int(round(max_area * 0.12)), int(round((height * width) * 0.0005)))
    major_boxes = [box for box in component_boxes if box[4] >= area_floor]
    if not major_boxes:
        major_boxes = [max(component_boxes, key=lambda item: item[4])]

    if axis == "x":
        projections = [(box[0], box[2]) for box in major_boxes]
    else:
        projections = [(box[1], box[3]) for box in major_boxes]

    if not projections:
        return []

    return _cluster_ranges_by_centers(
        projections,
        axis_size=(width if axis == "x" else height),
    )


def _build_close_kernel(height: int, width: int, axis: str) -> np.ndarray:
    if axis == "x":
        kernel_width = max(5, width // 180)
        kernel_height = max(3, height // 240)
    else:
        kernel_width = max(3, width // 240)
        kernel_height = max(5, height // 180)
    return np.ones((kernel_height, kernel_width), dtype=np.uint8)


def _merge_near_ranges(ranges: list[tuple[int, int]], join_gap: int) -> list[tuple[int, int]]:
    if not ranges:
        return []
    merged: list[tuple[int, int]] = []
    current_start, current_end = ranges[0]
    for start, end in ranges[1:]:
        if start - current_end <= join_gap:
            current_end = max(current_end, end)
            continue
        merged.append((current_start, current_end))
        current_start, current_end = start, end
    merged.append((current_start, current_end))
    return merged


def _cluster_ranges_by_centers(ranges: list[tuple[int, int]], axis_size: int) -> list[tuple[int, int]]:
    if not ranges:
        return []

    centers = [((start + end) * 0.5, start, end, end - start + 1) for start, end in ranges]
    centers.sort(key=lambda item: item[0])
    median_span = float(np.median(np.asarray([item[3] for item in centers], dtype=np.float64)))
    gap_threshold = max(8.0, median_span * 0.65, axis_size * 0.03)

    clusters: list[list[tuple[float, int, int, int]]] = [[centers[0]]]
    for item in centers[1:]:
        previous_center = clusters[-1][-1][0]
        if item[0] - previous_center > gap_threshold:
            clusters.append([item])
        else:
            clusters[-1].append(item)

    merged_ranges: list[tuple[int, int]] = []
    for cluster in clusters:
        start = min(item[1] for item in cluster)
        end = max(item[2] for item in cluster)
        merged_ranges.append((start, end))

    return _merge_near_ranges(merged_ranges, join_gap=max(2, axis_size // 150))


def _trim_transparent_edges(image: Image.Image, min_component_area: int = 1) -> Image.Image:
    rgba_image = image.convert("RGBA")
    alpha_channel = np.asarray(rgba_image.getchannel("A"), dtype=np.uint8)
    raw_mask = alpha_channel > 0
    clean_mask = _remove_small_components(raw_mask, min_component_area=max(1, min_component_area))
    positions = np.argwhere(clean_mask)
    if positions.size == 0:
        return rgba_image

    min_y, min_x = positions.min(axis=0)
    max_y, max_x = positions.max(axis=0)
    return rgba_image.crop((int(min_x), int(min_y), int(max_x) + 1, int(max_y) + 1))


def _count_non_transparent_pixels(image: Image.Image) -> int:
    alpha_channel = np.asarray(image.convert("RGBA").getchannel("A"), dtype=np.uint8)
    return int(np.count_nonzero(alpha_channel > 0))


def _extract_foreground_mask(image: Image.Image) -> np.ndarray:
    alpha_channel = np.asarray(image.getchannel("A"), dtype=np.uint8)
    alpha_mask = alpha_channel > 0
    if not bool(alpha_mask.any()):
        return alpha_mask

    coverage = float(np.count_nonzero(alpha_mask)) / float(alpha_mask.size)
    if coverage < 0.9:
        return alpha_mask

    rgb = np.asarray(image.convert("RGB"), dtype=np.int16)
    background_mask = _estimate_edge_connected_background(rgb)
    foreground_mask = (~background_mask) & alpha_mask
    if bool(foreground_mask.any()):
        return foreground_mask

    return alpha_mask


def _estimate_edge_connected_background(rgb: np.ndarray) -> np.ndarray:
    height, width = rgb.shape[:2]
    if height == 0 or width == 0:
        return np.zeros((height, width), dtype=bool)

    corner_and_edge_samples = np.array(
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
    background_color = np.median(corner_and_edge_samples, axis=0)

    diff = rgb.astype(np.float32) - background_color.astype(np.float32)
    distance = np.sqrt(np.sum(diff * diff, axis=2))
    edge_distance = np.concatenate([distance[0, :], distance[height - 1, :], distance[:, 0], distance[:, width - 1]])
    edge_noise_floor = float(np.percentile(edge_distance, 75))
    threshold = max(8.0, edge_noise_floor + 6.0)

    candidate_background = distance <= threshold
    return _border_connected_components(candidate_background)


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

    filtered_mask = np.zeros_like(mask, dtype=bool)
    for label in range(1, labels_count):
        area = int(stats[label, cv2.CC_STAT_AREA])
        if area < min_component_area:
            continue
        filtered_mask[labels == label] = True

    return filtered_mask
