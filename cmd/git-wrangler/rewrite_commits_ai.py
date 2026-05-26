import json
import os
import re
import subprocess
import sys
import time
import urllib.error
import urllib.request


def color_enabled():
    if os.environ.get("NO_COLOR") or os.environ.get("CLICOLOR") == "0" or os.environ.get("TERM") == "dumb":
        return False
    if os.environ.get("CLICOLOR_FORCE") and os.environ.get("CLICOLOR_FORCE") != "0":
        return True
    return sys.stdout.isatty()


if color_enabled():
    RED = "\033[31m"
    GREEN = "\033[32m"
    YELLOW = "\033[33m"
    CYAN = "\033[36m"
    RESET = "\033[0m"
else:
    RED = ""
    GREEN = ""
    YELLOW = ""
    CYAN = ""
    RESET = ""


CONVENTIONAL_RE = re.compile(
    r"^(feat|fix|docs|style|refactor|test|chore|perf|ci|build|revert)(\([^)]+\))?!?: .+"
)

SECRET_PATTERNS = [
    re.compile(r"(['\"]?)(sk-[a-zA-Z0-9_-]{20,}|sk_[a-zA-Z0-9_-]{20,})(['\"]?)"),
    re.compile(r"(['\"]?)(gh[pousr]_[a-zA-Z0-9_]{20,})(['\"]?)"),
    re.compile(r"\bAKIA[0-9A-Z]{16}\b"),
    re.compile(r"(password|passwd|pwd|secret|api_key|apikey|auth_token|access_token|private_key)\s*[:=]\s*['\"][^'\"]{8,}['\"]", re.I),
    re.compile(r"(mongodb(\+srv)?|postgres(ql)?|mysql|redis)://[^@\s]+@[^\s]+", re.I),
    re.compile(r"Bearer\s+[a-zA-Z0-9_.-]+", re.I),
]


def fail(message):
    print(f"{RED}Error: {message}{RESET}", file=sys.stderr)
    sys.exit(1)


def run_git(repo_dir, args, check=True):
    proc = subprocess.run(
        ["git"] + args,
        cwd=repo_dir,
        text=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
    )
    if check and proc.returncode != 0:
        raise RuntimeError(proc.stderr.strip() or proc.stdout.strip() or "git command failed")
    return proc.stdout


def ask_yes_no(question):
    try:
        with open("/dev/tty", "r+", encoding="utf-8") as tty:
            tty.write(question)
            tty.flush()
            answer = tty.readline().strip().lower()
    except OSError:
        if not sys.stdin.isatty():
            fail("interactive terminal is required.")
        sys.stdout.write(question)
        sys.stdout.flush()
        answer = sys.stdin.readline().strip().lower()
    return answer in ("y", "yes")


def repo_display_name(repo_dir, root_dir):
    rel = os.path.relpath(repo_dir, root_dir)
    if rel == ".":
        return os.path.basename(root_dir)
    return os.path.basename(repo_dir)


def is_conventional(message):
    first_line = message.strip().splitlines()[0] if message.strip() else ""
    return bool(CONVENTIONAL_RE.match(first_line))


def is_sensitive_path(path):
    normalized = path.replace("\\", "/").lower()
    base = normalized.rsplit("/", 1)[-1]
    if base == ".env" or base.startswith(".env."):
        return True
    if base in {"id_rsa", "id_rsa.pub", "id_ed25519", "id_ed25519.pub", "credentials.json", "secrets.json"}:
        return True
    if base.endswith((".pem", ".key", ".p12", ".pfx")):
        return True
    if ("secret" in normalized or "credential" in normalized) and base.endswith((".json", ".yml", ".yaml", ".toml", ".ini", ".env")):
        return True
    return False


def redact_line(line):
    redacted = line
    for pattern in SECRET_PATTERNS:
        redacted = pattern.sub("[REDACTED]", redacted)
    return redacted


def diff_path(line):
    match = re.match(r"diff --git a/(.*?) b/(.*)$", line)
    if match:
        return match.group(2)
    return ""


def redact_diff(diff_text):
    redacted_lines = []
    hiding_file = False
    for line in diff_text.splitlines():
        if line.startswith("diff --git "):
            path = diff_path(line)
            hiding_file = is_sensitive_path(path)
            redacted_lines.append(redact_line(line))
            if hiding_file:
                redacted_lines.append("[SENSITIVE FILE CONTENT HIDDEN]")
            continue
        if hiding_file:
            continue
        redacted_lines.append(redact_line(line))
    return "\n".join(redacted_lines)


