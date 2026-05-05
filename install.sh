#!/usr/bin/env bash
# ──────────────────────────────────────────────────────────────────────────────
# Git Wrangler — Installer
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/kaufmann-dev/git-wrangler/main/install.sh | bash
#
# Or download and inspect first:
#   curl -fsSL https://raw.githubusercontent.com/kaufmann-dev/git-wrangler/main/install.sh -o install.sh
#   less install.sh
#   bash install.sh
#
# Environment variables:
#   WRANGLER_INSTALL_DIR  — Override the installation directory
#                           (default: $HOME/.wrangler)
# ──────────────────────────────────────────────────────────────────────────────

set -euo pipefail

REPO_URL="https://github.com/kaufmann-dev/git-wrangler.git"
REPO_TARBALL="https://github.com/kaufmann-dev/git-wrangler/archive/refs/heads/main.tar.gz"
DEFAULT_INSTALL_DIR="$HOME/.wrangler"

# ── Helpers ──────────────────────────────────────────────────────────────────

BOLD="\033[1m"
RED="\033[31m"
GREEN="\033[32m"
YELLOW="\033[33m"
CYAN="\033[36m"
RESET="\033[0m"

info()    { printf "${CYAN}▸${RESET} %s\n" "$1"; }
success() { printf "${GREEN}✔${RESET} %s\n" "$1"; }
warn()    { printf "${YELLOW}⚠${RESET} %s\n" "$1"; }
error()   { printf "${RED}✖ %s${RESET}\n" "$1" >&2; }
fatal()   { error "$1"; exit 1; }

# ── Cleanup ──────────────────────────────────────────────────────────────────

CLEANUP_DIR=""

cleanup() {
    if [ -n "$CLEANUP_DIR" ] && [ -d "$CLEANUP_DIR" ]; then
        rm -rf "$CLEANUP_DIR"
    fi
}

trap cleanup EXIT

# ── Main ─────────────────────────────────────────────────────────────────────

main() {
    printf "\n${BOLD}Git Wrangler — Installer${RESET}\n\n"

    # ── Detect OS ────────────────────────────────────────────────────────────

    local os arch kernel
    kernel="$(uname -s)"

    case "$kernel" in
        Linux*)
            if grep -qiE "microsoft|wsl" /proc/version 2>/dev/null; then
                os="windows-wsl"
            else
                os="linux"
            fi
            ;;
        Darwin*)  os="macos" ;;
        MINGW*|MSYS*|CYGWIN*) os="windows-gitbash" ;;
        *)        fatal "Unsupported operating system: $kernel" ;;
    esac

    arch="$(uname -m)"
    case "$arch" in
        x86_64|amd64)  arch="amd64" ;;
        aarch64|arm64) arch="arm64" ;;
        *)             warn "Unrecognised architecture: $arch (proceeding anyway)" ;;
    esac

    info "Detected OS: $os ($arch)"

    # ── Check prerequisites ──────────────────────────────────────────────────

    if ! command -v git &>/dev/null; then
        fatal "git is not installed. Please install git first: https://git-scm.com"
    fi

    local has_curl=false has_wget=false
    command -v curl  &>/dev/null && has_curl=true
    command -v wget  &>/dev/null && has_wget=true

    if [ "$has_curl" = false ] && [ "$has_wget" = false ]; then
        fatal "Neither curl nor wget is available. Please install one and try again."
    fi

    # ── Determine install directory ──────────────────────────────────────────

    local install_dir="${WRANGLER_INSTALL_DIR:-$DEFAULT_INSTALL_DIR}"

    # ── Install / Update ─────────────────────────────────────────────────────

    if [ -d "$install_dir/.git" ]; then
        # Existing git-based install — pull latest
        info "Existing installation found at $install_dir — updating…"
        if git -C "$install_dir" pull --ff-only --quiet 2>/dev/null; then
            success "Updated to latest version"
        else
            warn "Could not fast-forward. Re-cloning…"
            rm -rf "$install_dir"
            install_fresh "$install_dir" "$has_curl"
        fi
    elif [ -d "$install_dir" ] && [ -f "$install_dir/wrangler" ]; then
        # Existing tarball-based install — replace
        info "Existing installation found at $install_dir — reinstalling…"
        rm -rf "$install_dir"
        install_fresh "$install_dir" "$has_curl"
    else
        install_fresh "$install_dir" "$has_curl"
    fi

    # ── Make scripts executable ──────────────────────────────────────────────

    chmod +x "$install_dir/wrangler"
    if [ -d "$install_dir/libexec" ]; then
        chmod +x "$install_dir/libexec/"*
    fi

    # ── Add to PATH ──────────────────────────────────────────────────────────

    local bin_dir="$install_dir"
    local shell_name rc_file line_to_add
    line_to_add="export PATH=\"$bin_dir:\$PATH\""

    # Determine which shell config file to update
    shell_name="$(basename "${SHELL:-/bin/bash}")"
    case "$shell_name" in
        zsh)  rc_file="$HOME/.zshrc" ;;
        fish) rc_file="" ;; # handled separately
        *)    rc_file="$HOME/.bashrc" ;;
    esac

    # Also source from .profile / .bash_profile for login shells
    local profile_file=""
    if [ "$shell_name" = "bash" ]; then
        if [ -f "$HOME/.bash_profile" ]; then
            profile_file="$HOME/.bash_profile"
        elif [ -f "$HOME/.profile" ]; then
            profile_file="$HOME/.profile"
        fi
    fi

    local path_configured=false

    if [ "$shell_name" = "fish" ]; then
        local fish_line="fish_add_path $bin_dir"
        local fish_config="$HOME/.config/fish/config.fish"
        if [ -f "$fish_config" ] && grep -qF "$bin_dir" "$fish_config" 2>/dev/null; then
            path_configured=true
        else
            mkdir -p "$(dirname "$fish_config")"
            printf "\n# Git Wrangler\n%s\n" "$fish_line" >> "$fish_config"
            path_configured=true
            info "Added wrangler to PATH in $fish_config"
        fi
    else
        # Check if already on PATH
        if echo "$PATH" | tr ':' '\n' | grep -qxF "$bin_dir" 2>/dev/null; then
            path_configured=true
        fi

        # Add to rc file
        if [ -n "$rc_file" ]; then
            if [ -f "$rc_file" ] && grep -qF "$bin_dir" "$rc_file" 2>/dev/null; then
                path_configured=true
            elif ! $path_configured; then
                printf "\n# Git Wrangler\n%s\n" "$line_to_add" >> "$rc_file"
                path_configured=true
                info "Added wrangler to PATH in $rc_file"
            fi
        fi

        # Add to profile file too (if separate from rc_file)
        if [ -n "$profile_file" ] && [ "$profile_file" != "$rc_file" ]; then
            if ! grep -qF "$bin_dir" "$profile_file" 2>/dev/null; then
                printf "\n# Git Wrangler\n%s\n" "$line_to_add" >> "$profile_file"
                info "Added wrangler to PATH in $profile_file"
            fi
        fi
    fi

    # Export for current session so the verification step works
    export PATH="$bin_dir:$PATH"

    # ── Verify ───────────────────────────────────────────────────────────────

    printf "\n"
    if command -v wrangler &>/dev/null; then
        success "Git Wrangler installed successfully!"
    else
        success "Git Wrangler installed to $install_dir"
    fi

    printf "\n"
    info "Installation directory: $install_dir"

    if [ "$path_configured" = true ]; then
        printf "\n  ${BOLD}Restart your shell${RESET} or run:\n"
        if [ "$shell_name" = "fish" ]; then
            printf "    source ~/.config/fish/config.fish\n"
        else
            printf "    source %s\n" "${rc_file:-~/.bashrc}"
        fi
        printf "\n  Then run:\n"
    else
        printf "\n  Add wrangler to your PATH:\n"
        printf "    export PATH=\"%s:\$PATH\"\n\n" "$bin_dir"
        printf "  Then run:\n"
    fi

    printf "    ${BOLD}wrangler help${RESET}\n\n"
}

