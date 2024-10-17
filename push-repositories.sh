#!/bin/bash

# ==============================================================================
# Usage: ./push-repositories.sh [--force]
# 
# Description:
# Iterates through Git repositories found in the current directory and its 
# immediate subdirectories, checks if there are changes to push, and performs 
# a Git push operation with optional force flag.
# ==============================================================================

force=false

# Parse command-line arguments
while [[ $# -gt 0 ]]; do
  case "$1" in
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

        # Perform Git push based on force flag
        if [ "$force" = true ]; then
            if push_output=$(git push --force origin HEAD 2>&1); then
                if ! printf "$push_output" | grep -q "Everything up-to-date"; then
                    printf "\e[32mGit push completed for $repo_name_display\e[0m\n"
                else
                    printf "\e[33mNo changes to push for $repo_name_display. Skipping...\e[0m\n"
                fi
            else
                printf "\e[31mError: Git push failed for $repo_name_display:\n$push_output\e[0m\n\n"
            fi
        else
            if push_output=$(git push origin HEAD 2>&1); then
                if ! printf "$push_output" | grep -q "Everything up-to-date"; then
                    printf "\e[32mGit push completed for $repo_name_display\e[0m\n"
                else
                    printf "\e[33mNo changes to push for $repo_name_display. Skipping...\e[0m\n"
                fi
            else
                printf "\e[31mError: Git push failed for $repo_name_display:\n$push_output\e[0m\n\n"
            fi
        fi
    )
done
