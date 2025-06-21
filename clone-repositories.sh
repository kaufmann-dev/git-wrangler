#!/bin/bash

visibility="all"
user=""
limit=100
into=""

while [[ "$#" -gt 0 ]]; do
    case $1 in
        --visibility)
            visibility="$2"
            shift 2
            ;;
        --user)
            user="$2"
            shift 2
            ;;
        --limit)
            limit="$2"
            shift 2
            ;;
        --into)
            into="$2"
            shift 2
            ;;
        *)
            printf "\e[31mUnknown option: $1\e[0m\n"
            exit 1
            ;;
    esac
done

if [[ -z "$user" ]]; then
    printf "\e[31mError: The --user option is required.\e[0m\n"
    exit 1
fi

if ! command -v gh &> /dev/null; then
    printf "\e[31mError: 'gh' (GitHub CLI) is not installed. Please install it first.\e[0m\n"
    exit 1
fi

if ! command -v git &> /dev/null; then
    printf "\e[31mError: 'git' is not installed. Please install it first.\e[0m\n"
    exit 1
fi

if [[ ! "$limit" =~ ^[1-9][0-9]*$ ]]; then
    printf "\e[31mError: --limit must be 1 or greater.\e[0m\n"
    exit 1
fi

if [[ "$visibility" == "private" || "$visibility" == "all" ]]; then
    if ! gh auth status | grep -q "Logged in to .* account $user "; then
        printf "\e[31mError: You are not logged in as the specified user: $user. Set --visibility to 'public' or use 'gh auth login'.\e[0m\n"
        exit 1
    fi
fi

if [[ "$visibility" == "private" || "$visibility" == "public" ]]; then
    if [[ "$(gh repo list "$user" --limit 1 --visibility "$visibility" 2>/dev/null | wc -l)" -eq 0 ]]; then
        printf "\e[31mError: No $visibility repositories found for '$user'.\e[0m\n"
        exit 1
    fi
else
    if [[ "$(gh repo list "$user" --limit 1 2>/dev/null | wc -l)" -eq 0 ]]; then
        printf "\e[31mError: No repositories found for '$user'.\e[0m\n"
        exit 1
    fi
fi

if [[ "$visibility" != "all" && "$visibility" != "public" && "$visibility" != "private" ]]; then
    printf "\e[31mError: Invalid visibility option. Use 'all', 'public', or 'private'.\e[0m\n"
    exit 1
fi

if [[ -z "$into" ]]; then
    into="$user"
fi

if [[ -d "$into" && ! -w "$into" ]] || ! mkdir -p "$into"; then
    printf "\e[31mError: Unable to create or access the specified directory '$into'.\e[0m\n"
    exit 1
fi

if [[ "$visibility" == "public" || "$visibility" == "private" ]]; then
    while read -r repo _; do
        repo_name=$(basename "$repo")
        if [[ -d "$into/$repo_name" ]]; then
            printf "\e[33m$repo_name already exists in $(realpath "$into"). Skipping...\e[0m\n"
        else
            gh repo clone "$repo" "$into/$repo_name" > /dev/null 2>&1
            printf "\e[32mCloned $repo_name into $(realpath "$into/$repo_name")\e[0m\n"
        fi
    done < <(gh repo list "$user" --visibility "$visibility" --limit "$limit")
else
    while read -r repo _; do
        repo_name=$(basename "$repo")
        if [[ -d "$into/$repo_name" ]]; then
            printf "\e[33m$repo_name already exists in $(realpath "$into"). Skipping...\e[0m\n"
        else
            gh repo clone "$repo" "$into/$repo_name" > /dev/null 2>&1
            printf "\e[32mCloned $repo_name into $(realpath "$into/$repo_name")\e[0m\n"
        fi
    done < <(gh repo list "$user" --limit "$limit")
fi