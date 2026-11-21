// One-off migration tool: resequences an existing playlist directory whose
// "NNNN " filename prefixes have drifted/collided (see the yt-dlp
// n_entries/playlist_index bug this repo moved away from). Not part of the
// shipped app - run once per affected playlist directory, then delete.
package main

import (
	"encoding/csv"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"go.senan.xyz/taglib"
)

type entry struct {
	path       string // current full path
	restOfName string // filename with the old "NNNN " prefix stripped
	dateKey    string // YYYYMMDD sort key, normalized from the embedded DATE tag
	oldNumber  int    // old numeric prefix, used as a tiebreak within the same dateKey
}

func normalizeDateKey(date string) string {
	date = strings.TrimSpace(date)
	switch {
	case len(date) >= 8:
		return date[:8]
	case len(date) == 4:
		return date + "0101"
	default:
		// Shouldn't happen (every file in this library has a DATE tag),
		// but don't let a missing tag crash the sort - just push it last.
		return "99999999"
	}
}

func main() {
	path := flag.String("path", "", "Playlist directory to resequence")
	apply := flag.Bool("apply", false, "Actually rename/retag files (default is a dry run)")
	logPath := flag.String("log", "", "CSV file to write the old->new filename mapping to")
	fixPaddingOnly := flag.Bool("fix-padding-only", false, "Files are already correctly named: just rewrite TRACKNUMBER as zero-padded %04d, no rename/resort")
	flag.Parse()

	if *path == "" {
		fmt.Println("Pass --path <playlist dir>")
		os.Exit(1)
	}

	mp3Files, err := filepath.Glob(filepath.Join(*path, "*.mp3"))
	if err != nil {
		log.Fatal(err)
	}

	if *fixPaddingOnly {
		fixed := 0
		for _, p := range mp3Files {
			parts := strings.SplitN(filepath.Base(p), " ", 2)
			num, err := strconv.Atoi(parts[0])
			if len(parts) != 2 || err != nil {
				log.Printf("Skipping %s: no numeric \"NNNN \" prefix", filepath.Base(p))
				continue
			}
			if err := taglib.WriteTags(p, map[string][]string{
				taglib.TrackNumber: {fmt.Sprintf("%04d", num)},
			}, 0); err != nil {
				log.Printf("Failed to retag %s: %v", p, err)
				continue
			}
			fixed++
		}
		fmt.Printf("Fixed TRACKNUMBER padding on %d/%d files\n", fixed, len(mp3Files))
		return
	}

	var entries []entry
	for _, p := range mp3Files {
		base := filepath.Base(p)
		parts := strings.SplitN(base, " ", 2)
		if len(parts) != 2 {
			log.Printf("Skipping %s: no \"NNNN title\" split found", base)
			continue
		}
		oldNumber, err := strconv.Atoi(parts[0])
		if err != nil {
			log.Printf("Skipping %s: prefix %q isn't numeric", base, parts[0])
			continue
		}

		tags, err := taglib.ReadTags(p)
		if err != nil {
			log.Printf("Skipping %s: failed to read tags: %v", base, err)
			continue
		}
		var date string
		if d := tags[taglib.Date]; len(d) > 0 {
			date = d[0]
		}

		entries = append(entries, entry{
			path:       p,
			restOfName: parts[1],
			dateKey:    normalizeDateKey(date),
			oldNumber:  oldNumber,
		})
	}

	sort.Slice(entries, func(i, j int) bool {
		if entries[i].dateKey != entries[j].dateKey {
			return entries[i].dateKey < entries[j].dateKey
		}
		return entries[i].oldNumber < entries[j].oldNumber
	})

	fmt.Printf("%d files parsed, %d skipped\n", len(entries), len(mp3Files)-len(entries))
	fmt.Println("First 10 in new order:")
	for i, e := range entries {
		if i >= 10 {
			break
		}
		fmt.Printf("  %04d <- old %04d (date %s) %s\n", i+1, e.oldNumber, e.dateKey, e.restOfName)
	}
	fmt.Println("Last 10 in new order:")
	for i := len(entries) - 10; i < len(entries); i++ {
		if i < 0 {
			continue
		}
		e := entries[i]
		fmt.Printf("  %04d <- old %04d (date %s) %s\n", i+1, e.oldNumber, e.dateKey, e.restOfName)
	}

	if !*apply {
		fmt.Println("\nDry run only - pass --apply to actually rename/retag.")
		return
	}

	var csvRows [][]string
	dir := *path

	// Phase 1: move everything to collision-free temp names first, since
	// the new numbering overlaps with the old one.
	tempPaths := make([]string, len(entries))
	for i, e := range entries {
		tempPaths[i] = filepath.Join(dir, fmt.Sprintf(".renumber_tmp_%d", i))
		if err := os.Rename(e.path, tempPaths[i]); err != nil {
			log.Fatalf("phase 1 rename failed for %s: %v", e.path, err)
		}
	}

	// Phase 2: move each temp file to its final numbered name and rewrite
	// the TrackNumber tag to match.
	for i, e := range entries {
		newNumber := i + 1
		finalPath := filepath.Join(dir, fmt.Sprintf("%04d %s", newNumber, e.restOfName))
		if err := os.Rename(tempPaths[i], finalPath); err != nil {
			log.Fatalf("phase 2 rename failed for %s: %v", tempPaths[i], err)
		}
		if err := taglib.WriteTags(finalPath, map[string][]string{
			taglib.TrackNumber: {fmt.Sprintf("%04d", newNumber)},
		}, 0); err != nil {
			log.Printf("Failed to retag %s: %v", finalPath, err)
		}
		csvRows = append(csvRows, []string{filepath.Base(e.path), filepath.Base(finalPath)})
	}

	fmt.Printf("Resequenced %d files, new range 0001-%04d\n", len(entries), len(entries))

	if *logPath != "" {
		f, err := os.Create(*logPath)
		if err != nil {
			log.Fatal(err)
		}
		defer f.Close()
		w := csv.NewWriter(f)
		w.Write([]string{"old_filename", "new_filename"})
		w.WriteAll(csvRows)
		w.Flush()
		fmt.Printf("Mapping written to %s\n", *logPath)
	}
}
