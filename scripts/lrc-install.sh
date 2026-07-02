#!/bin/bash
# lrc installer - automatically downloads and installs the latest lrc CLI
# Usage: curl -fsSL https://hexmos.com/lrc-install.sh | bash
#   or:  wget -qO- https://hexmos.com/lrc-install.sh | bash
#
# Install model:
# - Installs to ~/.local/bin (user-writable, no sudo required).
# - Migration: if legacy sudo-installed binaries exist (/usr/local/bin/lrc,
#   git bin dir), attempt sudo removal once, then continue without sudo.
# - PATH: creates ~/.lrc/env (idempotent PATH script) and sources it from
#   shell rc files (~/.profile, ~/.bashrc, ~/.zshenv, etc.).
# - No shell restart required: PATH is exported in-session.
# - Internal release: set LRC_RELEASE_CHANNEL=internal to install the team build.
# - Hook install controls:
#   - set LRC_INSTALL_SKIP_HOOKS=1 to skip running `lrc hooks install`
#   - set LRC_INSTALL_HOOK_SURFACE=git|claude|all to control hook surface
#   - default hook surface is `git` when Claude is available and plugin
#     bootstrap is enabled; otherwise the default is `all`
# - Optional Claude plugin bootstrap controls:
#   - defaults to marketplace source HexmosTech/claude-lrc
#   - defaults to marketplace name claude-lrc
#   - set LRC_CLAUDE_PLUGIN_MARKETPLACE_SOURCE to override the marketplace source
#   - set LRC_CLAUDE_PLUGIN_MARKETPLACE_NAME to override the marketplace name
#   - set LRC_CLAUDE_PLUGIN_NAME to override the plugin name (default: lrc)
#   - set LRC_CLAUDE_PLUGIN_SCOPE=user|project|local (default: user)
#   - set LRC_INSTALL_BOOTSTRAP_CLAUDE_PLUGIN=0 to disable plugin bootstrap
# On Windows Git Bash/MSYS/MINGW/CYGWIN, this script attempts to hand off to
# PowerShell installer automatically.

set -e

LRC_INSTALL_SKIP_HOOKS="${LRC_INSTALL_SKIP_HOOKS:-0}"
LRC_INSTALL_HOOK_SURFACE_EXPLICIT=0
if [ "${LRC_INSTALL_HOOK_SURFACE+x}" = "x" ]; then
    LRC_INSTALL_HOOK_SURFACE_EXPLICIT=1
fi
LRC_INSTALL_HOOK_SURFACE="${LRC_INSTALL_HOOK_SURFACE:-all}"
LRC_INSTALL_BOOTSTRAP_CLAUDE_PLUGIN="${LRC_INSTALL_BOOTSTRAP_CLAUDE_PLUGIN:-1}"
LRC_CLAUDE_PLUGIN_MARKETPLACE_SOURCE="${LRC_CLAUDE_PLUGIN_MARKETPLACE_SOURCE:-HexmosTech/claude-lrc}"
LRC_CLAUDE_PLUGIN_MARKETPLACE_NAME="${LRC_CLAUDE_PLUGIN_MARKETPLACE_NAME:-claude-lrc}"
LRC_CLAUDE_PLUGIN_NAME="${LRC_CLAUDE_PLUGIN_NAME:-lrc}"
LRC_CLAUDE_PLUGIN_SCOPE="${LRC_CLAUDE_PLUGIN_SCOPE:-user}"

is_claude_plugin_installed() {
    local plugin_name="$1"

    if ! command -v claude >/dev/null 2>&1; then
        return 1
    fi

    if ! command -v python3 >/dev/null 2>&1; then
        return 1
    fi

    claude plugin list --json 2>/dev/null | python3 -c '
import json
import sys

plugin_name = sys.argv[1]

try:
    payload = json.load(sys.stdin)
except Exception:
    raise SystemExit(1)

if not isinstance(payload, list):
    raise SystemExit(1)

for entry in payload:
    if not isinstance(entry, dict):
        continue

    if entry.get("name") == plugin_name:
        raise SystemExit(0)

    plugin_id = entry.get("id")
    if isinstance(plugin_id, str) and plugin_id.split("@", 1)[0] == plugin_name:
        raise SystemExit(0)

raise SystemExit(1)
' "$plugin_name"
}

