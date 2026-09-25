package cases

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/lenokeckler/mediacase-platform/internal/models"
)

// Inspección de contenido (consigna §2: "inspeccionar cada archivo y determinar la operación
// correspondiente"): el routing por extensión (router.go) es la base, pero una extensión puede
// mentir (un .mp4 que en realidad es un .mkv, un .wav que en realidad es un .mp3…). SniffType lee
// solo el encabezado del archivo (alcanza con los primeros 64 KB) y decide el tipo real por su
// firma binaria, sin ejecutar ffmpeg ni bajar el archivo completo.

// sniffHeadSize es cuánto encabezado hace falta para reconocer cualquiera de las firmas de abajo.
const sniffHeadSize = 64 << 10

// SniffType inspecciona el encabezado de un archivo y devuelve su tipo y formato real. ok=false
// cuando el contenido no se pudo reconocer (vacío, corrupto, o un contenedor ambiguo como ASF que
// puede ser audio o video): en ese caso el llamador debe conservar el routing por extensión.
func SniffType(head []byte) (models.FileType, string, bool) {
	if len(head) == 0 {
		return "", "", false
	}
	if ft, format, ok := sniffFtyp(head); ok {
		return ft, format, ok
	}
	if ft, format, ok := sniffEBML(head); ok {
		return ft, format, ok
	}
	if ft, format, ok := sniffRIFF(head); ok {
		return ft, format, ok
	}
	if sniffMPEGTS(head) {
		return models.FileVideo, "ts", true
	}
	if sniffMPEGPS(head) {
		return models.FileVideo, "mpg", true
	}
	if sniffFLV(head) {
		return models.FileVideo, "flv", true
	}
	if hasFLAC(head) {
		return models.FileAudio, "flac", true
	}
	if ft, format, ok := sniffOgg(head); ok {
		return ft, format, ok
	}
	if sniffAIFF(head) {
		return models.FileAudio, "aiff", true
	}
	if hasDSD(head) {
		return models.FileAudio, "dsf", true
	}
	if hasID3(head) || looksLikeMP3(head) {
		return models.FileAudio, "mp3", true
	}
	if sniffADTS(head) {
		return models.FileAudio, "aac", true
	}
	if ft, format, ok := sniffImage(head); ok {
		return ft, format, ok
	}
	if isASF(head) {
		return "", "", false // contenedor ambiguo (audio o video): se conserva el routing por extensión
	}
	return "", "", false
}

// sniffFtyp reconoce el box "ftyp" de la familia MPEG-4 (mp4, mov, m4v, m4a, 3gp…): el tipo lo
// da la "major brand" a partir del byte 8.
func sniffFtyp(head []byte) (models.FileType, string, bool) {
	if len(head) < 12 || string(head[4:8]) != "ftyp" {
		return "", "", false
	}
	brand := strings.TrimRight(string(head[8:12]), " \x00")
	switch {
	case brand == "M4A" || brand == "M4B":
		return models.FileAudio, "m4a", true // audio en contenedor MPEG-4
	case brand == "qt":
		return models.FileVideo, "mov", true
	case brand == "M4V":
		return models.FileVideo, "m4v", true
	case strings.HasPrefix(brand, "3gp"):
		return models.FileVideo, "3gp", true
	default:
		// Marcas genéricas (isom, mp42, dash…): el mismo contenedor sirve para audio solo o para
		// video. Deciden las pistas (cajas hdlr del moov); si el moov no cae en el encabezado
		// (archivo sin faststart) no se sabe, y se respeta la extensión.
		return sniffMP4Tracks(head)
	}
}

// sniffMP4Tracks busca las cajas hdlr (tamaño, "hdlr", versión/flags, pre_defined, handler) y
// mira si alguna pista es de video ("vide") o solo hay sonido ("soun").
func sniffMP4Tracks(head []byte) (models.FileType, string, bool) {
	var video, sound bool
	for rest := head; ; {
		i := bytes.Index(rest, []byte("hdlr"))
		if i < 0 || i+16 > len(rest) {
			break
		}
		switch string(rest[i+12 : i+16]) {
		case "vide":
			video = true
		case "soun":
			sound = true
		}
		rest = rest[i+4:]
	}
	switch {
	case video:
		return models.FileVideo, "mp4", true
	case sound:
		return models.FileAudio, "m4a", true
	default:
		return "", "", false
	}
}

