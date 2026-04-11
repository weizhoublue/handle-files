#!/usr/bin/env python3
"""
Compare files between two directories by size and optionally copy inconsistencies.

Usage:
    python sync_check.py <src_dir> <dst_dir> [copy]

Arguments:
    src_dir   Source directory (required)
    dst_dir   Destination directory (required)
    copy      'true' or 'false' (default: 'false')
              'false' — detect and report differences only
              'true'  — copy missing/size-mismatch files from src to dst

Requires: Python 3.7+
"""
from __future__ import annotations

import sys
import shutil
from pathlib import Path


def scan_files(directory: Path) -> dict[str, int]:
    """Walk directory recursively. Returns {relative_path_str: file_size_bytes}."""
    result: dict[str, int] = {}
    for path in directory.rglob("*"):
        if not path.is_file():
            continue
        rel = path.relative_to(directory).as_posix()
        try:
            size = path.stat().st_size
        except OSError as e:
            print(f"  [WARN] Cannot stat {path}: {e}")
            continue
        result[rel] = size
    return result


def find_case_conflicts(file_map: dict[str, int]) -> list[list[str]]:
    """Return groups of paths that collide on a case-insensitive filesystem.

    Each group contains 2+ paths whose lowercase forms are identical.
    """
    from collections import defaultdict
    buckets: dict[str, list[str]] = defaultdict(list)
    for rel in file_map:
        buckets[rel.lower()].append(rel)
    return [sorted(paths) for paths in buckets.values() if len(paths) > 1]


def compare(
    src_map: dict[str, int],
    dst_map: dict[str, int],
) -> tuple[set[str], set[str], set[str]]:
    """Return (missing, extra, size_mismatch) as sets of relative path strings.

    missing       — in src, not in dst
    extra         — in dst, not in src
    size_mismatch — in both, but sizes differ
    """
    src_keys = set(src_map)
    dst_keys = set(dst_map)

    missing = src_keys - dst_keys
    extra = dst_keys - src_keys
    size_mismatch = {
        k for k in src_keys & dst_keys
        if src_map[k] != dst_map[k]
    }
    return missing, extra, size_mismatch


def copy_files(src_dir: Path, dst_dir: Path, relative_paths: set[str]) -> tuple[int, int]:
    """Copy files from src_dir to dst_dir. Creates parent dirs as needed.

    Returns (copied_count, failed_count).
    """
    copied = 0
    failed = 0

    for rel in sorted(relative_paths):
        src_path = src_dir / rel
        dst_path = dst_dir / rel

        try:
            dst_path.parent.mkdir(parents=True, exist_ok=True)
        except OSError as e:
            print(f"  [WARN] Cannot create directory {dst_path.parent}: {e}")
            failed += 1
            continue

        if not src_path.exists():
            print(f"  [WARN] Source file missing, skipping: {src_path}")
            failed += 1
            continue

        print(f"  [COPY] {rel}")
        try:
            shutil.copy2(src_path, dst_path)
            copied += 1
        except OSError as e:
            print(f"  [WARN] Copy failed for {rel}: {e}")
            if dst_path.exists():
                try:
                    dst_path.unlink()
                except OSError:
                    pass
            failed += 1

    return copied, failed


