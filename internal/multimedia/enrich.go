package multimedia

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"image/jpeg"
	"log"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"

	"github.com/lenokeckler/mediacase-platform/internal/models"
)

type enrichContainer struct {
	audioOnly bool
	recipe    []string
	lyricsKey string
}

var enrichContainers = map[string]enrichContainer{
	"mp3":  {audioOnly: true, recipe: audioRecipes["mp3"], lyricsKey: "lyrics"},
	"flac": {audioOnly: true, recipe: audioRecipes["flac"], lyricsKey: "lyrics"},
	"ogg":  {audioOnly: true, recipe: audioRecipes["ogg"], lyricsKey: "lyrics"},
	"m4a":  {audioOnly: true, recipe: audioRecipes["aac"], lyricsKey: "lyrics"},
	"mp4":  {recipe: videoRecipes["mp4"], lyricsKey: "description"},
	"mkv":  {recipe: videoRecipes["mkv"], lyricsKey: "description"},
}

const coverWidth = 640

func Enrich(ctx context.Context, inputPath, target string, e *models.Enrichment, cb progressFn) (string, error) {
	c, ok := enrichContainers[target]
	if !ok {
		return "", fmt.Errorf("enrich: el contenedor %q no admite portada ni etiquetas", target)
	}
	cover, err := makeCover(ctx, inputPath, c.audioOnly)
	if err != nil {
		return "", err
	}
	defer os.Remove(cover)
	report(cb, 15)

	tags, err := mergedTags(ctx, inputPath, e, c.lyricsKey)
	if err != nil {
		return "", err
	}
	if target == "ogg" {

		pic, err := vorbisPictureBlock(cover)
		if err != nil {
			return "", err
		}
		tags["METADATA_BLOCK_PICTURE"] = pic
	}
	metaFile, err := writeFFMetadata(inputPath, tags)
	if err != nil {
		return "", err
	}
	defer os.Remove(metaFile)
	report(cb, 20)

	out := outputPath(inputPath, "."+target)
	build := func(copyStreams bool) []string {

		usesCoverInput := target != "mkv" && target != "ogg"
		args := []string{"-y", "-i", inputPath}
		metaIdx := 1
		if usesCoverInput {
			args = append(args, "-i", cover)
			metaIdx = 2
		}
		args = append(args, "-f", "ffmetadata", "-i", metaFile)
		switch {
		case target == "mkv":

			args = append(args, "-map", "0", "-attach", cover, "-metadata:s:t", "mimetype=image/jpeg", "-metadata:s:t", "filename=cover.jpg")
		case target == "ogg":
			args = append(args, "-map", "0:a")
		case c.audioOnly:
			args = append(args, "-map", "0:a", "-map", "1:v", "-disposition:v", "attached_pic")
		default:
			args = append(args, "-map", "0", "-map", "1:v", "-disposition:v:1", "attached_pic")
		}
		args = append(args, "-map_metadata", strconv.Itoa(metaIdx))
		if target == "mp3" {
			args = append(args, "-id3v2_version", "3", "-metadata:s:v", "title=Album cover", "-metadata:s:v", "comment=Cover (front)")
		}
		switch {
		case copyStreams:
			args = append(args, "-c", "copy")
		case c.audioOnly:
			args = append(args, c.recipe...)
			args = append(args, "-c:v", "copy")
		default:
			args = append(args, c.recipe...)
			args = append(args, "-c:v:1", "copy")
		}
		return append(args, "-progress", "pipe:2", "-nostats", out)
	}

	log.Printf("[enrich] %s → %s (remux)", inputPath, out)
	if _, err := runWithProgress(ctx, "enrich", build(true), scaleProgress(cb, 20, 100), out); err != nil {
		if ctx.Err() != nil {
			return "", err
		}

		log.Printf("[enrich] el remux no aplica (%v); recodificando a %s", err, target)
		if _, err := runWithProgress(ctx, "enrich", build(false), scaleProgress(cb, 20, 100), out); err != nil {
			return "", err
		}
	}
	if fi, err := os.Stat(out); err != nil || fi.Size() == 0 {
		return "", fmt.Errorf("enrich: ffmpeg no produjo salida para %s", inputPath)
	}
	report(cb, 100)
	return out, nil
}

