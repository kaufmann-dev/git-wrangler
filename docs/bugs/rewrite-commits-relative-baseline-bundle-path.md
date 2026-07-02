# rewrite-commits relative baseline bundle path

Fixed: 2026-07-02 16:20:05 CEST (+0200)

Commit: 7b4ac31eb96c521397738649ab68bebb20d5e58f

## Symptom

`git-wrangler rewrite-commits` generated AI commit messages successfully, then failed every repository during the apply phase with only generic `could not rewrite commit messages` errors and a final summary of zero rewritten commits.

## Confirmed Root Cause

Discovered repositories used relative `gitDir` values such as `access-manager/.git`. Baseline capture built bundle paths from those relative values, then passed them to `git bundle create` while running Git with `-C access-manager`. Git resolved the bundle path inside the repository directory, producing doubled paths such as `access-manager/access-manager/.git/git-wrangler/baseline/bundles/<capture>.bundle`, which did not exist.

`rewrite-commits` also rendered only captured subprocess output for apply failures. When the subprocess produced no output or the failure happened before the subprocess output was captured, the real error was hidden.

## Changes Made

Baseline bundle creation and import now pass absolute bundle paths to Git subprocesses while keeping portable relative bundle paths in the baseline manifest.

The shared CLI error fallback now lives with rendering helpers, and affected bulk mutation paths use captured subprocess output when present or the underlying error text when output is empty.

Regression tests cover relative discovered `gitDir` baseline capture, absolute bundle paths during baseline import, and `rewrite-commits` apply failures with empty subprocess output.
