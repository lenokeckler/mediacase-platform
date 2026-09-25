package cases

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/lenokeckler/mediacase-platform/internal/models"
)

var extToType = map[string]models.FileType{

	".mp4": models.FileVideo, ".mkv": models.FileVideo, ".avi": models.FileVideo, ".mov": models.FileVideo,
	".webm": models.FileVideo, ".m4v": models.FileVideo, ".flv": models.FileVideo, ".wmv": models.FileVideo,
	".ts": models.FileVideo, ".mts": models.FileVideo, ".3gp": models.FileVideo, ".mpg": models.FileVideo,
	".mpeg": models.FileVideo,

	".mp3": models.FileAudio, ".wav": models.FileAudio, ".flac": models.FileAudio, ".aac": models.FileAudio,
	".ogg": models.FileAudio, ".m4a": models.FileAudio, ".opus": models.FileAudio, ".wma": models.FileAudio,
	".aiff": models.FileAudio, ".aif": models.FileAudio, ".dsf": models.FileAudio, ".dff": models.FileAudio,

	".jpg": models.FileImage, ".jpeg": models.FileImage, ".png": models.FileImage, ".gif": models.FileImage,
	".webp": models.FileImage, ".bmp": models.FileImage, ".tif": models.FileImage, ".tiff": models.FileImage,
}

var opsByType = map[models.FileType][]models.Operation{
	models.FileVideo: {models.OpConvert, models.OpExtractAudio, models.OpThumbnail, models.OpMetadata, models.OpEnrichVideo},
	models.FileAudio: {models.OpConvertAudio, models.OpThumbnail, models.OpMetadata, models.OpEnrichAudio},
	models.FileImage: {models.OpThumbnail, models.OpMetadata},
}

var targetsByOp = map[models.Operation][]string{
	models.OpConvert:      {"mp4", "mkv", "webm"},
	models.OpExtractAudio: {"mp3", "wav", "flac", "aac"},
	models.OpConvertAudio: {"flac", "mp3", "wav", "aac", "ogg"},
	models.OpThumbnail:    {"jpg", "png", "webp"},
	models.OpMetadata:     {"json"},

	models.OpEnrichAudio: {"mp3", "flac", "ogg", "m4a"},
	models.OpEnrichVideo: {"mp4", "mkv"},
}

var ThumbnailWidths = []int{320, 640, 1280}

var identityExcludedOps = []models.Operation{models.OpConvert, models.OpConvertAudio}

var identityPreferredOps = []models.Operation{models.OpEnrichAudio, models.OpEnrichVideo}

var extAliases = map[string]string{"jpeg": "jpg", "tiff": "tif", "aiff": "aif", "m4a": "aac", "mpeg": "mpg"}

var poolByOp = map[models.Operation]string{
	models.OpConvert:      "video",
	models.OpExtractAudio: "video",
	models.OpConvertAudio: "audio",
	models.OpThumbnail:    "metadata",
	models.OpMetadata:     "metadata",
	models.OpEnrichAudio:  "metadata",
	models.OpEnrichVideo:  "metadata",
}

type RouteDecision struct {
	FileType  models.FileType
	Operation models.Operation
	Target    string
	Width     int
	Pool      string
}

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

func DefaultOperation(ft models.FileType) models.Operation { return opsByType[ft][0] }

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

func SameFormat(a, b string) bool { return normExt(a) == normExt(b) }

func excludesIdentity(op models.Operation) bool {
	for _, o := range identityExcludedOps {
		if o == op {
			return true
		}
	}
	return false
}

func PoolFor(ft models.FileType) string { return poolByOp[DefaultOperation(ft)] }

func PoolForOp(op models.Operation) string { return poolByOp[op] }

func Route(filename string, requested models.Operation) (RouteDecision, error) {
	return RouteWith(filename, requested, "", 0)
}

func RouteWith(filename string, requested models.Operation, target string, width int) (RouteDecision, error) {
	ft, err := DetectFileType(filename)
	if err != nil {
		return RouteDecision{}, err
	}
	return RouteAs(ft, filename, requested, target, width)
}

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

type Catalog struct {
	OpsByType   map[models.FileType][]models.Operation `json:"ops_by_type"`
	TargetsByOp map[models.Operation][]string          `json:"targets_by_op"`
	PoolByOp    map[models.Operation]string            `json:"pool_by_op"`
	ThumbWidths []int                                  `json:"thumbnail_widths"`
	Extensions  []string                               `json:"extensions"`

	IdentityExcludedOps []models.Operation `json:"identity_excluded_ops"`

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

func IsEnrich(op models.Operation) bool { return prefersIdentity(op) }

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

func titleFromFilename(filename string) string {
	base := strings.TrimSuffix(filepath.Base(filename), filepath.Ext(filename))
	base = strings.NewReplacer("_", " ", "-", " ").Replace(base)
	return strings.Join(strings.Fields(base), " ")
}
