from __future__ import annotations

import logging
from dataclasses import dataclass
from pathlib import Path
from typing import Iterable

import numpy as np
from PIL import Image

from bg_remove import remove_background
from frame_split import split_frames_with_meta
from io_safe import atomic_save_png, build_output_path, safe_delete
from transparency import has_meaningful_transparency
from unfaker_runner import run_unfaker

logger = logging.getLogger(__name__)

STAGE_REMOVE_BACKGROUND = "remove_background"
STAGE_FRAME_SPLIT = "frame_split"
STAGE_UNFAKER = "unfaker"
SUPPORTED_STAGES = (STAGE_REMOVE_BACKGROUND, STAGE_FRAME_SPLIT, STAGE_UNFAKER)


@dataclass(frozen=True)
class PipelineConfig:
    output_dir: Path
    temp_dir: Path
    unfaker_command: str
    background_model: str
    alpha_ratio_threshold: float
    alpha_value_threshold: int
    min_frame_area: int
    unfaker_timeout_seconds: int
    stages: tuple[str, ...] = SUPPORTED_STAGES


class PixelPipeline:
    def __init__(self, config: PipelineConfig) -> None:
        self._config = config
        self._validate_stages(config.stages)

    def process_file(self, input_file: Path) -> list[Path]:
        with Image.open(input_file) as loaded_image:
            source_image = loaded_image.convert("RGBA")

        images: list[Image.Image] = [source_image]
        stage_sequence = self._config.stages
        logger.info("event=pipeline_stages file=%s stages=%s", input_file, ",".join(stage_sequence))

        output_paths: list[Path] = []
        temporary_outputs: list[Path] = []
        base_name = input_file.stem

        try:
            for stage in stage_sequence:
                if stage == STAGE_REMOVE_BACKGROUND:
                    images = [self._apply_remove_background(input_file=input_file, image=image) for image in images]
                    logger.info("event=stage_done file=%s stage=%s images=%d", input_file, stage, len(images))
                    continue

                if stage == STAGE_FRAME_SPLIT:
                    split_images: list[Image.Image] = []
                    for image in images:
                        split_result = split_frames_with_meta(image, min_frame_area=self._config.min_frame_area)
                        split_images.extend(split_result.frames)
                        logger.info(
                            "event=split_done file=%s frames=%d rows=%d cols=%d",
                            input_file,
                            len(split_result.frames),
                            len(split_result.row_segments),
                            len(split_result.col_segments),
                        )
                    images = split_images
                    logger.info("event=stage_done file=%s stage=%s images=%d", input_file, stage, len(images))
                    continue

                if stage == STAGE_UNFAKER:
                    images = self._apply_unfaker(
                        input_file=input_file,
                        images=images,
                        temporary_outputs=temporary_outputs,
                    )
                    logger.info("event=stage_done file=%s stage=%s images=%d", input_file, stage, len(images))
                    continue

                raise ValueError(f"Unsupported stage: {stage}")

            for index, image in enumerate(images, start=1):
                destination = build_output_path(self._config.output_dir, base_name=base_name, frame_index=index)
                atomic_save_png(image.convert("RGBA"), destination)
                output_paths.append(destination)
                logger.info("event=output_saved file=%s frame=%d output=%s", input_file, index, destination)

            safe_delete(input_file)
            logger.info("event=file_done file=%s outputs=%d", input_file, len(output_paths))
            return output_paths
        finally:
            for temporary_output in temporary_outputs:
                safe_delete(temporary_output)

    @staticmethod
    def _validate_stages(stages: Iterable[str]) -> None:
        stage_list = list(stages)
        if not stage_list:
            raise ValueError("stages must contain at least one stage")
        for stage in stage_list:
            if stage not in SUPPORTED_STAGES:
                raise ValueError(f"Unsupported stage: {stage}")

    def _apply_remove_background(self, input_file: Path, image: Image.Image) -> Image.Image:
        should_skip_bg_removal = has_meaningful_transparency(
            image,
            alpha_ratio_threshold=self._config.alpha_ratio_threshold,
            alpha_value_threshold=self._config.alpha_value_threshold,
        )
        if should_skip_bg_removal:
            logger.info("event=bg_skip file=%s reason=already_transparent", input_file)
            return image

        logger.info("event=bg_removed file=%s", input_file)
        return remove_background(image, model_name=self._config.background_model)

    def _apply_unfaker(
        self,
        input_file: Path,
        images: list[Image.Image],
        temporary_outputs: list[Path],
    ) -> list[Image.Image]:
        processed_images: list[Image.Image] = []
        for index, frame in enumerate(images, start=1):
            source_metrics = _image_metrics(frame)
            unfaker_result = run_unfaker(
                frame=frame,
                unfaker_command=self._config.unfaker_command,
                temp_dir=self._config.temp_dir,
                timeout_seconds=self._config.unfaker_timeout_seconds,
            )
            temporary_outputs.append(unfaker_result.output_path)

            with Image.open(unfaker_result.output_path) as out_image:
                result_image = out_image.convert("RGBA")
            output_metrics = _image_metrics(result_image)

            if _is_degraded_output(source=source_metrics, output=output_metrics):
                logger.warning(
                    "event=unfaker_degraded_output file=%s frame=%d "
                    "source_size=%dx%d source_non_transparent=%d "
                    "output_size=%dx%d output_non_transparent=%d action=fallback_to_source_frame",
                    input_file,
                    index,
                    source_metrics.width,
                    source_metrics.height,
                    source_metrics.non_transparent_pixels,
                    output_metrics.width,
                    output_metrics.height,
                    output_metrics.non_transparent_pixels,
                )
                result_image = frame.convert("RGBA")

            processed_images.append(result_image)
            logger.info("event=unfaker_done file=%s frame=%d", input_file, index)

        return processed_images


@dataclass(frozen=True)
class ImageMetrics:
    width: int
    height: int
    total_pixels: int
    non_transparent_pixels: int


def _image_metrics(image: Image.Image) -> ImageMetrics:
    rgba_image = image.convert("RGBA")
    alpha_channel = np.asarray(rgba_image.getchannel("A"), dtype=np.uint8)
    non_transparent_pixels = int(np.count_nonzero(alpha_channel > 0))
    height, width = alpha_channel.shape
    return ImageMetrics(
        width=int(width),
        height=int(height),
        total_pixels=int(alpha_channel.size),
        non_transparent_pixels=non_transparent_pixels,
    )


def _is_degraded_output(source: ImageMetrics, output: ImageMetrics) -> bool:
    if output.non_transparent_pixels <= 0:
        return True

    if source.total_pixels <= 0 or output.total_pixels <= 0:
        return True

    source_coverage = source.non_transparent_pixels / float(source.total_pixels)
    output_coverage = output.non_transparent_pixels / float(output.total_pixels)
    expected_non_transparent = source.non_transparent_pixels * (
        output.total_pixels / float(source.total_pixels)
    )

    if output.non_transparent_pixels < max(8, int(round(expected_non_transparent * 0.18))):
        return True

    if output_coverage < max(0.015, source_coverage * 0.12):
        return True

    return False
