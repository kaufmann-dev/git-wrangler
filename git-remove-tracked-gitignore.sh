#!/bin/bash

find . -maxdepth 2 -type f -name '.gitignore' | while read -r gitignore_file; do
    repo_dir=$(dirname "$gitignore_file")

    cd "$repo_dir" || exit

    if [ "$repo_name" = "." ]; then
        repo_name_display="${PWD##*/}"
    else
        repo_name_display=$(basename "$repo_name")
    fi

    ignored_files=$(git ls-files --ignored --exclude-standard -o -z)
    if [ -n "$ignored_files" ]; then
        git rm --cached -z $ignored_files
        git commit -m "Stop tracking files defined in .gitignore" -q
        echo -e "\e[32mStopped tracking files defined in .gitignore for $repo_name_display\e[0m"
    else
        echo -e "\e[33mNo files tracked that were defined in .gitignore for $repo_name_display\e[0m"
    fi
done