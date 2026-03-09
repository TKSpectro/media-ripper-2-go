package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/robfig/cron/v3"
	"go.senan.xyz/taglib"
)

type URLInfo struct {
	Name           string `json:"name"`
	URL            string `json:"url"`
	Ignore         bool   `json:"ignore,omitempty"`
	OverwriteTitle bool   `json:"overwriteTitle,omitempty"`
}

func loadURLs(filePath string) ([]URLInfo, error) {
	file, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	var urls []URLInfo
	if err := json.Unmarshal(file, &urls); err != nil {
		return nil, err
	}
	return urls, nil
}

func ensureDir(dir string) error {
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return os.MkdirAll(dir, os.ModePerm)
	}
	return nil
}

func getLatestFileTime(playlistPath string) time.Time {
	var latestTime time.Time
	checkedFiles := 0

	log.Printf("Scanning playlist path for latest mp3: %s", playlistPath)

	// Walk through all files in the playlist directory
	err := filepath.Walk(playlistPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			log.Printf("Skipping path due to error (%s): %v", path, err)
			return nil // Skip errors
		}

		// Only check mp3 files
		if !info.IsDir() && strings.HasSuffix(strings.ToLower(path), ".mp3") {
			checkedFiles++
			if info.ModTime().After(latestTime) {
				latestTime = info.ModTime()
			}
		}
		return nil
	})
	if err != nil {
		log.Printf("Failed to walk playlist path %s: %v", playlistPath, err)
		return time.Time{}
	}

	// Add 1 minute buffer to avoid re-processing the same files
	if !latestTime.IsZero() {
		log.Printf("Latest mp3 mod time in %s: %s (from %d files)", playlistPath, latestTime.Format(time.RFC3339), checkedFiles)
		latestTime = latestTime.Add(1 * time.Minute)
		log.Printf("Using buffered comparison time: %s", latestTime.Format(time.RFC3339))
	} else {
		log.Printf("No mp3 files found in %s", playlistPath)
	}

	return latestTime
}

