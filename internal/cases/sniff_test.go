package cases

import (
	"testing"

	"github.com/lenokeckler/mediacase-platform/internal/models"
)

func TestSniffType_FirmasBinarias(t *testing.T) {
	tests := []struct {
		name       string
		head       []byte
		wantType   models.FileType
		wantFormat string
	}{
		{"mp4 (ftyp isom + pista vide)", mp4Head("isom", "soun", "vide"), models.FileVideo, "mp4"},
		{"m4a (ftyp isom, solo pista soun)", mp4Head("isom", "soun"), models.FileAudio, "m4a"},
		{"mov (ftyp qt)", append([]byte{0, 0, 0, 0x14}, append([]byte("ftyp"), []byte("qt  ")...)...), models.FileVideo, "mov"},
		{"m4a (ftyp M4A)", append([]byte{0, 0, 0, 0x18}, append([]byte("ftyp"), []byte("M4A ")...)...), models.FileAudio, "m4a"},
		{"3gp (ftyp 3gp5)", append([]byte{0, 0, 0, 0x18}, append([]byte("ftyp"), []byte("3gp5")...)...), models.FileVideo, "3gp"},
		{"mkv (EBML + matroska)", append([]byte{0x1A, 0x45, 0xDF, 0xA3}, []byte("...matroska...")...), models.FileVideo, "mkv"},
		{"webm (EBML + webm)", append([]byte{0x1A, 0x45, 0xDF, 0xA3}, []byte("...webm...")...), models.FileVideo, "webm"},
		{"wav (RIFF/WAVE)", []byte("RIFF\x00\x00\x00\x00WAVEfmt "), models.FileAudio, "wav"},
		{"avi (RIFF/AVI )", []byte("RIFF\x00\x00\x00\x00AVI LIST"), models.FileVideo, "avi"},
		{"webp (RIFF/WEBP)", []byte("RIFF\x00\x00\x00\x00WEBPVP8 "), models.FileImage, "webp"},
		{"flv", []byte("FLV\x01\x05\x00\x00\x00\x09"), models.FileVideo, "flv"},
		{"flac", []byte("fLaC\x00\x00\x00\x22"), models.FileAudio, "flac"},
		{"ogg vorbis", append([]byte("OggS\x00\x02"), []byte("....\x01vorbis....")...), models.FileAudio, "ogg"},
		{"ogg opus", append([]byte("OggS\x00\x02"), []byte("....OpusHead....")...), models.FileAudio, "opus"},
		{"ogg theora (video)", append([]byte("OggS\x00\x02"), []byte("....\x80theora....")...), models.FileVideo, "ogv"},
		{"aiff", []byte("FORM\x00\x00\x00\x00AIFFCOMM"), models.FileAudio, "aiff"},
		{"dsf", []byte("DSD \x00\x00\x00\x00\x00\x00\x00\x00"), models.FileAudio, "dsf"},
		{"mp3 con ID3v2", []byte("ID3\x03\x00\x00\x00\x00\x00\x00"), models.FileAudio, "mp3"},
		{"aac ADTS (sin CRC)", []byte{0xFF, 0xF1, 0x50, 0x80, 0x00, 0x1F, 0xFC}, models.FileAudio, "aac"},
		{"aac ADTS (con CRC)", []byte{0xFF, 0xF9, 0x50, 0x80, 0x00, 0x1F, 0xFC}, models.FileAudio, "aac"},
		{"jpg", []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10}, models.FileImage, "jpg"},
		{"png", []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}, models.FileImage, "png"},
		{"gif89a", []byte("GIF89a"), models.FileImage, "gif"},
		{"bmp", []byte("BM\x46\x00\x00\x00"), models.FileImage, "bmp"},
		{"tiff little-endian", []byte("II*\x00\x08\x00\x00\x00"), models.FileImage, "tif"},
		{"tiff big-endian", []byte("MM\x00*\x00\x08\x00\x00"), models.FileImage, "tif"},
		{"mpeg-ps", []byte{0x00, 0x00, 0x01, 0xBA, 0x44}, models.FileVideo, "mpg"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ft, format, ok := SniffType(tc.head)
			if !ok || ft != tc.wantType || format != tc.wantFormat {
				t.Errorf("SniffType(%q) = %v %v %v; quería %v %v true", tc.name, ft, format, ok, tc.wantType, tc.wantFormat)
			}
		})
	}
}

