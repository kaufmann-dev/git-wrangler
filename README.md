# BashScripts
I am currently working on this repository. Scripts are not sufficiently tested or even not fully developed. Descriptions not yet added. Plase stand by.

## git-remove-tracked-gitignore.sh

## git-clone.sh
Clones GitHub repositories based on specified criteria (visibility, user, limit) and organizes them into a designated directory, checking for existing repositories and displaying status messages.
#### Syntax
```
./script.sh [--user <username>] [--visibility <all|public|private>] [--limit <number>] [--into <directory>]
```
#### Options
* `--user` (required): Specify the GitHub username whose repositories to clone.
* `--visibility` (optional): Set the visibility of repositories to clone (default: "all").
* `--limit` (optional): Set the maximum number of repositories to clone (default: 100).
* `--into` (optional): Specify the target directory to organize cloned repositories (default: username).
## git-info.sh

## git-push.sh

## git-rename-commits.sh

## git-rename-authors.sh

## git-rename-branches.sh

## git-remove-secrets.sh
