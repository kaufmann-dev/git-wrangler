# remote fetch and clone single-repository stalls

Fixed: 2026-07-02 16:38:00 CEST (+0200)

Commit: 39e5c78309c4afaf250387b049aa3387c53e5928

## Symptom

Bulk remote operations could appear stuck on one repository and then fail that repository after a long delay. A `rewrite-commits` auto-fetch could report `git fetch timed out after 30s` while Git's own stderr showed a much longer connection failure. `clone` could fail one repository in a large batch after a transient GitHub transport error.

## Confirmed Root Cause

Remote commands used context deadlines, but the real subprocess runner only relied on `exec.CommandContext` to kill the direct `git` or `gh` process. Git and GitHub CLI commands can spawn child helper processes that keep stdout/stderr pipes open after the direct process is cancelled, so wall-clock runtime could exceed the intended timeout.

Safe remote transport operations also had no retry policy. A single transient GitHub connection failure therefore became a final per-repository failure for auto-fetch, explicit fetch, reset preparation fetch, or clone.

## Changes Made

The real subprocess runner now configures process-tree cancellation and a short wait delay so timed-out commands do not wait for child helper processes to exit naturally.

Safe remote transport operations now retry transient failures with short backoff: automatic `git fetch --prune origin`, explicit `git-wrangler fetch`, reset preparation fetches, `gh repo list`, and `gh repo clone`.

Clone retries clean up only partial target directories created by the failed clone attempt. Existing target directories remain successful skips, and authentication, permission, not-found, target-exists, merge/rebase, and push failures are not retried.

History rewrite commands still stop before planning or mutation if auto-fetch fails after retries, but now print a `--no-fetch` hint for intentional local-ref runs.
