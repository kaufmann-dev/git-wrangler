#!/bin/bash

force=false

while [[ $# -gt 0 ]]; do
  case "$1" in
    --force)
      force=true
      shift
      ;;
    *)
      echo "Unknown option: $1"
      exit 1
      ;;
  esac
done

find . -maxdepth 2 -type d -name '.git' | while read -r git_dir; do
    repo_path=$(dirname "$git_dir")

    if [ "$repo_path" = "." ]; then
        repo_name_display="${PWD##*/}"
    else
        repo_name_display=$(basename "$repo_path")
    fi

    cd "$repo_path" || exit

    if [ "$force" = true ]; then
        if git push --force; then
            echo "\e[32mGit push --force completed for repository: $repo_name_display\e[0m"
        else
            echo "\e[31mGit push --force failed for repository: $repo_name_display: $(git push --force 2>&1 | tail -n 1)\e[0m"
        fi
    else
        if git push; then
            echo "\e[32mGit push completed for repository: $repo_name_display\e[0m"
        else
            echo "\e[31mGit push failed for repository: $repo_name_display: $(git push 2>&1 | tail -n 1)\e[0m"
        fi
    fi
done
