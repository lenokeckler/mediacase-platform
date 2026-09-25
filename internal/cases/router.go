// Package cases contiene la lógica de casos: routing por tipo de contenido, barrier/join
// para cerrar un caso y construcción del reporte consolidado.
package cases

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/lenokeckler/mediacase-platform/internal/models"
)

// Routing por tipo: el coordinador inspecciona cada archivo del caso y decide la operación y el
// formato de salida. El cliente puede sugerir operación y destino, pero solo se aceptan si
// aplican a ese tipo de archivo.

var extToType = map[string]models.FileType{
	// video
	".mp4": models.FileVideo, ".mkv": models.FileVideo, ".avi": models.FileVideo, ".mov": models.FileVideo,
	".webm": models.FileVideo, ".m4v": models.FileVideo, ".flv": models.FileVideo, ".wmv": models.FileVideo,
	".ts": models.FileVideo, ".mts": models.FileVideo, ".3gp": models.FileVideo, ".mpg": models.FileVideo,
	".mpeg": models.FileVideo,
	// audio
	".mp3": models.FileAudio, ".wav": models.FileAudio, ".flac": models.FileAudio, ".aac": models.FileAudio,
	".ogg": models.FileAudio, ".m4a": models.FileAudio, ".opus": models.FileAudio, ".wma": models.FileAudio,
	".aiff": models.FileAudio, ".aif": models.FileAudio, ".dsf": models.FileAudio, ".dff": models.FileAudio,
	// imagen
	".jpg": models.FileImage, ".jpeg": models.FileImage, ".png": models.FileImage, ".gif": models.FileImage,
	".webp": models.FileImage, ".bmp": models.FileImage, ".tif": models.FileImage, ".tiff": models.FileImage,
}

// Operaciones válidas por tipo de contenido. La primera de cada lista es la que el
// coordinador elige cuando el cliente no pide ninguna.
var opsByType = map[models.FileType][]models.Operation{
	models.FileVideo: {models.OpConvert, models.OpExtractAudio, models.OpThumbnail, models.OpMetadata, models.OpEnrichVideo},
	models.FileAudio: {models.OpConvertAudio, models.OpThumbnail, models.OpMetadata, models.OpEnrichAudio},
	models.FileImage: {models.OpThumbnail, models.OpMetadata},
}

// Formatos de salida válidos por operación. El primero es el que se elige por defecto.
var targetsByOp = map[models.Operation][]string{
	models.OpConvert:      {"mp4", "mkv", "webm"},
	models.OpExtractAudio: {"mp3", "wav", "flac", "aac"},
	models.OpConvertAudio: {"flac", "mp3", "wav", "aac", "ogg"},
	models.OpThumbnail:    {"jpg", "png", "webp"},
	models.OpMetadata:     {"json"},
	// Contenedores que admiten portada embebida + etiquetas + letra/descripción.
	models.OpEnrichAudio: {"mp3", "flac", "ogg", "m4a"},
	models.OpEnrichVideo: {"mp4", "mkv"},
}

// Anchos válidos de miniatura; el primero es el default.
var ThumbnailWidths = []int{320, 640, 1280}

// Operaciones que cambian el formato del archivo: ofrecer el formato de origen como salida no
// tiene sentido (mp4 → mp4), así que se omite de la lista y del default. Miniatura no está
// aquí a propósito: png → png a 320 px es un cambio de tamaño, no de formato, y conserva la
// transparencia.
var identityExcludedOps = []models.Operation{models.OpConvert, models.OpConvertAudio}

// Operaciones que conservan el formato: "enriquecer" integra recursos dentro del mismo archivo,
// así que el default es el formato de origen si el contenedor lo admite (mp3 → mp3, mkv → mkv) y,
// si no, el primero de la lista (wav → mp3, avi → mp4).
var identityPreferredOps = []models.Operation{models.OpEnrichAudio, models.OpEnrichVideo}

// Extensiones distintas del mismo formato, para que .jpeg → JPG o .m4a → AAC tampoco se ofrezcan.
var extAliases = map[string]string{"jpeg": "jpg", "tiff": "tif", "aiff": "aif", "m4a": "aac", "mpeg": "mpg"}

