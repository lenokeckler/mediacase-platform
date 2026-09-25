package multimedia

import "context"

func Thumbnail(ctx context.Context, inputPath string, cb progressFn) (string, error) {
	return ThumbnailTo(ctx, inputPath, "jpg", 320, cb)
}
