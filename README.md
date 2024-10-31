# Git Wrangler
Welcome to the Git Wrangler repository! In this repository, you will find a collection of a few useful scripts I have created for the management of git repositories. For all scripts to work, please make sure you have installed `gh`, `git` and `git filter-repo`. This repository contains the following scripts:
* [`clone-repositories.sh`](#clone-repositories-sh): Clones multiple GitHub repositories.
* [`push-repositories.sh`](#push-repositories-sh): Pushes multiple repositories.
* [`pull-repositories.sh`](#pull-repositories-sh): Pulls multiple repositories.
* [`repository-info.sh`](#repository-info-sh): Shows basic repository information.
* [`add-license.sh`](#add-license-sh): Adds or replaces a LICENSE file. Defaults to MIT.
* [`rewrite-authors.sh`](#rewrite-authors-sh): Rewrites author and committer names and emails.
* [`remove-secrets.sh`](#remove-secrets-sh): Permanently purges files containing sensitive data from the entire Git history.
* [`untrack-ignored-files.sh`](#untrack-ignored-files-sh): Removes tracked files that match exclusion rules in .gitignore.
* [`fix-gitignore-files.sh`](#fix-gitignore-files-sh): Audits and fixes .gitignore files by adding missing entries for tracked files.
* [`rename-branches.sh`](#rename-branches-sh): Renames a specified branch to a new name.
* [`rewrite-commits.sh`](#rewrite-commits-sh): Rewrites commit messages to adhere to the Conventional Commits standard.



<a id="clone-repositories-sh"></a>

## clone-repositories.sh
Clones GitHub repositories based on specified criteria (visibility, user, limit) and organizes them into a designated directory, checking for existing repositories and displaying status messages.
#### Syntax
```
./clone-repositories.sh --user <username> [--visibility <all|public|private>] [--limit <number>] [--into <directory>]
```
#### Options
* `--user <username>` (required): Specify the GitHub username whose repositories to clone.
* `--visibility <all|public|private>` (optional): Set the visibility of repositories to clone (default: "all").
* `--limit <number>` (optional): Set the maximum number of repositories to clone (default: 100).
* `--into <directory>` (optional): Specify the target directory to organize cloned repositories (default: username).



<a id="push-repositories-sh"></a>

## push-repositories.sh
Iterates through Git repositories found in the current directory and its immediate subdirectories, checks if there are changes to push, and performs a Git push operation with optional force flag.
#### Syntax
```
./push-repositories.sh [--force]
```
#### Options
* `--force` (optional): Forcefully pushes changes to Git repositories, overwriting remote branches if necessary.



<a id="pull-repositories-sh"></a>

## pull-repositories.sh
Iterates through Git repositories found in the current directory and its immediate subdirectories, and performs a Git pull operation to fetch and integrate changes from the remote repository.
#### Syntax
```
./pull-repositories.sh [--rebase] [--force]
```
#### Options
* `--rebase` (optional): Rebase local commits on top of the fetched remote branch instead of merging.
* `--force` (optional): Forcefully pulls changes, overwriting local changes if necessary.


<a id="repository-info-sh"></a>

## repository-info.sh
Iterates through Git repositories found in the current directory and its immediate subdirectories, and provides information about each repository including name, status, license, branches, remotes, commits and files.
#### Syntax
```
./repository-info.sh
```
#### Options
* `--repo <repository_name>` (optional): Specifies a single repository to analyze, instead of analyzing all repositories in the current directory.



<a id="add-license-sh"></a>

## add-license.sh
Iterates through Git repositories found in the current directory and creates or overwrites a license file with a given copyright holder's name. Uses the MIT license by default. You can change the license by editing the script.
#### Syntax
```
./add-license.sh
```
#### Options
* `--name <copyright_holder>` (required): Specifies the copyright holder's name.
* `--overwrite` (optional): If provided, replaces existing LICENSE files instead of skipping them.
* `--repo <repository_name>` (optional): Specifies a single repository to create a LICENSE file, instead of all repositories in the current directory.



<a id="rewrite-authors-sh"></a>

## rewrite-authors.sh
Iterates through Git repositories found in the current directory and its immediate subdirectories, updates author and committer information, with optional force mode, allowing users to specify a new name and email.
#### Syntax
```
./rewrite-authors.sh --name <new_name> --email <new_email> [--force]
```
#### Options
* `--name <new_name>` (required): Specifies the new name to be set as the author and committer in the Git repositories.
* `--email <new_email>` (required): Specifies the new email address to be set as the author and committer in the Git repositories.
* `--force` (optional): Enables force mode, allowing the script to update author and commiter information even if the repositories do not look like fresh clones.
* `--repo <repository_name>` (optional): Specifies a single repository instead of going through all repositories in the current directory.



<a id="remove-secrets-sh"></a>

## remove-secrets.sh
Permanently purges files containing sensitive data from the entire Git history of all managed repositories (across all branches and tags). It operates on all `.git` repositories found within a depth of 2.
#### Syntax
```
./remove-secrets.sh
```
#### Options
This script takes no arguments.



<a id="untrack-ignored-files-sh"></a>

## untrack-ignored-files.sh
Removes files from the Git index that are actively tracked but match exclusion rules in `.gitignore`. It untracks the files safely while leaving them on the local disk, and commits the removals automatically.
#### Syntax
```
./untrack-ignored-files.sh
```
#### Options
This script takes no arguments.



<a id="fix-gitignore-files-sh"></a>

## fix-gitignore-files.sh
Audits and fixes .gitignore files across Git repositories found in the current directory and its immediate subdirectories. Adds missing entries for tracked files that match common candidates (build artifacts, dependencies, IDE files, etc.) but are not yet covered by .gitignore. Does not untrack files, commit changes, or touch secrets.
#### Syntax
```
./fix-gitignore-files.sh
```
#### Options
This script takes no arguments.


<a id="rename-branches-sh"></a>

## rename-branches.sh
Renames a specified branch to a new name across all managed Git repositories.
#### Syntax
```
./rename-branches.sh --oldbranch <old_name> --newbranch <new_name>
```
#### Options
* `--oldbranch <old_name>` (required): Specifies the name of the existing Git branch to be renamed.
* `--newbranch <new_name>` (required): Specifies the new name for the Git branch.



<a id="rewrite-commits-sh"></a>

## rewrite-commits.sh
Rewrites the commit messages of Git repositories to adhere to the Conventional Commits standard. It categorizes commits based on file paths and statuses to automatically determine the type (e.g., feat, fix, docs, chore) and scope.
#### Syntax
```
./rewrite-commits.sh
```
#### Options
This script takes no arguments.
