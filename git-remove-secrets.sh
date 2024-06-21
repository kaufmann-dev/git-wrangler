#!/bin/bash

default_secrets=(
    "appsettings.json"
    ".env"
)

while [[ $# -gt 0 ]]; do
    case "$1" in
        --secrets)
            shift
            secrets_file="$1"
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
    shift
done

if ! command -v git &> /dev/null; then
    printf "\e[31mError: 'git' is not installed. Please install it first.\e[0m\n"
    exit 1
fi

if ! command -v git-filter-repo &> /dev/null; then
    printf "\e[31mError: 'git filter-repo' is not installed. Please install it first.\e[0m\n"
    exit 1
fi

if [ -n "$secrets_file" ]; then
    if [ -e "$secrets_file" ]; then
        IFS=$'\n' read -d '' -r -a default_secrets < "$secrets_file"
    else
        printf "Specified secrets file not found: $secrets_file\n"
        exit 1
    fi
fi

for repo in $(find . -maxdepth 2 -type d -name '.git'); do
    repo_path=$(dirname "$repo")

    if [ "$repo_dir" = "." ]; then
        repo_name_display="${PWD##*/}"
    else
        repo_name_display=$(basename "$repo_dir")
    fi
    
    secret_files=""
    for secret in "${default_secrets[@]}"; do
        secret_files+=" -o -name '$secret'"
    done
    secret_files=${secret_files:3}  # Remove leading " -o"

    found_secret_files=$(find "$repo_path" -type f \( $secret_files \))
    
    if [ -n "$found_secret_files" ]; then
        if [ "$force" = true ]; then
            error_message=$(git -C "$repo_path" filter-repo --path $found_secret_files --invert-paths --force 2>&1 >/dev/null)
        else
            error_message=$(git -C "$repo_path" filter-repo --path $found_secret_files --invert-paths 2>&1 >/dev/null)
        fi

        if [ $? -ne 0 ]; then
            printf "\e[31mError: Unable to remove secret files from $repo_name_display:\n$error_message\e[0m\n"
            continue
        fi

        printf "\e[32mSecret files successfully removed from $repo_name_display\e[0m\n"

    else
        printf "\e[33mNo secret files found in $repo_name_display. Skipping...\e[0m\n"
    fi
done
