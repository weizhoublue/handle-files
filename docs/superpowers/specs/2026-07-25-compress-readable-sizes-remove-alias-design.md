# Compression readable sizes and remove alias design

## Goal

Make successful `compress-vedio` records show file sizes in an appropriate
decimal unit and add the `-r` alias for `--remove`.

## Size formatting

The compression package will provide an internal formatter for non-negative
byte counts. It will select the largest suitable decimal SI unit from `B`,
`KB`, `MB`, `GB`, and `TB`, using 1000 as the unit step.

Values below 1000 will display as an integer byte count, for example `999 B`.
Larger values will display one decimal digit, for example `10.0 MB` and
`1.2 GB`.

Successful compression records will replace exact-byte fields with
human-readable size fields:

```text
original_size=51.0 MB output_size=4.4 MB reduction_size=46.6 MB
```

The existing `input` and `output` fields continue to hold file paths, and
`reduction_percent` remains unchanged. Renaming the fields avoids labeling
human-readable values as byte counts.

## Remove option

`-r` will bind to the same value as `--remove`. Its default remains `true`,
and the flag parser will accept both `-r false` and `-r=false`, matching the
long-option behavior.

Usage text and README tables will list the option as `--remove, -r`.

## Validation

Tests will cover:

1. Size formatting at byte, KB, MB, GB, and TB boundaries.
2. Successful log records using `*_size` fields and no longer emitting the
   removed `*_bytes` fields.
3. Parsing `-r` with explicit true and false values while preserving the
   default.
4. README and command usage showing the short alias.

## Scope

Changes are limited to compression options, compression service tests, and
the compression CLI documentation. Compression behavior, deletion timing,
and FFmpeg invocation are unchanged.
