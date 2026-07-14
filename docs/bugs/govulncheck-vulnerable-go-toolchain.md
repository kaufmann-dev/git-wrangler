# govulncheck vulnerable Go toolchain failure

Fixed: 2026-07-15 01:19:56 CEST (+0200)

Commit: 5c747188846d055e514e17f610d0ae9b08c5d5a6

## Symptom

CI's `govulncheck ./...` step exited with status 3 and reported reachable
standard-library vulnerability paths through the AI and authentication HTTP
clients. The scan produced five annotations involving `crypto/tls`,
`net/textproto`, `crypto/x509`, `mime`, and `os`.

## Confirmed Root Cause

The module declared only `go 1.26`, allowing CI to use Go 1.26.3. A local scan
with that version reproduced all five findings. The Go vulnerability database
reports fixes in Go 1.26.4 for two findings and Go 1.26.5 for the remaining
findings, so changing application call sites would not remove the vulnerable
standard-library implementation.

## Changes Made

The module now requires Go 1.26.5, ensuring `actions/setup-go` and release
builds use a toolchain containing all reported standard-library fixes.