def main() -> None:
    if sys.version_info < (3, 7):
        print(f"Error: Python 3.7 or later is required (current: {sys.version.split()[0]})")
        sys.exit(1)

    if len(sys.argv) < 3:
        print("Usage: python sync_check.py <src_dir> <dst_dir> [copy]")
        print("  copy: 'true' or 'false' (default: 'false')")
        sys.exit(1)

    src_dir = Path(sys.argv[1]).resolve()
    dst_dir = Path(sys.argv[2]).resolve()
    copy_arg = sys.argv[3].lower() if len(sys.argv) >= 4 else "false"

    if copy_arg not in ("true", "false"):
        print(f"Error: copy must be 'true' or 'false', got: {copy_arg!r}")
        sys.exit(1)

    do_copy = copy_arg == "true"

    if not src_dir.exists() or not src_dir.is_dir():
        print(f"Error: Source directory does not exist or is not a directory: {src_dir}")
        sys.exit(1)

    if not dst_dir.exists() or not dst_dir.is_dir():
        print(f"Error: Destination directory does not exist or is not a directory: {dst_dir}")
        sys.exit(1)

    if src_dir == dst_dir:
        print("Error: Source and destination directories must be different.")
        sys.exit(1)

    print(f"[INFO] Source      : {src_dir}")
    print(f"[INFO] Destination : {dst_dir}")
    print(f"[INFO] Mode        : {'COPY (will overwrite mismatches)' if do_copy else 'REPORT ONLY (no changes)'}")
    print()

    print("[SCAN] Scanning source directory...")
    src_map = scan_files(src_dir)
    print(f"[SCAN] Source files found      : {len(src_map)}")

    print("[SCAN] Scanning destination directory...")
    dst_map = scan_files(dst_dir)
    print(f"[SCAN] Destination files found : {len(dst_map)}")

    # Warn about case conflicts before comparing
    src_conflicts = find_case_conflicts(src_map)
    if src_conflicts:
        print()
        print(f"[WARN] Source has {len(src_conflicts)} case conflict(s) — these files differ only in")
        print("       case and cannot both exist on a case-insensitive destination filesystem:")
        for group in src_conflicts:
            paths_str = "  vs  ".join(f"'{src_dir / p}' ({src_map[p]:,} B)" for p in group)
            print(f"  [WARN]   {paths_str}")
        if do_copy:
            print("  [WARN] copy=true: copying will overwrite the conflicting file on destination!")
    print()

    missing, extra, size_mismatch = compare(src_map, dst_map)
    consistent_count = len(src_map) - len(missing) - len(size_mismatch)

    for rel in sorted(missing):
        print(f"  [MISS]  src: {src_dir / rel}  ({src_map[rel]:,} bytes)")
        print(f"          dst: {dst_dir / rel}  (absent)")

    for rel in sorted(extra):
        print(f"  [EXTRA] src: {src_dir / rel}  (absent)")
        print(f"          dst: {dst_dir / rel}  ({dst_map[rel]:,} bytes)")

    # Split size_mismatch: src larger (will copy) vs dst larger (skip)
    diff_src_larger = {r for r in size_mismatch if src_map[r] > dst_map[r]}
    diff_dst_larger = size_mismatch - diff_src_larger

    for rel in sorted(diff_src_larger):
        print(f"  [DIFF]  src: {src_dir / rel}  ({src_map[rel]:,} bytes)  <- src is larger, will copy")
        print(f"          dst: {dst_dir / rel}  ({dst_map[rel]:,} bytes)")

    for rel in sorted(diff_dst_larger):
        print(f"  [DIFF]  src: {src_dir / rel}  ({src_map[rel]:,} bytes)")
        print(f"          dst: {dst_dir / rel}  ({dst_map[rel]:,} bytes)  <- dst is larger, skip")

    if not missing and not extra and not size_mismatch:
        print("  [INFO] All files are consistent.")

    # Copy section: missing + size_mismatch where src is larger
    copied = failed = 0
    to_copy = missing | diff_src_larger

    if do_copy and to_copy:
        print()
        print(f"[INFO] Copying {len(to_copy)} file(s) to destination...")
        copied, failed = copy_files(src_dir, dst_dir, to_copy)
    elif not do_copy and to_copy:
        print()
        print(f"  [SKIP] {len(to_copy)} file(s) need copying. Run with 'true' to copy.")

    # Summary
    print()
    print("=" * 50)
    print("Scan complete")
    print(f"  Source files      : {len(src_map)}")
    dst_file_count = len(dst_map) + copied if do_copy else len(dst_map)
    print(f"  Destination files : {dst_file_count}")
    print()
    print("Differences:")
    print(f"  Missing  (src only)       : {len(missing)}")
    print(f"  Extra    (dst only)       : {len(extra)}")
    print(f"  Size diff, src larger     : {len(diff_src_larger)}  (will copy)")
    print(f"  Size diff, dst larger     : {len(diff_dst_larger)}  (skip)")
    print(f"  Consistent                : {consistent_count}")
    if src_conflicts:
        print(f"  Case conflicts (src) : {len(src_conflicts)}  [WARN] see above")

    if do_copy:
        print()
        print(f"  Copied              : {copied}  ({len(missing)} missing + {len(diff_src_larger)} size diff src larger)")
        if failed:
            print(f"  Copy failed (warn)  : {failed}")


if __name__ == "__main__":
    main()
