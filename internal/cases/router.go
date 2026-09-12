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
	models.FileVideo: {models.OpConvert, models.OpExtractAudio, models.OpThumbnail, models.OpMetadata},
	models.FileAudio: {models.OpConvertAudio, models.OpThumbnail, models.OpMetadata},
	models.FileImage: {models.OpThumbnail, models.OpMetadata},
}

// Formatos de salida válidos por operación. El primero es el que se elige por defecto.
var targetsByOp = map[models.Operation][]string{
	models.OpConvert:      {"mp4", "mkv", "webm"},
	models.OpExtractAudio: {"mp3", "wav", "flac", "aac"},
	models.OpConvertAudio: {"flac", "mp3", "wav", "aac", "ogg"},
	models.OpThumbnail:    {"jpg", "png", "webp"},
	models.OpMetadata:     {"json"},
}

// Anchos válidos de miniatura; el primero es el default.
var ThumbnailWidths = []int{320, 640, 1280}

// Pool de workers por operación (ver docs/architecture.md, "Modelo de asignación"): transcodificar
// video es lo pesado (pool video), audio va aparte, y miniaturas y metadatos son livianos (pool
// metadata). Con el planificador por afinidad cualquier nodo puede ayudar en otro pool.
var poolByOp = map[models.Operation]string{
	models.OpConvert:      "video",
	models.OpExtractAudio: "video",
	models.OpConvertAudio: "audio",
	models.OpThumbnail:    "metadata",
	models.OpMetadata:     "metadata",
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

// DefaultTarget es el formato de salida por defecto de una operación.
func DefaultTarget(op models.Operation) string { return targetsByOp[op][0] }

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
	op := requested
	if op == "" {
		op = DefaultOperation(ft)
	} else if !operationApplies(ft, op) {
		return RouteDecision{}, fmt.Errorf("la operación %q no aplica a %s (%s)", op, ft, filename)
	}
	target = strings.ToLower(strings.TrimPrefix(target, "."))
	if target == "" {
		target = DefaultTarget(op)
	} else if !targetApplies(op, target) {
		return RouteDecision{}, fmt.Errorf("el formato %q no aplica a %s (%s); válidos: %s",
			target, op, filename, strings.Join(targetsByOp[op], ", "))
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
}

func GetCatalog() Catalog {
	exts := make([]string, 0, len(extToType))
	for e := range extToType {
		exts = append(exts, e)
	}
	return Catalog{OpsByType: opsByType, TargetsByOp: targetsByOp, PoolByOp: poolByOp, ThumbWidths: ThumbnailWidths, Extensions: exts}
}

func operationApplies(ft models.FileType, op models.Operation) bool {
	for _, o := range opsByType[ft] {
		if o == op {
			return true
		}
	}
	return false
}

func targetApplies(op models.Operation, target string) bool {
	for _, t := range targetsByOp[op] {
		if t == target {
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
