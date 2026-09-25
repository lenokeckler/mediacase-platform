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
		if d.Target != DefaultTargetFor(tc.op, tc.file) || d.Target == "" {
			t.Errorf("%s: sin destino por defecto: %+v", tc.file, d)
		}
	}
}

func TestRouteWith_DestinoYAncho(t *testing.T) {
	d, err := RouteWith("v.mkv", models.OpConvert, "webm", 0)
	if err != nil || d.Target != "webm" || d.Pool != "video" {
		t.Fatalf("%+v err=%v", d, err)
	}
	d, err = RouteWith("v.mkv", models.OpExtractAudio, "FLAC", 0) // mayúsculas y punto se toleran
	if err != nil || d.Target != "flac" {
		t.Fatalf("%+v err=%v", d, err)
	}
	if _, err := RouteWith("v.mkv", models.OpConvert, "mp3", 0); err == nil {
		t.Error("mp3 no es un destino de convert (video)")
	}
	d, err = RouteWith("i.png", models.OpThumbnail, "webp", 640)
	if err != nil || d.Width != 640 || d.Target != "webp" || d.Pool != "metadata" {
		t.Fatalf("%+v err=%v", d, err)
	}
	if _, err := RouteWith("i.png", models.OpThumbnail, "", 500); err == nil {
		t.Error("ancho fuera del catálogo debe rechazarse")
	}
	d, _ = RouteWith("v.mp4", models.OpConvert, "", 640)
	if d.Width != 0 {
		t.Error("el ancho solo aplica a miniaturas")
	}
}

func TestRoute_MetadatosVaAlPoolLiviano(t *testing.T) {
	for _, f := range []string{"v.mp4", "a.flac", "i.jpg"} {
		d, err := Route(f, models.OpMetadata)
		if err != nil || d.Target != "json" || d.Pool != "metadata" {
			t.Errorf("%s: %+v err=%v", f, d, err)
		}
	}
	// La miniatura de un video también es liviana: pool metadata, no video.
	if d, _ := Route("v.mp4", models.OpThumbnail); d.Pool != "metadata" {
		t.Errorf("miniatura de video debe ir al pool metadata: %+v", d)
	}
}

func TestDetectFileType_FormatosNuevos(t *testing.T) {
	for f, want := range map[string]models.FileType{"x.aiff": models.FileAudio, "x.dsf": models.FileAudio, "x.opus": models.FileAudio, "x.m4v": models.FileVideo, "x.tiff": models.FileImage} {
		if ft, err := DetectFileType(f); err != nil || ft != want {
			t.Errorf("%s: %v %v", f, ft, err)
		}
	}
}

