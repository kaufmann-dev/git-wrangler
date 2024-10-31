#!/bin/bash

force=false

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

repos=$(find . -maxdepth 2 -type d -name ".git")

for repo in $repos; do
    (
        repo_dir=$(dirname "$repo")

        if [ "$repo_dir" = "." ]; then
            repo_name_display="${PWD##*/}"
        else
            repo_name_display=$(basename "$repo_dir")
        fi

        cd "$repo_dir" || exit

        if [ "$force" = true ]; then
            error_message=$(git filter-repo --partial --force --email-callback "return b'$NEW_EMAIL'" --name-callback "return b'$NEW_NAME'" 2>&1 >/dev/null)
        else
            error_message=$(git filter-repo --partial --email-callback "return b'$NEW_EMAIL'" --name-callback "return b'$NEW_NAME'" 2>&1 >/dev/null)
        fi

        if [ $? -ne 0 ]; then
            printf "\e[31mError: Could not update git author and commiter information for $repo_name_display: $(echo "$error_message" | tr '\n' ' ' | sed 's/ \{2,\}/ /g')\e[0m\n"
            continue
        fi

        printf "\e[32mAuthor and commiter information updated for $repo_name_display\e[0m\n"

        cd .. || exit
    )
done