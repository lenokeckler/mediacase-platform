// "Integración de letras o recursos informativos asociados" (consigna): Enrich devuelve el mismo
// audio o video con una portada embebida, etiquetas (título, artista, álbum, fecha, comentario) y
// la letra o descripción, todo dentro del contenedor. Conserva las etiquetas que el archivo ya
// traía. Primero intenta un remux con -c copy (liviano: por eso corre en el pool metadata); si el
// contenedor de destino no acepta los códecs del origen, recodifica con la receta del destino.
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

// Contenedores que admiten portada + etiquetas + letra/descripción, y cómo se les escribe.
type enrichContainer struct {
	audioOnly bool
	recipe    []string // códecs si hay que recodificar
	lyricsKey string   // clave de la letra (audio) o descripción (video) en ese contenedor
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

// Enrich integra los recursos asociados de e en inputPath y devuelve la ruta del archivo
// resultante con extensión target. e puede ser nil: entonces solo se embebe la portada.
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
		// Ogg no acepta una pista de portada: va como bloque de imagen FLAC en los comentarios Vorbis.
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
		// Todas las entradas primero (ffmpeg exige las opciones de salida después de la última -i).
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
			// En Matroska la portada viaja como adjunto, no como pista.
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
			args = append(args, "-c:v", "copy") // la única pista de video es la portada
		default:
			args = append(args, c.recipe...)
			args = append(args, "-c:v:1", "copy") // la portada (segunda pista de video) no se recodifica
		}
		return append(args, "-progress", "pipe:2", "-nostats", out)
	}

	log.Printf("[enrich] %s → %s (remux)", inputPath, out)
	if _, err := runWithProgress(ctx, "enrich", build(true), scaleProgress(cb, 20, 100), out); err != nil {
		if ctx.Err() != nil {
			return "", err
		}
		// Los códecs del origen no caben en el contenedor de destino (wav → mp3, avi → mp4…).
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

// makeCover genera la portada: el fotograma del segundo 1 para video, la forma de onda sobre
// fondo oscuro para audio. Siempre JPEG, que es lo que todos los contenedores y reproductores aceptan.
func makeCover(ctx context.Context, inputPath string, audioOnly bool) (string, error) {
	cover := outputPath(inputPath, "_cover.jpg")
	w := strconv.Itoa(coverWidth)
	h := strconv.Itoa(coverWidth * 9 / 16)
	attempts := [][]string{
		{"-y", "-ss", "1", "-i", inputPath, "-frames:v", "1", "-vf", "scale=" + w + ":-2", "-q:v", "3", cover},
		{"-y", "-i", inputPath, "-frames:v", "1", "-vf", "scale=" + w + ":-2", "-q:v", "3", cover}, // video de menos de 1 s
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

// mergedTags son las etiquetas del archivo original más las del cliente (que ganan si repiten).
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
				continue // los pone el propio ffmpeg
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

// writeFFMetadata escribe las etiquetas en el formato ffmetadata (evita los límites y el
// escapado de la línea de comandos: letras de varias líneas, '=' o ';' en los textos).
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

// vorbisPictureBlock arma el bloque PICTURE de FLAC (tipo 3 = portada frontal) en base64, que es
// como Ogg/Vorbis transporta la carátula (METADATA_BLOCK_PICTURE).
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

// scaleProgress reubica el 0-100 de ffmpeg dentro de [lo, hi] del progreso total.
func scaleProgress(cb progressFn, lo, hi int) progressFn {
	if cb == nil {
		return nil
	}
	return func(p int) { cb(lo + p*(hi-lo)/100) }
}
