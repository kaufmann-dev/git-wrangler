#!/bin/bash

# ==============================================================================
# Usage: ./rename-branches.sh --oldbranch <old_name> --newbranch <new_name>
# 
# Description:
# Renames a specified branch to a new name across all managed Git repositories.
# ==============================================================================

oldbranch=""
newbranch=""

# Parse command-line arguments
while [[ $# -gt 0 ]]; do
    case "$1" in
        --oldbranch)
            oldbranch="$2"
            shift 2
            ;;
        --newbranch)
            newbranch="$2"
            shift 2
            ;;
        *)
            printf "\e[31mUnknown option: %s\e[0m\n" "$1"
            exit 1
            ;;
    esac
done

# Validate required arguments
if [[ -z "$oldbranch" || -z "$newbranch" ]]; then
    printf "\e[31mError: Both --oldbranch and --newbranch options must be provided.\e[0m\n"
    exit 1
fi

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
while IFS= read -r git_dir; do
    (
        # Get repository directory and name
        repo_dir=$(dirname "$git_dir")

        if [ "$repo_dir" = "." ]; then
            repo_name_display="${PWD##*/}"
        else
            repo_name_display=$(basename "$repo_dir")
        fi

        # Validate Git repository accessibility
        if ! cd "$repo_dir" >/dev/null 2>&1; then
            printf "\e[31mError: Directory is inaccessible: %s\e[0m\n" "$repo_name_display"
            exit 1
        fi

        if ! git_output=$(git rev-parse --is-inside-work-tree 2>&1); then
            printf "\e[31mError: Not a valid git repository for %s:\n%s\e[0m\n" "$repo_name_display" "$git_output"
            exit 1
        fi

        # Check if the old branch exists
        if ! git rev-parse --verify --quiet "refs/heads/$oldbranch" >/dev/null 2>&1; then
            printf "\e[33mOld branch '%s' does not exist in %s. Skipping...\e[0m\n" "$oldbranch" "$repo_name_display"
            exit 0
        fi

        # Check if the new branch name is already taken
        if git rev-parse --verify --quiet "refs/heads/$newbranch" >/dev/null 2>&1; then
            printf "\e[33mNew branch '%s' already exists in %s. Skipping...\e[0m\n" "$newbranch" "$repo_name_display"
            exit 0
        fi

        # Perform branch rename
        if rename_output=$(git branch -m "$oldbranch" "$newbranch" 2>&1); then
            printf "\e[32mBranch renamed from '%s' to '%s' for %s\e[0m\n" "$oldbranch" "$newbranch" "$repo_name_display"
        else
            printf "\e[31mError: Failed to rename branch in %s:\n%s\e[0m\n\n" "$repo_name_display" "$rename_output"
        fi
    )
done <<< "$git_repositories"