// sniffEBML reconoce Matroska/WebM por su firma EBML; el doctype (buscado en el encabezado) dice
// cuál de los dos es.
func sniffEBML(head []byte) (models.FileType, string, bool) {
	if len(head) < 4 || head[0] != 0x1A || head[1] != 0x45 || head[2] != 0xDF || head[3] != 0xA3 {
		return "", "", false
	}
	if bytes.Contains(head, []byte("webm")) {
		return models.FileVideo, "webm", true
	}
	return models.FileVideo, "mkv", true
}

// sniffRIFF reconoce los contenedores RIFF: WAVE (audio), AVI (video) y WEBP (imagen).
func sniffRIFF(head []byte) (models.FileType, string, bool) {
	if len(head) < 12 || string(head[0:4]) != "RIFF" {
		return "", "", false
	}
	switch string(head[8:12]) {
	case "WAVE":
		return models.FileAudio, "wav", true
	case "AVI ":
		return models.FileVideo, "avi", true
	case "WEBP":
		return models.FileImage, "webp", true
	}
	return "", "", false
}

// sniffMPEGTS reconoce MPEG Transport Stream por sus paquetes de 188 bytes alineados con 0x47.
func sniffMPEGTS(head []byte) bool {
	return len(head) >= 377 && head[0] == 0x47 && head[188] == 0x47 && head[376] == 0x47
}

// sniffMPEGPS reconoce MPEG Program Stream por su "pack header" inicial.
func sniffMPEGPS(head []byte) bool {
	return len(head) >= 4 && head[0] == 0x00 && head[1] == 0x00 && head[2] == 0x01 && head[3] == 0xBA
}

// sniffFLV reconoce Flash Video por su firma "FLV".
func sniffFLV(head []byte) bool {
	return len(head) >= 3 && head[0] == 'F' && head[1] == 'L' && head[2] == 'V'
}

// hasID3 reconoce una etiqueta ID3v2 al inicio del archivo (mp3 con metadatos incrustados).
func hasID3(head []byte) bool {
	return len(head) >= 3 && head[0] == 'I' && head[1] == 'D' && head[2] == '3'
}

// hasFLAC reconoce FLAC por su firma "fLaC".
func hasFLAC(head []byte) bool { return len(head) >= 4 && string(head[0:4]) == "fLaC" }

// sniffOgg reconoce contenedores Ogg y, cuando el encabezado alcanza a incluir el marcador del
// códec, distingue Opus, Vorbis y Theora; sin marcador reconocido queda ambiguo.
func sniffOgg(head []byte) (models.FileType, string, bool) {
	if len(head) < 4 || string(head[0:4]) != "OggS" {
		return "", "", false
	}
	switch {
	case bytes.Contains(head, []byte("OpusHead")):
		return models.FileAudio, "opus", true
	case bytes.Contains(head, []byte("\x01vorbis")):
		return models.FileAudio, "ogg", true
	case bytes.Contains(head, []byte("\x80theora")):
		return models.FileVideo, "ogv", true
	}
	return "", "", false
}

// sniffAIFF reconoce AIFF/AIFF-C por el contenedor "FORM" con subtipo AIFF o AIFC.
func sniffAIFF(head []byte) bool {
	if len(head) < 12 || string(head[0:4]) != "FORM" {
		return false
	}
	sub := string(head[8:12])
	return sub == "AIFF" || sub == "AIFC"
}

// hasDSD reconoce DSF (Direct Stream Digital) por su firma "DSD ".
func hasDSD(head []byte) bool { return len(head) >= 4 && string(head[0:4]) == "DSD " }

