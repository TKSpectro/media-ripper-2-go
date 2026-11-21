# Build stage
FROM golang:1.27 AS builder

WORKDIR /build

# Copy go mod files (create them if they don't exist)
COPY go.mod go.sum* ./

# Download dependencies
RUN go mod download || true

# Copy source code
COPY . .

# Build the application. taglib (go.senan.xyz/taglib) runs TagLib as an
# embedded WASM module via wazero, so no cgo/libtag toolchain is needed.
RUN CGO_ENABLED=0 GOOS=linux go build -a -o media-ripper .

# Final stage
FROM denoland/deno:2.9.8

# Install runtime dependencies (Debian-based image)
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    ffmpeg \
    python3 \
    tzdata \
    curl \
    unzip \
 && rm -rf /var/lib/apt/lists/*

# Download yt-dlp binary from GitHub releases and make it executable
RUN curl -fsSL "https://github.com/yt-dlp/yt-dlp/releases/latest/download/yt-dlp" -o /usr/local/bin/yt-dlp \
 && chmod 0755 /usr/local/bin/yt-dlp

# Copy binary from builder
COPY --from=builder /build/media-ripper /usr/local/bin/media-ripper

# Create necessary directories
RUN mkdir -p /config/internal /config/temp /data

WORKDIR /home/ripper

# Set environment variables
ENV HOME=/home/ripper \
    PATH=/usr/local/bin:$PATH

ENTRYPOINT ["/usr/local/bin/media-ripper"]
CMD ["--path", "/data", "--internal_path", "/config/internal", "--temp_path", "/config/temp", "--schedule"]