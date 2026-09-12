// internal/multimedia/convert.go
// FFmpeg wrapper: convierte cualquier video a MP4 (H.264 + AAC).
// El callback de progreso recibe 0-100 conforme FFmpeg avanza.
package multimedia

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
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
// probeHasStream pregunta a ffprobe si el archivo tiene un stream del tipo dado.
// Un ffprobe ausente o roto se reporta como error, no como "no hay stream".
func probeHasStream(ctx context.Context, inputPath, streamType string) (bool, error) {
	cmd := exec.CommandContext(ctx, "ffprobe",
		"-v", "error",
		"-select_streams", streamType+":0",
		"-show_entries", "stream=codec_type",
		"-of", "default=noprint_wrappers=1:nokey=1",
		inputPath,
	)
	out, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			// ffprobe corrió pero el archivo no se pudo leer (corrupto, formato inválido)
			return false, fmt.Errorf("ffprobe: %s", strings.TrimSpace(string(exitErr.Stderr)))
		}
		return false, fmt.Errorf("ffprobe no disponible: %w", err)
	}
	return strings.TrimSpace(string(out)) != "", nil
}

// CheckTools verifica que ffmpeg y ffprobe estén en el PATH. Sin ellos el worker es inútil.
func CheckTools() error {
	for _, tool := range []string{"ffmpeg", "ffprobe"} {
		if _, err := exec.LookPath(tool); err != nil {
			return fmt.Errorf("%s no está en el PATH: %w", tool, err)
		}
	}
	return nil
}

// outputSeq garantiza unicidad aunque dos llamadas caigan en el mismo tick del reloj
// (en Windows la resolución de time.Now es ~15 ms).
var outputSeq atomic.Uint64

// outputPath crea una ruta de salida única en el directorio temporal con la extensión dada.
func outputPath(inputPath, ext string) string {
	dir := os.TempDir()
	base := strings.TrimSuffix(filepath.Base(inputPath), filepath.Ext(inputPath))
	ts := strconv.FormatInt(time.Now().UnixNano(), 36)
	seq := strconv.FormatUint(outputSeq.Add(1), 36)
	return filepath.Join(dir, fmt.Sprintf("%s_%s_%s%s", base, ts, seq, ext))
}

// Convert convierte a MP4 (H.264 + AAC), el destino por defecto. Ver ConvertTo.
func Convert(ctx context.Context, inputPath string, cb progressFn) (string, error) {
	return ConvertTo(ctx, inputPath, "mp4", cb)
}
