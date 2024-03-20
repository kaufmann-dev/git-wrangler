#!/bin/bash

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
            echo "Unknown option: $1"
            exit 1
            ;;
    esac
done

if [ -z "${oldbranch+x}" ] || [ -z "${newbranch+x}" ]; then
    echo "Error: Both --oldbranch and --newbranch options must be provided."
    echo "Usage: $0 --oldbranch <old_branch_name> --newbranch <new_branch_name>"
    exit 1
fi

find . -maxdepth 2 -type d -name '.git' | while read git_dir; do
    repo_name=$(dirname "$git_dir")

    if [ "$repo_name" = "." ]; then
        repo_name_display="${PWD##*/}"
    else
        repo_name_display=$(basename "$repo_name")
    fi

    cd "$repo_name" || exit

    if git rev-parse --verify --quiet "refs/heads/$oldbranch" > /dev/null; then
        git branch -m "$oldbranch" "$newbranch"
        echo -e "\e[32mBranch $oldbranch im Repository $repo_name_display umbenannt zu $newbranch.\e[0m"
    else
        echo -e "\e[33mIm Repository $repo_name_display existiert kein Branch $oldbranch.\e[0m"
    fi

    cd .. || exit
done