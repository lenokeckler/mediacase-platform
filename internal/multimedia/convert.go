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

type progressFn func(int)

var durationRe = regexp.MustCompile(`Duration:\s+(\d+):(\d+):(\d+)\.(\d+)`)

var timeRe = regexp.MustCompile(`time=(\d+):(\d+):(\d+)\.(\d+)`)

func parseFFmpegTime(h, m, s, frac string) float64 {
	hi, _ := strconv.ParseFloat(h, 64)
	mi, _ := strconv.ParseFloat(m, 64)
	si, _ := strconv.ParseFloat(s, 64)
	fi, _ := strconv.ParseFloat("0."+frac, 64)
	return hi*3600 + mi*60 + si + fi
}

const ffmpegTailLines = 3

func streamProgress(r io.Reader, cb progressFn) []string {
	var totalSeconds float64
	var last []string
	scanner := bufio.NewScanner(r)
	scanner.Split(scanLines)

	for scanner.Scan() {
		line := scanner.Text()
		if isDiagnosticLine(line) {
			last = append(last, strings.TrimSpace(line))
			if len(last) > ffmpegTailLines {
				last = last[1:]
			}
		}

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
	return last
}

var progressLineRe = regexp.MustCompile(`^[a-z_0-9]+=\S*$`)

func isDiagnosticLine(line string) bool {
	line = strings.TrimSpace(line)
	return line != "" && !progressLineRe.MatchString(line)
}

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

			return false, fmt.Errorf("ffprobe: %s", strings.TrimSpace(string(exitErr.Stderr)))
		}
		return false, fmt.Errorf("ffprobe no disponible: %w", err)
	}
	return strings.TrimSpace(string(out)) != "", nil
}

func CheckTools() error {
	for _, tool := range []string{"ffmpeg", "ffprobe"} {
		if _, err := exec.LookPath(tool); err != nil {
			return fmt.Errorf("%s no está en el PATH: %w", tool, err)
		}
	}
	return nil
}

var outputSeq atomic.Uint64

func outputPath(inputPath, ext string) string {
	dir := os.TempDir()
	base := strings.TrimSuffix(filepath.Base(inputPath), filepath.Ext(inputPath))
	ts := strconv.FormatInt(time.Now().UnixNano(), 36)
	seq := strconv.FormatUint(outputSeq.Add(1), 36)
	return filepath.Join(dir, fmt.Sprintf("%s_%s_%s%s", base, ts, seq, ext))
}

func Convert(ctx context.Context, inputPath string, cb progressFn) (string, error) {
	return ConvertTo(ctx, inputPath, "mp4", cb)
}
