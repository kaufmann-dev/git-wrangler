# trailer parser staticcheck S1017 failure

Fixed: 2026-07-15 01:14:14 CEST (+0200)

Commit: 70554741c29cb8ee7aebed7925a8145453c4579b

## Symptom

CI failed while running `staticcheck ./...` with `S1017` on
`internal/trailers/trailers.go`. The trailer parser checked whether a parsed
line ended in `\r` before removing that suffix.

## Confirmed Root Cause

`strings.TrimSuffix` already returns its input unchanged when the requested
suffix is absent. Guarding it with `strings.HasSuffix` was redundant and
triggered staticcheck's simplification diagnostic.

## Changes Made

The parser now calls `strings.TrimSuffix` unconditionally when normalizing a
message line. CRLF input still loses its trailing carriage return, while LF
input remains unchanged.
