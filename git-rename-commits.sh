#!/bin/bash

messages=()
min_msg_length=5
standard_msg="Commit changes"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --messages)
      shift
      messages=("$@")
      break
      ;;
    --minmsglength)
      shift
      min_msg_length=$1
      shift
      ;;
    --standardmsg)
      shift
      standard_msg=$1
      shift
      ;;
    *)
      echo "Unknown option: $1"
      exit 1
      ;;
  esac
done

find . -maxdepth 2 -type d -name '.git' | while read -r git_dir; do
  repo_path=$(dirname "$git_dir")

  if [ -d "$repo_path/.git" ]; then
    echo "Processing repository: $repo_path"

    cd "$repo_path" || exit

    if git log --pretty=format:%n --name-status -1 | grep -E '^[ADRM]\s+[^[:space:]]+$' &> /dev/null; then
      file_status=$(git log --pretty=format:%n --name-status -1 | awk '{print $1}')
      file_name=$(git log --pretty=format:%n --name-status -1 | awk '{print $2}')

      case "$file_status" in
        'A')
          new_message="Added $file_name"
          old_message=$(git log --pretty=format:%B -1)
          git filter-repo --message-callback '
            return message.encode("utf-8").replace(b"old_string_added", b"'"$new_message"'")
          ' --force
          echo "Commit message of commit $(git rev-parse HEAD) changed from '$old_message' to '$new_message'"
          ;;
        'D')
          new_message="Removed $file_name"
          old_message=$(git log --pretty=format:%B -1)
          git filter-repo --message-callback '
            return message.encode("utf-8").replace(b"old_string_removed", b"'"$new_message"'")
          ' --force
          echo "Commit message of commit $(git rev-parse HEAD) changed from '$old_message' to '$new_message'"
          ;;
        'M')
          new_message="Changed $file_name"
          old_message=$(git log --pretty=format:%B -1)
          git filter-repo --message-callback '
            return message.encode("utf-8").replace(b"old_string_modified", b"'"$new_message"'")
          ' --force
          echo "Commit message of commit $(git rev-parse HEAD) changed from '$old_message' to '$new_message'"
          ;;
        *)
          ;;
      esac
    else
      old_message=$(git log --pretty=format:%B -1)
      if [[ "${messages[@]}" =~ "$old_message" || ${#old_message} -lt $min_msg_length ]]; then
        git filter-repo --message-callback '
          return message.encode("utf-8").replace(b"old_string_other", b"'"$standard_msg"'")
        ' --force
        echo "Commit message of commit $(git rev-parse HEAD) changed from '$old_message' to '$standard_msg'"
      fi
    fi

    echo "Repository processed successfully."
    echo ""
  fi
done
