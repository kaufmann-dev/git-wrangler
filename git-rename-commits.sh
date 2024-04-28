#!/bin/bash

min_msg_length=5
standard_msg="Commit changes"

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
    printf "\e[31mGit filter-repo is not installed. Please install it first.\e[0m\n"
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

    git log --pretty=format:%H --reverse | while read -r commit_hash; do
      file_count=$(git log --pretty=format:%n --name-status -1 "$commit_hash" | grep -E '^[ADRM]\s+[^[:space:]]+$' | wc -l)
      old_message=$(git log --pretty=format:%B -1 "$commit_hash")
      message_length=${#old_message}

      if ((message_length <= min_msg_length)) || [[ " ${messages[@]} " =~ " $old_message " ]]; then
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

          git filter-repo --partial --message-callback '
            return message.decode("utf-8").replace("'"$old_message"'", "'"$new_message"'").encode("utf-8")
          ' --force > /dev/null 2>&1

          printf "Commit message of commit $commit_hash changed from '$old_message' to '$new_message'\n"
        else
          git filter-repo --partial --message-callback '
            return message.decode("utf-8").replace("'"$old_message"'", "'"$standard_msg"'").encode("utf-8")
          ' --force > /dev/null 2>&1

          printf "Commit message of commit $commit_hash changed from '$old_message' to '$standard_msg'\n"
        fi
      fi
    done

    printf "\e[32mChanged commit messages for $repo_name_display successfully.\e[0m\n"
    printf ""
  )
done