bootstrap_claude_plugin_in_background() {
    if [ "$LRC_INSTALL_BOOTSTRAP_CLAUDE_PLUGIN" != "1" ]; then
        return 0
    fi

    if [ "${LRC_BOOTSTRAP_SOURCE:-}" = "claude-plugin" ]; then
        return 0
    fi

    if [ -z "$LRC_CLAUDE_PLUGIN_MARKETPLACE_SOURCE" ] || [ -z "$LRC_CLAUDE_PLUGIN_MARKETPLACE_NAME" ]; then
        return 0
    fi

    if ! command -v claude >/dev/null 2>&1; then
        return 0
    fi

    if is_claude_plugin_installed "$LRC_CLAUDE_PLUGIN_NAME"; then
        echo -e "${GREEN}Claude plugin '$LRC_CLAUDE_PLUGIN_NAME' already installed; skipping bootstrap${NC}"
        return 0
    fi

    echo -e "${YELLOW}Bootstrapping Claude plugin '$LRC_CLAUDE_PLUGIN_NAME' in the background...${NC}"
    (
        export LRC_BOOTSTRAP_SOURCE=git-lrc
        claude plugin marketplace add "$LRC_CLAUDE_PLUGIN_MARKETPLACE_SOURCE" --scope "$LRC_CLAUDE_PLUGIN_SCOPE" >/dev/null 2>&1 || true
        claude plugin install "$LRC_CLAUDE_PLUGIN_NAME@$LRC_CLAUDE_PLUGIN_MARKETPLACE_NAME" --scope "$LRC_CLAUDE_PLUGIN_SCOPE" >/dev/null 2>&1 || true
    ) >/dev/null 2>&1 &
}

resolve_default_hook_surface() {
    if [ "$LRC_INSTALL_HOOK_SURFACE_EXPLICIT" = "1" ]; then
        printf '%s\n' "$LRC_INSTALL_HOOK_SURFACE"
        return 0
    fi

    if [ "$LRC_INSTALL_BOOTSTRAP_CLAUDE_PLUGIN" = "1" ] \
        && [ "${LRC_BOOTSTRAP_SOURCE:-}" != "claude-plugin" ] \
        && command -v claude >/dev/null 2>&1; then
        printf 'git\n'
        return 0
    fi

    printf '%s\n' "$LRC_INSTALL_HOOK_SURFACE"
}

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

print_windows_handoff_help() {
    local reason="$1"
    echo ""
    echo -e "${YELLOW}Windows + Git Bash detected.${NC}"
    echo -e "${YELLOW}Could not automatically launch the PowerShell installer: ${reason}${NC}"
    echo ""
    echo "Run this in PowerShell (copy/paste):"
    echo "  iwr -useb https://hexmos.com/lrc-install.ps1 | iex"
    echo ""
    echo "If needed, first open PowerShell manually, then run the command above."
    echo ""
}


# Require git to be present; we also install lrc alongside the git binary
if ! command -v git >/dev/null 2>&1; then
    echo -e "${RED}Error: git is not installed. Please install git and retry.${NC}"
    exit 1
fi
GIT_BIN="$(command -v git)"
GIT_DIR="$(dirname "$GIT_BIN")"

# Release manifest URL (public by default, internal when explicitly requested)
RELEASE_CHANNEL="${LRC_RELEASE_CHANNEL:-public}"
MANIFEST_URL="https://f005.backblazeb2.com/file/hexmos/lrc/latest.json"
if [ "$RELEASE_CHANNEL" = "internal" ]; then
    MANIFEST_URL="https://f005.backblazeb2.com/file/hexmos/lrc/internal.json"
fi

# Install location (user-writable, no sudo needed)
INSTALL_DIR="$HOME/.local/bin"
INSTALL_PATH="$INSTALL_DIR/lrc"
GIT_INSTALL_PATH="$INSTALL_DIR/git-lrc"

echo "[*] lrc Installer"
echo "================"
echo ""

