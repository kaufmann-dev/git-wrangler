# interactive prompt arrow keys rendered literally

Fixed: 2026-07-18 00:52:35 CEST (+0200)

Commit: ac3bd9db7eb24d0bd923567a8b35ffdf42f6e24d

## Symptom

Pressing the left arrow while entering a `rename-repo` description printed
`^[[D` instead of moving the cursor. The same escape bytes appeared in other
visible prompts and could be inserted silently into hidden prompt values.

## Confirmed Root Cause

The shared prompt session read visible input with `bufio.Reader.ReadString` and
hidden input with canonical password reading. Neither path interpreted VT100
cursor-key sequences. A live PTY reproduction at the shared `init` prompt
rendered `github.co^[[D^[[D`, confirming that the defect affected the common
prompt implementation rather than only `rename-repo`.

## Changes Made

Real TTY prompts now use one persistent `golang.org/x/term` line editor for
visible and hidden input. The editor supports cursor-aware insertion, preserves
already-buffered input between prompts, disables cross-prompt history, and
keeps secrets unrendered.

The shared session restores terminal state on success, failure, or cancellation
and preserves immediate `Ctrl+C`, `Ctrl+D`, and EOF cancellation even after
partial input. Prompt read failures now stop commands without being mistaken
for blank values or declined confirmations.

Regression tests cover split arrow sequences, Unicode insertion, hidden input,
multi-prompt buffering, cancellation with partial input, and terminal
restoration. A live PTY check confirms cursor editing and unchanged terminal
state after cancellation.
