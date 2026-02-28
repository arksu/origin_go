#!/usr/bin/env python3
from __future__ import annotations

import argparse
import logging
import time
from dataclasses import dataclass
from pathlib import Path

SUPPORTED_EXTENSIONS = (".png", ".jpg", ".jpeg", ".webp")
LOGGER = logging.getLogger("image-pixel-processor")


@dataclass
class FileObservation:
    size_bytes: int
    mtime_ns: int
    last_change_at: float


class StableFileTracker:
    def __init__(self, stable_for_seconds: float) -> None:
        if stable_for_seconds <= 0:
            raise ValueError("stable_for_seconds must be > 0")
        self._stable_for_seconds = stable_for_seconds
        self._observed: dict[Path, FileObservation] = {}

    def collect_ready(self, files: list[Path]) -> list[Path]:
        now = time.monotonic()
        seen_paths = set(files)
        ready: list[Path] = []

        for file_path in files:
            stats = file_path.stat()
            current = FileObservation(
                size_bytes=int(stats.st_size),
                mtime_ns=int(stats.st_mtime_ns),
                last_change_at=now,
            )

            previous = self._observed.get(file_path)
            if previous is None:
                self._observed[file_path] = current
                continue

            changed = (
                previous.size_bytes != current.size_bytes or previous.mtime_ns != current.mtime_ns
            )
            if changed:
                self._observed[file_path] = current
                continue

            current.last_change_at = previous.last_change_at
            self._observed[file_path] = current
            if now - current.last_change_at >= self._stable_for_seconds:
                ready.append(file_path)

        stale_paths = [path for path in self._observed if path not in seen_paths]
        for stale in stale_paths:
            self._observed.pop(stale, None)

        return ready

    def forget(self, file_path: Path) -> None:
        self._observed.pop(file_path, None)


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Watch input directory and process new images into pixel-art outputs.")
    parser.add_argument("--input", required=True, help="Input directory to watch.")
    parser.add_argument("--output", required=True, help="Output directory for processed PNG files.")
    parser.add_argument(
        "--failed-dir",
        default="input_failed",
        help="Directory where files are moved when processing fails.",
    )
    parser.add_argument(
        "--unfaker-command",
        required=True,
        help=(
            "Command used to run Unfaker. Use placeholders {input} and {output} when needed. "
            "If placeholders are omitted, input and output paths are appended as positional args."
        ),
    )
    parser.add_argument("--poll-interval-ms", type=int, default=500, help="Polling interval in milliseconds.")
    parser.add_argument(
        "--stable-for-ms",
        type=int,
        default=500,
        help="File must stay unchanged for this time before processing starts.",
    )
    parser.add_argument(
        "--alpha-ratio-threshold",
        type=float,
        default=0.02,
        help="Ratio threshold for alpha<alpha-value-threshold to consider transparency meaningful.",
    )
    parser.add_argument(
        "--alpha-value-threshold",
        type=int,
        default=250,
        help="Pixel is counted as transparent when alpha is below this value.",
    )
    parser.add_argument(
        "--frame-min-area",
        type=int,
        default=16,
        help="Minimum non-transparent pixel count required for one detected frame.",
    )
    parser.add_argument(
        "--component-min-area",
        type=int,
        default=None,
        help=argparse.SUPPRESS,
    )
    parser.add_argument(
        "--extensions",
        default=",".join(ext.lstrip(".") for ext in SUPPORTED_EXTENSIONS),
        help="Comma-separated image extensions to process.",
    )
    parser.add_argument(
        "--unfaker-timeout-seconds",
        type=int,
        default=120,
        help="Timeout for one Unfaker subprocess call.",
    )
    parser.add_argument(
        "--tmp-dir",
        default="",
        help="Temporary directory for intermediate frame files. Default: <output>/.tmp",
    )
    parser.add_argument(
        "--workers",
        type=int,
        default=1,
        help="Reserved for future parallelism. Current version supports only 1 worker.",
    )
    parser.add_argument(
        "--log-level",
        default="INFO",
        choices=("DEBUG", "INFO", "WARNING", "ERROR"),
        help="Log verbosity.",
    )
    return parser.parse_args()


def validate_args(args: argparse.Namespace) -> None:
    if args.poll_interval_ms <= 0:
        raise ValueError("poll-interval-ms must be > 0")
    if args.stable_for_ms <= 0:
        raise ValueError("stable-for-ms must be > 0")
    if args.unfaker_timeout_seconds <= 0:
        raise ValueError("unfaker-timeout-seconds must be > 0")
    if args.workers != 1:
        raise ValueError("Current implementation supports only --workers=1")
    if not 0.0 <= args.alpha_ratio_threshold <= 1.0:
        raise ValueError("alpha-ratio-threshold must be within [0.0, 1.0]")
    if not 0 <= args.alpha_value_threshold <= 255:
        raise ValueError("alpha-value-threshold must be within [0, 255]")
    if args.frame_min_area <= 0:
        raise ValueError("frame-min-area must be > 0")
    if args.component_min_area is not None and args.component_min_area <= 0:
        raise ValueError("component-min-area must be > 0")