def truncate_text(text, limit):
    if len(text) <= limit:
        return text
    suffix = "\n[TRUNCATED TO PER-COMMIT CONTEXT BUDGET]"
    return text[: max(0, limit - len(suffix))] + suffix


def limited_lines(text, max_lines):
    lines = [line for line in text.splitlines() if line.strip()]
    if len(lines) <= max_lines:
        return "\n".join(lines)
    return "\n".join(lines[:max_lines] + [f"[TRUNCATED {len(lines) - max_lines} LINES]"])


def build_context(repo_dir, repo_name, commit_hash, char_budget):
    name_status = run_git(repo_dir, ["diff-tree", "--root", "--no-commit-id", "--name-status", "-r", commit_hash])
    numstat = run_git(repo_dir, ["diff-tree", "--root", "--no-commit-id", "--numstat", "-r", commit_hash])
    if not name_status.strip() and not numstat.strip():
        return ""
    diff_text = run_git(
        repo_dir,
        ["show", "--format=", "--no-color", "--no-ext-diff", "--find-renames", "--find-copies", "--unified=3", commit_hash],
    )
    context = "\n".join(
        [
            f"Repository: {repo_name}",
            f"Commit: {commit_hash[:12]}",
            "",
            "Files changed:",
            limited_lines(name_status, 80),
            "",
            "Stats:",
            limited_lines(numstat, 80),
            "",
            "Redacted diff snippet:",
            redact_diff(diff_text),
        ]
    )
    return truncate_text(context, char_budget)


def collect_items(repos_file, char_budget, skip_conventional):
    root_dir = os.getcwd()
    items = []
    stats = {
        "repo_count": 0,
        "total_commits": 0,
        "skipped_formatted": 0,
        "skipped_empty": 0,
        "skipped_unborn": 0,
    }
    with open(repos_file, "r", encoding="utf-8") as f:
        git_dirs = [line.strip() for line in f if line.strip()]
    for repo_index, git_dir in enumerate(git_dirs):
        repo_dir = os.path.abspath(os.path.dirname(git_dir))
        repo_name = repo_display_name(repo_dir, root_dir)
        stats["repo_count"] += 1
        try:
            run_git(repo_dir, ["rev-parse", "HEAD"])
        except RuntimeError:
            stats["skipped_unborn"] += 1
            continue
        commits_output = run_git(repo_dir, ["rev-list", "--reverse", "--all"])
        commits = [line.strip() for line in commits_output.splitlines() if line.strip()]
        stats["total_commits"] += len(commits)
        for commit_hash in commits:
            old_message = run_git(repo_dir, ["log", "-1", "--format=%B", commit_hash]).strip()
            if skip_conventional and is_conventional(old_message):
                stats["skipped_formatted"] += 1
                continue
            context = build_context(repo_dir, repo_name, commit_hash, char_budget)
            if not context.strip():
                stats["skipped_empty"] += 1
                continue
            item_id = f"c{len(items) + 1:06d}"
            items.append(
                {
                    "id": item_id,
                    "repo_index": repo_index,
                    "repo_dir": repo_dir,
                    "repo_name": repo_name,
                    "hash": commit_hash,
                    "old_message": old_message,
                    "context": context,
                }
            )
    return items, stats


def chat_endpoint(base_url):
    base = base_url.rstrip("/")
    if base.endswith("/chat/completions"):
        return base
    return base + "/chat/completions"


def parse_json_response(content):
    text = content.strip()
    if text.startswith("```"):
        text = re.sub(r"^```(?:json)?\s*", "", text)
        text = re.sub(r"\s*```$", "", text)
    try:
        return json.loads(text)
    except json.JSONDecodeError:
        start = text.find("{")
        end = text.rfind("}")
        if start != -1 and end != -1 and end > start:
            return json.loads(text[start : end + 1])
        raise


def normalize_message(message):
    if not isinstance(message, str):
        return ""
    lines = [line.strip() for line in message.strip().strip('"').strip("'").splitlines() if line.strip()]
    return lines[0] if lines else ""


def validate_message(message):
    return bool(message) and len(message) <= 120 and bool(CONVENTIONAL_RE.match(message))


def extract_messages(data):
    if isinstance(data, dict) and isinstance(data.get("messages"), list):
        rows = data["messages"]
    elif isinstance(data, list):
        rows = data
    elif isinstance(data, dict):
        rows = [{"id": key, "message": value} for key, value in data.items()]
    else:
        rows = []
    result = {}
    for row in rows:
        if not isinstance(row, dict):
            continue
        item_id = str(row.get("id", "")).strip()
        message = normalize_message(row.get("message", ""))
        if item_id:
            result[item_id] = message
    return result


