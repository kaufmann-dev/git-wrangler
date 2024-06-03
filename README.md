# BashScripts
I am currently working on this repository. Scripts are not sufficiently tested or even not fully developed. Descriptions not yet added. Plase stand by.

## git-remove-tracked-gitignore.sh

## gh-clone.sh
Clones GitHub repositories based on specified criteria (visibility, user, limit) and organizes them into a designated directory, checking for existing repositories and displaying status messages.
#### Syntax
```
./gh-clone.sh [--user <username>] [--visibility <all|public|private>] [--limit <number>] [--into <directory>]
```
#### Options
* `--user <username>` (required): Specify the GitHub username whose repositories to clone.
* `--visibility <all|public|private>` (optional): Set the visibility of repositories to clone (default: "all").
* `--limit <number>` (optional): Set the maximum number of repositories to clone (default: 100).
* `--into <directory>` (optional): Specify the target directory to organize cloned repositories (default: username).
## git-info.sh
Iterates through directories up to two levels deep, identifies Git repositories, and provides information about each repository including name, branch count, list of branches, authors and committers, and total commit count.
#### Syntax
```
./git-info.sh
```
## git-push.sh
Iterates through Git repositories found in the current directory and its immediate subdirectories, checks if there are changes to push, and performs a Git push operation with optional force flag.
#### Syntax
```
./git-push.sh [--force]
```
#### Options
* `--force` (optional): Forcefully pushes changes to Git repositories, overwriting remote branches if necessary.
## git-rename-commits.sh

## git-rename-authors.sh

## git-rename-branches.sh

## git-remove-secrets.sh
