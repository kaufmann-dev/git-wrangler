#!/bin/bash

min_msg_length=5
standard_msg="Commit changes"
force=false

while [[ $# -gt 0 ]]; do
  case "$1" in
    --messages)
      tmp_messages=$2
      shift 2
      ;;
    --minmsglength)
      min_msg_length=$2
      shift 2
      ;;
    --standardmsg)
      standard_msg=$2
      shift 2
      ;;
    --force)
      force=true
      shift
      ;;
    *)
      printf "\e[31mUnknown option: $1\e[0m\n"
      exit 1
      ;;
  esac
done

IFS=',' read -r -a messages <<< "$tmp_messages"

if ! command -v git &> /dev/null; then
    printf "\e[31mError: 'git' is not installed. Please install it first.\e[0m\n"
    exit 1
fi

if ! command -v git-filter-repo &> /dev/null; then
    printf "\e[31mError: 'git filter-repo' is not installed. Please install it first.\e[0m\n"
    exit 1
fi

find . -maxdepth 2 -type d -name '.git' | while read -r git_dir; do
  (
    repo_path=$(dirname "$git_dir")

    if [ "$repo_path" = "." ]; then
        repo_name_display="${PWD##*/}"
    else
        repo_name_display=$(basename "$repo_path")
    fi

    printf "Processing repository: $repo_name_display\n"

    cd "$repo_path" || exit

    commit_changed_flag=false

    git log --pretty=format:%H --reverse | while read -r commit_hash; do
      file_count=$(git log --pretty=format:%n --name-status -1 "$commit_hash" | grep -E '^[ADRM]\s+[^[:space:]]+$' | wc -l)
      old_message=$(git log --pretty=format:%B -1 "$commit_hash")
      message_length=${#old_message}

      if ((message_length <= min_msg_length)) || { [[ ${messages[*]} =~ $old_message ]]; }; then
        if ((file_count == 1)); then
            file_status=$(git log --pretty=format:%n --name-status -1 "$commit_hash" | awk '{print $1}')
            file_name=$(git log --pretty=format:%n --name-status -1 "$commit_hash" | awk '{print $2}')

            case "$file_status" in
                'A')
                    new_message="Added $file_name"
                    ;;
                'D')
                    new_message="Removed $file_name"
                    ;;
                'M')
                    new_message="Changed $file_name"
                    ;;
                *)
                    new_message="$standard_msg"
                    ;;
            esac

            if [ "$force" = true ]; then
                error_message=$(git filter-repo --partial --message-callback 'return message.decode("utf-8").replace("'"$old_message"'", "'"$new_message"'").encode("utf-8")' --force 2>&1 >/dev/null)
            else
                error_message=$(git filter-repo --partial --message-callback 'return message.decode("utf-8").replace("'"$old_message"'", "'"$new_message"'").encode("utf-8")' 2>&1 >/dev/null)
            fi

            if [ $? -ne 0 ]; then
                printf "\e[31mError: Could not update commit message for $commit_hash in $repo_name_display:\n$error_message\e[0m\n\n"
                continue
            fi

            printf "Commit message of commit $commit_hash changed from '$old_message' to '$new_message'\n"
            commit_changed_flag=true
        else
            if [ "$force" = true ]; then
                error_message=$(git filter-repo --partial --message-callback 'return message.decode("utf-8").replace("'"$old_message"'", "'"$standard_msg"'").encode("utf-8")' --force 2>&1 >/dev/null)
            else
                error_message=$(git filter-repo --partial --message-callback 'return message.decode("utf-8").replace("'"$old_message"'", "'"$standard_msg"'").encode("utf-8")' 2>&1 >/dev/null)
            fi

            if [ $? -ne 0 ]; then
                printf "\e[31mError: Could not update commit message for $commit_hash in $repo_name_display:\n$error_message\e[0m\n\n"
                continue
            fi

            printf "Commit message of commit $commit_hash changed from '$old_message' to '$standard_msg'\n"
            commit_changed_flag=true
        fi
    fi
    done

    if $commit_changed_flag; then
      printf "\e[32mChanged commit messages for $repo_name_display successfully.\e[0m\n"
    else
      printf "\e[33mNo comit messages changed in $repo_name_display. Skipping...\e[0m\n"
    fi
  )
done
