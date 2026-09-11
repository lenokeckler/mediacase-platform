// internal/multimedia/thumbnail.go
// FFmpeg wrapper: captura un fotograma JPEG del primer segundo de un video.
// Si el archivo es audio, genera una imagen de waveform.
package multimedia

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
)

// Thumbnail extracts a JPEG frame from inputPath.
// For audio files it generates a waveform PNG instead.
// The callback receives 100 on completion.
func Thumbnail(ctx context.Context, inputPath string, cb progressFn) (string, error) {
	out := outputPath(inputPath, ".jpg")
	log.Printf("[thumbnail] %s → %s", inputPath, out)

	// Grab the first available video frame (no fixed seek so it works for any length).
	// scale=320:-2 keeps aspect ratio and rounds height to the nearest even number.
	args := []string{
		"-y",
		"-i", inputPath,
		"-frames:v", "1",
		"-q:v", "2",
		"-vf", "scale=320:-2",
		out,
	}

	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	if out2, err := cmd.CombinedOutput(); err != nil {
		// Video frame failed — try waveform (works for audio streams).
		log.Printf("[thumbnail] video frame failed (%v), trying waveform", err)
		log.Printf("[thumbnail] ffmpeg output: %s", string(out2))

		waveArgs := []string{
			"-y",
			"-i", inputPath,
			"-filter_complex", "showwavespic=s=320x100:colors=#1DB954",
			"-frames:v", "1",
			out,
		}
		waveCmd := exec.CommandContext(ctx, "ffmpeg", waveArgs...)
		if waveOut, waveErr := waveCmd.CombinedOutput(); waveErr != nil {
			return "", fmt.Errorf("thumbnail failed (video and waveform): %w — %s", waveErr, string(waveOut))
		}
	}

	// Guard: ffmpeg can exit 0 but write no frames (e.g. empty file).
	fi, statErr := os.Stat(out)
	if statErr != nil || fi.Size() == 0 {
		return "", fmt.Errorf("thumbnail: ffmpeg produced no output for %s", inputPath)
	}

	if cb != nil {
		cb(100)
	}
	return out, nil
}