func makeCover(ctx context.Context, inputPath string, audioOnly bool) (string, error) {
	cover := outputPath(inputPath, "_cover.jpg")
	w := strconv.Itoa(coverWidth)
	h := strconv.Itoa(coverWidth * 9 / 16)
	attempts := [][]string{
		{"-y", "-ss", "1", "-i", inputPath, "-frames:v", "1", "-vf", "scale=" + w + ":-2", "-q:v", "3", cover},
		{"-y", "-i", inputPath, "-frames:v", "1", "-vf", "scale=" + w + ":-2", "-q:v", "3", cover},
	}
	if audioOnly {
		attempts = [][]string{{"-y", "-i", inputPath, "-filter_complex",
			"color=c=0x0f172a:s=" + w + "x" + h + "[bg];[0:a]showwavespic=s=" + w + "x" + h + ":colors=#818cf8|#c4b5fd[w];[bg][w]overlay=format=auto",
			"-frames:v", "1", "-q:v", "3", cover}}
	}
	var lastErr error
	for _, args := range attempts {
		cmd := exec.CommandContext(ctx, "ffmpeg", args...)
		lowerPriority(cmd)
		out, err := cmd.CombinedOutput()
		if err == nil {
			if fi, statErr := os.Stat(cover); statErr == nil && fi.Size() > 0 {
				return cover, nil
			}
			err = fmt.Errorf("salida vacía")
		}
		lastErr = fmt.Errorf("%w — %s", err, firstLines(string(out), 3))
	}
	return "", fmt.Errorf("enrich: no se pudo generar la portada de %s: %v", inputPath, lastErr)
}

func mergedTags(ctx context.Context, inputPath string, e *models.Enrichment, lyricsKey string) (map[string]string, error) {
	tags := map[string]string{}
	raw, err := exec.CommandContext(ctx, "ffprobe", "-v", "error", "-print_format", "json", "-show_format", inputPath).Output()
	if err != nil {
		return nil, fmt.Errorf("enrich: ffprobe %s: %w", inputPath, err)
	}
	var p struct {
		Format struct {
			Tags map[string]string `json:"tags"`
		} `json:"format"`
	}
	if err := json.Unmarshal(raw, &p); err == nil {
		for k, v := range p.Format.Tags {
			switch strings.ToLower(k) {
			case "encoder", "major_brand", "minor_version", "compatible_brands":
				continue
			}
			tags[strings.ToLower(k)] = v
		}
	}
	if e != nil {
		for k, v := range map[string]string{"title": e.Title, "artist": e.Artist, "album": e.Album,
			"date": e.Date, "comment": e.Comment, lyricsKey: e.Lyrics} {
			if strings.TrimSpace(v) != "" {
				tags[k] = v
			}
		}
	}
	return tags, nil
}

func writeFFMetadata(inputPath string, tags map[string]string) (string, error) {
	keys := make([]string, 0, len(tags))
	for k := range tags {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteString(";FFMETADATA1\n")
	for _, k := range keys {
		b.WriteString(ffmetaEscape(k) + "=" + ffmetaEscape(tags[k]) + "\n")
	}
	path := outputPath(inputPath, "_meta.txt")
	return path, os.WriteFile(path, []byte(b.String()), 0o644)
}

var ffmetaEscaper = strings.NewReplacer(`\`, `\\`, "=", `\=`, ";", `\;`, "#", `\#`, "\r\n", "\\\n", "\n", "\\\n")

func ffmetaEscape(s string) string { return ffmetaEscaper.Replace(s) }

func vorbisPictureBlock(coverPath string) (string, error) {
	data, err := os.ReadFile(coverPath)
	if err != nil {
		return "", err
	}
	cfg, err := jpeg.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("enrich: portada ilegible: %w", err)
	}
	var buf bytes.Buffer
	putU32 := func(v uint32) { _ = binary.Write(&buf, binary.BigEndian, v) }
	putStr := func(s string) { putU32(uint32(len(s))); buf.WriteString(s) }
	putU32(3)
	putStr("image/jpeg")
	putStr("Cover (front)")
	putU32(uint32(cfg.Width))
	putU32(uint32(cfg.Height))
	putU32(24)
	putU32(0)
	putU32(uint32(len(data)))
	buf.Write(data)
	return base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}

func report(cb progressFn, pct int) {
	if cb != nil {
		cb(pct)
	}
}

func scaleProgress(cb progressFn, lo, hi int) progressFn {
	if cb == nil {
		return nil
	}
	return func(p int) { cb(lo + p*(hi-lo)/100) }
}
