# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

A Go CLI/daemon that backs up YouTube playlists: it shells out to `yt-dlp` to download and
extract audio as MP3, then uses `taglib` to write track number and album tags. It can run
once or stay resident on a cron schedule. Shipped as a Docker image (`spectr0/media-ripper`)
built on `denoland/deno` (yt-dlp needs a JS runtime for some extraction, hence Deno) with
`ffmpeg`, `yt-dlp` and the compiled Go binary layered in.

The entire application is `main.go` (single file, no internal packages).

## Build / run / release

```bash
# Build
go build -o media-ripper .

# Run once against the local test fixtures
go run . --path data/test --config config/urls.json

# Build the Docker image (this is how it's actually shipped/tested)
docker build -t spectr0/media-ripper:latest .
docker-compose up -d --build
```

There are no automated tests in this repo. "Testing" a change means building the Docker
image and running it against `config/` and `data/` (or using the VS Code launch config,
which runs `go run` with `--path test`).

CLI flags (see `main.go:206` `main()`):
- `--path` (required) — where MP3s are downloaded to, one subfolder per playlist (lowercased name)
- `--config` — path to `urls.json`; defaults to `/config/urls.json`, falling back to `~/.config/media-ripper-2-go/urls.json`
- `--internal_path` — yt-dlp `--download-archive` files live here (tracks what's already downloaded)
- `--temp_path` — scratch space (declared but not currently passed to yt-dlp)
- `--schedule` — run as a resident cron daemon instead of once
- `--cron` — cron expression for `--schedule` mode (default: `0 5,10,15,20 * * *`)

Release builds are cut via GitHub Actions (`.github/workflows/release.yml`) on GitHub
Release creation, producing a `linux/amd64` binary attached to the release. `updater.sh`
on a host machine pulls that binary; it is not part of the Docker flow.

## Architecture

Everything happens in `main.go`, in this flow:

1. **Config loading** (`loadURLs`) — reads a JSON array of `URLInfo` (`name`, `url`, `ignore`,
   `overwriteTitle`) from `urls.json`. `ignore: true` entries are skipped entirely.
2. **Per-playlist incremental tagging** (`getLatestFileTime`) — before downloading, it walks
   the playlist's existing output directory to find the newest MP3's mtime (+1 min buffer).
   This timestamp is the cutoff used later to find only *newly downloaded* files, so tagging
   doesn't re-touch files from previous runs.
3. **Download** (`downloadPlaylist`) — shells out to `yt-dlp` per playlist with
   `--download-archive` (dedupes already-downloaded videos across runs) and an `--output`
   template of just `title [id].ext` — deliberately *not* numbered by yt-dlp.
4. **Numbering + tagging** (`processDownloads`, `getNextTrackNumber`/`saveNextTrackNumber`) —
   runs `find <playlist_dir> -newermt <cutoff>` to get just the files from this run, sorts
   them by mtime (download-completion order), and assigns each a `NNNN` prefix from a counter
   we own and persist at `internal_path/COUNTER_<PLAYLIST>.txt`, then writes `TrackNumber`/
   `Album` tags via `go.senan.xyz/taglib` (cgo binding, requires libtag at build time — see
   Dockerfile builder stage). We used to let yt-dlp compute the number from
   `%(n_entries+1-playlist_index)`, but that drifts (and silently produces duplicate numbers)
   whenever YouTube's reported playlist size/position for a run doesn't match reality — see
   the git history around the numbering rewrite for the full incident.
5. **Scheduling** (`runScheduler`) — if `--schedule` is passed, runs once immediately, then
   registers the cron job and blocks until SIGINT/SIGTERM.

Key invariants:
- The "what's new" detection in steps 2/4 depends on filesystem mtimes, not yt-dlp's own exit
  data — if you change the download/output flow, make sure newly-written files still land with
  a later mtime than the pre-download scan, or incremental tagging will tag nothing (or
  everything).
- The counter in step 4 is bootstrapped once per playlist (if `COUNTER_<PLAYLIST>.txt` is
  missing) by scanning existing filenames for the highest 4-digit prefix; after that it's only
  ever read from/written to that file, never re-derived from filenames. That bootstrap is only
  safe because every file from the old numbering scheme genuinely has a real track number —
  don't reintroduce filename-sniffing as the steady-state source of truth, since a title that
  happens to start with digits (e.g. "30 Second Timer") would be misread as an existing number.

`config/urls.json` is the per-deployment playlist list (gitignored in practice, see README);
`data/test/` contains sample MP3s used for local testing of the tagging flow against
`--path data/test`.
