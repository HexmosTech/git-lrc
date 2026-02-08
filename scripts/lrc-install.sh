#!/bin/bash
# lrc installer - automatically downloads and installs the latest lrc CLI
# Usage: curl -fsSL https://your-domain/lrc-install.sh | bash
#   or:  wget -qO- https://your-domain/lrc-install.sh | bash

set -e

print_sudo_help() {
    echo ""
    echo -e "${YELLOW}Troubleshooting:${NC}"
    echo "  1) Try running 'sudo su' in your terminal to verify sudo works."
    echo "  2) If sudo fails here, try a different system terminal (some terminals do not prompt correctly)."
    echo "  3) If sudo access is not available, we cannot install lrc. Please file an issue with details: https://github.com/HexmosTech/LiveReview/issues"
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

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo "🚀 lrc Installer"
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
        echo -e "${RED}Error: Windows detected. Please use lrc-install.ps1 for Windows.${NC}"
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
echo -e "${GREEN}✓${NC} Detected platform: ${PLATFORM}"

# Try to obtain sudo early so we can install into /usr/local/bin
SUDO_AVAILABLE=false
if [ "$(id -u)" -eq 0 ]; then
    SUDO_AVAILABLE=true
    echo -e "${GREEN}✓${NC} Running as root; will install to /usr/local/bin"
elif command -v sudo >/dev/null 2>&1; then
    echo -n "Requesting sudo for install to /usr/local/bin... "
    if sudo -v >/dev/null 2>&1; then
        SUDO_AVAILABLE=true
        echo -e "${GREEN}✓${NC}"
    else
        echo -e "${RED}✗${NC} Could not acquire sudo permissions."
        print_sudo_help
        exit 1
    fi
else
    echo -e "${RED}✗${NC} sudo is not available on this system."
    print_sudo_help
    exit 1
fi

# Authorize with B2
echo -n "Authorizing with Backblaze B2... "
AUTH_RESPONSE=$(curl -s -u "${B2_KEY_ID}:${B2_APP_KEY}" \
    "https://api.backblazeb2.com/b2api/v2/b2_authorize_account")

if [ $? -ne 0 ] || [ -z "$AUTH_RESPONSE" ]; then
    echo -e "${RED}✗${NC}"
    echo -e "${RED}Error: Failed to authorize with B2${NC}"
    exit 1
fi

# Parse JSON (handle multiline)
AUTH_TOKEN=$(echo "$AUTH_RESPONSE" | tr -d '\n' | sed -n 's/.*"authorizationToken": "\([^"]*\)".*/\1/p')
API_URL=$(echo "$AUTH_RESPONSE" | tr -d '\n' | sed -n 's/.*"apiUrl": "\([^"]*\)".*/\1/p')
DOWNLOAD_URL=$(echo "$AUTH_RESPONSE" | tr -d '\n' | sed -n 's/.*"downloadUrl": "\([^"]*\)".*/\1/p')

if [ -z "$AUTH_TOKEN" ] || [ -z "$API_URL" ]; then
    echo -e "${RED}✗${NC}"
    echo -e "${RED}Error: Failed to parse B2 authorization response${NC}"
    echo "Response: $AUTH_RESPONSE"
    exit 1
fi
echo -e "${GREEN}✓${NC}"

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
    echo -e "${RED}✗${NC}"
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
    echo -e "${RED}✗${NC}"
    echo -e "${RED}Error: No versions found in ${B2_BUCKET_NAME}/${B2_PREFIX}/${NC}"
    exit 1
fi

LATEST_VERSION="$VERSIONS"
echo -e "${GREEN}✓${NC} Latest version: ${LATEST_VERSION}"

# Construct download URL
BINARY_NAME="lrc"
DOWNLOAD_PATH="${B2_PREFIX}/${LATEST_VERSION}/${PLATFORM}/${BINARY_NAME}"
FULL_URL="${DOWNLOAD_URL}/file/${B2_BUCKET_NAME}/${DOWNLOAD_PATH}"

echo -n "Downloading lrc ${LATEST_VERSION} for ${PLATFORM}... "
TMP_FILE=$(mktemp)
HTTP_CODE=$(curl -s -w "%{http_code}" -o "$TMP_FILE" -H "Authorization: ${AUTH_TOKEN}" "$FULL_URL")

if [ "$HTTP_CODE" != "200" ]; then
    echo -e "${RED}✗${NC}"
    echo -e "${RED}Error: Failed to download (HTTP $HTTP_CODE)${NC}"
    echo -e "${RED}URL: $FULL_URL${NC}"
    rm -f "$TMP_FILE"
    exit 1
fi

if [ ! -s "$TMP_FILE" ]; then
    echo -e "${RED}✗${NC}"
    echo -e "${RED}Error: Downloaded file is empty${NC}"
    rm -f "$TMP_FILE"
    exit 1
fi
echo -e "${GREEN}✓${NC}"

# Install to /usr/local/bin/lrc first
INSTALL_PATH="/usr/local/bin/lrc"
echo -n "Installing to ${INSTALL_PATH}... "
if [ "$SUDO_AVAILABLE" = true ]; then
    sudo mkdir -p "/usr/local/bin"
    if ! sudo mv "$TMP_FILE" "$INSTALL_PATH" 2>/dev/null; then
        echo -e "${RED}✗${NC}"
        echo -e "${RED}Error: Failed to install to ${INSTALL_PATH}${NC}"
        print_sudo_help
        exit 1
    fi
    sudo chmod +x "$INSTALL_PATH"
else
    echo -e "${RED}✗${NC}"
    echo -e "${RED}Error: sudo is required to install to /usr/local/bin${NC}"
    print_sudo_help
    exit 1
fi
echo -e "${GREEN}✓${NC}"

# Copy to git directory as git-lrc (git subcommand)
GIT_INSTALL_PATH="${GIT_DIR}/git-lrc"
echo -n "Installing to ${GIT_INSTALL_PATH} (git subcommand)... "
if [ "$SUDO_AVAILABLE" = true ]; then
    if ! sudo cp "$INSTALL_PATH" "$GIT_INSTALL_PATH" 2>/dev/null; then
        echo -e "${RED}✗${NC}"
        echo -e "${RED}Error: Failed to install to ${GIT_INSTALL_PATH}${NC}"
        exit 1
    fi
    sudo chmod +x "$GIT_INSTALL_PATH"
else
    echo -e "${RED}✗${NC}"
    echo -e "${RED}Error: sudo is required to install to ${GIT_DIR}${NC}"
    exit 1
fi
echo -e "${GREEN}✓${NC}"

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
            echo -e "${GREEN}✓${NC}"
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
        echo -e "${GREEN}✓${NC}"
        echo -e "${GREEN}Config file created with your API credentials${NC}"
    fi
fi

# Install global hooks via lrc
echo -n "Running 'lrc hooks install' to set up global hooks... "
if "$INSTALL_PATH" hooks install >/dev/null 2>&1; then
    echo -e "${GREEN}✓${NC}"
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
        echo -e "${GREEN}✓${NC}"
    else
        echo -e "${YELLOW}(skipped)${NC}"
    fi
fi

# Verify installation
if ! command -v lrc >/dev/null 2>&1; then
    echo ""
    echo -e "${YELLOW}Warning: 'lrc' command not found in PATH${NC}"
    echo -e "${YELLOW}You may need to add ${INSTALL_DIR} to your PATH or run:${NC}"
    echo -e "${YELLOW}  ${INSTALL_PATH} --version${NC}"
else
    echo ""
    echo -e "${GREEN}✓ Installation complete!${NC}"
    echo ""
    lrc version
fi

echo ""
echo "Run 'lrc --help' to get started"