// Pool de workers por operación (ver docs/architecture.md, "Modelo de asignación"): transcodificar
// video es lo pesado (pool video), audio va aparte, y miniaturas y metadatos son livianos (pool
// metadata). Con el planificador por afinidad cualquier nodo puede ayudar en otro pool.
var poolByOp = map[models.Operation]string{
	models.OpConvert:      "video",
	models.OpExtractAudio: "video",
	models.OpConvertAudio: "audio",
	models.OpThumbnail:    "metadata",
	models.OpMetadata:     "metadata",
	models.OpEnrichAudio:  "metadata", // remux con -c copy + portada: liviano
	models.OpEnrichVideo:  "metadata",
}

// RouteDecision es lo que el coordinador decide para un archivo del caso.
type RouteDecision struct {
	FileType  models.FileType
	Operation models.Operation
	Target    string // formato de salida: mp4, mp3, jpg, json…
	Width     int    // solo miniaturas
	Pool      string
}

// DetectFileType clasifica por extensión. Error si no está soportada.
func DetectFileType(filename string) (models.FileType, error) {
	ext := strings.ToLower(filepath.Ext(filename))
	ft, ok := extToType[ext]
	if !ok {
		if ext == "" {
			return "", fmt.Errorf("archivo sin extensión: %q", filename)
		}
		return "", fmt.Errorf("formato no soportado: %q", ext)
	}
	return ft, nil
}

// DefaultOperation es la operación que el coordinador elige para un tipo si el cliente no pide una.
func DefaultOperation(ft models.FileType) models.Operation { return opsByType[ft][0] }

// TargetsFor son los formatos de salida válidos de una operación sobre un archivo concreto:
// los de targetsByOp menos el formato de origen cuando la operación es una conversión.
func TargetsFor(op models.Operation, filename string) []string {
	all := targetsByOp[op]
	if !excludesIdentity(op) {
		return all
	}
	src := normExt(strings.TrimPrefix(filepath.Ext(filename), "."))
	out := make([]string, 0, len(all))
	for _, t := range all {
		if normExt(t) != src {
			out = append(out, t)
		}
	}
	return out
}

// DefaultTargetFor es el formato de salida que el coordinador elige para op sobre filename.
func DefaultTargetFor(op models.Operation, filename string) string {
	valid := TargetsFor(op, filename)
	if prefersIdentity(op) {
		src := strings.ToLower(strings.TrimPrefix(filepath.Ext(filename), "."))
		if contains(valid, src) {
			return src
		}
	}
	return valid[0]
}

func prefersIdentity(op models.Operation) bool {
	for _, o := range identityPreferredOps {
		if o == op {
			return true
		}
	}
	return false
}

func normExt(ext string) string {
	ext = strings.ToLower(ext)
	if a, ok := extAliases[ext]; ok {
		return a
	}
	return ext
}

// SameFormat dice si dos formatos (extensión o los que devuelve SniffType) son el mismo,
// considerando los alias (jpeg/jpg, m4a/aac…).
func SameFormat(a, b string) bool { return normExt(a) == normExt(b) }

func excludesIdentity(op models.Operation) bool {
	for _, o := range identityExcludedOps {
		if o == op {
			return true
		}
	}
	return false
}

// PoolFor decide qué pool de workers ejecuta una operación. Se mantiene la firma por tipo
// para el default (la operación por defecto de ese tipo).
func PoolFor(ft models.FileType) string { return poolByOp[DefaultOperation(ft)] }

// PoolForOp decide el pool de una operación concreta.
func PoolForOp(op models.Operation) string { return poolByOp[op] }

// Route valida operación y destino pedidos (o elige los defaults) y devuelve la decisión completa.
func Route(filename string, requested models.Operation) (RouteDecision, error) {
	return RouteWith(filename, requested, "", 0)
}

// RouteWith es Route con formato de salida y ancho de miniatura opcionales.
func RouteWith(filename string, requested models.Operation, target string, width int) (RouteDecision, error) {
	ft, err := DetectFileType(filename)
	if err != nil {
		return RouteDecision{}, err
	}
	return RouteAs(ft, filename, requested, target, width)
}

