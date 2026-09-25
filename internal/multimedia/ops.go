package multimedia

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

var videoRecipes = map[string][]string{
	"mp4":  {"-c:v", "libx264", "-preset", "fast", "-crf", "23", "-c:a", "aac", "-b:a", "128k", "-movflags", "+faststart"},
	"mkv":  {"-c:v", "libx264", "-preset", "fast", "-crf", "23", "-c:a", "aac", "-b:a", "128k"},
	"webm": {"-c:v", "libvpx-vp9", "-deadline", "realtime", "-cpu-used", "8", "-crf", "33", "-b:v", "0", "-c:a", "libopus", "-b:a", "96k"},
}

var videoNormalize = []string{"-vf", "scale=trunc(iw/2)*2:trunc(ih/2)*2", "-pix_fmt", "yuv420p"}

var audioRecipes = map[string][]string{
	"mp3":  {"-c:a", "libmp3lame", "-b:a", "192k", "-ar", "44100"},
	"wav":  {"-c:a", "pcm_s16le", "-ar", "44100", "-ac", "2"},
	"flac": {"-c:a", "flac", "-compression_level", "5"},
	"aac":  {"-c:a", "aac", "-b:a", "192k"},
	"ogg":  {"-c:a", "libvorbis", "-q:a", "5"},
}

func ConvertTo(ctx context.Context, inputPath, target string, cb progressFn) (string, error) {
	recipe, ok := videoRecipes[target]
	if !ok {
		return "", fmt.Errorf("convert: formato de salida %q no soportado", target)
	}
	if has, err := probeHasStream(ctx, inputPath, "v"); err != nil {
		return "", fmt.Errorf("convert: %w", err)
	} else if !has {
		return "", fmt.Errorf("convert: la entrada no tiene pista de video; use una operación de audio")
	}
	out := outputPath(inputPath, "."+target)
	log.Printf("[convert] %s → %s", inputPath, out)
	args := append([]string{"-y", "-i", inputPath}, recipe...)
	args = append(args, videoNormalize...)
	args = append(args, "-progress", "pipe:2", "-nostats", out)
	return runWithProgress(ctx, "convert", args, cb, out)
}

func ExtractAudioTo(ctx context.Context, inputPath, target string, cb progressFn) (string, error) {
	recipe, ok := audioRecipes[target]
	if !ok {
		return "", fmt.Errorf("extract_audio: formato de salida %q no soportado", target)
	}
	if has, err := probeHasStream(ctx, inputPath, "a"); err != nil {
		return "", fmt.Errorf("extract_audio: %w", err)
	} else if !has {
		return "", fmt.Errorf("extract_audio: la entrada no tiene pista de audio")
	}
	out := outputPath(inputPath, "."+target)
	log.Printf("[extract_audio] %s → %s", inputPath, out)
	args := append([]string{"-y", "-i", inputPath, "-vn"}, recipe...)
	args = append(args, "-progress", "pipe:2", "-nostats", out)
	return runWithProgress(ctx, "extract_audio", args, cb, out)
}

func ConvertAudioTo(ctx context.Context, inputPath, target string, cb progressFn) (string, error) {
	recipe, ok := audioRecipes[target]
	if !ok {
		return "", fmt.Errorf("convert_audio: formato de salida %q no soportado", target)
	}
	if has, err := probeHasStream(ctx, inputPath, "a"); err != nil {
		return "", fmt.Errorf("convert_audio: %w", err)
	} else if !has {
		return "", fmt.Errorf("convert_audio: la entrada no tiene pista de audio")
	}
	out := outputPath(inputPath, "."+target)
	log.Printf("[convert_audio] %s → %s", inputPath, out)
	args := append([]string{"-y", "-i", inputPath, "-vn"}, recipe...)
	args = append(args, "-progress", "pipe:2", "-nostats", out)
	return runWithProgress(ctx, "convert_audio", args, cb, out)
}

func ThumbnailTo(ctx context.Context, inputPath, target string, width int, cb progressFn) (string, error) {
	if target != "jpg" && target != "png" && target != "webp" {
		return "", fmt.Errorf("thumbnail: formato de salida %q no soportado", target)
	}
	if width <= 0 {
		width = 320
	}
	out := outputPath(inputPath, "."+target)
	log.Printf("[thumbnail] %s → %s (%dpx)", inputPath, out, width)
	w := strconv.Itoa(width)
	args := []string{"-y", "-i", inputPath, "-frames:v", "1", "-vf", "scale=" + w + ":-2"}
	if target == "jpg" {
		args = append(args, "-q:v", "2")
	}
	args = append(args, out)
	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	lowerPriority(cmd)
	if out2, err := cmd.CombinedOutput(); err != nil {

		log.Printf("[thumbnail] sin fotograma (%v), probando forma de onda", err)
		log.Printf("[thumbnail] ffmpeg: %s", firstLines(string(out2), 3))
		h := strconv.Itoa(width * 5 / 16)
		waveArgs := []string{"-y", "-i", inputPath, "-filter_complex", "showwavespic=s=" + w + "x" + h + ":colors=#6366f1", "-frames:v", "1", out}
		waveCmd := exec.CommandContext(ctx, "ffmpeg", waveArgs...)
		if waveOut, waveErr := waveCmd.CombinedOutput(); waveErr != nil {
			return "", fmt.Errorf("thumbnail falló (fotograma y forma de onda): %w — %s", waveErr, firstLines(string(waveOut), 3))
		}
	}
	if fi, err := os.Stat(out); err != nil || fi.Size() == 0 {
		return "", fmt.Errorf("thumbnail: ffmpeg no produjo salida para %s", inputPath)
	}
	if cb != nil {
		cb(100)
	}
	return out, nil
}

