package multimedia

import "context"

func ExtractAudio(ctx context.Context, inputPath string, cb progressFn) (string, error) {
	return ExtractAudioTo(ctx, inputPath, "mp3", cb)
}

func ConvertAudio(ctx context.Context, inputPath string, cb progressFn) (string, error) {
	return ConvertAudioTo(ctx, inputPath, "flac", cb)
}
