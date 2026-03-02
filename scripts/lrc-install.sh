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
# On Windows Git Bash/MSYS/MINGW/CYGWIN, this script attempts to hand off to
# PowerShell installer automatically.

set -e

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

# B2 read-only credentials (hardcoded)
B2_KEY_ID="00536b4c5851afd0000000006"
B2_APP_KEY="K005DV+hNk6/fdQr8oXHmRsdo8U2YAU"
B2_BUCKET_NAME="hexmos"
B2_PREFIX="lrc"

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
        for p in "${LEGACY_PATHS[@]}"; do
            echo -n "  Removing $p... "
            if sudo rm -f "$p"; then
                echo -e "${GREEN}OK${NC}"
            else
                echo -e "${RED}FAIL${NC}"
                CLEANUP_FAILED=true
            fi
        done
        if [ "$CLEANUP_FAILED" = true ]; then
            echo -e "${YELLOW}Warning: Some legacy binaries could not be removed. They may shadow the new install.${NC}"
        else
            echo -e "${GREEN}OK${NC} Legacy binaries removed."
        fi
    else
        echo -e "${YELLOW}Warning: sudo is not available to remove legacy binaries.${NC}"
        echo -e "${YELLOW}The old binaries in system directories may shadow the new install at ~/.local/bin.${NC}"
        echo -e "${YELLOW}To fix, manually run: sudo rm -f ${LEGACY_PATHS[*]}${NC}"
    fi
    echo ""
fi

# ---------------------------------------------------------------------------
# Ensure install directory exists
# ---------------------------------------------------------------------------
mkdir -p "$INSTALL_DIR"

# Authorize with B2
echo -n "Authorizing with Backblaze B2... "
AUTH_RESPONSE=$(curl -s -u "${B2_KEY_ID}:${B2_APP_KEY}" \
    "https://api.backblazeb2.com/b2api/v2/b2_authorize_account")

if [ $? -ne 0 ] || [ -z "$AUTH_RESPONSE" ]; then
    echo -e "${RED}FAIL${NC}"
    echo -e "${RED}Error: Failed to authorize with B2${NC}"
    exit 1
fi

# Parse JSON (handle multiline)
AUTH_TOKEN=$(echo "$AUTH_RESPONSE" | tr -d '\n' | sed -n 's/.*"authorizationToken": "\([^"]*\)".*/\1/p')
API_URL=$(echo "$AUTH_RESPONSE" | tr -d '\n' | sed -n 's/.*"apiUrl": "\([^"]*\)".*/\1/p')
DOWNLOAD_URL=$(echo "$AUTH_RESPONSE" | tr -d '\n' | sed -n 's/.*"downloadUrl": "\([^"]*\)".*/\1/p')

if [ -z "$AUTH_TOKEN" ] || [ -z "$API_URL" ]; then
    echo -e "${RED}FAIL${NC}"
    echo -e "${RED}Error: Failed to parse B2 authorization response${NC}"
    echo "Response: $AUTH_RESPONSE"
    exit 1
fi
echo -e "${GREEN}OK${NC}"

# List files in the lrc/ folder to find versions
echo -n "Finding latest version... "
LIST_RESPONSE=$(curl -s -X POST "${API_URL}/b2api/v2/b2_list_file_names" \
    -H "Authorization: ${AUTH_TOKEN}" \
    -H "Content-Type: application/json" \
    -d "{
        \"bucketId\": \"33d6ab74ac456875919a0f1d\",
        \"startFileName\": \"${B2_PREFIX}/\",
        \"prefix\": \"${B2_PREFIX}/\",
        \"maxFileCount\": 10000
    }")

if [ $? -ne 0 ] || [ -z "$LIST_RESPONSE" ]; then
    echo -e "${RED}FAIL${NC}"
    echo -e "${RED}Error: Failed to list files from B2${NC}"
    exit 1
fi

