#!/bin/bash

# ==============================================================================
# Usage: ./untrack-ignored-files.sh
# 
# Description:
# Removes files from the Git index that are actively tracked but match 
# exclusion rules in .gitignore. It untracks the files safely while leaving 
# them on the local disk, and commits the removals automatically.
# ==============================================================================

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
        # Get repository directory and name
        repo_dir=$(dirname "$git_dir")

        if [ "$repo_dir" = "." ]; then
            repo_name_display="${PWD##*/}"
        else
            repo_name_display=$(basename "$repo_dir")
        fi

        cd "$repo_dir" || exit

        # Stop tracking files that match .gitignore patterns
        if [ -f ".gitignore" ]; then
            # Identify tracked files matching ignore patterns
            if git ls-files --ignored --cached --exclude-standard | grep -q .; then
                # Untrack and commit ignored files
                if error_message=$(git ls-files --ignored --cached --exclude-standard -z | xargs -0 -r git rm --cached -q -- 2>&1); then
                    if commit_output=$(git commit -q -m "Stop tracking files defined in .gitignore" 2>&1); then
                        printf "\e[32mStopped tracking and committed ignored files for $repo_name_display\e[0m\n"
                    else
                        printf "\e[31mError: Could not commit changes for $repo_name_display:\n$commit_output\e[0m\n\n"
                    fi
                else
                    printf "\e[31mError: Could not untrack files for $repo_name_display:\n$error_message\e[0m\n\n"
                fi
            else
                printf "\e[33mNo currently tracked files match .gitignore in $repo_name_display. Skipping...\e[0m\n"
            fi
        else
            printf "\e[33mNo .gitignore file found for $repo_name_display. Skipping...\e[0m\n"
        fi
    )
done