def configure_logging(log_level: str) -> None:
    logging.basicConfig(
        level=getattr(logging, log_level),
        format="%(asctime)s %(levelname)s %(name)s %(message)s",
    )


def normalize_extensions(raw_extensions: str) -> tuple[str, ...]:
    parts = [part.strip().lower() for part in raw_extensions.split(",") if part.strip()]
    if not parts:
        raise ValueError("extensions must not be empty")
    normalized = tuple(part if part.startswith(".") else f".{part}" for part in parts)
    return normalized


def main() -> None:
    args = parse_args()
    validate_args(args)
    configure_logging(args.log_level)

    try:
        from io_safe import (
            acquire_processing_lock,
            ensure_directory,
            list_candidate_inputs,
            move_to_failed,
            release_processing_lock,
        )
        from pipeline import PipelineConfig, PixelPipeline
    except ModuleNotFoundError as exc:
        script_dir = Path(__file__).resolve().parent
        venv_path = script_dir / ".venv"
        message = (
            f"Missing Python dependency: {exc.name!r}.\n"
            "This tool should run inside a virtual environment.\n"
            f"Create and activate it:\n"
            f"  python3 -m venv {venv_path}\n"
            f"  source {venv_path}/bin/activate\n"
            f"  python3 -m pip install -r {script_dir / 'requirements.txt'}\n"
            f"Or run via helper script:\n"
            f"  {script_dir / 'run_watcher.sh'} --input ... --output ... --unfaker-command ...\n"
        )
        raise SystemExit(message) from exc

    input_dir = Path(args.input).expanduser().resolve()
    output_dir = Path(args.output).expanduser().resolve()
    failed_dir = Path(args.failed_dir).expanduser().resolve()
    temp_dir = Path(args.tmp_dir).expanduser().resolve() if args.tmp_dir else output_dir / ".tmp"
    extensions = normalize_extensions(args.extensions)
    min_frame_area = args.component_min_area if args.component_min_area is not None else args.frame_min_area

    ensure_directory(input_dir)
    ensure_directory(output_dir)
    ensure_directory(failed_dir)
    ensure_directory(temp_dir)

    if input_dir == output_dir:
        raise ValueError("input and output directories must be different")

    pipeline = PixelPipeline(
        PipelineConfig(
            output_dir=output_dir,
            temp_dir=temp_dir,
            unfaker_command=args.unfaker_command,
            alpha_ratio_threshold=args.alpha_ratio_threshold,
            alpha_value_threshold=args.alpha_value_threshold,
            min_frame_area=min_frame_area,
            unfaker_timeout_seconds=args.unfaker_timeout_seconds,
        )
    )
    tracker = StableFileTracker(stable_for_seconds=args.stable_for_ms / 1000.0)
    poll_interval_seconds = args.poll_interval_ms / 1000.0

    LOGGER.info(
        "event=watcher_start input=%s output=%s failed_dir=%s temp_dir=%s extensions=%s",
        input_dir,
        output_dir,
        failed_dir,
        temp_dir,
        ",".join(extensions),
    )

    while True:
        try:
            candidates = list_candidate_inputs(input_dir, extensions=extensions)
            ready_files = tracker.collect_ready(candidates)

            for file_path in ready_files:
                lock_path = None
                try:
                    lock_path = acquire_processing_lock(file_path)
                except FileExistsError:
                    LOGGER.debug("event=skip_locked file=%s", file_path)
                    continue

                LOGGER.info("event=file_detected file=%s", file_path)

                try:
                    pipeline.process_file(file_path)
                    tracker.forget(file_path)
                except Exception as exc:  # pragma: no cover - runtime protection path
                    LOGGER.exception("event=file_failed file=%s error=%s", file_path, exc)
                    if file_path.exists():
                        try:
                            failed_file = move_to_failed(file_path, failed_dir=failed_dir, reason=str(exc))
                            LOGGER.error("event=file_moved_to_failed file=%s failed=%s", file_path, failed_file)
                        except Exception as move_exc:  # pragma: no cover - runtime protection path
                            LOGGER.exception(
                                "event=file_failed_move_error file=%s error=%s",
                                file_path,
                                move_exc,
                            )
                    tracker.forget(file_path)
                finally:
                    if lock_path is not None:
                        release_processing_lock(lock_path)

            time.sleep(poll_interval_seconds)
        except KeyboardInterrupt:
            LOGGER.info("event=watcher_stop reason=keyboard_interrupt")
            break


if __name__ == "__main__":
    main()
