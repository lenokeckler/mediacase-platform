// internal/multimedia/audio.go
// Atajos de audio con el destino por defecto; las recetas por formato están en ops.go.
package multimedia

import "context"

// ExtractAudio saca la pista de audio de un video a MP3.
func ExtractAudio(ctx context.Context, inputPath string, cb progressFn) (string, error) {
	return ExtractAudioTo(ctx, inputPath, "mp3", cb)
}

// ConvertAudio convierte cualquier audio a FLAC (sin pérdida).
func ConvertAudio(ctx context.Context, inputPath string, cb progressFn) (string, error) {
	return ConvertAudioTo(ctx, inputPath, "flac", cb)
}
