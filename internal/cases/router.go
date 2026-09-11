// Package cases contiene la lógica de casos: routing por tipo de contenido, barrier/join
// para cerrar un caso y construcción del reporte consolidado.
package cases

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/lenokeckler/mediacase-platform/internal/models"
)

// Routing por tipo: el coordinador inspecciona cada archivo del caso y decide la operación.
// El cliente puede sugerir una operación, pero solo se acepta si aplica a ese tipo.

var extToType = map[string]models.FileType{
	".mp4": models.FileVideo, ".mkv": models.FileVideo, ".avi": models.FileVideo,
	".mov": models.FileVideo, ".webm": models.FileVideo,
	".mp3": models.FileAudio, ".wav": models.FileAudio, ".flac": models.FileAudio,
	".aac": models.FileAudio, ".ogg": models.FileAudio, ".m4a": models.FileAudio,
	".jpg": models.FileImage, ".jpeg": models.FileImage, ".png": models.FileImage,
	".gif": models.FileImage, ".webp": models.FileImage, ".bmp": models.FileImage,
}

// Operaciones válidas por tipo de contenido. La primera de cada lista es la que el
// coordinador elige cuando el cliente no pide ninguna.
var opsByType = map[models.FileType][]models.Operation{
	models.FileVideo: {models.OpConvert, models.OpExtractAudio, models.OpThumbnail},
	models.FileAudio: {models.OpConvertAudio, models.OpThumbnail},
	models.FileImage: {models.OpThumbnail},
}

// Pool de workers por tipo de contenido (ver docs/architecture.md, "Modelo de asignación").
var poolByType = map[models.FileType]string{
	models.FileVideo: "video",
	models.FileAudio: "audio",
	models.FileImage: "metadata",
}

// RouteDecision es lo que el coordinador decide para un archivo del caso.
type RouteDecision struct {
	FileType  models.FileType
	Operation models.Operation
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

// PoolFor decide qué pool de workers ejecuta un tipo de contenido.
func PoolFor(ft models.FileType) string { return poolByType[ft] }

// Route valida la operación pedida (o elige la default) y devuelve la decisión completa.
func Route(filename string, requested models.Operation) (RouteDecision, error) {
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
	return RouteDecision{FileType: ft, Operation: op, Pool: PoolFor(ft)}, nil
}

func operationApplies(ft models.FileType, op models.Operation) bool {
	for _, o := range opsByType[ft] {
		if o == op {
			return true
		}
	}
	return false
}
