# rewrite-commits dropped Git trailers

Fixed: 2026-07-15 01:03:44 CEST (+0200)

Commit: 3144f044b531dd6289c4d3c357a86aa2403e8f4f

## Symptom

`git-wrangler rewrite-commits` replaced a commit's complete message with the
AI-generated subject and optional body. Valid final trailers such as
`Co-authored-by`, `Signed-off-by`, and `Reviewed-by` disappeared from every
commit whose message was rewritten.

## Confirmed Root Cause

The AI plan compared and wrote only `FormatMessage(result)`. Its
`git-filter-repo` callback assigned that generated text directly to
`commit.message`, so no code parsed or merged the original final trailer block
before callback generation. Inspecting rewritten commit objects confirmed this
was saved-message data loss, not a log or display problem.

## Changes Made

A shared internal trailer parser now follows Git's final-paragraph and
`--no-divider` behavior while retaining original trailer casing, spelling,
order, and folded lines. AI rewrite plans merge that raw trailer block into the
generated subject/body before unchanged-message comparison and callback
generation. `rewrite-commits --remove-coauthors` removes only case-insensitive
`Co-authored-by` entries from commits that otherwise receive generated
messages.

The `commit` command now accepts repeatable, locally appended `--coauthor
"Name <email>"` values. The new `rewrite-coauthors add`, `replace`, and `remove`
history workflows provide validated coauthor-only mutation without exposing
generic attestation editing. Cumulative rollback metadata also tracks current
parent mappings so later baseline generations reconnect through excluded or
replayed commits to the first-baselined history.
