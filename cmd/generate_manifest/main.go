package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type FileEntry struct {
	Filename   string   `json:"filename"`
	Path       string   `json:"path"`
	Type       string   `json:"type"`
	Format     string   `json:"format"`
	SizeBytes  int64    `json:"size_bytes"`
	MimeType   string   `json:"mime_type"`
	Operations []string `json:"operations"`
}

type Manifest struct {
	Total int          `json:"total"`
	Files []FileEntry `json:"files"`
}

func guessType(filename string) string {
	ext := strings.ToLower(filepath.Ext(filename))
	videoExts := map[string]bool{".mp4": true, ".mkv": true, ".avi": true, ".mov": true}
	if videoExts[ext] {
		return "video"
	}
	return "audio"
}

func guessMimeType(filename string) string {
	ext := strings.ToLower(filepath.Ext(filename))
	mimes := map[string]string{
		".mp4":  "video/mp4",
		".mkv":  "video/x-matroska",
		".avi":  "video/x-msvideo",
		".mov":  "video/quicktime",
		".mp3":  "audio/mpeg",
		".wav":  "audio/wav",
		".aac":  "audio/aac",
		".flac": "audio/flac",
		".ogg":  "audio/ogg",
	}
	if mime, ok := mimes[ext]; ok {
		return mime
	}
	return "application/octet-stream"
}

func main() {
	filesDir := flag.String("files", "dataset/files", "Directory containing multimedia files")
	outFile := flag.String("out", "dataset/manifest.json", "Output manifest.json file")
	flag.Parse()

	// Read files directory
	entries, err := os.ReadDir(*filesDir)
	if err != nil {
		log.Fatalf("Error reading directory: %v", err)
	}

	var files []FileEntry
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		filename := entry.Name()
		fullPath := filepath.Join(*filesDir, filename)

		// Get file info
		info, err := os.Stat(fullPath)
		if err != nil {
			log.Printf("Warning: skipping %s: %v", filename, err)
			continue
		}

		// Determine type and operations
		fileType := guessType(filename)
		var ops []string
		if fileType == "video" {
			ops = []string{"convert", "thumbnail"}
		} else {
			ops = []string{"convert", "extract_audio"}
		}

		ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(filename)), ".")

		files = append(files, FileEntry{
			Filename:   filename,
			Path:       "/app/dataset/files/" + filename,
			Type:       fileType,
			Format:     ext,
			SizeBytes:  info.Size(),
			MimeType:   guessMimeType(filename),
			Operations: ops,
		})
	}

	// Sort by filename
	sort.Slice(files, func(i, j int) bool {
		return files[i].Filename < files[j].Filename
	})

	manifest := Manifest{
		Total: len(files),
		Files: files,
	}

	// Write manifest
	data, err := json.MarshalIndent(manifest, "", "    ")
	if err != nil {
		log.Fatalf("Error marshaling JSON: %v", err)
	}

	if err := os.WriteFile(*outFile, data, 0o644); err != nil {
		log.Fatalf("Error writing manifest: %v", err)
	}

	fmt.Printf("✅ Manifest generated: %d files → %s\n", manifest.Total, *outFile)
}