# Extract unique versions (looking for paths like lrc/vX.Y.Z/)
VERSIONS=$(echo "$LIST_RESPONSE" | tr -d '\n' | grep -o "\"fileName\": *\"${B2_PREFIX}/v[0-9][^/]*/[^\"]*\"" | \
    sed 's|.*"fileName": *"'${B2_PREFIX}'/\(v[0-9][^/]*\)/.*|\1|' | sort -u | sort -V | tail -1)

if [ -z "$VERSIONS" ]; then
    # Fallback: look for files in version directories
    VERSIONS=$(echo "$LIST_RESPONSE" | grep -o "\"fileName\":\"${B2_PREFIX}/v[^/]*/[^\"]*\"" | \
        sed 's|.*"'${B2_PREFIX}'/\(v[^/]*\)/.*|\1|' | sort -uV | tail -1)
fi

if [ -z "$VERSIONS" ]; then
    echo -e "${RED}FAIL${NC}"
    echo -e "${RED}Error: No versions found in ${B2_BUCKET_NAME}/${B2_PREFIX}/${NC}"
    exit 1
fi

LATEST_VERSION="$VERSIONS"
echo -e "${GREEN}OK${NC} Latest version: ${LATEST_VERSION}"

# Construct download URL
BINARY_NAME="lrc"
DOWNLOAD_PATH="${B2_PREFIX}/${LATEST_VERSION}/${PLATFORM}/${BINARY_NAME}"
FULL_URL="${DOWNLOAD_URL}/file/${B2_BUCKET_NAME}/${DOWNLOAD_PATH}"

echo -n "Downloading lrc ${LATEST_VERSION} for ${PLATFORM}... "
TMP_FILE=$(mktemp)
HTTP_CODE=$(curl -s -w "%{http_code}" -o "$TMP_FILE" -H "Authorization: ${AUTH_TOKEN}" "$FULL_URL")

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
    CONFIG_DIR="$HOME/.config"
    CONFIG_FILE="$HOME/.lrc.toml"
    
    # Check if config already exists
    if [ -f "$CONFIG_FILE" ]; then
        echo -e "${YELLOW}Note: Config file already exists at $CONFIG_FILE${NC}"
        echo -n "Replace existing config? [y/N]: "
        # Read from terminal even when stdin is piped
        if [ -t 0 ]; then
            read -r REPLACE_CONFIG
        else
            read -r REPLACE_CONFIG < /dev/tty 2>/dev/null || REPLACE_CONFIG="n"
        fi
        if [[ "$REPLACE_CONFIG" =~ ^[Yy]$ ]]; then
            echo -n "Replacing config file at $CONFIG_FILE... "
            mkdir -p "$CONFIG_DIR"
            cat > "$CONFIG_FILE" <<EOF
api_key = "$LRC_API_KEY"
api_url = "$LRC_API_URL"
EOF
            chmod 600 "$CONFIG_FILE"
            echo -e "${GREEN}OK${NC}"
            echo -e "${GREEN}Config file replaced with your API credentials${NC}"
        else
            echo -e "${YELLOW}Skipping config creation to preserve existing settings${NC}"
        fi
    else
        echo -n "Creating config file at $CONFIG_FILE... "
        mkdir -p "$CONFIG_DIR"
        cat > "$CONFIG_FILE" <<EOF
api_key = "$LRC_API_KEY"
api_url = "$LRC_API_URL"
EOF
        chmod 600 "$CONFIG_FILE"
        echo -e "${GREEN}OK${NC}"
        echo -e "${GREEN}Config file created with your API credentials${NC}"
    fi
fi

# Install global hooks via lrc
echo -n "Running 'lrc hooks install' to set up global hooks... "
if "$INSTALL_PATH" hooks install >/dev/null 2>&1; then
    echo -e "${GREEN}OK${NC}"
else
    echo -e "${YELLOW}(warning)${NC} Failed to run 'lrc hooks install'. You may need to run it manually."
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
