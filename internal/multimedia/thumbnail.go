// internal/multimedia/thumbnail.go
// Atajo de miniatura con el destino por defecto (JPG, 320 px). Ver ThumbnailTo en ops.go.
package multimedia

import "context"

func Thumbnail(ctx context.Context, inputPath string, cb progressFn) (string, error) {
	return ThumbnailTo(ctx, inputPath, "jpg", 320, cb)
}
