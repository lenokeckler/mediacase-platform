// internal/multimedia/multimedia_test.go
// Tests unitarios para los wrappers de FFmpeg.
// Requiere FFmpeg instalado en el ambiente de test (está en el Docker del worker).
package multimedia

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// makeSyntheticVideo crea un video de prueba de 3 segundos con FFmpeg lavfi.
func makeSyntheticVideo(t *testing.T, format string) string {
	t.Helper()
	out := filepath.Join(t.TempDir(), "test_input."+format)
	args := []string{
		"-y",
		"-f", "lavfi", "-i", "color=c=red:size=320x240:rate=10",
		"-f", "lavfi", "-i", "sine=frequency=440:sample_rate=44100",
		"-t", "3",
		"-c:v", "libx264", "-preset", "ultrafast", "-crf", "40",
		"-c:a", "aac", "-b:a", "32k",
		out,
	}
	cmd := exec.Command("ffmpeg", args...)
	if out2, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("ffmpeg no disponible o falló: %v\n%s", err, string(out2))
	}
	return out
}

// makeSyntheticAudio crea un audio de prueba de 3 segundos con FFmpeg lavfi.
func makeSyntheticAudio(t *testing.T, format string) string {
	t.Helper()
	out := filepath.Join(t.TempDir(), "test_input."+format)
	args := []string{
		"-y",
		"-f", "lavfi",
		"-i", "sine=frequency=440:sample_rate=44100",
		"-t", "3",
		out,
	}
	cmd := exec.Command("ffmpeg", args...)
	if out2, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("ffmpeg no disponible o falló: %v\n%s", err, string(out2))
	}
	return out
}

func TestConvert_MP4Output(t *testing.T) {
	input := makeSyntheticVideo(t, "mkv")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var lastPct int
	result, err := Convert(ctx, input, func(pct int) { lastPct = pct })
	if err != nil {
		t.Fatalf("Convert falló: %v", err)
	}
	defer os.Remove(result)

	fi, err := os.Stat(result)
	if err != nil {
		t.Fatalf("archivo resultado no existe: %v", err)
	}
	if fi.Size() == 0 {
		t.Fatal("archivo resultado está vacío")
	}
	if !strings.HasSuffix(result, ".mp4") {
		t.Errorf("se esperaba .mp4, se obtuvo %s", result)
	}
	t.Logf("Convert OK: %s (%d bytes, progreso final=%d%%)", result, fi.Size(), lastPct)
}

func TestExtractAudio_MP3Output(t *testing.T) {
	input := makeSyntheticVideo(t, "mp4")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, err := ExtractAudio(ctx, input, func(pct int) {})
	if err != nil {
		t.Fatalf("ExtractAudio falló: %v", err)
	}
	defer os.Remove(result)

	fi, err := os.Stat(result)
	if err != nil {
		t.Fatalf("archivo resultado no existe: %v", err)
	}
	if fi.Size() == 0 {
		t.Fatal("archivo resultado está vacío")
	}
	if !strings.HasSuffix(result, ".mp3") {
		t.Errorf("se esperaba .mp3, se obtuvo %s", result)
	}
	t.Logf("ExtractAudio OK: %s (%d bytes)", result, fi.Size())
}

func TestThumbnail_JPEGOutput(t *testing.T) {
	input := makeSyntheticVideo(t, "mp4")
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	var called bool
	result, err := Thumbnail(ctx, input, func(pct int) { called = true })
	if err != nil {
		t.Fatalf("Thumbnail falló: %v", err)
	}
	defer os.Remove(result)

	fi, err := os.Stat(result)
	if err != nil {
		t.Fatalf("archivo resultado no existe: %v", err)
	}
	if fi.Size() == 0 {
		t.Fatal("archivo resultado está vacío")
	}
	if !strings.HasSuffix(result, ".jpg") {
		t.Errorf("se esperaba .jpg, se obtuvo %s", result)
	}
	if !called {
		t.Error("el callback de progreso nunca fue llamado")
	}
	t.Logf("Thumbnail OK: %s (%d bytes)", result, fi.Size())
}

func TestConvert_ContextCancelado(t *testing.T) {
	input := makeSyntheticVideo(t, "mp4")
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancelar inmediatamente

	_, err := Convert(ctx, input, nil)
	if err == nil {
		t.Error("se esperaba error con contexto cancelado, se obtuvo nil")
	}
}

func TestParseFFmpegTime(t *testing.T) {
	cases := []struct {
		h, m, s, cs string
		want         float64
	}{
		{"0", "0", "5", "0", 5.0},
		{"0", "1", "0", "0", 60.0},
		{"1", "0", "0", "0", 3600.0},
		{"0", "0", "10", "50", 10.5},
	}
	for _, c := range cases {
		got := parseFFmpegTime(c.h, c.m, c.s, c.cs)
		if got != c.want {
			t.Errorf("parseFFmpegTime(%s,%s,%s,%s) = %f, want %f",
				c.h, c.m, c.s, c.cs, got, c.want)
		}
	}
}

func TestOutputPath_Unicidad(t *testing.T) {
	p1 := outputPath("/some/video.mkv", ".mp4")
	p2 := outputPath("/some/video.mkv", ".mp4")
	if p1 == p2 {
		t.Error("outputPath debe generar nombres únicos en cada llamada")
	}
}