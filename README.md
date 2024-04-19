# BashScripts
I am currently working on this repository. Scripts are not sufficiently tested or even not fully developed. Descriptions not yet added. Plase stand by.

## git-remove-tracked-gitignore.sh

## git-clone.sh
Clones GitHub repositories based on specified criteria (visibility, user, limit) and organizes them into a designated directory, checking for existing repositories and displaying status messages.
#### Syntax
```
./script.sh [--visibility <all|public|private>] [--user <username>] [--limit <number>] [--into <directory>]
```
#### Options
* `--visibility`: Set the visibility of repositories to clone (default: "all").
* `--user`: Specify the GitHub username whose repositories to clone (mandatory for private repositories).
* `--limit`: Set the maximum number of repositories to clone (default: 100).
* `--into`: Specify the target directory to organize cloned repositories (default: username).
## git-info.sh

## git-push.sh

## git-rename-commits.sh

## git-rename-authors.sh

## git-rename-branches.sh

## git-remove-secrets.sh
