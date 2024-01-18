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
            echo "Unknown option: $1"
            exit 1
            ;;
    esac
done

if [[ -z "$NEW_NAME" || -z "$NEW_EMAIL" ]]; then
    echo "Error: Both --name and --email options must be provided."
    exit 1
fi

repos=$(find . -mindepth 1 -maxdepth 2 -type d -name ".git")

for repo in $repos; do
    echo "Updating repository: $repo"

    repo_dir=$(dirname "$repo")
    cd "$repo_dir" || exit

    git filter-repo --partial --email-callback "return b'$NEW_EMAIL'" \
                    --name-callback "return b'$NEW_NAME'"

    git push --force

    cd - || exit
done

echo "Author and committer information updated for all repositories."