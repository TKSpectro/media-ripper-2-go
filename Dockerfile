# Build stage
FROM golang:1.23-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git gcc musl-dev taglib-dev

WORKDIR /build

# Copy go mod files (create them if they don't exist)
COPY go.mod go.sum* ./

# Download dependencies
RUN go mod download || true

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=1 GOOS=linux go build -a -installsuffix cgo -o media-ripper .

# Final stage
FROM alpine:3.19

# Install runtime dependencies
RUN apk add --no-cache \
    ca-certificates \
    ffmpeg \
    python3 \
    py3-pip \
    taglib \
    tzdata \
    findutils \
    curl \
    unzip

# Install yt-dlp
RUN pip3 install --no-cache-dir --break-system-packages yt-dlp

# Install glibc (required for Deno on Alpine)
RUN apk add --no-cache gcompat

# Install Deno from GitHub releases
RUN curl -fsSL https://github.com/denoland/deno/releases/latest/download/deno-x86_64-unknown-linux-gnu.zip -o /tmp/deno.zip && \
    unzip /tmp/deno.zip -d /usr/local/bin && \
    chmod +x /usr/local/bin/deno && \
    rm /tmp/deno.zip

# Create non-root user
RUN addgroup -g 1000 ripper && \
    adduser -D -u 1000 -G ripper ripper

# Copy binary from builder
COPY --from=builder /build/media-ripper /usr/local/bin/media-ripper

# Create necessary directories
RUN mkdir -p /config/internal /config/temp /data && \
    chown -R ripper:ripper /config /data

# Switch to non-root user
USER ripper

WORKDIR /home/ripper

# Set environment variables
ENV HOME=/home/ripper \
    PATH=/usr/local/bin:$PATH

ENTRYPOINT ["/usr/local/bin/media-ripper"]
CMD ["--path", "/data", "--internal_path", "/config/internal", "--temp_path", "/config/temp", "--schedule"]