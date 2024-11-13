#!/bin/bash

# ==============================================================================
# Usage: ./rewrite-commits.sh
# 
# Description:
# Rewrites the commit messages of Git repositories to adhere to the Conventional 
# Commits standard. It categorizes commits based on file paths and statuses 
# to automatically determine the type (e.g., feat, fix, docs, chore) and scope.
# ==============================================================================

# Function to determine conventional commit components based on file diffs
# This function reads 'git diff-tree --name-status' output from stdin and
# determines the conventional commit components based on file paths and statuses.
# Outputs: "<type>: <action> <target>"
# ==============================================================================
categorize_commit() {
    local first_file=""
    local has_docs=false
    local has_tests=false
    local has_config=false
    local has_src=false
    local additions=0
    local deletions=0
    local modifications=0

    while read -r status filepath; do
        if [[ -z "$status" ]]; then continue; fi
        
        # Grab first character of status (A, D, M, R, etc.)
        status="${status:0:1}"
        
        if [[ -z "$first_file" ]]; then
            first_file="$filepath"
        fi

        case "$status" in
            A) additions=$((additions + 1)) ;;
            D) deletions=$((deletions + 1)) ;;
            *) modifications=$((modifications + 1)) ;;
        esac

        # File classification
        if [[ "$filepath" =~ \.md$|\.txt$|\.rst$|^LICENSE|^docs/ ]]; then
            has_docs=true
        elif [[ "$filepath" =~ ^test/|^spec/|_test\.|spec\.|\.test\. ]]; then
            has_tests=true
        elif [[ "$filepath" =~ ^\.github/|^Makefile$|^Dockerfile$|\.yml$|^\w+\.json$ ]]; then
            has_config=true
        else
            has_src=true
        fi
    done

    # 1. Determine Type
    local type="chore"
    if [[ "$has_src" == false && "$has_config" == false && "$has_tests" == false && "$has_docs" == true ]]; then
        type="docs"
    elif [[ "$has_src" == false && "$has_config" == false && "$has_docs" == false && "$has_tests" == true ]]; then
        type="test"
    elif [[ "$has_src" == false && "$has_tests" == false && "$has_docs" == false && "$has_config" == true ]]; then
        type="chore"
    elif [[ "$additions" -gt 0 && "$deletions" -eq 0 && "$has_src" == true ]]; then
        type="feat"
    elif [[ "$deletions" -gt 0 && "$additions" -eq 0 && "$modifications" -eq 0 ]]; then
        type="chore"
    elif [[ "$has_src" == true && ( "$modifications" -gt 0 || ( "$additions" -gt 0 && "$deletions" -gt 0 ) ) ]]; then
        type="fix"
    else
        type="chore" # Anything else or ambiguous -> chore
    fi

    # 2. Determine Action & Target (Scope)
    # Scope is derived from the most-changed file or directory name
    local total_files=$((additions + deletions + modifications))
    
    # If no files were processed (e.g., empty commit), return empty to skip
    if [[ "$total_files" -eq 0 ]]; then
        return
    fi
    
    local target="$first_file"
    if [[ "$total_files" -gt 1 ]]; then
        target=$(dirname "$first_file")
        if [[ "$target" == "." ]]; then
            target=$(basename "$first_file")
        else
            target="$target/"
        fi
    fi

    local action="update"
    if [[ "$additions" -gt 0 && "$deletions" -eq 0 && "$modifications" -eq 0 ]]; then
        action="add"
    elif [[ "$deletions" -gt 0 && "$additions" -eq 0 && "$modifications" -eq 0 ]]; then
        action="remove"
    fi

    echo "${type}: ${action} ${target}"
}
# Parse command-line arguments
while [[ $# -gt 0 ]]; do
    case "$1" in
        *)
            printf "\e[31mUnknown option: %s\e[0m\n" "$1"
            exit 1
            ;;
    esac
done

# Check prerequisites
if ! command -v git &> /dev/null; then
    printf "\e[31mError: 'git' is not installed. Please install it first.\e[0m\n"
    exit 1
fi

if ! command -v git-filter-repo &> /dev/null; then
    printf "\e[31mError: 'git filter-repo' is not installed. Please install it first.\e[0m\n"
    exit 1
fi

# Find target repositories
git_repositories=$(find . -maxdepth 2 -type d -name '.git')

if [ -z "$git_repositories" ]; then
    printf "\e[33mNo Git repositories found in the specified directory.\e[0m\n"
    exit 0
fi

# Iterate through each repository
while IFS= read -r git_dir; do
    (
        # Get repository path and display name
        repo_path=$(dirname "$git_dir")

        if [ "$repo_path" = "." ]; then
            repo_name_display="${PWD##*/}"
        else
            repo_name_display=$(basename "$repo_path")
        fi

        cd "$repo_path" || exit

        # Check if repository actually has any commits
        if ! git rev-parse HEAD &> /dev/null; then
            printf "\e[33mRepository has no commits in %s. Skipping...\e[0m\n" "$repo_name_display"
            continue
        fi

        # Capture remote origin URL to restore later
        remote_url=$(git remote get-url origin 2>/dev/null)

        # Generate commit mapping for historical rewrite
        map_file=$(mktemp)
        needs_rewrite=false

        while read -r commit_hash; do
            original_msg=$(git log -1 --format="%B" "$commit_hash")
            
            # Check if commit message already complies with Conventional Commits
            # Only checking the first line for the typical structure: type(scope): prefix or type: prefix.
            first_line=$(echo "$original_msg" | head -n 1)
            if [[ "$first_line" =~ ^(feat|fix|docs|chore|test|build|ci|perf|refactor|style)(\(.*\))?:\  ]]; then
                continue
            fi

            # Check diff
            diff_output=$(git diff-tree --root --no-commit-id --name-status -r "$commit_hash" 2>/dev/null)
            if [[ -z "$diff_output" ]]; then
                # Empty commit (touches no files), keep original message unchanged
                continue
            fi
            
            new_msg=$(echo "$diff_output" | categorize_commit)
            if [[ -n "$new_msg" && "$new_msg" != "$original_msg" ]]; then
                # Output format: <commit-sha>|<new-message>
                echo "${commit_hash}|${new_msg}" >> "$map_file"
                needs_rewrite=true
            fi
        done < <(git rev-list --all)

        if [ "$needs_rewrite" = false ]; then
            printf "\e[33mNo commits require rewriting in %s (already format compliant). Skipping...\e[0m\n" "$repo_name_display"
            rm -f "$map_file"
            continue
        fi

        # Convert temp-file paths for Python on Git Bash for Windows.
        python_map_file="$map_file"
        if command -v cygpath >/dev/null 2>&1; then
            python_map_file=$(cygpath -am "$map_file")
        fi

        # Prepare callback script for git-filter-repo.
        # Passing a callback file is more reliable here than an inline multiline string.
        callback_file=$(mktemp)
        cat > "$callback_file" <<EOF
import os
map_file = '$python_map_file'
mapping = {}
if os.path.exists(map_file):
    with open(map_file, 'r', encoding='utf-8') as f:
        for line in f:
            parts = line.rstrip('\n').split('|', 1)
            if len(parts) == 2:
                mapping[parts[0].encode('utf-8')] = parts[1].encode('utf-8') + b'\n'

if commit.original_id in mapping:
    commit.message = mapping[commit.original_id]
EOF

        # Execute historical rewrite using git-filter-repo
        if filter_output=$(git filter-repo --partial --commit-callback "$callback_file" --force 2>&1); then
            printf "\e[32mRewrote commit messages for %s\e[0m\n" "$repo_name_display"
            
            # Restore original remote origin if applicable
            if [ -n "$remote_url" ]; then
                git remote add origin "$remote_url" 2>/dev/null
            fi
        else
            printf "\e[31mError: Could not update commit messages for %s:\n%s\e[0m\n\n" "$repo_name_display" "$filter_output"
        fi

        rm -f "$map_file" "$callback_file"

    )
done <<< "$git_repositories"
