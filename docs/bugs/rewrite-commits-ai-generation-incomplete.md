# rewrite-commits incomplete AI generation

Fixed: 2026-07-02 16:57:53 CEST (+0200)

Commit: 2df6e25045e6ea5f78469cdef617595a0d70de5c

## Symptom

`git-wrangler rewrite-commits --skip-conventional --require-scope` completed repository fetch and scan, sent AI requests, then stopped with `AI generation is incomplete; no history was changed`. The run reported many retry reasons such as incomplete JSON, truncated output, missing or invalid messages, `unexpected EOF`, HTTP/2 connection closure, and connection reset errors. Only a few commits remained failed at the end, but the command correctly refused to rewrite history with incomplete AI results.

## Confirmed Root Cause

The AI generation path asked the model to return JSON through prompt text only, but did not send the OpenAI-compatible `response_format: {"type":"json_object"}` request field supported by providers such as DeepSeek. Under a large 335-batch run, some responses were therefore malformed, empty, or truncated.

The retry path also treated most failures the same way. It retried batches and then individual commits, but remaining single-commit failures had no final low-context recovery path. Permanent API errors such as invalid credentials were also eligible for the same retry loop even though repeating them could not succeed.

Lowering `--rpm` from 300 to 60 might reduce transport pressure, but the observed final failures were not dominated by rate-limit or timeout errors. The main reproducible issues were response-shape and recovery behavior, so the default `--rpm 300` remains unchanged.

## Changes Made

AI generation requests now include `response_format: {"type":"json_object"}` while keeping the existing JSON prompt and generated-message validation.

Malformed, empty, incomplete, and truncated AI responses now produce normalized retryable errors. Truncated or incomplete response retries also shrink request context while later attempts keep increasing the output token budget.

Permanent auth/model/API errors are no longer retried. Transient transport errors, HTTP 429, HTTP 5xx, malformed JSON, output truncation, and invalid generated messages remain retryable.

If batch and individual retries still leave failed commits, generation now runs a final deterministic single-commit recovery pass with smaller context before reporting failure. Remaining failures are grouped by reason and include guidance to retry, lower `--rpm` only for repeated transport failures, or reduce `--batch-size`/use a stronger model for JSON or message-quality failures.
