#!/bin/bash
# Usage: ./rename-branches.sh --oldbranch <old_name> --newbranch <new_name>
# Renames a specified branch to a new name across all managed Git repositories.

oldbranch=""
newbranch=""

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
            printf "\e[31mUnknown option: $1\e[0m\n"
            exit 1
            ;;
    esac
done

if [[ -z "$oldbranch" || -z "$newbranch" ]]; then
    printf "\e[31mError: Both --oldbranch and --newbranch options must be provided.\e[0m\n"
    exit 1
fi

if ! command -v git &> /dev/null; then
    printf "\e[31mError: 'git' is not installed. Please install it first.\e[0m\n"
    exit 1
fi

git_repositories=$(find . -maxdepth 2 -type d -name '.git')

if [ -z "$git_repositories" ]; then
    printf "\e[33mNo Git repositories found in the specified directory.\e[0m\n"
    exit 0
fi

echo "$git_repositories" | while read git_dir; do
    (
        repo_dir=$(dirname "$git_dir")

        if [ "$repo_dir" = "." ]; then
            repo_name_display="${PWD##*/}"
        else
            repo_name_display=$(basename "$repo_dir")
        fi

        if ! cd "$repo_dir" >/dev/null 2>&1; then
            printf "\e[31mError: Directory is inaccessible: $repo_name_display\e[0m\n"
            exit 1
        fi

        if ! git_output=$(git rev-parse --is-inside-work-tree 2>&1); then
            printf "\e[31mError: Not a valid git repository for $repo_name_display:\n$git_output\e[0m\n"
            exit 1
        fi

        if ! git rev-parse --verify --quiet "refs/heads/$oldbranch" >/dev/null 2>&1; then
            printf "\e[33mOld branch '$oldbranch' does not exist in $repo_name_display. Skipping...\e[0m\n"
            exit 0
        fi

        if git rev-parse --verify --quiet "refs/heads/$newbranch" >/dev/null 2>&1; then
            printf "\e[33mNew branch '$newbranch' already exists in $repo_name_display. Skipping...\e[0m\n"
            exit 0
        fi

        if rename_output=$(git branch -m "$oldbranch" "$newbranch" 2>&1); then
            printf "\e[32mBranch renamed from '$oldbranch' to '$newbranch' for $repo_name_display\e[0m\n"
        else
            printf "\e[31mError: Failed to rename branch in $repo_name_display:\n$rename_output\e[0m\n\n"
        fi
    )
done