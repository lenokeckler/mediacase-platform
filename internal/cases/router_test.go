package cases

import (
	"testing"

	"github.com/lenokeckler/mediacase-platform/internal/models"
)

func TestDetectFileType(t *testing.T) {
	tests := map[string]models.FileType{
		"boda.mp4": models.FileVideo, "x.MKV": models.FileVideo, "a.webm": models.FileVideo, "c.mov": models.FileVideo,
		"discurso.mp3": models.FileAudio, "b.flac": models.FileAudio, "c.wav": models.FileAudio, "d.OGG": models.FileAudio,
		"foto.jpg": models.FileImage, "d.PNG": models.FileImage, "e.jpeg": models.FileImage,
		"casos/boda/clip.mp4": models.FileVideo, // claves con "carpeta"
	}
	for name, want := range tests {
		got, err := DetectFileType(name)
		if err != nil || got != want {
			t.Errorf("%s: got %v (err=%v), want %v", name, got, err, want)
		}
	}
	for _, bad := range []string{"archivo.exe", "sin-extension", "doc.pdf", ""} {
		if _, err := DetectFileType(bad); err == nil {
			t.Errorf("%q debería ser no soportado", bad)
		}
	}
}

func TestRoute_ElCoordinadorDecideSiElClienteNoPide(t *testing.T) {
	tests := []struct {
		file string
		op   models.Operation
		pool string
	}{
		{"v.mp4", models.OpConvert, "video"},
		{"a.mp3", models.OpConvertAudio, "audio"},
		{"i.jpg", models.OpThumbnail, "metadata"},
	}
	for _, tc := range tests {
		d, err := Route(tc.file, "")
		if err != nil || d.Operation != tc.op || d.Pool != tc.pool {
			t.Errorf("%s: %+v err=%v; quería op=%s pool=%s", tc.file, d, err, tc.op, tc.pool)
		}
	}
}

func TestRoute_RespetaOperacionValidaDelCliente(t *testing.T) {
	d, err := Route("v.mp4", models.OpExtractAudio)
	if err != nil || d.Operation != models.OpExtractAudio || d.FileType != models.FileVideo || d.Pool != "video" {
		t.Fatalf("%+v err=%v", d, err)
	}
	d, err = Route("v.mp4", models.OpThumbnail)
	if err != nil || d.Operation != models.OpThumbnail {
		t.Fatalf("miniatura de video es válida: %+v err=%v", d, err)
	}
}

func TestRoute_RechazaOperacionIncompatible(t *testing.T) {
	if _, err := Route("a.mp3", models.OpExtractAudio); err == nil {
		t.Error("extract_audio sobre un audio no tiene sentido; debe rechazarse")
	}
	if _, err := Route("v.mp4", models.OpConvertAudio); err == nil {
		t.Error("convert_audio sobre un video debe rechazarse")
	}
	if _, err := Route("i.jpg", models.OpConvert); err == nil {
		t.Error("convert (video) sobre una imagen debe rechazarse")
	}
	if _, err := Route("v.mp4", models.Operation("inventada")); err == nil {
		t.Error("operación desconocida debe rechazarse")
	}
}

func TestPoolFor_CubreTodosLosTipos(t *testing.T) {
	for _, ft := range []models.FileType{models.FileVideo, models.FileAudio, models.FileImage} {
		if PoolFor(ft) == "" {
			t.Errorf("tipo %s sin pool", ft)
		}
	}
}