# Detect OS
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
case "$OS" in
    linux*)
        PLATFORM_OS="linux"
        ;;
    darwin*)
        PLATFORM_OS="darwin"
        ;;
    msys*|mingw*|cygwin*)
        echo -e "${YELLOW}Windows shell detected (${OS}). Switching to PowerShell installer...${NC}"

        PS_CMD=""
        if command -v powershell.exe >/dev/null 2>&1; then
            PS_CMD="powershell.exe"
        elif command -v powershell >/dev/null 2>&1; then
            PS_CMD="powershell"
        elif command -v pwsh.exe >/dev/null 2>&1; then
            PS_CMD="pwsh.exe"
        elif command -v pwsh >/dev/null 2>&1; then
            PS_CMD="pwsh"
        fi

        if [ -z "$PS_CMD" ]; then
            print_windows_handoff_help "PowerShell is not available in PATH"
            exit 1
        fi

        MARKER_FILE="$(mktemp)"
        MARKER_FILE_PS="$MARKER_FILE"
        if command -v cygpath >/dev/null 2>&1; then
            MARKER_FILE_PS="$(cygpath -w "$MARKER_FILE")"
        fi

        PS_INSTALL_CMD="\$ErrorActionPreference='Stop'; try { iwr -useb https://hexmos.com/lrc-install.ps1 | iex; \$marker = \$env:LRC_INSTALL_MARKER; if (-not [string]::IsNullOrWhiteSpace(\$marker)) { Set-Content -Path \$marker -Value 'LRC_INSTALL_OK' -NoNewline -Encoding ascii }; exit 0 } catch { Write-Host \$_.Exception.Message; exit 1 }"

        PS_STATUS=1
        LRC_INSTALL_MARKER="$MARKER_FILE_PS" "$PS_CMD" -NoProfile -ExecutionPolicy Bypass -Command "$PS_INSTALL_CMD" || PS_STATUS=$?

        if [ -f "$MARKER_FILE" ] && grep -qx "LRC_INSTALL_OK" "$MARKER_FILE"; then
            rm -f "$MARKER_FILE"
            exit 0
        fi

        rm -f "$MARKER_FILE"

        if [ "$PS_STATUS" -ne 0 ]; then
            print_windows_handoff_help "PowerShell installer command failed"
            exit 1
        fi

        print_windows_handoff_help "PowerShell installer did not report a success marker"
        exit 1
        ;;
    *)
        echo -e "${RED}Error: Unsupported operating system: $OS${NC}"
        exit 1
        ;;
esac
# Detect architecture
ARCH=$(uname -m)
case "$ARCH" in
    x86_64|amd64)
        PLATFORM_ARCH="amd64"
        ;;
    aarch64|arm64)
        PLATFORM_ARCH="arm64"
        ;;
    *)
        echo -e "${RED}Error: Unsupported architecture: $ARCH${NC}"
        exit 1
        ;;
esac

PLATFORM="${PLATFORM_OS}-${PLATFORM_ARCH}"
echo -e "${GREEN}OK${NC} Detected platform: ${PLATFORM}"

# ---------------------------------------------------------------------------
# Legacy binary cleanup (one-time migration from sudo-installed locations)
# ---------------------------------------------------------------------------
LEGACY_PATHS=()

# Check /usr/local/bin/lrc
if [ -f "/usr/local/bin/lrc" ]; then
    LEGACY_PATHS+=("/usr/local/bin/lrc")
fi
# Check /usr/local/bin/git-lrc
if [ -f "/usr/local/bin/git-lrc" ]; then
    LEGACY_PATHS+=("/usr/local/bin/git-lrc")
fi
# Check git bin dir (e.g. /usr/bin/git-lrc) — only if it differs from /usr/local/bin
GIT_DIR_GIT_LRC="${GIT_DIR}/git-lrc"
if [ -f "$GIT_DIR_GIT_LRC" ] && [ "$GIT_DIR" != "/usr/local/bin" ]; then
    LEGACY_PATHS+=("$GIT_DIR_GIT_LRC")
fi

