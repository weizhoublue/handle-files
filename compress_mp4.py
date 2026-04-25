#!/usr/bin/env python3
"""
Recursively compress all MP4 files under a given directory using ffmpeg (H.264 + AAC).
Each file is re-encoded to reduce file size, then the original is deleted.
Files already ending with '_output' are skipped, making the script safe to re-run.

Usage:
    python compress_mp4.py <directory> [dry_run] [confirm]

Arguments:
    directory   Root directory to scan (all subdirectories are included)
    dry_run     'true' or 'false' (default: true)
                'true'  — preview which files would be processed, no changes made
                'false' — actually compress files and delete originals
    confirm     'true' or 'false' (default: true)
                'true'  — prompt for y/n confirmation before processing each file
                'false' — process all files without prompting (unattended mode)
"""

from __future__ import annotations

import os
import sys
import subprocess
from pathlib import Path

if sys.version_info < (3, 7):
    print(f"Error: Python 3.7 or later is required (current: {sys.version.split()[0]})")
    sys.exit(1)


def find_mp4_files(directory: Path) -> list[Path]:
    """Find all MP4 files recursively, skipping already-processed ones."""
    mp4_files = []
    for file in sorted(directory.rglob("*.mp4")):
        stem = file.stem
        if stem.endswith("_output"):
            print(f"  [SKIP] Already processed: {file}")
            continue
        mp4_files.append(file)
    return mp4_files


def compress_file(input_path: Path, dry_run: bool, confirm: bool) -> bool:
    """Compress a single MP4 file. Returns True on success."""
    output_path = input_path.with_name(input_path.stem + "_output" + input_path.suffix)

    input_size_mb = input_path.stat().st_size / 1024 / 1024
    print(f"\n  [{'DRY RUN' if dry_run else 'COMPRESS'}] {input_path} ({input_size_mb:.1f} MB)")
    print(f"           -> {output_path}")

    if dry_run:
        return True

    if confirm:
        answer = input("  Proceed with this file? [y/N]: ").strip().lower()
        if answer != "y":
            print("  [SKIP] Skipped by user.")
            return None  # None = skipped by user, False = ffmpeg error

    cmd = [
        "ffmpeg",
        "-i", str(input_path),
        "-c:v", "libx264",
        "-crf", "23",
        "-preset", "slow",
        "-c:a", "aac",
        "-b:a", "192k",
        str(output_path),
    ]

    print(f"  [CMD] {' '.join(cmd)}")

    try:
        result = subprocess.run(cmd, check=True)
    except subprocess.CalledProcessError as e:
        print(f"  [ERROR] ffmpeg failed (exit code {e.returncode}) for {input_path}, skipping deletion of original.")
        if output_path.exists():
            output_path.unlink()
            print(f"  [CLEANUP] Removed incomplete output: {output_path}")
        return False

    output_size_mb = output_path.stat().st_size / 1024 / 1024
    ratio = output_size_mb / input_size_mb * 100 if input_size_mb > 0 else 0

    print(f"  [DONE] {input_size_mb:.1f} MB -> {output_size_mb:.1f} MB ({ratio:.1f}%)")

    input_path.unlink()
    print(f"  [DELETE] Removed original: {input_path}")

    return True


def check_ffmpeg() -> None:
    """Ensure ffmpeg is installed and available in PATH."""
    try:
        subprocess.run(
            ["ffmpeg", "-version"],
            stdout=subprocess.DEVNULL,
            stderr=subprocess.DEVNULL,
            check=True,
        )
    except FileNotFoundError:
        print("Error: ffmpeg is not installed or not found in PATH.")
        print("  macOS:  brew install ffmpeg")
        print("  Ubuntu: sudo apt install ffmpeg")
        sys.exit(1)


def main():
    if len(sys.argv) < 2:
        print("Usage: python compress_mp4.py <directory> [dry_run] [confirm]")
        print("  dry_run: 'true' or 'false' (default: true)")
        print("  confirm: 'true' or 'false' (default: true)")
        sys.exit(1)

    directory = Path(sys.argv[1]).resolve()
    dry_run_arg = sys.argv[2].lower() if len(sys.argv) >= 3 else "true"
    confirm_arg = sys.argv[3].lower() if len(sys.argv) >= 4 else "true"

    if dry_run_arg not in ("true", "false"):
        print(f"Error: dry_run must be 'true' or 'false', got: {dry_run_arg!r}")
        sys.exit(1)

    if confirm_arg not in ("true", "false"):
        print(f"Error: confirm must be 'true' or 'false', got: {confirm_arg!r}")
        sys.exit(1)

    dry_run = dry_run_arg == "true"
    confirm = confirm_arg == "true"

    check_ffmpeg()

    if not directory.exists():
        print(f"Error: Directory does not exist: {directory}")
        sys.exit(1)

    if not directory.is_dir():
        print(f"Error: Path is not a directory: {directory}")
        sys.exit(1)

    print(f"Directory : {directory}")
    print(f"Mode      : {'DRY RUN (no changes will be made)' if dry_run else 'LIVE (files will be compressed and originals deleted)'}")
    if not dry_run:
        print(f"Confirm   : {'yes (will prompt before each file)' if confirm else 'no (unattended)'}")
    print()

    print("Scanning for MP4 files...")
    mp4_files = find_mp4_files(directory)

    if not mp4_files:
        print("\nNo MP4 files to process.")
        return

    print(f"\nFound {len(mp4_files)} file(s) to process:")

    success_count = 0
    skip_count = 0
    fail_count = 0

    for mp4_file in mp4_files:
        result = compress_file(mp4_file, dry_run, confirm)
        if result is True:
            success_count += 1
        elif result is None:
            skip_count += 1
        else:
            fail_count += 1
            
        if not dry_run:
            total_processed = success_count + skip_count + fail_count
            print(f" ")
            print(f"  [PROGRESS] Processed {total_processed}/{len(mp4_files)} files.")
            print(f" ")
            print(f"============================================================================  ")

    print()
    print("=" * 50)
    if dry_run:
        print(f"DRY RUN complete. {success_count} file(s) would be processed.")
        print("Run with 'false' as second argument to execute for real.")
    else:
        parts = [f"{success_count} succeeded"]
        if skip_count:
            parts.append(f"{skip_count} skipped by user")
        if fail_count:
            parts.append(f"{fail_count} failed")
        print(f"Done. {', '.join(parts)}.")


if __name__ == "__main__":
    main()
