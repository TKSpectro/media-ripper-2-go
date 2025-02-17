package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

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
		"--download-archive", filepath.Join(config["internal_path"], "ARCHIVE_"+strings.ToUpper(playlistName)+".txt"),
		"--output", filepath.Join(config["data_path"], playlistName, "%(n_entries+1-playlist_index)04d %(title|Unknown)s [%(id)s].%(ext)s"),
		urlInfo.URL,
	}

	cmd := exec.Command("yt-dlp", cmdArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

func main() {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		log.Fatal(err)
	}

	defaultConfigPath := filepath.Join(homeDir, ".config", "media-ripper-2-go")

	var dataPath, internalPath, tempPath string
	flag.StringVar(&dataPath, "path", "", "Path to store downloaded files")
	flag.StringVar(&internalPath, "internal_path", filepath.Join(defaultConfigPath, "internal"), "Internal storage path")
	flag.StringVar(&tempPath, "temp_path", filepath.Join(defaultConfigPath, "temp"), "Temporary storage path")
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

	urls, err := loadURLs(filepath.Join(defaultConfigPath, "urls.json"))
	if err != nil {
		log.Fatal(err)
	}

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

}
