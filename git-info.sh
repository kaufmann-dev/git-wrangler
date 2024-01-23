#!/bin/bash

echo "------------------------------------------------------------------"

find . -maxdepth 2 -type d -name '.git' | while read git_dir; do
    repo_name=$(dirname "$git_dir")

    cd "$repo_name" || exit

    if [ "$repo_name" = "." ]; then
        echo -e "Repository:            \e[1;32m"${PWD##*/}"\e[0m"
    else
        echo -e "Repository:            \e[1;32m$(basename "$repo_name")\e[0m"
    fi

    branch_count=$(git branch -a | grep -v 'remotes' | wc -l)
    echo "Anzahl der Branches:   $branch_count"

    echo -n "Branches:              "
    # git branch -a | grep -v 'remotes' | sed -n 's/^\* //p; 2,$s/^/                     /p'
    git branch -a | grep -v 'remotes' | awk '{sub(/^\* /, ""); if (NR==1) print $0; else printf "%-21s%s\n", "", $0}'

    echo -n "Autoren und Committer: "
    # git log --format='%an <%ae>' | sort -u | sed '1!s/^/ /' | sed '2,$s/^/                      /'
    git log --format='%an <%ae>' | sort -u | awk 'NR==1{print $0} NR>1{printf "%-23s%s\n", "", $0}'

    commit_count=$(git rev-list --all --count)
    echo "Gesamtanzahl Commits:  $commit_count"

    cd .. || exit

    echo "------------------------------------------------------------------"
done