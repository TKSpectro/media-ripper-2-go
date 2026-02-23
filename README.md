# media-ripper-2-go

This Docker setup packages your YouTube backup solution in a minimal Alpine Linux image with built-in scheduling.

**Built-in Scheduler**: Runs automatically at 05:00, 10:00, 15:00, and 20:00 (configurable via cron expression).

## Features

- Automated YouTube playlist downloads with yt-dlp
- MP3 conversion with metadata tagging
- Deno runtime for advanced extraction
- Configurable scheduling with cron expressions
- Runs as non-root user (UID 1000)
- Docker image published to Docker Hub

## Setup Instructions

### 1. Prepare Configuration

Create your `config/urls.json` file:

```bash
cp config/urls.json.example config/urls.json
```

Edit `config/urls.json` with your YouTube playlist URLs:

```json
[
  {
    "name": "MyPlaylist",
    "url": "https://www.youtube.com/playlist?list=YOUR_PLAYLIST_ID",
    "ignore": false,
    "overwriteTitle": false
  }
]
```

### 2. Build and Run

**Local Development:**

```bash
# Build and run locally
docker-compose build
docker-compose up -d
```

**Using Docker Hub Image:**

```bash
docker pull spectr0/media-ripper:latest
docker run -d \
  -v ./config:/config \
  -v ./data:/data \
  -e TZ=UTC \
  spectr0/media-ripper:latest
```

The media-ripper will automatically run at:

- 05:00 (5 AM)
- 10:00 (10 AM)
- 15:00 (3 PM)
- 20:00 (8 PM)

### 3. Command Line Options

```bash
# Run once immediately (no scheduling)
media-ripper --path /data

# Run with custom config path
media-ripper --path /data --config /custom/path/urls.json

# Run with scheduling (default: 5am, 10am, 3pm, 8pm)
media-ripper --path /data --schedule

# Run with custom schedule (every 4 hours)
media-ripper --path /data --schedule --cron "0 */4 * * *"

# Run with custom schedule (daily at midnight)
media-ripper --path /data --schedule --cron "0 0 * * *"
```

### 4. Change Timezone (Optional)

To change the schedule timezone, edit the `TZ` environment variable in `docker-compose.yml`:

```yaml
environment:
  - TZ=America/New_York  # Example: Eastern Time
```

Then the schedule (5 AM, 10 AM, 3 PM, 8 PM) will run in your local timezone.

## Publishing to Docker Hub

### Build and Push New Version

```bash
# Build the Docker image
docker build -t spectr0/media-ripper:latest .

# Login to Docker Hub (if not already logged in)
docker login

# Push to Docker Hub
docker push spectr0/media-ripper:latest
```

### Tag Specific Versions

```bash
# Build and tag with version
docker build -t spectr0/media-ripper:latest -t spectr0/media-ripper:v1.0 .

# Push both tags
docker push spectr0/media-ripper:latest
docker push spectr0/media-ripper:v1.0
```

## TrueNAS SCALE Setup

### Initial Setup

1. **Navigate to Apps** in TrueNAS web interface
2. Click **Install Custom App**
3. Configure the application:

**Application Name:** `media-ripper`

**Image Configuration:**

- **Image Repository:** `spectr0/media-ripper`
- **Image Tag:** `latest`
- **Image Pull Policy:** `Always` (to auto-update) or `IfNotPresent`

**Environment Variables:**

- `TZ` = Your timezone (e.g., `America/New_York`)

**Storage - Host Path Volumes:**

Volume 1 - Config:

- **Host Path:** `/mnt/your-pool/apps/media-ripper/config`
- **Mount Path:** `/config`
- **Read Only:** No

Volume 2 - Data:

- **Host Path:** `/mnt/your-pool/media/downloads`
- **Mount Path:** `/data`
- **Read Only:** No

**Resource Limits (Optional):**

- Set CPU and Memory limits as needed

### Prepare TrueNAS Directories

SSH into TrueNAS or use the Shell and run:

```bash
# Create directories
mkdir -p /mnt/your-pool/apps/media-ripper/config
mkdir -p /mnt/your-pool/media/downloads

# Set permissions for UID 1000 (container user)
chown -R 1000:1000 /mnt/your-pool/apps/media-ripper/config
chown -R 1000:1000 /mnt/your-pool/media/downloads

# If using ACLs, add ACL entry for UID 1000
setfacl -R -m u:1000:rwx /mnt/your-pool/apps/media-ripper/config
setfacl -R -d -m u:1000:rwx /mnt/your-pool/apps/media-ripper/config
```

### Upload Configuration

Copy your `urls.json` to the TrueNAS config directory:

```bash
# From your development machine
scp urls.json root@truenas-ip:/mnt/your-pool/apps/media-ripper/config/

# On TrueNAS, set correct permissions
chown 1000:1000 /mnt/your-pool/apps/media-ripper/config/urls.json
chmod 644 /mnt/your-pool/apps/media-ripper/config/urls.json
```

### Update to Latest Version

When you've pushed a new version to Docker Hub:

**Option 1: Restart the App (if Image Pull Policy is "Always")**

1. Go to **Apps** in TrueNAS
2. Find your `media-ripper` app
3. Click the three dots → **Stop**
4. Wait for it to stop, then click **Start**
5. TrueNAS will pull the latest image automatically