// sniffADTS reconoce AAC crudo en contenedor ADTS por su sync word (FFF1 sin CRC, FFF9 con CRC).
func sniffADTS(head []byte) bool {
	return len(head) >= 2 && head[0] == 0xFF && (head[1] == 0xF1 || head[1] == 0xF9)
}

// sniffImage reconoce JPEG, PNG, GIF, BMP y TIFF por su firma.
func sniffImage(head []byte) (models.FileType, string, bool) {
	switch {
	case len(head) >= 3 && head[0] == 0xFF && head[1] == 0xD8 && head[2] == 0xFF:
		return models.FileImage, "jpg", true
	case len(head) >= 8 && bytes.Equal(head[0:8], []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}):
		return models.FileImage, "png", true
	case len(head) >= 6 && (string(head[0:6]) == "GIF87a" || string(head[0:6]) == "GIF89a"):
		return models.FileImage, "gif", true
	case len(head) >= 2 && head[0] == 'B' && head[1] == 'M':
		return models.FileImage, "bmp", true
	case len(head) >= 4 && (string(head[0:4]) == "II*\x00" || string(head[0:4]) == "MM\x00*"):
		return models.FileImage, "tif", true
	}
	return "", "", false
}

// asfGUID es el GUID del objeto de cabecera ASF (Windows Media): el contenedor puede llevar audio
// (WMA) o video (WMV) y no se puede distinguir sin más contexto, así que se trata como ambiguo.
var asfGUID = []byte{0x30, 0x26, 0xB2, 0x75, 0x8E, 0x66, 0xCF, 0x11, 0xA6, 0xD9, 0x00, 0xAA, 0x00, 0x62, 0xCE, 0x6C}

func isASF(head []byte) bool { return bytes.HasPrefix(head, asfGUID) }

// mp3BitrateKbps[versión][layer][índice] son las tablas de bitrate de la cabecera MPEG audio.
// versión: 1 = MPEG1, 2 = MPEG2/2.5 (comparten tabla salvo layer I). layer: 1, 2 o 3.
var mp3BitrateKbps = map[int]map[int][16]int{
	1: {
		1: {0, 32, 64, 96, 128, 160, 192, 224, 256, 288, 320, 352, 384, 416, 448, -1},
		2: {0, 32, 48, 56, 64, 80, 96, 112, 128, 160, 192, 224, 256, 320, 384, -1},
		3: {0, 32, 40, 48, 56, 64, 80, 96, 112, 128, 160, 192, 224, 256, 320, -1},
	},
	2: {
		1: {0, 32, 48, 56, 64, 80, 96, 112, 128, 144, 160, 176, 192, 224, 256, -1},
		2: {0, 8, 16, 24, 32, 40, 48, 56, 64, 80, 96, 112, 128, 144, 160, -1},
		3: {0, 8, 16, 24, 32, 40, 48, 56, 64, 80, 96, 112, 128, 144, 160, -1},
	},
}

// mp3SampleRate[grupo][índice]: 0 = MPEG1, 1 = MPEG2, 2 = MPEG2.5.
var mp3SampleRate = [3][3]int{
	{44100, 48000, 32000},
	{22050, 24000, 16000},
	{11025, 12000, 8000},
}