func Metadata(ctx context.Context, inputPath string, cb progressFn) (string, error) {
	cmd := exec.CommandContext(ctx, "ffprobe", "-v", "error", "-print_format", "json",
		"-show_format", "-show_streams", inputPath)
	raw, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("ffprobe: %w", err)
	}
	summary, err := summarizeProbe(raw)
	if err != nil {
		return "", err
	}
	out := outputPath(inputPath, ".json")
	if err := os.WriteFile(out, summary, 0o644); err != nil {
		return "", err
	}
	if cb != nil {
		cb(100)
	}
	return out, nil
}

func summarizeProbe(raw []byte) ([]byte, error) {
	var p struct {
		Format struct {
			Filename   string            `json:"filename"`
			FormatName string            `json:"format_name"`
			FormatLong string            `json:"format_long_name"`
			Duration   string            `json:"duration"`
			Size       string            `json:"size"`
			BitRate    string            `json:"bit_rate"`
			Tags       map[string]string `json:"tags"`
		} `json:"format"`
		Streams []map[string]any `json:"streams"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, fmt.Errorf("ffprobe json: %w", err)
	}
	dur, _ := strconv.ParseFloat(p.Format.Duration, 64)
	size, _ := strconv.ParseInt(p.Format.Size, 10, 64)
	br, _ := strconv.ParseInt(p.Format.BitRate, 10, 64)
	type stream struct {
		Index      int    `json:"index"`
		Type       string `json:"type"`
		Codec      string `json:"codec"`
		CodecLong  string `json:"codec_long,omitempty"`
		Width      int    `json:"width,omitempty"`
		Height     int    `json:"height,omitempty"`
		FPS        string `json:"fps,omitempty"`
		SampleRate string `json:"sample_rate,omitempty"`
		Channels   int    `json:"channels,omitempty"`
		BitRate    string `json:"bit_rate,omitempty"`
	}
	var streams []stream
	for _, s := range p.Streams {
		st := stream{Type: str(s["codec_type"]), Codec: str(s["codec_name"]), CodecLong: str(s["codec_long_name"]),
			FPS: str(s["avg_frame_rate"]), SampleRate: str(s["sample_rate"]), BitRate: str(s["bit_rate"])}
		st.Index = num(s["index"])
		st.Width, st.Height, st.Channels = num(s["width"]), num(s["height"]), num(s["channels"])
		if st.FPS == "0/0" {
			st.FPS = ""
		}
		streams = append(streams, st)
	}
	out := map[string]any{
		"file":             p.Format.Filename,
		"container":        p.Format.FormatName,
		"container_long":   p.Format.FormatLong,
		"duration_seconds": dur,
		"size_bytes":       size,
		"bit_rate":         br,
		"streams":          streams,
		"tags":             p.Format.Tags,
		"ffprobe":          json.RawMessage(raw),
	}
	return json.MarshalIndent(out, "", "  ")
}

func str(v any) string {
	s, _ := v.(string)
	return s
}

func num(v any) int {
	switch x := v.(type) {
	case float64:
		return int(x)
	case string:
		n, _ := strconv.Atoi(x)
		return n
	}
	return 0
}

func firstLines(s string, n int) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, " | ")
}

func runWithProgress(ctx context.Context, op string, args []string, cb progressFn, out string) (string, error) {
	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	lowerPriority(cmd)
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return "", fmt.Errorf("stderr pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("ffmpeg start: %w", err)
	}
	lowerPriorityStarted(cmd)
	tail := make(chan []string, 1)
	go func() { tail <- streamProgress(stderr, cb) }()
	err = cmd.Wait()
	last := <-tail
	if err != nil {
		if len(last) > 0 {
			return "", fmt.Errorf("ffmpeg %s: %w: %s", op, err, strings.Join(last, " | "))
		}
		return "", fmt.Errorf("ffmpeg %s: %w", op, err)
	}
	return out, nil
}