func downloadPlaylist(urlInfo URLInfo, config map[string]string) error {
	if urlInfo.Ignore {
		return nil
	}
	playlistName := strings.ToLower(urlInfo.Name)

	cmdArgs := []string{
		"--extract-audio",
		"--audio-format", "mp3",
		"--add-metadata",
		"--remote-components", "ejs:github",
		"--download-archive", filepath.Join(config["internal_path"], "ARCHIVE_"+strings.ToUpper(playlistName)+".txt"),
		"--output", filepath.Join(config["data_path"], playlistName, "%(n_entries+1-playlist_index)04d %(title|Unknown)s [%(id)s].%(ext)s"),
		urlInfo.URL,
	}

	cmd := exec.Command("yt-dlp", cmdArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

func processDownloads(urls []URLInfo, config map[string]string) {
	log.Println("Starting download process...")
	for _, urlInfo := range urls {
		playlistName := strings.ToLower(urlInfo.Name)

		// Get the latest file modification time in the playlist directory
		playlistPath := filepath.Join(config["data_path"], playlistName)
		lastFileTime := getLatestFileTime(playlistPath)

		if err := downloadPlaylist(urlInfo, config); err != nil {
			log.Printf("Failed to download %s: %v", urlInfo.URL, err)
		}
		log.Printf("Downloaded %s", urlInfo.Name)

		log.Printf("Tagging %s", urlInfo.Name)

		var cmd *exec.Cmd

		if lastFileTime.IsZero() {
			// First run - tag all mp3 files
			log.Printf("First run for %s - tagging all files", urlInfo.Name)
			cmd = exec.Command("find", playlistPath, "-type", "f", "-name", "*.mp3")
		} else {
			// Find files modified after the latest existing file time
			log.Printf("Finding files for %s modified after %s", urlInfo.Name, lastFileTime.Format(time.RFC3339))
			cmd = exec.Command("find", playlistPath, "-type", "f", "-name", "*.mp3", "-newermt", lastFileTime.Format(time.RFC3339))
		}

		output, err := cmd.Output()
		if err != nil {
			// Ignore error and just continue to the next playlist
			log.Printf("Failed to find files for %s: %v", urlInfo.Name, err)
			continue
		}

		files := strings.Split(strings.TrimSpace(string(output)), "\n")
		// Filter out empty strings
		var validFiles []string
		for _, file := range files {
			if file != "" {
				validFiles = append(validFiles, file)
			}
		}

		log.Printf("Found %d files to tag", len(validFiles))
		successCount := 0
		for _, file := range validFiles {
			trackNumber := strings.Split(filepath.Base(file), " ")[0]

			err := taglib.WriteTags(file, map[string][]string{
				// Multi-valued tags allowed
				taglib.TrackNumber: {trackNumber},
				taglib.Album:       {playlistName},
			}, 0)
			if err != nil {
				log.Printf("Failed to tag %s: %v", file, err)
				continue
			}
			successCount++
		}
		if successCount > 0 {
			log.Printf("Tagged %d/%d files for %s", successCount, len(validFiles), urlInfo.Name)
		} else if len(validFiles) > 0 {
			log.Printf("Failed to tag any files for %s", urlInfo.Name)
		}
	}
	log.Println("Download process completed")
}

func runScheduler(urls []URLInfo, config map[string]string, cronSchedule string) {
	// Run once immediately on startup
	processDownloads(urls, config)

	// Create a new cron scheduler with seconds precision
	c := cron.New(cron.WithLogger(cron.VerbosePrintfLogger(log.New(os.Stdout, "cron: ", log.LstdFlags))))

	// Schedule jobs using the provided cron expression
	// Cron format: minute hour day month dayofweek
	_, err := c.AddFunc(cronSchedule, func() {
		processDownloads(urls, config)
	})
	if err != nil {
		log.Fatal("Failed to add cron job:", err)
	}

	log.Printf("Scheduler started with schedule: %s", cronSchedule)
	c.Start()

	// Wait for interrupt signal to gracefully shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	log.Println("Shutting down scheduler...")
	c.Stop()
}

func main() {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		log.Fatal(err)
	}

	defaultConfigPath := filepath.Join(homeDir, ".config", "media-ripper-2-go")

	var dataPath, internalPath, tempPath, configPath, cronSchedule string
	var schedule bool
	flag.StringVar(&dataPath, "path", "", "Path to store downloaded files")
	flag.StringVar(&internalPath, "internal_path", filepath.Join(defaultConfigPath, "internal"), "Internal storage path")
	flag.StringVar(&tempPath, "temp_path", filepath.Join(defaultConfigPath, "temp"), "Temporary storage path")
	flag.StringVar(&configPath, "config", "", "Path to urls.json config file")
	flag.BoolVar(&schedule, "schedule", false, "Run in scheduled mode")
	flag.StringVar(&cronSchedule, "cron", "0 5,10,15,20 * * *", "Cron schedule (default: 5am, 10am, 3pm, 8pm)")
	flag.Parse()

	if dataPath == "" {
		fmt.Println("No path passed! Pass a path with --path")
		os.Exit(1)
	}

	config := map[string]string{
		"internal_path": internalPath,
		"data_path":     dataPath,
		"temp_path":     tempPath,
	}

	for _, path := range config {
		if err := ensureDir(path); err != nil {
			log.Fatal(err)
		}
	}

	// Determine config file path: flag > /config > ~/.config
	if configPath == "" {
		configPath = "/config/urls.json"
		if _, err := os.Stat(configPath); os.IsNotExist(err) {
			configPath = filepath.Join(defaultConfigPath, "urls.json")
		}
	}

	log.Printf("Loading config from: %s", configPath)
	urls, err := loadURLs(configPath)
	if err != nil {
		log.Fatal(err)
	}

	if schedule {
		// Run in scheduled mode with the provided cron expression
		runScheduler(urls, config, cronSchedule)
	} else {
		// Run once immediately
		processDownloads(urls, config)
	}
}
