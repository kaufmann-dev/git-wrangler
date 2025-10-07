#!/bin/bash

# ==============================================================================
# Usage: ./clone-repositories.sh --user <username> [--visibility <all|public|private>] [--limit <number>] [--into <directory>]
# 
# Description:
# Clones GitHub repositories based on specified criteria (visibility, user, limit) 
# and organizes them into a designated directory, checking for existing 
# repositories and displaying status messages.
# ==============================================================================

visibility="all"
user=""
limit=100
into=""

clone_repository() {
    local repo="$1"
    local repo_name
    repo_name=$(basename "$repo")

    if [[ -d "$into/$repo_name" ]]; then
        printf "\e[33m%s already exists in %s. Skipping...\e[0m\n" "$repo_name" "$(realpath "$into")"
        return
    fi

    if clone_output=$(gh repo clone "$repo" "$into/$repo_name" 2>&1); then
        printf "\e[32mCloned %s into %s\e[0m\n" "$repo_name" "$(realpath "$into/$repo_name")"
    else
        printf "\e[31mError: Failed to clone %s:\n%s\e[0m\n\n" "$repo_name" "$clone_output"
    fi
}

# Parse command-line arguments
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
            printf "\e[31mUnknown option: %s\e[0m\n" "$1"
            exit 1
            ;;
    esac
done

# Validate required arguments
if [[ -z "$user" ]]; then
    printf "\e[31mError: The --user option is required.\e[0m\n"
    exit 1
fi

# Check prerequisites
if ! command -v gh &> /dev/null; then
    printf "\e[31mError: 'gh' (GitHub CLI) is not installed. Please install it first.\e[0m\n"
    exit 1
fi

if ! command -v git &> /dev/null; then
    printf "\e[31mError: 'git' is not installed. Please install it first.\e[0m\n"
    exit 1
fi

# Validate limit option
if [[ ! "$limit" =~ ^[1-9][0-9]*$ ]]; then
    printf "\e[31mError: --limit must be 1 or greater.\e[0m\n"
    exit 1
fi

# Check authentication for private repositories
if [[ "$visibility" == "private" || "$visibility" == "all" ]]; then
    if ! gh auth status | grep -q "Logged in to .* account $user "; then
        printf "\e[31mError: You are not logged in as the specified user: %s. Set --visibility to 'public' or use 'gh auth login'.\e[0m\n" "$user"
        exit 1
    fi
fi

# Check if repositories exist for the user
if [[ "$visibility" == "private" || "$visibility" == "public" ]]; then
    if [[ "$(gh repo list "$user" --limit 1 --visibility "$visibility" 2>/dev/null | wc -l)" -eq 0 ]]; then
        printf "\e[31mError: No %s repositories found for '%s'.\e[0m\n" "$visibility" "$user"
        exit 1
    fi
else
    if [[ "$(gh repo list "$user" --limit 1 2>/dev/null | wc -l)" -eq 0 ]]; then
        printf "\e[31mError: No repositories found for '%s'.\e[0m\n" "$user"
        exit 1
    fi
fi

# Validate visibility option
if [[ "$visibility" != "all" && "$visibility" != "public" && "$visibility" != "private" ]]; then
    printf "\e[31mError: Invalid visibility option. Use 'all', 'public', or 'private'.\e[0m\n"
    exit 1
fi

# Set default into directory if not specified
if [[ -z "$into" ]]; then
    into="$user"
fi

# Create or access the target directory
if [[ -d "$into" && ! -w "$into" ]] || ! mkdir -p "$into"; then
    printf "\e[31mError: Unable to create or access the specified directory '%s'.\e[0m\n" "$into"
    exit 1
fi

# Clone repositories based on visibility
if [[ "$visibility" == "public" || "$visibility" == "private" ]]; then
    while read -r repo _; do
        clone_repository "$repo"
    done < <(gh repo list "$user" --visibility "$visibility" --limit "$limit")
else
    while read -r repo _; do
        clone_repository "$repo"
    done < <(gh repo list "$user" --limit "$limit")
fi