**Option 2: Force Update**

1. Go to **Apps** → `media-ripper`
2. Click **Edit**
3. Change Image Tag to something temporary (e.g., `temp`)
4. Save (this will fail but forces a cleanup)
5. Click **Edit** again
6. Change Image Tag back to `latest`
7. Save (this forces a fresh pull)

**Option 3: Via Shell**

```bash
# SSH into TrueNAS and run:
k3s kubectl delete pod -n ix-media-ripper -l app.kubernetes.io/instance=media-ripper
# TrueNAS will automatically recreate with the latest image
```

### Customize Schedule in TrueNAS

To use a custom schedule, add command arguments in the app configuration:

1. Go to **Apps** → `media-ripper` → **Edit**
2. Find **Container Args** or **Command** section
3. Add: `--cron "0 */6 * * *"` (for every 6 hours, for example)
4. Save

### 5. Manual Run (Optional)

To run immediately without waiting for the schedule:

**Docker Compose:**

```bash
docker-compose exec media-ripper /usr/local/bin/media-ripper --path /data --internal_path /config/internal --temp_path /config/temp
```

### 6. Directory Structure

```
.
├── config/
│   ├── urls.json          # Your playlist URLs
│   ├── internal/          # Archive files (auto-created)
│   └── temp/              # Temporary files (auto-created)
├── data/                  # Downloaded MP3 files (auto-created)
├── Dockerfile
├── docker-compose.yml
├── go.mod
└── main.go
```

## Volume Mounts

- `./config:/config` - Configuration and archive files
- `./data:/data` - Downloaded media files

## Cron Schedule Format

The `--cron` flag accepts standard cron expressions:

```
<minute> <hour> <day-of-month> <month> <day-of-week>
```

Examples:

- `0 5,10,15,20 * * *` - 5am, 10am, 3pm, 8pm (default)
- `0 */6 * * *` - Every 6 hours (on the hour)
- `0 */4 * * *` - Every 4 hours
- `0 2 * * *` - Daily at 2 AM
- `0 2,14 * * *` - Twice daily at 2 AM and 2 PM
- `0 9 * * 1` - Every Monday at 9 AM
- `30 3 * * *` - Daily at 3:30 AM

## Logs

**Docker Compose:**

```bash
docker-compose logs -f media-ripper
```

**TrueNAS:**
View logs in the Apps section or via kubectl:

```bash
k3s kubectl logs -n ix-media-ripper -l app.kubernetes.io/instance=media-ripper -f
```

## Updating

### Update yt-dlp

yt-dlp is installed during the Docker build. To get the latest version, rebuild and republish:

```bash
docker build -t spectr0/media-ripper:latest --no-cache .
docker push spectr0/media-ripper:latest
```

Then update TrueNAS as described above.

## Troubleshooting

### Permission errors on mounted volumes

Ensure the `config` and `data` directories have proper permissions for UID 1000:

**Docker Compose:**

```bash
sudo chown -R 1000:1000 config data
```

**TrueNAS with ACLs:**

```bash
setfacl -R -m u:1000:rwx /mnt/your-pool/apps/media-ripper/config
setfacl -R -d -m u:1000:rwx /mnt/your-pool/apps/media-ripper/config
```

### Build fails with taglib errors

The Dockerfile includes all necessary build dependencies. If issues persist, try:

```bash
docker build --no-cache -t spectr0/media-ripper:latest .
```

### Jobs not running on schedule

Check the container logs to verify the scheduler started:

```bash
docker logs media-ripper
```

You should see:

```
Scheduler started with schedule: 0 5,10,15,20 * * *
```

Verify the container is running:

```bash
docker ps | grep media-ripper
```

### Invalid cron expression

If you see "Failed to add cron job" errors, verify your cron expression syntax:

- Format: `minute hour day-of-month month day-of-week`
- All fields are required
- Use `*` for "any"
- Use `,` to separate multiple values: `0,30 * * * *`
- Use `/` for intervals: `*/15 * * * *`

### TrueNAS app won't start

1. Check logs in Apps section
2. Verify paths exist and have correct permissions
3. Ensure `urls.json` exists in config directory with correct permissions
4. Check that image pull was successful

### Deno runtime warnings

If you see warnings about JavaScript runtime, the application will still work but may have issues with some YouTube videos. Ensure Deno is properly installed in the image (it should be by default).

## Quick Reference

### Development Workflow

```bash
# Make code changes
# ...

# Build Docker image
docker build -t spectr0/media-ripper:latest .

# Test locally
docker run --rm \
  -v $(pwd)/config:/config \
  -v $(pwd)/data:/data \
  spectr0/media-ripper:latest

# If tests pass, push to Docker Hub
docker push spectr0/media-ripper:latest

# Update TrueNAS (see TrueNAS Setup section above)
```

### Common Commands

```bash
# View logs
docker logs -f media-ripper

# Run immediately (skip schedule)
docker exec media-ripper /usr/local/bin/media-ripper --path /data

# Check cron schedule
docker exec media-ripper ps aux | grep media-ripper

# Restart container
docker restart media-ripper

# Pull latest from Docker Hub
docker pull spectr0/media-ripper:latest
```
