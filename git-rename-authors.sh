#!/bin/bash

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

repos=$(find . -mindepth 1 -maxdepth 2 -type d -name ".git")

for repo in $repos; do
    (
        printf "\e[32mAuthor and commiter information updated for $repo\e[0m\n"

        repo_dir=$(dirname "$repo")
        cd "$repo_dir" || exit

        git filter-repo --partial --email-callback "return b'$NEW_EMAIL'" \
                        --name-callback "return b'$NEW_NAME'"

        cd - || exit
    )
done