// RouteAs es RouteWith con el tipo de contenido ya decidido: lo usa el coordinador cuando la
// inspección de contenido (sniff.go) determina que la extensión no corresponde al tipo real.
func RouteAs(ft models.FileType, filename string, requested models.Operation, target string, width int) (RouteDecision, error) {
	op := requested
	if op == "" {
		op = DefaultOperation(ft)
	} else if !operationApplies(ft, op) {
		return RouteDecision{}, fmt.Errorf("la operación %q no aplica a %s (%s)", op, ft, filename)
	}
	target = strings.ToLower(strings.TrimPrefix(target, "."))
	valid := TargetsFor(op, filename)
	if target == "" {
		target = DefaultTargetFor(op, filename)
	} else if !contains(valid, target) {
		if excludesIdentity(op) && targetApplies(op, target) {
			return RouteDecision{}, fmt.Errorf("%s ya está en %s: convertirlo a %s no cambia el formato; válidos: %s",
				filename, target, target, strings.Join(valid, ", "))
		}
		return RouteDecision{}, fmt.Errorf("el formato %q no aplica a %s (%s); válidos: %s",
			target, op, filename, strings.Join(valid, ", "))
	}
	if op == models.OpThumbnail {
		if width == 0 {
			width = ThumbnailWidths[0]
		} else if !widthApplies(width) {
			return RouteDecision{}, fmt.Errorf("ancho de miniatura %d no válido (%s); válidos: 320, 640, 1280", width, filename)
		}
	} else {
		width = 0
	}
	return RouteDecision{FileType: ft, Operation: op, Target: target, Width: width, Pool: poolByOp[op]}, nil
}

// Catalog describe operaciones y formatos válidos por tipo, para que el dashboard y el cliente
// ofrezcan exactamente lo que el coordinador acepta.
type Catalog struct {
	OpsByType   map[models.FileType][]models.Operation `json:"ops_by_type"`
	TargetsByOp map[models.Operation][]string          `json:"targets_by_op"`
	PoolByOp    map[models.Operation]string            `json:"pool_by_op"`
	ThumbWidths []int                                  `json:"thumbnail_widths"`
	Extensions  []string                               `json:"extensions"`
	// Operaciones en las que el formato de origen no se ofrece como salida, y las extensiones
	// que cuentan como el mismo formato; el dashboard filtra con esto igual que RouteWith.
	IdentityExcludedOps []models.Operation `json:"identity_excluded_ops"`
	// Operaciones cuyo default es el formato de origen cuando está en la lista (enriquecer).
	IdentityPreferredOps []models.Operation `json:"identity_preferred_ops"`
	ExtAliases           map[string]string  `json:"ext_aliases"`
}

func GetCatalog() Catalog {
	exts := make([]string, 0, len(extToType))
	for e := range extToType {
		exts = append(exts, e)
	}
	return Catalog{OpsByType: opsByType, TargetsByOp: targetsByOp, PoolByOp: poolByOp, ThumbWidths: ThumbnailWidths, Extensions: exts,
		IdentityExcludedOps: identityExcludedOps, IdentityPreferredOps: identityPreferredOps, ExtAliases: extAliases}
}

func operationApplies(ft models.FileType, op models.Operation) bool {
	for _, o := range opsByType[ft] {
		if o == op {
			return true
		}
	}
	return false
}

func targetApplies(op models.Operation, target string) bool { return contains(targetsByOp[op], target) }

func contains(list []string, x string) bool {
	for _, t := range list {
		if t == x {
			return true
		}
	}
	return false
}

func widthApplies(w int) bool {
	for _, x := range ThumbnailWidths {
		if x == w {
			return true
		}
	}
	return false
}

// IsEnrich dice si la operación integra recursos asociados (enrich_audio / enrich_video).
func IsEnrich(op models.Operation) bool { return prefersIdentity(op) }

// DefaultEnrichment completa los recursos asociados de una sub-tarea de enriquecimiento: lo que
// el cliente mandó gana; si falta, el título sale del nombre del archivo y el álbum del nombre
// del caso. Para las demás operaciones devuelve nil.
func DefaultEnrichment(op models.Operation, caseName, filename string, given *models.Enrichment) *models.Enrichment {
	if !IsEnrich(op) {
		return nil
	}
	e := models.Enrichment{}
	if given != nil {
		e = *given
	}
	if strings.TrimSpace(e.Title) == "" {
		e.Title = titleFromFilename(filename)
	}
	if strings.TrimSpace(e.Album) == "" {
		e.Album = strings.TrimSpace(caseName)
	}
	return &e
}

// titleFromFilename: "audio_medium_07-final_mix.flac" → "audio medium 07 final mix".
func titleFromFilename(filename string) string {
	base := strings.TrimSuffix(filepath.Base(filename), filepath.Ext(filename))
	base = strings.NewReplacer("_", " ", "-", " ").Replace(base)
	return strings.Join(strings.Fields(base), " ")
}