func TestRoute_RespetaOperacionValidaDelCliente(t *testing.T) {
	d, err := Route("v.mp4", models.OpExtractAudio)
	if err != nil || d.Operation != models.OpExtractAudio || d.FileType != models.FileVideo || d.Pool != "video" || d.Target != "mp3" {
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

// Convertir un archivo al formato que ya tiene no es una conversión: el formato de origen no se
// ofrece ni se elige por defecto en convert y convert_audio. En miniatura sí (es un cambio de
// tamaño, no de formato: png → png 320 px conserva la transparencia).
func TestRoute_NoOfreceElFormatoDeOrigenEnConversiones(t *testing.T) {
	tests := []struct {
		file    string
		op      models.Operation
		wantDef string
		absent  string
	}{
		{"v.mp4", models.OpConvert, "mkv", "mp4"},
		{"v.mkv", models.OpConvert, "mp4", "mkv"},
		{"a.flac", models.OpConvertAudio, "mp3", "flac"},
		{"a.mp3", models.OpConvertAudio, "flac", "mp3"},
		{"a.m4a", models.OpConvertAudio, "flac", "aac"}, // m4a es aac en contenedor mp4
	}
	for _, tc := range tests {
		d, err := Route(tc.file, tc.op)
		if err != nil || d.Target != tc.wantDef {
			t.Errorf("%s %s: target=%q err=%v; quería %q", tc.file, tc.op, d.Target, err, tc.wantDef)
		}
		for _, x := range TargetsFor(tc.op, tc.file) {
			if x == tc.absent {
				t.Errorf("%s %s: TargetsFor ofrece el formato de origen %q", tc.file, tc.op, x)
			}
		}
		if _, err := RouteWith(tc.file, tc.op, tc.absent, 0); err == nil {
			t.Errorf("%s %s → %s: debía rechazarse (mismo formato)", tc.file, tc.op, tc.absent)
		}
	}
	// Miniatura y extracción de audio no filtran: el origen nunca coincide o la operación no es
	// una conversión de formato.
	if d, err := Route("i.png", models.OpThumbnail); err != nil || d.Target != "jpg" {
		t.Errorf("png thumbnail: %+v err=%v", d, err)
	}
	if d, err := RouteWith("i.png", models.OpThumbnail, "png", 0); err != nil || d.Target != "png" {
		t.Errorf("png → png miniatura debe aceptarse: %+v err=%v", d, err)
	}
	if got := TargetsFor(models.OpExtractAudio, "v.mp4"); len(got) != 4 {
		t.Errorf("extract_audio no debía filtrar: %v", got)
	}
	// El catálogo dice qué operaciones excluyen el origen para que el dashboard filtre igual.
	c := GetCatalog()
	if len(c.IdentityExcludedOps) != 2 || c.ExtAliases["m4a"] != "aac" {
		t.Errorf("catálogo sin la info de exclusión: %+v", c)
	}
}

// "Enriquecer" integra recursos asociados (portada, etiquetas, letra) dentro del mismo archivo:
// el default es el formato de origen cuando el contenedor los admite, y si no, el primero de la
// lista (wav → mp3, avi → mp4). Es liviano (remux) → pool metadata.
func TestRoute_EnriquecerPrefiereElFormatoDeOrigen(t *testing.T) {
	tests := []struct {
		file    string
		op      models.Operation
		wantDef string
	}{
		{"a.mp3", models.OpEnrichAudio, "mp3"},
		{"a.flac", models.OpEnrichAudio, "flac"},
		{"a.ogg", models.OpEnrichAudio, "ogg"},
		{"a.m4a", models.OpEnrichAudio, "m4a"},
		{"a.wav", models.OpEnrichAudio, "mp3"},
		{"a.aac", models.OpEnrichAudio, "mp3"},
		{"v.mp4", models.OpEnrichVideo, "mp4"},
		{"v.mkv", models.OpEnrichVideo, "mkv"},
		{"v.mov", models.OpEnrichVideo, "mp4"}, // QuickTime no guarda portada ni descripción
		{"v.avi", models.OpEnrichVideo, "mp4"},
		{"v.webm", models.OpEnrichVideo, "mp4"},
	}
	for _, tc := range tests {
		d, err := Route(tc.file, tc.op)
		if err != nil || d.Target != tc.wantDef || d.Pool != "metadata" {
			t.Errorf("%s %s: %+v err=%v; quería target=%s pool=metadata", tc.file, tc.op, d, err, tc.wantDef)
		}
	}
	// Sigue siendo posible pedir otro contenedor de la lista.
	if d, err := RouteWith("a.mp3", models.OpEnrichAudio, "flac", 0); err != nil || d.Target != "flac" {
		t.Errorf("mp3 → flac enriquecido: %+v err=%v", d, err)
	}
	// No aplica a imágenes ni cruzado de tipo.
	if _, err := Route("i.jpg", models.OpEnrichAudio); err == nil {
		t.Error("enrich_audio sobre imagen debía rechazarse")
	}
	if _, err := Route("a.mp3", models.OpEnrichVideo); err == nil {
		t.Error("enrich_video sobre audio debía rechazarse")
	}
	// La operación por defecto de cada tipo no cambia.
	if d, _ := Route("a.mp3", ""); d.Operation != models.OpConvertAudio {
		t.Errorf("default de audio cambió: %s", d.Operation)
	}
	c := GetCatalog()
	if len(c.IdentityPreferredOps) != 2 {
		t.Errorf("catálogo sin identity_preferred_ops: %+v", c.IdentityPreferredOps)
	}
}

// El coordinador completa los recursos que el cliente no mandó: título legible a partir del
// nombre del archivo y álbum = nombre del caso. Lo que el cliente manda, gana. Para las demás
// operaciones no se genera nada.
func TestDefaultEnrichment(t *testing.T) {
	got := DefaultEnrichment(models.OpEnrichAudio, "concierto-s1", "audio_medium_07-final_mix.flac", nil)
	if got == nil || got.Title != "audio medium 07 final mix" || got.Album != "concierto-s1" {
		t.Errorf("defaults: %+v", got)
	}
	got = DefaultEnrichment(models.OpEnrichVideo, "boda", "clip.mp4", &models.Enrichment{Title: "Entrada", Lyrics: "descripción"})
	if got.Title != "Entrada" || got.Album != "boda" || got.Lyrics != "descripción" {
		t.Errorf("el cliente gana: %+v", got)
	}
	if DefaultEnrichment(models.OpConvert, "boda", "clip.mp4", &models.Enrichment{Title: "x"}) != nil {
		t.Error("las demás operaciones no llevan recursos")
	}
	if got := DefaultEnrichment(models.OpEnrichAudio, "", "a.mp3", nil); got.Album != "" || got.Title != "a" {
		t.Errorf("sin nombre de caso no se inventa álbum: %+v", got)
	}
}
