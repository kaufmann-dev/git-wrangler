#!/usr/bin/env bash
# ──────────────────────────────────────────────────────────────────────────────
# Git Wrangler — Installer
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/kaufmann-dev/git-wrangler/main/install.sh | bash
#
# Environment variables:
#   WRANGLER_INSTALL_DIR  Override installation directory  [default: ~/.wrangler]
#   WRANGLER_BIN_DIR      Override bin/symlink directory   [default: ~/.local/bin]
#   WRANGLER_BRANCH       Override branch to install from  [default: main]
# ──────────────────────────────────────────────────────────────────────────────

set -euo pipefail

# ── Colors (degrade gracefully when not a terminal) ──────────────────────────

setup_colors() {
    if [ -t 1 ] && command -v tput >/dev/null 2>&1; then
        BOLD="$(tput bold 2>/dev/null || printf '')"
        RED="$(tput setaf 1 2>/dev/null || printf '')"
        GREEN="$(tput setaf 2 2>/dev/null || printf '')"
        YELLOW="$(tput setaf 3 2>/dev/null || printf '')"
        CYAN="$(tput setaf 4 2>/dev/null || printf '')"
        RESET="$(tput sgr0 2>/dev/null || printf '')"
    else
        BOLD="" RED="" GREEN="" YELLOW="" CYAN="" RESET=""
    fi
}

# ── Logging helpers ──────────────────────────────────────────────────────────

info()    { printf "${BOLD}${CYAN}▸${RESET} %s\n" "$1"; }
success() { printf "${GREEN}✔${RESET} %s\n" "$1"; }
warn()    { printf "${YELLOW}⚠ %s${RESET}\n" "$1"; }
error()   { printf "${RED}✖ %s${RESET}\n" "$1" >&2; }
fatal()   { error "$1"; exit 1; }

# ── Helpers ──────────────────────────────────────────────────────────────────

has() { command -v "$1" >/dev/null 2>&1; }

is_on_path() {
    case ":$PATH:" in
        *":$1:"*) return 0 ;;
        *)        return 1 ;;
    esac
}

# ── OS / architecture detection ─────────────────────────────────────────────

detect_os() {
    local kernel
    kernel="$(uname -s)"
    case "$kernel" in
        Linux*)
            if [ -f /proc/version ] && grep -qi microsoft /proc/version 2>/dev/null; then
                printf 'wsl'
            else
                printf 'linux'
            fi
            ;;
        Darwin*)              printf 'macos' ;;
        MINGW*|MSYS*|CYGWIN*) printf 'windows' ;;
        *)                    printf '%s' "$kernel" ;;
    esac
}

detect_arch() {
    local arch
    arch="$(uname -m)"
    case "$arch" in
        x86_64|amd64)   printf 'amd64' ;;
        aarch64|arm64)  printf 'arm64' ;;
        armv*)          printf 'arm' ;;
        *)              printf '%s' "$arch" ;;
    esac
}

# ── Resolve bin directory ────────────────────────────────────────────────────
# Prefer ~/.local/bin (XDG-ish, no sudo), fall back to /usr/local/bin on macOS.

resolve_bin_dir() {
    if [ -n "${WRANGLER_BIN_DIR:-}" ]; then
        printf '%s' "$WRANGLER_BIN_DIR"
        return
    fi

    local local_bin="$HOME/.local/bin"

    if [ -d "$local_bin" ]; then
        printf '%s' "$local_bin"
    elif [ -d "/usr/local/bin" ] && [ -w "/usr/local/bin" ]; then
        printf '%s' "/usr/local/bin"
    else
        printf '%s' "$local_bin"
    fi
}

# ── Install ──────────────────────────────────────────────────────────────────

do_install() {
    local install_dir="${WRANGLER_INSTALL_DIR:-$HOME/.wrangler}"
    local bin_dir
    bin_dir="$(resolve_bin_dir)"
    local branch="${WRANGLER_BRANCH:-main}"
    local repo_url="https://github.com/kaufmann-dev/git-wrangler.git"
    local os arch
    os="$(detect_os)"
    arch="$(detect_arch)"

    printf "\n${BOLD}Git Wrangler — Installer${RESET}\n\n"
    info "OS: $os ($arch)"
    info "Install dir: $install_dir"
    info "Bin dir: $bin_dir"
    info "Branch: $branch"
    printf "\n"

    # ── Prerequisites ────────────────────────────────────────────────────

    if ! has git; then
        fatal "git is required but was not found. Install it first: https://git-scm.com"
    fi

    # ── Clone or update ──────────────────────────────────────────────────

    if [ -d "$install_dir/.git" ]; then
        printf "\n"
        success "Git Wrangler is already installed at $install_dir"
        printf "\n"
        info "To update to the latest version, run:"
        printf "\n    ${BOLD}wrangler update${RESET}\n\n"
        exit 0
    elif [ -d "$install_dir" ]; then
        error "Directory $install_dir already exists but is not a git repository."
        fatal "Remove it manually and re-run this script."
    else
        info "Cloning repository…"
        local clone_output
        if clone_output=$(git clone --depth 1 --branch "$branch" "$repo_url" "$install_dir" 2>&1); then
            success "Cloned successfully"
        else
            error "Failed to clone repository:"
            printf '%s\n' "$clone_output" >&2
            exit 1
        fi
    fi

    # ── Make scripts executable ──────────────────────────────────────────

    chmod +x "$install_dir/wrangler" 2>/dev/null || true
    chmod +x "$install_dir"/libexec/wrangler-* 2>/dev/null || true

    # ── Create symlink ───────────────────────────────────────────────────

    if [ ! -d "$bin_dir" ]; then
        info "Creating bin directory: $bin_dir"
        mkdir -p "$bin_dir"
    fi

    local link_target="$bin_dir/wrangler"

    if [ -L "$link_target" ]; then
        local existing
        existing="$(readlink "$link_target" 2>/dev/null || true)"
        if [ "$existing" = "$install_dir/wrangler" ]; then
            info "Symlink already correct"
        else
            ln -sf "$install_dir/wrangler" "$link_target"
            success "Updated symlink → $install_dir/wrangler"
        fi
    elif [ -e "$link_target" ]; then
        warn "A file already exists at $link_target (not a symlink)."
        fatal "Remove it manually and re-run this script."
    else
        ln -s "$install_dir/wrangler" "$link_target"
        success "Created symlink → $install_dir/wrangler"
    fi

    # ── Done ─────────────────────────────────────────────────────────────

    printf "\n"
    success "Git Wrangler installed successfully!"
    printf "\n"

    if ! is_on_path "$bin_dir"; then
        warn "$bin_dir is not in your \$PATH."
        printf "\n"
        info "Add it to your shell profile:"
        printf "\n"
        printf '    export PATH="%s:$PATH"\n' "$bin_dir"
        printf "\n"
        info "Then restart your terminal."
    fi

    info "Run ${BOLD}wrangler help${RESET} to get started."
    printf "\n"
}


# ── Entry point ──────────────────────────────────────────────────────────────
# Wrapping in main() prevents partial execution if the download is interrupted.

main() {
    setup_colors
    do_install
}

main "$@"
