// internal/multimedia/audio.go
// FFmpeg wrappers for audio operations.
package multimedia

import (
	"context"
	"fmt"
	"log"
	"os/exec"
)

// ExtractAudio extracts the audio track from inputPath and saves it as MP3.
// Returns a clear error if the input has no audio stream.
func ExtractAudio(ctx context.Context, inputPath string, cb progressFn) (string, error) {
	if !probeHasStream(ctx, inputPath, "a") {
		return "", fmt.Errorf("extract_audio: input has no audio stream — try a different file or operation")
	}

	out := outputPath(inputPath, ".mp3")
	log.Printf("[extract_audio] %s → %s", inputPath, out)

	args := []string{
		"-y",
		"-i", inputPath,
		"-vn",
		"-c:a", "libmp3lame",
		"-b:a", "192k",
		"-ar", "44100",
		"-progress", "pipe:2",
		"-nostats",
		out,
	}

	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return "", fmt.Errorf("stderr pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("ffmpeg start: %w", err)
	}

	go streamProgress(stderr, cb)

	if err := cmd.Wait(); err != nil {
		return "", fmt.Errorf("ffmpeg extract_audio: %w", err)
	}
	return out, nil
}

// ConvertAudio converts any audio file to WAV format (PCM 16-bit stereo 44.1 kHz).
// Returns a clear error if the input has no audio stream.
func ConvertAudio(ctx context.Context, inputPath string, cb progressFn) (string, error) {
	if !probeHasStream(ctx, inputPath, "a") {
		return "", fmt.Errorf("convert_audio: input has no audio stream — try a different file or operation")
	}

	out := outputPath(inputPath, ".wav")
	log.Printf("[convert_audio] %s → %s", inputPath, out)

	args := []string{
		"-y",
		"-i", inputPath,
		"-vn",
		"-c:a", "pcm_s16le",
		"-ar", "44100",
		"-ac", "2",
		"-progress", "pipe:2",
		"-nostats",
		out,
	}

	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return "", fmt.Errorf("stderr pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("ffmpeg start: %w", err)
	}

	go streamProgress(stderr, cb)

	if err := cmd.Wait(); err != nil {
		return "", fmt.Errorf("ffmpeg convert_audio: %w", err)
	}
	return out, nil
}
