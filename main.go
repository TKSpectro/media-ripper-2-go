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

func downloadPlaylist(urlInfo URLInfo, config map[string]string) error {
	if urlInfo.Ignore {
		return nil
	}
	playlistName := strings.ToLower(urlInfo.Name)

	cmdArgs := []string{
		"--extract-audio",
		"--audio-format", "mp3",
		"--add-metadata",
		// "--remote-components ejs:github",
		// "--js-runtimes", "deno:/bin/deno",
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
		if err := downloadPlaylist(urlInfo, config); err != nil {
			log.Printf("Failed to download %s: %v", urlInfo.URL, err)
		}
		log.Printf("Downloaded %s", urlInfo.Name)

		log.Printf("Tagging %s", urlInfo.Name)
		// Run through all files in the playlist and put all mp3 files that have been created after the timestamp into an array
		cmd := exec.Command("find", config["data_path"]+"/"+strings.ToLower(urlInfo.Name), "-type", "f", "-name", "*.mp3", "-ctime", "-0.05")
		output, err := cmd.Output()
		if err != nil {
			// Ignore error and just continue to the next playlist
			log.Printf("Failed to find files for %s: %v", urlInfo.Name, err)
			continue
		}

		files := strings.Split(strings.TrimSpace(string(output)), "\n")
		log.Printf("Found %d files", len(files))
		for _, file := range files {
			trackNumber := strings.Split(filepath.Base(file), " ")[0]

			err := taglib.WriteTags(file, map[string][]string{
				// Multi-valued tags allowed
				taglib.TrackNumber: {trackNumber},
				taglib.Album:       {strings.ToLower(urlInfo.Name)},
			}, 0)
			if err != nil {
				log.Printf("Failed to tag %s: %v", file, err)
				continue
			}
		}
		log.Printf("Tagged %s", urlInfo.Name)
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
