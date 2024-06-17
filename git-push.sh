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

git_repositories=$(find . -maxdepth 2 -type d -name '.git')

if [ -z "$git_repositories" ]; then
    printf "\e[33mNo Git repositories found in the specified directory.\e[0m\n"
    exit 0
fi

echo "$git_repositories" | while read git_dir; do
    (
        repo_path=$(dirname "$git_dir")

        if [ "$repo_path" = "." ]; then
            repo_name_display="${PWD##*/}"
        else
            repo_name_display=$(basename "$repo_path")
        fi

        cd "$repo_path" || exit

        if [ "$force" = true ]; then
            push_output=$(git push --force origin HEAD 2>&1)
        else
            push_output=$(git push origin HEAD 2>&1)
        fi

        push_output=$(echo "$push_output" | tr '\n' ' ' | sed 's/ \{2,\}/ /g')

        if [ "$?" -eq 0 ]; then
            if ! printf "$push_output" | grep -q "Everything up-to-date"; then
                printf "\e[32mGit push completed for $repo_name_display\e[0m\n"
            else
                printf "\e[33mNo changes to push for $repo_name_display. Skipping...\e[0m\n"
            fi
        else
            printf "\e[31mGit push failed for $repo_name_display: $push_output\e[0m\n"
        fi
    )
done