if [ ${#LEGACY_PATHS[@]} -gt 0 ]; then
    echo ""
    echo -e "${YELLOW}Found legacy sudo-installed binaries:${NC}"
    for p in "${LEGACY_PATHS[@]}"; do
        echo "  $p"
    done
    echo -e "${YELLOW}These may shadow the new user-local install. Attempting removal...${NC}"

    # First try removing directly (works when files are user-writable)
    NEED_SUDO_PATHS=()
    for p in "${LEGACY_PATHS[@]}"; do
        echo -n "  Removing $p... "
        if rm -f "$p" 2>/dev/null; then
            echo -e "${GREEN}OK${NC}"
        else
            echo -e "${YELLOW}(needs sudo)${NC}"
            NEED_SUDO_PATHS+=("$p")
        fi
    done

    if [ ${#NEED_SUDO_PATHS[@]} -gt 0 ]; then
        SUDO_OK=false
        if [ "$(id -u)" -eq 0 ]; then
            SUDO_OK=true
        elif command -v sudo >/dev/null 2>&1; then
            if sudo -v >/dev/null 2>&1; then
                SUDO_OK=true
            fi
        fi

        if [ "$SUDO_OK" = true ]; then
            CLEANUP_FAILED=false
            for p in "${NEED_SUDO_PATHS[@]}"; do
                echo -n "  Removing with sudo $p... "
                if sudo rm -f "$p"; then
                    echo -e "${GREEN}OK${NC}"
                else
                    echo -e "${RED}FAIL${NC}"
                    CLEANUP_FAILED=true
                fi
            done
            if [ "$CLEANUP_FAILED" = true ]; then
                echo -e "${RED}Error: Some legacy binaries could not be removed. Aborting to avoid shadowed install.${NC}"
                exit 1
            fi
        else
            echo -e "${RED}Error: sudo is not available to remove legacy binaries that require elevated permissions.${NC}"
            echo -e "${RED}Please remove these and rerun: ${NEED_SUDO_PATHS[*]}${NC}"
            exit 1
        fi
    fi

    # Final verification: do not proceed if any legacy binaries remain
    REMAINING_LEGACY=()
    for p in "${LEGACY_PATHS[@]}"; do
        if [ -f "$p" ]; then
            REMAINING_LEGACY+=("$p")
        fi
    done
    if [ ${#REMAINING_LEGACY[@]} -gt 0 ]; then
        echo -e "${RED}Error: Legacy binaries still present: ${REMAINING_LEGACY[*]}${NC}"
        echo -e "${RED}Aborting to avoid shadowed install.${NC}"
        exit 1
    fi

    echo -e "${GREEN}OK${NC} Legacy binaries removed."
    echo ""
fi

# ---------------------------------------------------------------------------
# Ensure install directory exists
# ---------------------------------------------------------------------------
mkdir -p "$INSTALL_DIR"

# Resolve latest release from public manifest
echo -n "Checking remote repository for latest lrc release... "
MANIFEST_RESPONSE=$(curl -fsSL "${MANIFEST_URL}")
if [ $? -ne 0 ] || [ -z "$MANIFEST_RESPONSE" ]; then
    echo -e "${RED}FAIL${NC}"
    echo -e "${RED}Error: Failed to fetch public release manifest${NC}"
    exit 1
fi

LATEST_VERSION=$(echo "$MANIFEST_RESPONSE" | tr -d '\n' | sed -n 's/.*"latest_version"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p')
DOWNLOAD_BASE=$(echo "$MANIFEST_RESPONSE" | tr -d '\n' | sed -n 's/.*"download_base"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p')

if [ -z "$LATEST_VERSION" ] || [ -z "$DOWNLOAD_BASE" ]; then
    echo -e "${RED}FAIL${NC}"
    echo -e "${RED}Error: Release manifest is missing latest_version or download_base${NC}"
    exit 1
fi
echo -e "${GREEN}OK${NC}"
echo -e "${GREEN}OK${NC} Latest version: ${LATEST_VERSION}"

# Construct download URL from manifest metadata
BINARY_NAME="lrc"
FULL_URL="${DOWNLOAD_BASE}/${LATEST_VERSION}/${PLATFORM}/${BINARY_NAME}"

echo -n "Downloading lrc ${LATEST_VERSION} for ${PLATFORM}... "
TMP_FILE=$(mktemp)
HTTP_CODE=$(curl -s -w "%{http_code}" -o "$TMP_FILE" "$FULL_URL")

if [ "$HTTP_CODE" != "200" ]; then
    echo -e "${RED}FAIL${NC}"
    echo -e "${RED}Error: Failed to download (HTTP $HTTP_CODE)${NC}"
    echo -e "${RED}URL: $FULL_URL${NC}"
    rm -f "$TMP_FILE"
    exit 1
fi

if [ ! -s "$TMP_FILE" ]; then
    echo -e "${RED}FAIL${NC}"
    echo -e "${RED}Error: Downloaded file is empty${NC}"
    rm -f "$TMP_FILE"
    exit 1
fi
echo -e "${GREEN}OK${NC}"

# Install lrc to ~/.local/bin
echo -n "Installing to ${INSTALL_PATH}... "
if ! mv "$TMP_FILE" "$INSTALL_PATH"; then
    echo -e "${RED}FAIL${NC}"
    echo -e "${RED}Error: Failed to install to ${INSTALL_PATH}${NC}"
    rm -f "$TMP_FILE"
    exit 1
fi
chmod +x "$INSTALL_PATH"
echo -e "${GREEN}OK${NC}"

# Copy as git-lrc (git subcommand) — git discovers subcommands via $PATH
echo -n "Installing to ${GIT_INSTALL_PATH} (git subcommand)... "
if ! cp "$INSTALL_PATH" "$GIT_INSTALL_PATH"; then
    echo -e "${RED}FAIL${NC}"
    echo -e "${RED}Error: Failed to install to ${GIT_INSTALL_PATH}${NC}"
    exit 1
fi
chmod +x "$GIT_INSTALL_PATH"
echo -e "${GREEN}OK${NC}"

# ---------------------------------------------------------------------------
# PATH management — rustup-style env script + shell rc source lines
# ---------------------------------------------------------------------------
LRC_ENV_DIR="$HOME/.lrc"
LRC_ENV_FILE="$LRC_ENV_DIR/env"
SOURCE_LINE=". \"\$HOME/.lrc/env\""

# Create ~/.lrc/env with idempotent PATH logic
mkdir -p "$LRC_ENV_DIR"
cat > "$LRC_ENV_FILE" << 'ENVEOF'
#!/bin/sh
# lrc shell setup (auto-generated by lrc installer)
# Ensures ~/.local/bin is on PATH for lrc and git-lrc discovery
case ":${PATH}:" in
    *:"$HOME/.local/bin":*)
        ;;
    *)
        export PATH="$HOME/.local/bin:$PATH"
        ;;
esac
ENVEOF
chmod +x "$LRC_ENV_FILE"

# Helper: append source line to a shell rc file if not already present
add_source_line() {
    local rcfile="$1"
    if [ -f "$rcfile" ] && [ -r "$rcfile" ]; then
        if ! grep -qF '/.lrc/env' "$rcfile"; then
            echo "" >> "$rcfile"
            echo "# Added by lrc installer" >> "$rcfile"
            echo ". \"\$HOME/.lrc/env\"" >> "$rcfile"
            echo -e "  ${GREEN}OK${NC} Updated $rcfile"
        fi
    fi
}

# Helper: create rc file with source line (for shells where we must create it)
create_source_line() {
    local rcfile="$1"
    if [ ! -f "$rcfile" ]; then
        echo "# Added by lrc installer" > "$rcfile"
        echo ". \"\$HOME/.lrc/env\"" >> "$rcfile"
        echo -e "  ${GREEN}OK${NC} Created $rcfile"
    else
        add_source_line "$rcfile"
    fi
}

echo "Setting up PATH..."

# Always update ~/.profile (POSIX login shells)
create_source_line "$HOME/.profile"

# Detect current shell
CURRENT_SHELL="$(basename "${SHELL:-/bin/sh}")"

case "$CURRENT_SHELL" in
    bash)
        # Update existing bash config files
        # ~/.bashrc for interactive shells, ~/.bash_profile for login shells
        add_source_line "$HOME/.bashrc"
        add_source_line "$HOME/.bash_profile"
        ;;
    zsh)
        # zsh: ensure ~/.zshenv exists and has the source line
        # (macOS Catalina+ defaults to zsh but may not have any rc files yet)
        create_source_line "$HOME/.zshenv"
        add_source_line "$HOME/.zshrc"
        ;;
    fish)
        # fish uses different syntax; can't source POSIX scripts
        FISH_CONF_DIR="$HOME/.config/fish/conf.d"
        FISH_LRC_CONF="$FISH_CONF_DIR/lrc.fish"
        mkdir -p "$FISH_CONF_DIR"
        if [ ! -f "$FISH_LRC_CONF" ] || ! grep -qF '.local/bin' "$FISH_LRC_CONF"; then
            cat > "$FISH_LRC_CONF" << 'FISHEOF'
# lrc shell setup (auto-generated by lrc installer)
if not contains -- $HOME/.local/bin $PATH
    set -gx PATH $HOME/.local/bin $PATH
end
FISHEOF
            echo -e "  ${GREEN}OK${NC} Created $FISH_LRC_CONF"
        fi
        ;;
    *)
        # For other shells, ~/.profile is the best we can do
        ;;