# ── Download helpers ─────────────────────────────────────────────────────────

install_fresh() {
    local install_dir="$1"
    local has_curl="$2"

    # Prefer git clone for easy future updates
    info "Installing Git Wrangler to $install_dir…"

    if command -v git &>/dev/null; then
        info "Cloning repository…"
        if git clone --depth 1 --quiet "$REPO_URL" "$install_dir" 2>/dev/null; then
            success "Downloaded via git clone"
            return 0
        else
            warn "git clone failed, falling back to tarball download…"
        fi
    fi

    # Fallback: download tarball
    local tmp_dir
    tmp_dir="$(mktemp -d)"
    CLEANUP_DIR="$tmp_dir"

    local tarball="$tmp_dir/wrangler.tar.gz"
    download_file "$REPO_TARBALL" "$tarball" "$has_curl"

    info "Extracting…"
    mkdir -p "$install_dir"
    tar -xzf "$tarball" -C "$tmp_dir"

    # The tarball extracts to git-wrangler-main/
    local extracted_dir
    extracted_dir="$(find "$tmp_dir" -maxdepth 1 -type d -name 'git-wrangler-*' | head -n 1)"

    if [ -z "$extracted_dir" ]; then
        fatal "Failed to extract archive — unexpected directory structure"
    fi

    # Move contents to install dir
    cp -a "$extracted_dir/." "$install_dir/"
    success "Downloaded and extracted"

    CLEANUP_DIR=""
    rm -rf "$tmp_dir"
}

download_file() {
    local url="$1"
    local dest="$2"
    local has_curl="$3"

    info "Downloading from $url…"

    if [ "$has_curl" = true ]; then
        if ! curl -fsSL --retry 3 --retry-delay 2 -o "$dest" "$url"; then
            fatal "Download failed (curl). Check your network connection and try again."
        fi
    else
        if ! wget -q --tries=3 -O "$dest" "$url"; then
            fatal "Download failed (wget). Check your network connection and try again."
        fi
    fi
}

# ── Entry point ──────────────────────────────────────────────────────────────
# Wrapping in main() prevents partial execution if the download is interrupted.

main "$@"
