#!/bin/bash

COMMIT=false

while [[ $# -gt 0 ]]; do
    case "$1" in
        --commit)
            COMMIT=true
            shift
            ;;
        *)
            printf "\e[31mUnknown option: $1\e[0m\n"
            exit 1
            ;;
    esac
done

if ! command -v git &> /dev/null; then
    printf "\e[31mError: 'git' is not installed. Please install it first.\e[0m\n"
    exit 1
fi

find . -maxdepth 2 -type d -name '.git' | while read -r git_dir; do
    (
        repo_dir=$(dirname "$git_dir")

        cd "$repo_dir" || exit

        if [ "$repo_dir" = "." ]; then
            repo_name_display="${PWD##*/}"
        else
            repo_name_display=$(basename "$repo_dir")
        fi

        if [ -f ".gitignore" ]; then
            ignored_files=$(git ls-files --ignored --exclude-standard -o -z)
            if [ -n "$ignored_files" ]; then
                git rm --cached -z $ignored_files
                if [ "$COMMIT" = true ]; then
                    git commit -m "Stop tracking files defined in .gitignore" -q
                    printf "\e[32mStopped tracking files defined in .gitignore for $repo_name_display. Commit performed.\e[0m\n"
                else
                    printf "\e[32mStopped tracking files defined in .gitignore for $repo_name_display. (No commit performed)\e[0m\n"
                fi
            else
                printf "\e[33mNo files tracked that were defined in .gitignore for $repo_name_display. Skipping...\e[0m\n"
            fi
        else
            printf "\e[33mNo .gitignore file found for $repo_name_display. Skipping...\e[0m\n"
        fi
    )
done
