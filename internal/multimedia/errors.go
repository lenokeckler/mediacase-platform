package multimedia

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// ErrEmptyInput: el archivo de entrada no tiene ni un byte (ffmpeg diría "moov atom not found",
// que no explica nada al leer el reporte del caso).
var ErrEmptyInput = errors.New("archivo vacío (0 bytes): no hay contenido que procesar")

// CheckInput falla antes de llamar a ffmpeg si la entrada no existe o está vacía.
func CheckInput(path string) error {
	fi, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("entrada: %w", err)
	}
	if fi.Size() == 0 {
		return ErrEmptyInput
	}
	return nil
}

// ffmpegAddrRe quita la dirección de memoria de los prefijos de ffmpeg: "[mov,mp4 @ 0000024c…]".
var ffmpegAddrRe = regexp.MustCompile(` @ (0x)?[0-9a-fA-F]+\]`)

// CleanError deja el mensaje de error listo para el reporte del caso: sin la ruta temporal del
// worker (se deja solo el nombre del archivo), sin direcciones de memoria y en una sola línea.
func CleanError(err error, localInput string) string {
	msg := err.Error()
	if localInput != "" {
		msg = strings.ReplaceAll(msg, localInput, filepath.Base(localInput))
		msg = strings.ReplaceAll(msg, filepath.Dir(localInput)+string(filepath.Separator), "")
	}
	msg = ffmpegAddrRe.ReplaceAllString(msg, "]")
	return strings.Join(strings.Fields(msg), " ")
}
