from __future__ import annotations

import json
import os
import uuid
from pathlib import Path
from typing import TYPE_CHECKING, Iterable

if TYPE_CHECKING:
    from PIL import Image

HIDDEN_PREFIX = "."
LOCK_SUFFIX = ".processing.lock"
ERROR_SUFFIX = ".error.json"


def ensure_directory(path: Path) -> None:
    path.mkdir(parents=True, exist_ok=True)


def list_candidate_inputs(input_dir: Path, extensions: Iterable[str]) -> list[Path]:
    allowed_extensions = {ext.lower() for ext in extensions}
    candidates: list[Path] = []

    for entry in sorted(input_dir.iterdir()):
        if not entry.is_file():
            continue
        if entry.name.startswith(HIDDEN_PREFIX):
            continue
        if entry.name.endswith(LOCK_SUFFIX) or entry.name.endswith(ERROR_SUFFIX):
            continue
        if entry.suffix.lower() not in allowed_extensions:
            continue
        candidates.append(entry)

    return candidates


def acquire_processing_lock(file_path: Path) -> Path:
    lock_path = file_path.with_name(f"{file_path.name}{LOCK_SUFFIX}")
    fd = os.open(str(lock_path), os.O_CREAT | os.O_EXCL | os.O_WRONLY)
    os.close(fd)
    return lock_path


def release_processing_lock(lock_path: Path) -> None:
    if lock_path.exists():
        lock_path.unlink()


def build_output_path(output_dir: Path, base_name: str, frame_index: int) -> Path:
    base_filename = f"{base_name}_f{frame_index:04d}.png"
    candidate = output_dir / base_filename
    if not candidate.exists():
        return candidate

    version = 2
    while True:
        versioned = output_dir / f"{base_name}_f{frame_index:04d}_v{version}.png"
        if not versioned.exists():
            return versioned
        version += 1


def atomic_save_png(image: Image.Image, destination: Path) -> None:
    ensure_directory(destination.parent)
    temp_name = f".{destination.name}.tmp-{uuid.uuid4().hex}"
    temp_path = destination.parent / temp_name
    image.save(temp_path, format="PNG")
    os.replace(temp_path, destination)


def safe_delete(path: Path) -> None:
    if path.exists():
        path.unlink()


def move_to_failed(file_path: Path, failed_dir: Path, reason: str) -> Path:
    ensure_directory(failed_dir)
    destination = failed_dir / file_path.name
    if destination.exists():
        stem = destination.stem
        suffix = destination.suffix
        attempt = 2
        while destination.exists():
            destination = failed_dir / f"{stem}_v{attempt}{suffix}"
            attempt += 1

    os.replace(file_path, destination)
    error_path = destination.with_name(f"{destination.name}{ERROR_SUFFIX}")
    error_payload = {"source": str(file_path), "failed_file": str(destination), "reason": reason}
    error_path.write_text(json.dumps(error_payload, ensure_ascii=True, indent=2), encoding="utf-8")
    return destination
