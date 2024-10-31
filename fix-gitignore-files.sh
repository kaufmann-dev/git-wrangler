#!/bin/bash

# ==============================================================================
# Usage: ./fix-gitignore-files.sh
# 
# Description:
# Audits and fixes .gitignore files across Git repositories found in the 
# current directory and its immediate subdirectories. Adds missing entries 
# for tracked files that match common candidates (build artifacts, 
# dependencies, IDE files, etc.) but are not yet covered by .gitignore.
# ==============================================================================

# Candidate entries to audit and add to .gitignore if needed
CANDIDATE_ENTRIES=(
    "bin/"
    "obj/"
    ".idea/"
    "vendor/"
    "node_modules/"
    "dist/"
    "build/"
    "wp-includes/"
    ".DS_Store"
    "Thumbs.db"
    "*.log"
)

# Parse command-line arguments
while [[ $# -gt 0 ]]; do
    case "$1" in
        *)
            printf "\e[31mUnknown option: $1\e[0m\n"
            exit 1
            ;;
    esac
done

# Check prerequisites
if ! command -v git &> /dev/null; then
    printf "\e[31mError: 'git' is not installed. Please install it first.\e[0m\n"
    exit 1
fi

# Find target repositories
git_repositories=$(find . -maxdepth 2 -type d -name '.git')

if [ -z "$git_repositories" ]; then
    printf "\e[33mNo Git repositories found in the specified directory.\e[0m\n"
    exit 0
fi

# Iterate through each repository
echo "$git_repositories" | while read git_dir; do
    (
        # Get repository path and display name
        repo_path=$(dirname "$git_dir")

        if [ "$repo_path" = "." ]; then
            repo_name_display="${PWD##*/}"
        else
            repo_name_display=$(basename "$repo_path")
        fi

        cd "$repo_path" || exit

        added=()
        skipped_covered=()
        skipped_untracked=()

        for entry in "${CANDIDATE_ENTRIES[@]}"; do
            # Build appropriate pathspec for git ls-files
            case "$entry" in
                */)
                    # Directory pattern: match all tracked files under that directory
                    ls_files_pathspec="$entry"
                    ;;
                *)
                    # File or glob pattern: match recursively across all directories
                    ls_files_pathspec=":(glob)**/$entry"
                    ;;
            esac

            # Check if any files matching this entry are tracked in the index
            tracked_files=$(git ls-files --cached -- "$ls_files_pathspec" 2>/dev/null)

            if [ -z "$tracked_files" ]; then
                # Not tracked at all, nothing to ignore
                skipped_untracked+=("$entry")
                continue
            fi

            # Check if tracked files are already covered by .gitignore rules.
            # Limit to first 10 files and use check-ignore with -n (show non-ignored).
            # Returns 0 if at least one tracked file is NOT ignored.
            if printf '%s\n' "$tracked_files" | head -10 | git check-ignore --stdin -n -q 2>/dev/null; then
                # At least one tracked file is not yet covered.
                # Double-check: is this literal entry already present in .gitignore?
                # Catches cases where files remain tracked despite existing gitignore entry.
                if [ -f ".gitignore" ] && grep -qxF "$entry" .gitignore 2>/dev/null; then
                    skipped_covered+=("$entry")
                else
                    added+=("$entry")
                fi
            else
                # All checked tracked files are already ignored
                skipped_covered+=("$entry")
            fi
        done

        # Write new entries to .gitignore if any need to be added
        if [ ${#added[@]} -gt 0 ]; then
            # Ensure .gitignore ends with a newline if it already has content
            if [ -f ".gitignore" ] && [ -s ".gitignore" ] && [ "$(tail -c 1 .gitignore | wc -l)" -eq 0 ]; then
                printf '\n' >> .gitignore
            fi

            for entry in "${added[@]}"; do
                printf '%s\n' "$entry" >> .gitignore
            done
        fi

        # Print per-repository summary
        printf "\e[1;34m%s:\e[0m\n" "$repo_name_display"

        if [ ${#added[@]} -gt 0 ]; then
            added_list=$(printf '%s, ' "${added[@]}")
            printf "  \e[32mAdded:\e[0m %s\n" "${added_list%, }"
        fi

        if [ ${#skipped_covered[@]} -gt 0 ]; then
            covered_list=$(printf '%s, ' "${skipped_covered[@]}")
            printf "  \e[33mSkipped (tracked but covered):\e[0m %s\n" "${covered_list%, }"
        fi

        if [ ${#skipped_untracked[@]} -gt 0 ]; then
            untracked_list=$(printf '%s, ' "${skipped_untracked[@]}")
            printf "  \e[33mSkipped (not tracked):\e[0m %s\n" "${untracked_list%, }"
        fi

        if [ ${#added[@]} -eq 0 ] && [ ${#skipped_covered[@]} -eq 0 ] && [ ${#skipped_untracked[@]} -eq 0 ]; then
            printf "  \e[33mNo changes needed.\e[0m\n"
        fi
    )
done
