# BashScripts
## gh-clone.sh
Clones GitHub repositories based on specified criteria (visibility, user, limit) and organizes them into a designated directory, checking for existing repositories and displaying status messages.
#### Syntax
```
./gh-clone.sh --user <username> [--visibility <all|public|private>] [--limit <number>] [--into <directory>]
```
#### Options
* `--user <username>` (required): Specify the GitHub username whose repositories to clone.
* `--visibility <all|public|private>` (optional): Set the visibility of repositories to clone (default: "all").
* `--limit <number>` (optional): Set the maximum number of repositories to clone (default: 100).
* `--into <directory>` (optional): Specify the target directory to organize cloned repositories (default: username).
## git-info.sh
Iterates through Git repositories found in the current directory and its immediate subdirectories, and provides information about each repository including name, branch count, list of branches, authors and committers, and total commit count.
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
## git-remove-secrets.sh
```diff
- to be developed
```
## git-remove-tracked-gitignore.sh
Iterates through Git repositories found in the current directory and its immediate subdirectories, identifies and stops tracking files defined in their respective .gitignore files, and optionally performs a Git commit.
#### Syntax
```
./git-remove-tracked-gitignore.sh [--commit]
```
#### Options
* `--commit` (optional): Perform a Git commit after removing cached files defined in the .gitignore.
## git-rename-authors.sh
Iterates through Git repositories found in the current directory and its immediate subdirectories, updates author and committer information, with optional force mode, allowing users to specify a new name and email.
#### Syntax
```
./git-rename-authors.sh --name <new_name> --email <new_email> [--force]
```
#### Options
* `--name <new_name>` (required): Specifies the new name to be set as the author and committer in the Git repositories.
* `--email <new_email>` (required): Specifies the new email address to be set as the author and committer in the Git repositories.
* `--force` (optional): Enables force mode, allowing the script to update author and commiter information even if the repositories do not look like fresh clones.
## git-rename-branches.sh
Iterates through Git repositories found in the current directory and its immediate subdirectories, renames a specified branch (--oldbranch) to a new branch (--newbranch) across all repositories.
#### Syntax
```
./git-rename-branches.sh --oldbranch <old_branch_name> --newbranch <new_branch_name>
```
#### Options
* `--oldbranch <old_branch_name>` (required): Specifies the name of the existing Git branches that needs to be renamed.
* `--newbranch <new_branch_name>` (required): Specifies the new name for the Git branches.
## git-rename-commits.sh
