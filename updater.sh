#!/bin/bash

# Set repository and download directory
REPO="TKSpectro/media-ripper-2-go"
DOWNLOAD_DIR="$HOME/Applications/media-ripper-2-go"
BIN_NAME="media-ripper-2-linux-amd64"

# Ensure download directory exists
mkdir -p "$DOWNLOAD_DIR"

# Get the latest release download URL
LATEST_RELEASE_URL=$(curl -s "https://api.github.com/repos/$REPO/releases/latest" | grep "browser_download_url" | cut -d '"' -f 4 | grep -v '\.md5')

# Extract the filename from the URL
FILENAME=$(basename "$LATEST_RELEASE_URL")

# Download the latest release
echo "Downloading latest release: $FILENAME"
curl -L "$LATEST_RELEASE_URL" -o "$DOWNLOAD_DIR/$FILENAME"

# Make it executable
chmod +x "$DOWNLOAD_DIR/$FILENAME"

# Print success message
echo "Downloaded and made executable: $DOWNLOAD_DIR/$FILENAME"