from __future__ import annotations

import shlex
import subprocess
import tempfile
from dataclasses import dataclass
from pathlib import Path

from PIL import Image


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

    frame.convert("RGBA").save(input_path, format="PNG")
    output_path.unlink(missing_ok=True)

    command = _build_command(unfaker_command=unfaker_command, input_path=input_path, output_path=output_path)

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
