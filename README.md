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
Iterates through Git repositories found in the current directory and its immediate subdirectories, removes specified secret files from the history of the Git repositories.
#### Syntax
```
./git-remove-secrets.sh [--secrets <secrets_file>]
```
#### Options
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
Iterates through Git repositories found in the current directory and its immediate subdirectories, updates commit messages based on certain conditions. If only one file was changed in the commit, the script automatically detects what changes where made and changes updates the commit message accordingly, otherwise the default commit message is used.
#### Syntax
```
./git-rename-commits.sh [--messages "<msg1>,<msg2>,..."] [--minmsglength <number>] [--standardmsg <string>]
```
#### Options
* `--messages` (optional): Allows the user to provide a list of specific commit messages to target for replacement.
* `--minmsglength` (optional): Sets the minimum length for commit messages to be considered for replacement (default: 5).
* `--standardmsg` (optional): Specifies a default commit message to use when the existing message does not meet the criteria for replacement (default: Commit changes).
