#!/usr/bin/env bash

set -e

# Check if running on macOS
if [[ "$(uname)" != "Darwin" ]]; then
    echo "Error: This installer only supports macOS" >&2
    exit 1
fi

# Detect architecture
ARCH=$(uname -m)
if [[ "$ARCH" == "x86_64" ]]; then
    PLATFORM="darwin-amd64"
elif [[ "$ARCH" == "arm64" ]]; then
    PLATFORM="darwin-arm64"
else
    echo "Error: Unsupported architecture: $ARCH" >&2
    exit 1
fi

# GitHub repository
REPO="alanhaynes211/portsmith"
API_URL="https://api.github.com/repos/${REPO}/releases/latest"

echo "Fetching latest release..."

# Fetch latest release info
RELEASE_DATA=$(curl -s "$API_URL")

if [[ $(echo "$RELEASE_DATA" | grep -c "Not Found") -gt 0 ]]; then
    echo "Error: Repository not found or no releases available" >&2
    exit 1
fi

# Extract version and download URL
VERSION=$(echo "$RELEASE_DATA" | grep '"tag_name":' | sed -E 's/.*"tag_name": "([^"]+)".*/\1/')
DOWNLOAD_URL=$(echo "$RELEASE_DATA" | grep "browser_download_url.*${PLATFORM}.tar.gz" | sed -E 's/.*"browser_download_url": "([^"]+)".*/\1/')

if [[ -z "$VERSION" ]] || [[ -z "$DOWNLOAD_URL" ]]; then
    echo "Error: Could not find release for platform: $PLATFORM" >&2
    exit 1
fi

echo "Installing Portsmith $VERSION for $PLATFORM..."

# Create temporary directory
TMP_DIR=$(mktemp -d)
trap "rm -rf $TMP_DIR" EXIT
cd "$TMP_DIR"

# Download and extract
echo "Downloading..."
curl -sL -o portsmith.tar.gz "$DOWNLOAD_URL"
tar -xzf portsmith.tar.gz
cd portsmith-${VERSION}-${PLATFORM}

# Install binaries
echo "Installing binaries to /usr/local/bin..."
sudo cp portsmith portsmith-helper /usr/local/bin/
sudo chmod 755 /usr/local/bin/portsmith /usr/local/bin/portsmith-helper

# Configure sudoers
echo "Configuring sudoers..."
echo "$(whoami) ALL=(root) NOPASSWD: /usr/local/bin/portsmith-helper" | sudo tee /etc/sudoers.d/portsmith > /dev/null
sudo chmod 0440 /etc/sudoers.d/portsmith

# Install config
mkdir -p ~/.config/portsmith
if [[ ! -f ~/.config/portsmith/config.yaml ]]; then
    if [[ -f config.example.yaml ]]; then
        cp config.example.yaml ~/.config/portsmith/config.yaml
        echo "Config installed to ~/.config/portsmith/config.yaml"
    fi
else
    echo "Keeping existing config at ~/.config/portsmith/config.yaml"
fi

echo ""
echo "Portsmith $VERSION installed successfully!"
echo ""
echo "Next steps:"
echo "  1. Edit config:    \$EDITOR ~/.config/portsmith/config.yaml"
echo "  2. Run:            portsmith"
