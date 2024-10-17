#!/bin/bash

# ==============================================================================
# Usage: ./rewrite-authors.sh --name <new_name> --email <new_email> [--force] [--repo <repository_name>]
# 
# Description:
# Iterates through Git repositories found in the current directory and its 
# immediate subdirectories, updates author and committer information, with 
# optional force mode, allowing users to specify a new name and email.
# ==============================================================================

force=false
repo=""
NEW_NAME=""
NEW_EMAIL=""

# Parse command-line arguments
while [[ $# -gt 0 ]]; do
    case "$1" in
        --name)
            NEW_NAME="$2"
            shift 2
            ;;
        --email)
            NEW_EMAIL="$2"
            shift 2
            ;;
        --force)
            force=true
            shift
            ;;
        --repo)
            repo="$2"
            shift 2
            ;;
        *)
            printf "\e[31mUnknown option: $1\e[0m\n"
            exit 1
            ;;
    esac
done

# Validate required arguments
if [[ -z "$NEW_NAME" || -z "$NEW_EMAIL" ]]; then
    printf "\e[31mError: Both --name and --email options must be provided.\e[0m\n"
    exit 1
fi

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
if [ -n "$repo" ]; then
    repos=$(find "$repo" -maxdepth 2 -type d -name '.git')
else
    repos=$(find . -maxdepth 2 -type d -name '.git')
fi

if [ -z "$repos" ]; then
    printf "\e[33mNo Git repositories found in the specified directory.\e[0m\n"
    exit 0
fi

# Iterate through each repository
echo "$repos" | while read repo; do
    (
        # Get repository directory and name
        repo_dir=$(dirname "$repo")

        if [ "$repo_dir" = "." ]; then
            repo_name_display="${PWD##*/}"
        else
            repo_name_display=$(basename "$repo_dir")
        fi

        cd "$repo_dir" || exit

        # Update author and committer information using git-filter-repo
        if [ "$force" = true ]; then
            if error_message=$(git filter-repo --partial --force --email-callback "return b'$NEW_EMAIL'" --name-callback "return b'$NEW_NAME'" 2>&1 >/dev/null); then
                printf "\e[32mAuthor and commiter information updated for $repo_name_display\e[0m\n"
            else
                printf "\e[31mError: Could not update git author and commiter information for $repo_name_display:\n$error_message\e[0m\n\n"
            fi
        else
            if error_message=$(git filter-repo --partial --email-callback "return b'$NEW_EMAIL'" --name-callback "return b'$NEW_NAME'" 2>&1 >/dev/null); then
                printf "\e[32mAuthor and commiter information updated for $repo_name_display\e[0m\n"
            else
                printf "\e[31mError: Could not update git author and commiter information for $repo_name_display:\n$error_message\e[0m\n\n"
            fi
        fi
    )
done