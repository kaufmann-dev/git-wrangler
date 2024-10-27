#!/bin/bash

# ==============================================================================
# Usage: ./pull-repositories.sh [--rebase] [--force]
# 
# Description:
# Iterates through Git repositories found in the current directory and its 
# immediate subdirectories, and performs a Git pull operation to fetch and 
# integrate changes from the remote repository.
# ==============================================================================

rebase=false
force=false

# Parse command-line arguments
while [[ $# -gt 0 ]]; do
  case "$1" in
    --rebase)
      rebase=true
      shift
      ;;
    --force)
      force=true
      shift
      ;;
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

        # Build pull arguments
        pull_args=""
        if [ "$rebase" = true ]; then
            pull_args="$pull_args --rebase"
        fi
        if [ "$force" = true ]; then
            pull_args="$pull_args --force"
        fi

        # Perform Git pull
        if pull_output=$(git pull$pull_args 2>&1); then
            if ! printf "$pull_output" | grep -q "Already up to date"; then
                printf "\e[32mGit pull completed for $repo_name_display\e[0m\n"
            else
                printf "\e[33mAlready up to date for $repo_name_display. Skipping...\e[0m\n"
            fi
        else
            printf "\e[31mError: Git pull failed for $repo_name_display:\n$pull_output\e[0m\n\n"
        fi
    )
done