func TestSniffType_MPEGTS(t *testing.T) {
	head := make([]byte, 400)
	head[0], head[188], head[376] = 0x47, 0x47, 0x47
	ft, format, ok := SniffType(head)
	if !ok || ft != models.FileVideo || format != "ts" {
		t.Errorf("MPEG-TS: %v %v %v", ft, format, ok)
	}
}

func TestSniffType_Mp3PorFirmaDeCuadro(t *testing.T) {
	// Cuadro MPEG1 Layer III, 128 kbps, 44100 Hz, sin CRC: 0xFF 0xFB 0x90 0x00. Largo del
	// cuadro = 144*128000/44100 = 417 bytes (división entera); se repite en el offset 417 para
	// que looksLikeMP3 valide el SEGUNDO cuadro (no basta con encontrar un 0xFF suelto).
	frame := []byte{0xFF, 0xFB, 0x90, 0x00}
	buf := make([]byte, 900)
	copy(buf[0:], frame)
	copy(buf[417:], frame)
	ft, format, ok := SniffType(buf)
	if !ok || ft != models.FileAudio || format != "mp3" {
		t.Fatalf("mp3 por firma de cuadro: %v %v %v", ft, format, ok)
	}

	// Un solo cuadro válido cuyo "segundo cuadro" (en el offset 417) NO valida (queda en ceros)
	// no debe reconocerse como mp3: hace falta que el siguiente cuadro también sincronice.
	oneFrameOnly := make([]byte, 900)
	copy(oneFrameOnly[0:], frame)
	if _, _, ok := SniffType(oneFrameOnly); ok {
		t.Error("un cuadro válido sin un segundo cuadro que valide no debía reconocerse como mp3")
	}
}

func TestSniffType_AmbiguoOVacio(t *testing.T) {
	if _, _, ok := SniffType(nil); ok {
		t.Error("encabezado vacío debe ser ok=false")
	}
	if _, _, ok := SniffType([]byte{}); ok {
		t.Error("encabezado vacío (slice) debe ser ok=false")
	}
	if _, _, ok := SniffType([]byte("esto no es ningún formato reconocido")); ok {
		t.Error("contenido desconocido debe ser ok=false")
	}
	// GUID de cabecera ASF: puede ser audio (WMA) o video (WMV); se deja ambiguo a propósito.
	asf := append(append([]byte{}, asfGUID...), []byte("resto del archivo")...)
	if _, _, ok := SniffType(asf); ok {
		t.Error("ASF es ambiguo (audio o video): debía quedar ok=false")
	}
}

func TestSameFormat_Alias(t *testing.T) {
	if !SameFormat("JPEG", "jpg") || !SameFormat("m4a", "AAC") {
		t.Error("SameFormat debía considerar los alias")
	}
	if SameFormat("mp4", "mkv") {
		t.Error("mp4 y mkv no son el mismo formato")
	}
}

func TestRouteAs_TipoRealDistintoDeLaExtension(t *testing.T) {
	// clip.mp4 cuyo contenido real es audio (mp3): el cliente no pidió operación, así que se usa
	// el default del tipo REAL (convert_audio), no el del video que sugería la extensión.
	d, err := RouteAs(models.FileAudio, "clip.mp4", "", "", 0)
	if err != nil || d.Operation != models.OpConvertAudio || d.FileType != models.FileAudio {
		t.Fatalf("%+v err=%v", d, err)
	}
	// La operación pedida por el cliente (extract_audio, válida para video) no aplica al tipo
	// real (audio): RouteAs debe rechazarla; el llamador decide si cae al default.
	if _, err := RouteAs(models.FileAudio, "clip.mp4", models.OpExtractAudio, "", 0); err == nil {
		t.Error("extract_audio no aplica a un archivo cuyo tipo real es audio")
	}
}

// mp4Head arma un encabezado ftyp con la marca dada y una caja hdlr por pista.
func mp4Head(brand string, handlers ...string) []byte {
	head := append([]byte{0, 0, 0, 0x18}, append([]byte("ftyp"), []byte(brand)...)...)
	head = append(head, []byte("\x00\x00\x02\x00moov")...)
	for _, h := range handlers {
		head = append(head, []byte("\x00\x00\x00\x21hdlr\x00\x00\x00\x00\x00\x00\x00\x00"+h)...)
	}
	return head
}

func TestSniffType_MP4SinMoovEnElEncabezado(t *testing.T) {
	// Sin faststart el moov queda al final: no se sabe si es audio o video, manda la extensión.
	if _, _, ok := SniffType(mp4Head("isom")); ok {
		t.Error("ftyp genérico sin pistas visibles debía quedar indeterminado")
	}
}