def request_batch(batch, base_url, model, api_key, timeout_seconds):
    commits = [
        {
            "id": item["id"],
            "repository": item["repo_name"],
            "commit": item["hash"][:12],
            "context": item["context"],
        }
        for item in batch
    ]
    payload = {
        "model": model,
        "messages": [
            {
                "role": "system",
                "content": "You generate concise Conventional Commit messages. Return valid JSON only. Do not include Markdown or explanations.",
            },
            {
                "role": "user",
                "content": (
                    "Generate one Conventional Commit message for each commit below.\n"
                    "Return exactly this JSON shape: {\"messages\":[{\"id\":\"c000001\",\"message\":\"feat(scope): add thing\"}]}\n"
                    "Preserve every input id exactly once. Use lowercase messages, present tense, no trailing period.\n\n"
                    f"Commits:\n{json.dumps(commits, ensure_ascii=False)}"
                ),
            },
        ],
        "temperature": 0.2,
        "max_tokens": min(max(400, len(batch) * 160), 4000),
    }
    request = urllib.request.Request(
        chat_endpoint(base_url),
        data=json.dumps(payload).encode("utf-8"),
        headers={"Authorization": f"Bearer {api_key}", "Content-Type": "application/json"},
        method="POST",
    )
    try:
        with urllib.request.urlopen(request, timeout=timeout_seconds) as response:
            body = response.read().decode("utf-8")
    except urllib.error.HTTPError as exc:
        body = exc.read().decode("utf-8", errors="replace")
        raise RuntimeError(f"HTTP {exc.code}: {body[:500]}")
    except urllib.error.URLError as exc:
        raise RuntimeError(str(exc.reason))
    data = json.loads(body)
    content = data["choices"][0]["message"]["content"]
    return extract_messages(parse_json_response(content))


def process_batch(batch, base_url, model, api_key, timeout_seconds):
    accepted = {}
    pending = list(batch)
    errors = {}
    for attempt in range(1, 4):
        if not pending:
            break
        try:
            returned = request_batch(pending, base_url, model, api_key, timeout_seconds)
            next_pending = []
            for item in pending:
                message = returned.get(item["id"], "")
                if validate_message(message):
                    accepted[item["id"]] = message
                else:
                    errors[item["id"]] = "missing or invalid message"
                    next_pending.append(item)
            pending = next_pending
        except Exception as exc:
            errors = {item["id"]: str(exc) for item in pending}
        if pending and attempt < 3:
            print(f"{YELLOW}Retrying {len(pending)} commit(s) after failed batch attempt {attempt}.{RESET}")
            time.sleep(2 * attempt)
    failures = [{"item": item, "reason": errors.get(item["id"], "unknown error")} for item in pending]
    return accepted, failures


def process_items(items, base_url, model, api_key, batch_size, timeout_seconds):
    results = {}
    failures = []
    total_batches = (len(items) + batch_size - 1) // batch_size
    for batch_index, start in enumerate(range(0, len(items), batch_size), start=1):
        batch = items[start : start + batch_size]
        print(f"{CYAN}Generating batch {batch_index}/{total_batches} ({len(batch)} commit(s))...{RESET}")
        accepted, batch_failures = process_batch(batch, base_url, model, api_key, timeout_seconds)
        results.update(accepted)
        failures.extend(batch_failures)
    return results, failures


