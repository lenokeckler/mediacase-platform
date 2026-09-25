package multimedia

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/lenokeckler/mediacase-platform/internal/cases"
	"github.com/lenokeckler/mediacase-platform/internal/models"
)

func TestSniffAndConvert_ExtensionEnganosa(t *testing.T) {
	mp3 := makeSyntheticAudio(t, "mp3")
	raw, err := os.ReadFile(mp3)
	if err != nil {
		t.Fatalf("leer mp3 de prueba: %v", err)
	}

	misnamed := filepath.Join(filepath.Dir(mp3), "clip_con_extension_enganosa.mp4")
	if err := os.WriteFile(misnamed, raw, 0o644); err != nil {
		t.Fatalf("escribir mp3 con extensión .mp4: %v", err)
	}

	head := raw
	if len(head) > 65536 {
		head = head[:65536]
	}
	ft, format, ok := cases.SniffType(head)
	if !ok || ft != models.FileAudio || format != "mp3" {
		t.Fatalf("SniffType(mp3 con extensión .mp4) = tipo=%v formato=%v ok=%v; quería audio/mp3", ft, format, ok)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	out, err := ConvertAudioTo(ctx, misnamed, "wav", func(int) {})
	if err != nil {
		t.Fatalf("ConvertAudioTo sobre el archivo con extensión engañosa: %v", err)
	}
	defer os.Remove(out)
	if fi, statErr := os.Stat(out); statErr != nil || fi.Size() == 0 {
		t.Fatalf("salida vacía o inexistente: stat err=%v", statErr)
	}
}
