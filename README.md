# RepoManip
Welcome to the RepoManip repository! In this repository, you will find a collection of a few useful scripts I have created for the management of git repositories. For all scripts to work, please make sure you have installed `gh`, `git` and `git filter-repo`. This repository contains the following scripts:
* [`clone-repositories.sh`](#clone-repositories-sh): Clones multiple GitHub repositories.
* [`push-repositories.sh`](#push-repositories-sh): Pushes multiple repositories.
* [`repository-info.sh`](#repository-info-sh): Shows basic repository information.
* [`add-license.sh`](#add-license-sh): Adds or replaces a LICENSE file. Defaults to MIT.
* [`rewrite-authors.sh`](#rewrite-authors-sh): Rewrites author and committer names and emails.
<!--
* [`git-remove-secrets.sh`](#git-remove-secrets-sh): Remove secret files.
* [`git-remove-tracked-gitignore.sh`](#git-remove-tracked-gitignore-sh): Remove untracked files defined in .gitignore.
* [`git-rename-branches.sh`](#git-rename-branches-sh): Rename git branches.
* [`git-rename-commits.sh`](#git-rename-commits-sh): Rename commit messages.
-->



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



<!--
<a id="git-remove-secrets-sh"></a>

## git-remove-secrets.sh
Iterates through Git repositories found in the current directory and its immediate subdirectories, removes specified secret files from the history of the Git repositories.
#### Syntax
```
./git-remove-secrets.sh [--secrets <secrets_file>] [--force]
```
#### Options
* `--force` (optional): Forcefully removes secrets.
* `--secrets <secrets_file>` (optional): Specifies a file containing a list of secret file names to override the default list (default: see below).
#### Default secrets
* `appsettings.json`
* `.env`
* `.env.production`
* `.env.development`
* `.env.local`
* `config.js`
* `config.json`
* `database.yml`
* `secrets.yml`
* `credentials.json`
* `key.json`
* `key.txt`
* `settings.xml`
* `private.key`
* `private.pem`
* `id_rsa`
* `id_dsa`
* `access_token`
* `oauth_token`
* `auth.config`
* `docker-compose.override.yml`
* `.dockerenv`
* `aws-credentials`
* `google-credentials.json`
* `serviceAccountKey.json`
* `firebase-adminsdk.json`
* `firebase-service-account.json`
* `client_secret.json`



<a id="git-remove-tracked-gitignore-sh"></a>

## git-remove-tracked-gitignore.sh
Iterates through Git repositories found in the current directory and its immediate subdirectories, identifies and stops tracking files defined in their respective .gitignore files, and optionally performs a Git commit.
#### Syntax
```
./git-remove-tracked-gitignore.sh [--commit]
```
#### Options
* `--commit` (optional): Perform a Git commit after removing cached files defined in the .gitignore.



<a id="git-rename-branches-sh"></a>

## git-rename-branches.sh
Iterates through Git repositories found in the current directory and its immediate subdirectories, renames a specified branch (--oldbranch) to a new branch (--newbranch) across all repositories.
#### Syntax
```
./git-rename-branches.sh --oldbranch <old_branch_name> --newbranch <new_branch_name>
```
#### Options
* `--oldbranch <old_branch_name>` (required): Specifies the name of the existing Git branches that needs to be renamed.
* `--newbranch <new_branch_name>` (required): Specifies the new name for the Git branches.



<a id="git-rename-commits-sh"></a>

## git-rename-commits.sh
Iterates through Git repositories found in the current directory and its immediate subdirectories, updates commit messages based on certain conditions. If only one file was changed in the commit, the script automatically detects what changes where made and changes updates the commit message accordingly, otherwise the default commit message is used.
#### Syntax
```
./git-rename-commits.sh [--messages "<msg1>,<msg2>,..."] [--minmsglength <number>] [--standardmsg <string>] [--force]
```
#### Options
* `--messages "<msg1>,<msg2>,..."` (optional): Allows the user to provide a list of specific commit messages to target for replacement.
* `--minmsglength <number>` (optional): Sets the minimum length for commit messages to be considered for replacement (default: 5).
* `--standardmsg <string>` (optional): Specifies a default commit message to use when the existing message does not meet the criteria for replacement (default: Commit changes).
* `--force` (optional): Forcefully changes commit messages.
-->
