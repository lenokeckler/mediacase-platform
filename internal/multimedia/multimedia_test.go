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
	cancel()

	_, err := Convert(ctx, input, nil)
	if err == nil {
		t.Error("se esperaba error con contexto cancelado, se obtuvo nil")
	}
}

func TestParseFFmpegTime(t *testing.T) {
	cases := []struct {
		h, m, s, cs string
		want        float64
	}{
		{"0", "0", "5", "0", 5.0},
		{"0", "1", "0", "0", 60.0},
		{"1", "0", "0", "0", 3600.0},
		{"0", "0", "10", "50", 10.5},
		{"0", "6", "3", "18", 363.18},
		{"0", "0", "5", "123456", 5.123456},
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

func TestConvertTo_WebMYMKV(t *testing.T) {
	input := makeSyntheticVideo(t, "mp4")
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	for _, target := range []string{"webm", "mkv"} {
		out, err := ConvertTo(ctx, input, target, func(int) {})
		if err != nil {
			t.Fatalf("%s: %v", target, err)
		}
		if !strings.HasSuffix(out, "."+target) {
			t.Errorf("%s: salida %s", target, out)
		}
		if fi, _ := os.Stat(out); fi == nil || fi.Size() == 0 {
			t.Errorf("%s: salida vacía", target)
		}
		os.Remove(out)
	}
	if _, err := ConvertTo(ctx, input, "mp3", func(int) {}); err == nil {
		t.Error("mp3 no es destino de video")
	}
}

func TestConvertAudioTo_FormatosYMetadatos(t *testing.T) {
	input := makeSyntheticAudio(t, "wav")
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	for _, target := range []string{"flac", "mp3", "aac", "ogg"} {
		out, err := ConvertAudioTo(ctx, input, target, func(int) {})
		if err != nil {
			t.Fatalf("%s: %v", target, err)
		}
		if fi, _ := os.Stat(out); fi == nil || fi.Size() == 0 {
			t.Errorf("%s: salida vacía", target)
		}
		os.Remove(out)
	}
	out, err := Metadata(ctx, input, func(int) {})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(out)
	s := string(b)
	for _, want := range []string{`"container": "wav"`, `"duration_seconds": 3`, `"type": "audio"`, `"codec": "pcm_s16le"`, `"sample_rate": "44100"`} {
		if !strings.Contains(s, want) {
			t.Errorf("metadatos sin %s:\n%s", want, s[:min(len(s), 600)])
		}
	}
	os.Remove(out)
}

func TestThumbnailTo_PNG640(t *testing.T) {
	input := makeSyntheticVideo(t, "mp4")
	ctx := context.Background()
	out, err := ThumbnailTo(ctx, input, "png", 640, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(out, ".png") {
		t.Errorf("salida %s", out)
	}
	os.Remove(out)
}

func TestConvertTo_DimensionesImpares(t *testing.T) {
	input := filepath.Join(t.TempDir(), "impar.mkv")
	gen := exec.Command("ffmpeg", "-y", "-f", "lavfi", "-i", "testsrc2=size=321x179:rate=10", "-t", "2",
		"-c:v", "ffv1", input)
	if out, err := gen.CombinedOutput(); err != nil {
		t.Skipf("ffmpeg no disponible o falló: %v\n%s", err, string(out))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	for _, target := range []string{"mp4", "mkv", "webm"} {
		out, err := ConvertTo(ctx, input, target, func(int) {})
		if err != nil {
			t.Fatalf("%s: %v", target, err)
		}
		os.Remove(out)
	}
}

func TestConvertTo_ErrorConDetalleDeFFmpeg(t *testing.T) {
	input := filepath.Join(t.TempDir(), "roto.mp4")
	if err := os.WriteFile(input, []byte("esto no es un video"), 0o644); err != nil {
		t.Fatal(err)
	}
	args := []string{"-y", "-i", input, "-progress", "pipe:2", "-nostats", filepath.Join(t.TempDir(), "x.mp4")}
	_, err := runWithProgress(context.Background(), "convert", args, nil, "")
	if err == nil || !strings.Contains(err.Error(), "roto.mp4") {
		t.Errorf("el error debía traer el detalle de ffmpeg (nombra el archivo): %v", err)
	}
	t.Log(err)
}
