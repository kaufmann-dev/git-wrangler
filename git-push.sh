#!/bin/bash

force=false

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

if ! command -v git &> /dev/null; then
    printf "\e[31mError: 'git' is not installed. Please install it first.\e[0m\n"
    exit 1
fi

find . -maxdepth 2 -type d -name '.git' | while read -r git_dir; do
    repo_path=$(dirname "$git_dir")

    if [ "$repo_path" = "." ]; then
        repo_name_display="${PWD##*/}"
    else
        repo_name_display=$(basename "$repo_path")
    fi

    cd "$repo_path" || exit

    if git diff-index --quiet HEAD --; then
        printf "\e[33mNo changes to push for $repo_name_display. Skipping...\e[0m\n"
    else
        if [ "$force" = true ]; then
            if git push --force; then
                printf "\e[32mGit push --force completed for $repo_name_display\e[0m\n"
            else
                printf "\e[31mGit push --force failed for $repo_name_display: $(git push --force 2>&1 | tail -n 1)\e[0m\n"
            fi
        else
            if git push; then
                printf "\e[32mGit push completed for $repo_name_display\e[0m\n"
            else
                printf "\e[31mGit push failed for $repo_name_display: $(git push 2>&1 | tail -n 1)\e[0m\n"
            fi
        fi
    fi
done
