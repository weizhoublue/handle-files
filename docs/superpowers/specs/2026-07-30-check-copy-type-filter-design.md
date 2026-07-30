# Check-copy type filter design

## Goal

Add a repeatable `--type/-t` option to `check-copy` so users can limit preview, comparison, statistics, and copying to files with selected extensions.

Examples:

```text
check-copy -s /Volumes/red/1 -d /Volumes/black/1 -t jpg
check-copy -s /Volumes/red/1 -d /Volumes/black/1 -t jpg -t mp4 -c
```

When no type is specified, `check-copy` continues to process all regular files.

## CLI contract

The usage form becomes:

```text
check-copy --source/-s <directory> --destination/-d <directory> [--type/-t <extension>]... [--copy/-c]
```

`--type` and `-t` are aliases for the same repeatable option and may be mixed. Each value is normalized by:

1. Trimming leading and trailing whitespace.
2. Removing one optional leading `.`.
3. Converting the remaining extension to lowercase.
4. Removing duplicate normalized extensions.

Matching is case-insensitive, so `jpg`, `.jpg`, and `JPG` select the same files.

A type value is invalid when it is empty after normalization, is only `.`, contains another `.`, contains `/` or `\`, or contains whitespace after trimming. Parsing returns an `invalid --type value` error before scanning either directory. The command reports it through the existing `validation_failed` event and exits with a nonzero status.

Extensions use only the final filename suffix. `-t gz` matches `archive.tar.gz`, while `-t tar.gz` is invalid. A filename consisting only of a leading dot and a name, such as `.gitignore`, is treated as having no extension and does not match a type filter.

## Options and filtering

`internal/checkcopy.Options` gains a `Types []string` field containing normalized, deduplicated extensions without leading dots. An empty slice means no filtering.

The source and destination scans both receive the selected type set. When the set is non-empty, scanning adds a regular file only when its final extension matches the set. Filtering both directories establishes the selected file universe before comparison.

All downstream behavior operates on those filtered maps:

- Missing, extra, source-larger, and destination-larger classifications include only selected types.
- Preview output and copy candidate counts include only selected types.
- Scan and difference summaries count only selected types.
- Case-conflict detection and warnings include only selected types.
- Copy mode writes only selected types.

No-match runs succeed normally and produce the existing zero-count summaries. No new warning or summary event is introduced.

When `Types` is empty, scanning, comparison, reporting, and copying remain unchanged, including treatment of extensionless files and dotfiles.

## Existing behavior

The change does not alter:

- Source and destination directory validation.
- Exact relative-path comparison.
- Size-based copy candidate selection.
- Report-only mode.
- Destination confinement and symbolic-link protections.
- Metadata preservation.
- Partial-copy cleanup.
- No-space abort behavior.
- Existing logging formats.

These behaviors apply to the selected file universe when a type filter is present.

## Documentation

Update `internal/checkcopy.Usage()` and `docs/readme.md` to document:

- Repeatable `--type/-t`.
- Case-insensitive matching.
- Optional leading dot.
- Default processing of all types when omitted.
- Single-type and multiple-type examples.

## Tests

Unit tests will cover:

1. Repeated `-t` and `--type` values, including mixed aliases.
2. Normalization of case and optional leading dots.
3. Deduplication.
4. Empty `Types` when no filter is supplied.
5. Rejection of empty, dot-only, multi-dot, path-containing, and whitespace-containing values.
6. Final-extension matching for names such as `archive.tar.gz`.
7. Exclusion of dotfiles such as `.gitignore`.
8. Filtering of both source and destination maps.
9. Filtered preview, summaries, case conflicts, and actual copying.
10. Preservation of existing all-file behavior without `-t`.

An integration test will run the built binary with `-t jpg -t mp4 -c`, verify that only those types are copied, and verify that help output documents the repeatable option.

## Scope

Changes are limited to `check-copy` option parsing, scan filtering, command usage, CLI documentation, and related unit and integration tests. The compression command and release workflow are unchanged.
