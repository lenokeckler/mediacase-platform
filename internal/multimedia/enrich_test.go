package multimedia

import (
	"context"
	"encoding/json"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lenokeckler/mediacase-platform/internal/models"
)

type probeResult struct {
	cover bool
	tags  map[string]string
}

func probeEnrichment(t *testing.T, path string) probeResult {
	t.Helper()
	raw, err := exec.Command("ffprobe", "-v", "error", "-print_format", "json",
		"-show_format", "-show_streams", path).Output()
	if err != nil {
		t.Fatalf("ffprobe %s: %v", path, err)
	}
	var p struct {
		Format struct {
			Tags map[string]string `json:"tags"`
		} `json:"format"`
		Streams []struct {
			CodecType   string            `json:"codec_type"`
			Disposition map[string]int    `json:"disposition"`
			Tags        map[string]string `json:"tags"`
		} `json:"streams"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		t.Fatalf("ffprobe json: %v", err)
	}
	r := probeResult{tags: map[string]string{}}
	for k, v := range p.Format.Tags {
		r.tags[strings.ToLower(k)] = v
	}
	for _, s := range p.Streams {
		if s.Disposition["attached_pic"] == 1 {
			r.cover = true
		}
		if s.CodecType == "attachment" && strings.HasPrefix(s.Tags["mimetype"], "image/") {
			r.cover = true
		}

		for k, v := range s.Tags {
			if _, dup := r.tags[strings.ToLower(k)]; !dup {
				r.tags[strings.ToLower(k)] = v
			}
		}
	}
	return r
}

func sampleEnrichment() *models.Enrichment {
	return &models.Enrichment{
		Title: "Tema de prueba", Artist: "Equipo JLJ", Album: "concierto-s1",
		Date: "2026", Comment: "lote-2026-09-06", Lyrics: "primera línea\nsegunda línea",
	}
}

func TestEnrich_PortadaEtiquetasYLetraPorContenedor(t *testing.T) {
	cases := []struct {
		name    string
		mk      func(*testing.T, string) string
		src     string
		target  string
		wantExt string
	}{
		{"mp3 → mp3 (copia)", makeSyntheticAudio, "mp3", "mp3", ".mp3"},
		{"flac → flac (copia)", makeSyntheticAudio, "flac", "flac", ".flac"},
		{"m4a → m4a (copia)", makeSyntheticAudio, "m4a", "m4a", ".m4a"},
		{"ogg → ogg (copia)", makeSyntheticAudio, "ogg", "ogg", ".ogg"},
		{"wav → mp3 (recodifica)", makeSyntheticAudio, "wav", "mp3", ".mp3"},
		{"mp4 → mp4 (copia)", makeSyntheticVideo, "mp4", "mp4", ".mp4"},
		{"mkv → mkv (copia)", makeSyntheticVideo, "mkv", "mkv", ".mkv"},
		{"mov → mp4 (remux, mismos códecs)", makeSyntheticVideo, "mov", "mp4", ".mp4"},
		{"mkv → mp4 (remux, mismos códecs)", makeSyntheticVideo, "mkv", "mp4", ".mp4"},
		{"avi → mp4 (recodifica)", makeSyntheticVideo, "avi", "mp4", ".mp4"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := tc.mk(t, tc.src)
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()
			var last int
			out, err := Enrich(ctx, in, tc.target, sampleEnrichment(), func(p int) { last = p })
			if err != nil {
				t.Fatalf("Enrich: %v", err)
			}
			if filepath.Ext(out) != tc.wantExt {
				t.Errorf("extensión %s, quería %s", filepath.Ext(out), tc.wantExt)
			}
			if last != 100 {
				t.Errorf("progreso final %d, quería 100", last)
			}
			r := probeEnrichment(t, out)
			if !r.cover {
				t.Errorf("sin portada embebida: %+v", r)
			}
			for k, want := range map[string]string{"title": "Tema de prueba", "artist": "Equipo JLJ", "album": "concierto-s1", "date": "2026"} {
				if r.tags[k] != want {
					t.Errorf("etiqueta %s = %q, quería %q (tags: %v)", k, r.tags[k], want, r.tags)
				}
			}
			if !hasText(r.tags, "primera línea") {
				t.Errorf("letra/descripción no quedó en el archivo: %v", r.tags)
			}
		})
	}
}

func hasText(tags map[string]string, s string) bool {
	for _, v := range tags {
		if strings.Contains(v, s) {
			return true
		}
	}
	return false
}

func TestEnrich_SoloPortadaSiNoHayEtiquetas(t *testing.T) {
	in := makeSyntheticAudio(t, "mp3")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	out, err := Enrich(ctx, in, "mp3", nil, nil)
	if err != nil {
		t.Fatalf("Enrich: %v", err)
	}
	if r := probeEnrichment(t, out); !r.cover {
		t.Errorf("sin portada: %+v", r)
	}
}

func TestEnrich_RechazaDestinoNoSoportado(t *testing.T) {
	in := makeSyntheticAudio(t, "mp3")
	if _, err := Enrich(context.Background(), in, "wav", sampleEnrichment(), nil); err == nil {
		t.Error("wav no admite portada ni letra: debía rechazarse")
	}
	if _, err := Enrich(context.Background(), in, "mov", sampleEnrichment(), nil); err == nil {
		t.Error("mov (QuickTime) no escribe portada ni descripción con ffmpeg: debía rechazarse")
	}
}

func TestEnrich_ConservaEtiquetasOriginalesYLetraMultilinea(t *testing.T) {
	in := makeSyntheticAudio(t, "flac")
	tagged := filepath.Join(t.TempDir(), "tagged.flac")
	if out, err := exec.Command("ffmpeg", "-y", "-i", in, "-metadata", "genre=Rock", "-metadata", "title=viejo", "-c", "copy", tagged).CombinedOutput(); err != nil {
		t.Skipf("ffmpeg: %v %s", err, out)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	lyrics := "verso 1: a=b; c\nverso 2 #final"
	out, err := Enrich(ctx, tagged, "flac", &models.Enrichment{Title: "nuevo", Lyrics: lyrics}, nil)
	if err != nil {
		t.Fatalf("Enrich: %v", err)
	}
	r := probeEnrichment(t, out)
	if r.tags["genre"] != "Rock" || r.tags["title"] != "nuevo" {
		t.Errorf("etiquetas: %v", r.tags)
	}
	if r.tags["lyrics"] != lyrics {
		t.Errorf("letra = %q, quería %q", r.tags["lyrics"], lyrics)
	}
}
