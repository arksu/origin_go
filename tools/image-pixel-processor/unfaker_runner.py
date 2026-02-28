from __future__ import annotations

import shlex
import subprocess
import tempfile
from dataclasses import dataclass
import logging
from pathlib import Path

import numpy as np
from PIL import Image

logger = logging.getLogger(__name__)


@dataclass(frozen=True)
class UnfakerResult:
    output_path: Path
    stdout: str
    stderr: str


class UnfakerError(RuntimeError):
    pass


def run_unfaker(
    frame: Image.Image,
    unfaker_command: str,
    temp_dir: Path,
    timeout_seconds: int,
) -> UnfakerResult:
    if not unfaker_command.strip():
        raise ValueError("unfaker_command is required")

    temp_dir.mkdir(parents=True, exist_ok=True)

    with tempfile.NamedTemporaryFile(suffix=".png", dir=temp_dir, delete=False) as input_file:
        input_path = Path(input_file.name)
    with tempfile.NamedTemporaryFile(suffix=".png", dir=temp_dir, delete=False) as output_file:
        output_path = Path(output_file.name)

    prepared_input = _prepare_frame_for_unfaker(frame)
    prepared_input.save(input_path, format="PNG")
    output_path.unlink(missing_ok=True)

    command = _build_command(unfaker_command=unfaker_command, input_path=input_path, output_path=output_path)
    command = _ensure_unfaker_default_scale(command)

    try:
        completed = subprocess.run(
            command,
            capture_output=True,
            text=True,
            check=False,
            timeout=timeout_seconds,
        )
    except subprocess.TimeoutExpired as exc:
        output_path.unlink(missing_ok=True)
        raise UnfakerError(f"Unfaker timeout after {timeout_seconds}s: {exc}") from exc
    finally:
        input_path.unlink(missing_ok=True)

    if completed.returncode != 0:
        output_path.unlink(missing_ok=True)
        raise UnfakerError(
            "Unfaker failed with non-zero exit code "
            f"{completed.returncode}. stdout={completed.stdout!r} stderr={completed.stderr!r}"
        )
    if not output_path.exists():
        raise UnfakerError(
            "Unfaker finished but did not produce an output file. "
            "Use placeholders {input}/{output} or positional input output arguments."
        )

    return UnfakerResult(output_path=output_path, stdout=completed.stdout, stderr=completed.stderr)


def _build_command(unfaker_command: str, input_path: Path, output_path: Path) -> list[str]:
    if "{input}" in unfaker_command or "{output}" in unfaker_command:
        command = unfaker_command.format(input=str(input_path), output=str(output_path))
        return shlex.split(command)

    tokens = shlex.split(unfaker_command)
    tokens.extend([str(input_path), str(output_path)])
    return tokens


def _ensure_unfaker_default_scale(command: list[str]) -> list[str]:
    if not command:
        return command
    if not _looks_like_unfaker_command(command):
        return command
    if _has_explicit_scale_flag(command):
        return command

    with_scale = list(command)
    with_scale.extend(["--scale", "1"])
    logger.debug("event=unfaker_scale_default_applied scale=1 command=%s", with_scale)
    return with_scale


def _looks_like_unfaker_command(command: list[str]) -> bool:
    joined = " ".join(command).lower()
    return "unfaker.py" in joined or "unfake.py" in joined


def _has_explicit_scale_flag(command: list[str]) -> bool:
    for token in command:
        if token == "--scale" or token == "-s":
            return True
        if token.startswith("--scale="):
            return True
    return False


def _prepare_frame_for_unfaker(frame: Image.Image) -> Image.Image:
    """Normalize RGBA before Unfaker.

    Why: rembg often returns soft alpha masks. Unfaker's default alpha threshold is 128,
    which can erase objects entirely when alpha values are low. We de-premultiply RGB from
    alpha and convert alpha to a hard mask to preserve visible sprites.
    """
    rgba = frame.convert("RGBA")
    rgba_array = np.asarray(rgba, dtype=np.uint16).copy()

    alpha = rgba_array[:, :, 3]
    non_zero_alpha = alpha > 0
    if not bool(non_zero_alpha.any()):
        return rgba

    for channel_index in range(3):
        channel = rgba_array[:, :, channel_index]
        channel[non_zero_alpha] = np.minimum(
            255,
            (channel[non_zero_alpha] * 255 + (alpha[non_zero_alpha] // 2)) // alpha[non_zero_alpha],
        )
        rgba_array[:, :, channel_index] = channel

    rgba_array[:, :, 3] = np.where(non_zero_alpha, 255, 0)
    return Image.fromarray(rgba_array.astype(np.uint8), mode="RGBA")
