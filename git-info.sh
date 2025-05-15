#!/bin/bash

while [[ $# -gt 0 ]]; do
    case "$1" in
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

printf -- "──────────────────────────────────────────────────────────────────\n"

echo "$git_repositories" | while read git_dir; do
    (
        repo_name=$(dirname "$git_dir")

        cd "$repo_name" || exit

        if [ "$repo_name" = "." ]; then
            printf "Repository:         \e[1;32m"${PWD##*/}"\e[0m\n"
        else
            printf "Repository:         \e[1;32m$(basename "$repo_name")\e[0m\n"
        fi

        branch_count=$(git branch -a | grep -v 'remotes' | wc -l | tr -d '[:space:]')
        printf "Total branches:     $branch_count\n"

        printf "Branches:           "
        # git branch -a | grep -v 'remotes' | sed -n 's/^\* //p; 2,$s/^/                     /p'
        git branch -a | grep -v 'remotes' | awk '{sub(/^\* /, ""); if (NR==1) print $0; else printf "%-18s%s\n", "", $0}'

        printf "Authors/committers: "
        # git log --format='%an <%ae>' | sort -u | sed '1!s/^/ /' | sed '2,$s/^/                      /'
        git log --format='%an <%ae>' | sort -u | awk 'NR==1{print $0} NR>1{printf "%-20s%s\n", "", $0}'

        commit_count=$(git rev-list --all --count)
        printf "Total commits:      $commit_count\n"

        printf -- "──────────────────────────────────────────────────────────────────\n"
    )
done