#!/bin/bash

# ==============================================================================
# Usage: ./remove-secrets.sh
# 
# Description:
# Permanently purges files containing sensitive data from the entire Git history
# of all managed repositories (across all branches and tags). It operates 
# on all '.git' repositories found within a depth of 2.
# ==============================================================================

# ==============================================================================
# EDITABLE PATTERNS BLOCK
# ==============================================================================
TARGET_PATTERNS=(
    ".env"
    ".env.*"
    "*.pem"
    "*.key"
    "*.p12"
    "*.pfx"
    "id_rsa"
    "id_rsa.pub"
    "id_ed25519"
    "id_ed25519.pub"
    "config.json"
    "secrets.json"
    "credentials.json"
    "*.secret"
)
# ==============================================================================

if ! command -v git &> /dev/null; then
    printf "\e[31mError: 'git' is not installed. Please install it first.\e[0m\n"
    exit 1
fi

git_repositories=$(find . -maxdepth 2 -type d -name '.git')

if [ -z "$git_repositories" ]; then
    printf "\e[33mNo Git repositories found in the specified directory.\e[0m\n"
    exit 0
fi

echo "$git_repositories" | while read git_dir; do
    (
        repo_path=$(dirname "$git_dir")

        if [ "$repo_path" = "." ]; then
            repo_name_display="${PWD##*/}"
        else
            repo_name_display=$(basename "$repo_path")
        fi

        # Check repository accessibility
        if ! cd "$repo_path" 2>/dev/null || ! git rev-parse --is-inside-work-tree &> /dev/null; then
            printf "\e[31mError: $repo_name_display is not a valid or accessible git repository. Skipping...\e[0m\n"
            exit 1
        fi

        # Check for git-filter-repo per repository as requested by edge cases
        if ! command -v git-filter-repo &> /dev/null; then
            printf "\e[31mError: 'git-filter-repo' is not installed. Skipping $repo_name_display...\e[0m\n"
            exit 1
        fi

        # Scan for matched patterns anywhere in the history
        matched_patterns=()
        for pattern in "${TARGET_PATTERNS[@]}"; do
            if matches=$(git log --all --oneline -- "$pattern" 2>/dev/null | head -n 1) && [ -n "$matches" ]; then
                matched_patterns+=("$pattern")
            fi
        done

        if [ ${#matched_patterns[@]} -eq 0 ]; then
            printf "\e[33mNo target patterns found in history. Skipping $repo_name_display cleanly...\e[0m\n"
            exit 0
        fi

        filter_repo_args=()
        for pattern in "${matched_patterns[@]}"; do
            filter_repo_args+=(--path-glob "$pattern")
        done

        # Capture remote origin URL before rewriting (filter-repo drops it)
        remote_url=$(git remote get-url origin 2>/dev/null)

        # Execute rewrite
        # We pass --force to git-filter-repo to bypass the fresh-clone requirement. The script is
        # explicitly intended to run on the current working repositories without user friction.
        if error_message=$(git filter-repo "${filter_repo_args[@]}" --invert-paths --use-base-name --force 2>&1 >/dev/null); then
            printf "\e[32mSuccessfully purged sensitive files from $repo_name_display\e[0m\n"
        else
            printf "\e[31mError: Rewrite failed for $repo_name_display:\n$error_message\e[0m\n\n"
            exit 1
        fi

        # Re-add remote if there was one
        if [ -n "$remote_url" ]; then
            git remote add origin "$remote_url" 2>/dev/null
        fi
    )
done