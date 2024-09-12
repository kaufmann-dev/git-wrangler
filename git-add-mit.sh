#!/bin/bash

repo=""
copyright_holder=""
overwrite=false

while [[ $# -gt 0 ]]; do
    case "$1" in
        --repo)
            repo="$2"
            shift 2
            ;;
        --name)
            copyright_holder="$2"
            shift 2
            ;;
        --overwrite)
            overwrite=true
            shift 1
            ;;
        *)
            printf "\e[31mUnknown option: $1\e[0m\n"
            exit 1
            ;;
    esac
done

if [ -z "$copyright_holder" ]; then
    printf "\e[31mError: Copyright holder name is required. Use --name <NAME>.\e[0m\n"
    exit 1
fi

if ! command -v git &> /dev/null; then
    printf "\e[31mError: 'git' is not installed. Please install it first.\e[0m\n"
    exit 1
fi

if [ -n "$repo" ]; then
    git_repositories=$(find "$repo" -maxdepth 2 -type d -name '.git')
else
    git_repositories=$(find . -maxdepth 2 -type d -name '.git')
fi

if [ -z "$git_repositories" ]; then
    printf "\e[33mNo Git repositories found in the specified directory.\e[0m\n"
    exit 0
fi

echo "$git_repositories" | while read git_dir; do
    (
        repo_root=$(dirname "$git_dir")

        repo_name=$(basename "$(dirname "$git_dir")")
        if [ "$repo_name" = "." ]; then
            repo_name="${PWD##*/}"
        fi

        license_file="$repo_root/LICENSE"

        if [ -f "$license_file" ]; then
            if [ "$overwrite" = true ]; then
                cat > "$license_file" <<EOL
MIT License

Copyright (c) $copyright_holder

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
EOL
                printf "\e[32mLICENSE file overwritten in repository: $repo_name\e[0m\n"
            else
                printf "\e[33mLICENSE file already exists in repository: $repo_name (use --overwrite to replace it)\e[0m\n"
            fi
        else
            cat > "$license_file" <<EOL
MIT License

Copyright (c) $copyright_holder

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
EOL
            printf "\e[32mLICENSE file created in repository: $repo_name\e[0m\n"
        fi
    )
done
