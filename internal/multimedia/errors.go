package multimedia

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var ErrEmptyInput = errors.New("archivo vacío (0 bytes): no hay contenido que procesar")

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

var ffmpegAddrRe = regexp.MustCompile(` @ (0x)?[0-9a-fA-F]+\]`)

func CleanError(err error, localInput string) string {
	msg := err.Error()
	if localInput != "" {
		msg = strings.ReplaceAll(msg, localInput, filepath.Base(localInput))
		msg = strings.ReplaceAll(msg, filepath.Dir(localInput)+string(filepath.Separator), "")
	}
	msg = ffmpegAddrRe.ReplaceAllString(msg, "]")
	return strings.Join(strings.Fields(msg), " ")
}
