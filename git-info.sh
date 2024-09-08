#!/bin/bash

repo=""

while [[ $# -gt 0 ]]; do
    case "$1" in
        --repo)
            repo="$2"
            shift 2
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

if [ -n "$repo" ]; then
    git_repositories=$(find "$repo" -maxdepth 2 -type d -name '.git')
else
    git_repositories=$(find . -maxdepth 2 -type d -name '.git')
fi

if [ -z "$git_repositories" ]; then
    printf "\e[33mNo Git repositories found in the specified directory.\e[0m\n"
    exit 0
fi

echo "$git_repositories" | while read git_dir; do
    (
        repo_name=$(dirname "$git_dir")

        cd "$repo_name" || exit

        # Name
        if [ "$repo_name" = "." ]; then
            printf "Repository:         \e[1;34m${PWD##*/}\e[0m\n"
        else
            printf "Repository:         \e[1;34m$(basename "$repo_name")\e[0m\n"
        fi

        # Status
        status=$(git status --porcelain 2>/dev/null)
        if [ -z "$status" ]; then
            printf "Status:             \e[32mClean\e[0m\n"
        else
            printf "Status:             \e[33mDirty (uncommitted changes or untracked files)\e[0m\n"
        fi

        # License
        printf "License:            "
        if [ -f "LICENSE" ]; then
            first_line=$(head -n 1 "LICENSE")
            if [ -z "$first_line" ]; then
                printf "\e[32mYes\e[0m\n"
            else
                if [ ${#first_line} -gt 70 ]; then
                    first_line="${first_line:0:67}..."
                fi
                printf "\e[32m'%s'\e[0m\n" "$first_line"
            fi
        else
            printf "\e[33mNone\e[0m\n"
        fi

        # Current Branch
        current_branch=$(git rev-parse --abbrev-ref HEAD 2>/dev/null)
        printf "Current branch:     %s\n" "$current_branch"

        # Ahead/Behind
        ahead_behind=$(git rev-list --left-right --count HEAD...@{u} 2>/dev/null)
        if [ -n "$ahead_behind" ]; then
            ahead=$(echo "$ahead_behind" | awk '{print $1}')
            behind=$(echo "$ahead_behind" | awk '{print $2}')
            printf "Ahead/behind:       %s ahead, %s behind remote\n" "$ahead" "$behind"
        else
            printf "Ahead/behind:       No upstream set\n"
        fi

        # Branches
        branch_count=$(git branch -a | grep -v 'remotes' | wc -l | tr -d '[:space:]')
        printf "Branches ($branch_count):       "
        git branch -a | grep -v 'remotes' | awk '{sub(/^\* /, ""); if (NR==1) print $0; else printf "%-18s%s\n", "", $0}'

        # Remotes
        remotes=$(git remote -v | awk '{print $1 " " $2}' | sort -u)
        if [ -n "$remotes" ]; then
            printf "Remotes:            %s\n" "$(echo "$remotes" | awk 'NR==1{print $2} NR>1{printf "%-20s%s\n", "", $2}')"
        else
            printf "Remotes:            None\n"
        fi

        # Initial commit
        initial_commit=$(git log --reverse --format="%ci - %s" 2>/dev/null | head -n 1)
        if [ -n "$initial_commit" ]; then
            printf "Initial commit:     %s\n" "$initial_commit"
        else
            printf "Initial commit:     None (repository is empty)\n"
        fi

        # Total commits
        commit_count=$(git rev-list --all --count)
        printf "Total commits:      $commit_count\n"

        # Commits last month
        commit_freq=$(git log --since="1 month ago" --format="%ci" 2>/dev/null | wc -l | tr -d '[:space:]')
        printf "Commits last month: %s\n" "$commit_freq"

        # Last commit
        last_commit=$(git log -1 --format="%ci - %s" 2>/dev/null)
        printf "Last commit:        %s\n" "$last_commit"

        # Top Authors
        printf "Top authors:        "
        git log --format='%an <%ae>' | sort | uniq -c | sort -rn | head -n 3 | awk '{
            count = $1;                    # Store the commit count
            sub(/^[ \t]*[0-9]+[ \t]*/, ""); # Remove count and surrounding whitespace
            name_email = $0;               # Remaining is name and email
            if (NR == 1) { printf "%s - %s\n", count, name_email }
            else { printf "%-20s%s - %s\n", "", count, name_email }
        }'

        # Largest Files
        git rev-list --objects --all 2>/dev/null | git cat-file --batch-check='%(objectsize) %(objectname) %(rest)' | sort -nr | awk '
        BEGIN { first = 1 }  # Track if this is the first row
        {
            size = $1;
            object = $2;
            path = ($3 != "" ? $3 : "");  
            if (path != "" && !(path in seen)) {  
                seen[path] = 1;
                if (size >= 1073741824) { size_str = sprintf("%.2f GB", size/1073741824) }
                else if (size >= 1048576) { size_str = sprintf("%.2f MB", size/1048576) }
                else if (size >= 1024) { size_str = sprintf("%.2f KB", size/1024) }
                else { size_str = sprintf("%d bytes", size) }

                if (first) { 
                    printf "Largest files:      %s - %s\n", size_str, path;
                    first = 0;
                } else { 
                    printf "%-20s%s - %s\n", "", size_str, path;
                }
            }
        }' | head -n 3
        
        printf -- "\n"
    )
done