def write_outputs(items, results, stats, manifest_file, summary_file, output_dir):
    changed_by_repo = {}
    changed_samples = []
    unchanged_generated = 0
    for item in items:
        message = results.get(item["id"])
        if not message:
            continue
        if message == item["old_message"].strip():
            unchanged_generated += 1
            continue
        key = (item["repo_index"], item["repo_dir"], item["repo_name"])
        changed_by_repo.setdefault(key, []).append((item["hash"], message))
        if len(changed_samples) < 12:
            changed_samples.append((item["repo_name"], item["hash"][:8], message))
    manifest_lines = []
    for (repo_index, repo_dir, repo_name), mappings in sorted(changed_by_repo.items()):
        callback_file = os.path.join(output_dir, f"callback-{repo_index}.py")
        with open(callback_file, "w", encoding="utf-8") as f:
            f.write("mapping = {}\n")
            for commit_hash, message in mappings:
                f.write(f"mapping[{commit_hash.encode('utf-8')!r}] = {(message + chr(10)).encode('utf-8')!r}\n")
            f.write("if commit.original_id in mapping:\n")
            f.write("    commit.message = mapping[commit.original_id]\n")
        manifest_lines.append(f"{repo_dir}\t{repo_name}\t{callback_file}\t{len(mappings)}")
    with open(manifest_file, "w", encoding="utf-8") as f:
        if manifest_lines:
            f.write("\n".join(manifest_lines) + "\n")
    changed_count = sum(len(mappings) for mappings in changed_by_repo.values())
    summary = [
        "",
        "AI commit rewrite summary",
        "-------------------------",
        f"Repositories scanned: {stats['repo_count']}",
        f"Repositories with generated rewrites: {len(changed_by_repo)}",
        f"Total commits found: {stats['total_commits']}",
        f"Commits selected for processing: {len(items)}",
        f"Commits sent to API: {len(items)}",
        f"Generated rewrites: {changed_count}",
        f"Generated but unchanged: {unchanged_generated}",
        f"Skipped empty/unreadable commits: {stats['skipped_empty']}",
    ]
    if stats["skipped_formatted"] > 0:
        summary.append(f"Skipped already Conventional Commits: {stats['skipped_formatted']}")
    if stats["skipped_unborn"] > 0:
        summary.append(f"Skipped repositories with no commits: {stats['skipped_unborn']}")
    if changed_samples:
        summary.extend(["", "Sample generated messages:"])
        for repo_name, short_hash, message in changed_samples:
            summary.append(f"  {repo_name} {short_hash}: {message}")
    if changed_count == 0:
        summary.extend(["", "No generated messages require rewriting."])
    with open(summary_file, "w", encoding="utf-8") as f:
        f.write("\n".join(summary) + "\n")


def main():
    base_url = os.environ["AI_BASE_URL"]
    model = os.environ["AI_MODEL"]
    api_key = os.environ["AI_API_KEY"]
    repos_file = os.environ["AI_REPOS_FILE"]
    manifest_file = os.environ["AI_MANIFEST_FILE"]
    summary_file = os.environ["AI_SUMMARY_FILE"]
    output_dir = os.environ["AI_OUTPUT_DIR"]
    batch_size = int(os.environ["AI_BATCH_SIZE"])
    char_budget = int(os.environ["AI_MAX_CHARS_PER_COMMIT"])
    timeout_seconds = int(os.environ["AI_TIMEOUT_SECONDS"])
    skip_conventional = os.environ["AI_SKIP_CONVENTIONAL"] == "true"
    print(f"{CYAN}Scanning repositories and preparing redacted commit context...{RESET}")
    items, stats = collect_items(repos_file, char_budget, skip_conventional)
    if not items:
        open(manifest_file, "w", encoding="utf-8").close()
        with open(summary_file, "w", encoding="utf-8") as f:
            if skip_conventional:
                f.write("No commits require AI rewriting. Existing Conventional Commit messages were skipped.\n")
            else:
                f.write("No commits with usable file context were found for AI rewriting.\n")
        return
    batches = (len(items) + batch_size - 1) // batch_size
    print("")
    print(f"{YELLOW}Data send notice{RESET}")
    print(f"Endpoint: {base_url}")
    print(f"Model: {model}")
    print(f"Repositories scanned: {stats['repo_count']}")
    print(f"Total commits found: {stats['total_commits']}")
    print(f"Commits to process: {len(items)}")
    if skip_conventional:
        print(f"Skipped already Conventional Commits: {stats['skipped_formatted']}")
    print(f"API batches: {batches}")
    print(f"Per-commit context budget: {char_budget} characters")
    print("The command will send file paths, stats, and redacted diff snippets.")
    print("Old commit messages and API keys are not sent in commit context.")
    if not ask_yes_no("Send this data to the configured API endpoint? [y/N] "):
        print(f"{YELLOW}Stopped before sending any data.{RESET}")
        open(manifest_file, "w", encoding="utf-8").close()
        with open(summary_file, "w", encoding="utf-8") as f:
            f.write("Operation cancelled before any API requests were made.\n")
        return
    results, failures = process_items(items, base_url, model, api_key, batch_size, timeout_seconds)
    while failures:
        print(f"{YELLOW}{len(failures)} commit(s) failed after automatic retries.{RESET}")
        if not ask_yes_no("Retry only failed commits now? [y/N] "):
            fail("AI generation is incomplete. No history was changed.")
        retry_items = [failure["item"] for failure in failures]
        retry_results, failures = process_items(retry_items, base_url, model, api_key, batch_size, timeout_seconds)
        results.update(retry_results)
    write_outputs(items, results, stats, manifest_file, summary_file, output_dir)


if __name__ == "__main__":
    main()