// parseMP3Frame valida una cabecera de cuadro MPEG audio (mp3) al inicio de b y devuelve el
// largo del cuadro. No confía en la extensión: solo en los 4 bytes de la cabecera.
func parseMP3Frame(b []byte) (int, bool) {
	if len(b) < 4 || b[0] != 0xFF || b[1]&0xE0 != 0xE0 {
		return 0, false
	}
	versionID := (b[1] >> 3) & 0x03 // 00=MPEG2.5, 01=reservado, 10=MPEG2, 11=MPEG1
	layerID := (b[1] >> 1) & 0x03   // 00=reservado, 01=Layer III, 10=Layer II, 11=Layer I
	if versionID == 1 || layerID == 0 {
		return 0, false
	}
	bitrateIdx := int(b[2] >> 4)
	sampleIdx := int((b[2] >> 2) & 0x03)
	padding := int((b[2] >> 1) & 0x01)
	if bitrateIdx == 0 || bitrateIdx == 15 || sampleIdx == 3 {
		return 0, false
	}
	isV1 := versionID == 3
	verGroup, layer := 2, 0
	if isV1 {
		verGroup = 1
	}
	switch layerID {
	case 3:
		layer = 1
	case 2:
		layer = 2
	case 1:
		layer = 3
	}
	kbps := mp3BitrateKbps[verGroup][layer][bitrateIdx]
	if kbps <= 0 {
		return 0, false
	}
	sampleGroup := 0
	switch versionID {
	case 2:
		sampleGroup = 1
	case 0:
		sampleGroup = 2
	}
	rate := mp3SampleRate[sampleGroup][sampleIdx]
	if rate == 0 {
		return 0, false
	}
	var frameLen int
	switch {
	case layer == 1:
		frameLen = (12*kbps*1000/rate + padding) * 4
	case layer == 2 || isV1:
		frameLen = 144*kbps*1000/rate + padding
	default: // Layer III, MPEG2/2.5 (LSF): fórmula reducida
		frameLen = 72*kbps*1000/rate + padding
	}
	if frameLen <= 4 {
		return 0, false
	}
	return frameLen, true
}

// looksLikeMP3 busca un cuadro MPEG audio válido cerca del inicio del encabezado y, cuando hay
// suficiente encabezado, valida que el SIGUIENTE cuadro también empiece donde debe: un solo cuadro
// "válido" por azar es común en datos binarios cualesquiera; dos consecutivos, no.
func looksLikeMP3(head []byte) bool {
	limit := len(head)
	if limit > 4096 {
		limit = 4096
	}
	for i := 0; i+4 <= limit; i++ {
		if head[i] != 0xFF {
			continue
		}
		frameLen, ok := parseMP3Frame(head[i:])
		if !ok {
			continue
		}
		if i+frameLen+4 <= len(head) {
			if _, ok2 := parseMP3Frame(head[i+frameLen:]); !ok2 {
				continue // el siguiente cuadro no valida: probablemente no es mp3
			}
		}
		return true
	}
	return false
}

// ── Notas de routing por inspección de contenido ────────────────────────────────────────────

// misleadingExtPrefix marca las notas que indican que la extensión no correspondía al contenido
// real; el reporte (report.go) las cuenta aparte en el resumen agregado.
const misleadingExtPrefix = "extensión engañosa:"

// MisleadingTypeNote es la nota cuando SniffType detecta un tipo de contenido DISTINTO del que
// sugiere la extensión: el coordinador re-enruta por el tipo real.
func MisleadingTypeNote(ext string, extType, realType models.FileType, format string) string {
	return fmt.Sprintf("%s .%s sugiere %s, pero el contenido real es %s (%s); se enrutó por el contenido real",
		misleadingExtPrefix, ext, extType, realType, format)
}

// MisleadingFormatNote es la nota cuando el tipo coincide pero el formato real es otro (p. ej. un
// .mp4 que en realidad es un .mkv): se conserva el tipo detectado por extensión.
func MisleadingFormatNote(ext, realFormat string) string {
	return fmt.Sprintf("%s .%s no es realmente %s: el contenido es %s; se mantiene el tipo detectado",
		misleadingExtPrefix, ext, ext, realFormat)
}

// EmptyFileNote es la nota cuando el archivo no se pudo inspeccionar porque está vacío: se
// conserva el routing por extensión (el worker fallará al procesarlo, lo cual es lo esperado).
const EmptyFileNote = "archivo vacío: no se pudo inspeccionar el contenido; se mantiene el enrutamiento por extensión"

// IsMisleadingExtNote dice si una nota de routing viene de una extensión que no correspondía al
// contenido real (lo usa el reporte para el resumen agregado).
func IsMisleadingExtNote(note string) bool { return strings.HasPrefix(note, misleadingExtPrefix) }