esac

# Export PATH in the current session so lrc works immediately
export PATH="$HOME/.local/bin:$PATH"

# Remove macOS quarantine attribute if present
if [ "$PLATFORM_OS" = "darwin" ]; then
    xattr -d com.apple.quarantine "$INSTALL_PATH" 2>/dev/null || true
    xattr -d com.apple.quarantine "$GIT_INSTALL_PATH" 2>/dev/null || true
fi

# Create config file if API key and URL are provided
if [ -n "$LRC_API_KEY" ] && [ -n "$LRC_API_URL" ]; then
    # Delegate to the newly compiled lrc binary for a secure, transaction-safe, native onboarding flow
    if "$INSTALL_PATH" onboard --api-url "$LRC_API_URL" --yes; then
        :
    else
        echo -e "${RED}Error: Onboarding failed.${NC}"
        exit 1
    fi
fi

# Install global hooks via lrc unless explicitly suppressed by the caller.
if [ "$LRC_INSTALL_SKIP_HOOKS" = "1" ]; then
    echo -e "${YELLOW}Skipping automatic hook installation because LRC_INSTALL_SKIP_HOOKS=1${NC}"
else
    LRC_RESOLVED_HOOK_SURFACE="$(resolve_default_hook_surface)"
    if [ "$LRC_INSTALL_HOOK_SURFACE_EXPLICIT" != "1" ] && [ "$LRC_RESOLVED_HOOK_SURFACE" = "git" ] && [ "$LRC_INSTALL_HOOK_SURFACE" != "git" ]; then
        echo -e "${YELLOW}Claude detected; defaulting hook surface to git so the lrc plugin owns Claude integration${NC}"
    fi
    echo -n "Running 'lrc hooks install --surface $LRC_RESOLVED_HOOK_SURFACE' to set up hooks... "
    if "$INSTALL_PATH" hooks install --surface "$LRC_RESOLVED_HOOK_SURFACE" >/dev/null 2>&1; then
        echo -e "${GREEN}OK${NC}"
    else
        echo -e "${YELLOW}(warning)${NC} Failed to run 'lrc hooks install --surface $LRC_RESOLVED_HOOK_SURFACE'. You may need to run it manually."
    fi
fi

# Track CLI installation if API key and URL are available
if [ -n "$LRC_API_KEY" ] && [ -n "$LRC_API_URL" ]; then
    echo -n "Notifying LiveReview about CLI installation... "
    TRACK_RESPONSE=$(curl -s -X POST "${LRC_API_URL}/api/v1/diff-review/cli-used" \
        -H "X-API-Key: ${LRC_API_KEY}" \
        -H "Content-Type: application/json" 2>&1)
    
    if [ $? -eq 0 ]; then
        echo -e "${GREEN}OK${NC}"
    else
        echo -e "${YELLOW}(skipped)${NC}"
    fi
fi

bootstrap_claude_plugin_in_background

# Verify installation
echo ""
echo -e "${GREEN}OK Installation complete!${NC}"
echo ""
"$INSTALL_PATH" version

echo ""
echo -e "To start using lrc in your ${YELLOW}current${NC} terminal, run:"
echo ""
echo -e "  ${GREEN}source ~/.lrc/env${NC}"
echo ""
echo "New terminal sessions will pick it up automatically."
echo "Run 'lrc --help' to get started."
