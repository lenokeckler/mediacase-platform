// internal/multimedia/convert.go
// FFmpeg wrapper: convierte cualquier video a MP4 (H.264 + AAC).
// El callback de progreso recibe 0-100 conforme FFmpeg avanza.
package multimedia

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// progressFn es llamado con el porcentaje de avance (0-100).
type progressFn func(int)

// durationRe extrae la duración total del header de FFmpeg.
var durationRe = regexp.MustCompile(`Duration:\s+(\d+):(\d+):(\d+)\.(\d+)`)

// timeRe extrae la posición actual de las líneas de progreso de FFmpeg.
var timeRe = regexp.MustCompile(`time=(\d+):(\d+):(\d+)\.(\d+)`)

// parseFFmpegTime convierte HH:MM:SS.cs a segundos totales.
func parseFFmpegTime(h, m, s, cs string) float64 {
	hi, _ := strconv.ParseFloat(h, 64)
	mi, _ := strconv.ParseFloat(m, 64)
	si, _ := strconv.ParseFloat(s, 64)
	csi, _ := strconv.ParseFloat(cs, 64)
	return hi*3600 + mi*60 + si + csi/100
}

// streamProgress lee stderr de FFmpeg y llama cb con el porcentaje calculado.
func streamProgress(r io.Reader, cb progressFn) {
	var totalSeconds float64
	scanner := bufio.NewScanner(r)
	scanner.Split(scanLines)

	for scanner.Scan() {
		line := scanner.Text()

		if totalSeconds == 0 {
			if m := durationRe.FindStringSubmatch(line); m != nil {
				totalSeconds = parseFFmpegTime(m[1], m[2], m[3], m[4])
			}
		}

		if m := timeRe.FindStringSubmatch(line); m != nil && totalSeconds > 0 {
			current := parseFFmpegTime(m[1], m[2], m[3], m[4])
			pct := int((current / totalSeconds) * 100)
			if pct > 100 {
				pct = 100
			}
			if cb != nil {
				cb(pct)
			}
		}
	}
}

// scanLines divide en \r o \n para capturar las líneas de progreso de FFmpeg.
func scanLines(data []byte, atEOF bool) (advance int, token []byte, err error) {
	if atEOF && len(data) == 0 {
		return 0, nil, nil
	}
	for i, b := range data {
		if b == '\n' || b == '\r' {
			return i + 1, data[:i], nil
		}
	}
	if atEOF {
		return len(data), data, nil
	}
	return 0, nil, nil
}

// probeHasStream returns true if the file contains at least one stream of the
// given type. streamType is "v" for video or "a" for audio.
// Uses ffprobe, which ships with every ffmpeg Alpine package.
func probeHasStream(ctx context.Context, inputPath, streamType string) bool {
	cmd := exec.CommandContext(ctx, "ffprobe",
		"-v", "error",
		"-select_streams", streamType+":0",
		"-show_entries", "stream=codec_type",
		"-of", "default=noprint_wrappers=1:nokey=1",
		inputPath,
	)
	out, _ := cmd.Output()
	return strings.TrimSpace(string(out)) != ""
}

// outputPath crea una ruta de salida única en /tmp con la extensión dada.
func outputPath(inputPath, ext string) string {
	dir := os.TempDir()
	base := strings.TrimSuffix(filepath.Base(inputPath), filepath.Ext(inputPath))
	ts := strconv.FormatInt(time.Now().UnixNano(), 36)
	return filepath.Join(dir, fmt.Sprintf("%s_%s%s", base, ts, ext))
}

// Convert convierte inputPath a MP4 usando H.264 + AAC.
// Retorna la ruta del archivo de salida y cualquier error.
func Convert(ctx context.Context, inputPath string, cb progressFn) (string, error) {
	if !probeHasStream(ctx, inputPath, "v") {
		return "", fmt.Errorf("convert: input has no video stream — use an audio operation instead")
	}

	out := outputPath(inputPath, ".mp4")
	log.Printf("[convert] %s → %s", inputPath, out)

	args := []string{
		"-y",
		"-i", inputPath,
		"-c:v", "libx264",
		"-preset", "fast",
		"-crf", "23",
		"-c:a", "aac",
		"-b:a", "128k",
		"-movflags", "+faststart",
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
		return "", fmt.Errorf("ffmpeg convert: %w", err)
	}
	return out, nil
}