#!/bin/bash

force=false
repo=""
NEW_NAME=""
NEW_EMAIL=""

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

if [[ -z "$NEW_NAME" || -z "$NEW_EMAIL" ]]; then
    printf "\e[31mError: Both --name and --email options must be provided.\e[0m\n"
    exit 1
fi

if ! command -v git &> /dev/null; then
    printf "\e[31mError: 'git' is not installed. Please install it first.\e[0m\n"
    exit 1
fi

if ! command -v git-filter-repo &> /dev/null; then
    printf "\e[31mError: 'git filter-repo' is not installed. Please install it first.\e[0m\n"
    exit 1
fi

if [ -n "$repo" ]; then
    repos=$(find "$repo" -maxdepth 2 -type d -name '.git')
else
    repos=$(find . -maxdepth 2 -type d -name '.git')
fi

if [ -z "$repos" ]; then
    printf "\e[33mNo Git repositories found in the specified directory.\e[0m\n"
    exit 0
fi

echo "$repos" | while read repo; do
    (
        repo_dir=$(dirname "$repo")

        if [ "$repo_dir" = "." ]; then
            repo_name_display="${PWD##*/}"
        else
            repo_name_display=$(basename "$repo_dir")
        fi

        cd "$repo_dir" || exit

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