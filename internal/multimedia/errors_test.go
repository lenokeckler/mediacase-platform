package multimedia

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCleanError(t *testing.T) {
	in := filepath.Join(os.TempDir(), "mediacase-in", "abc", "edge_truncado.mp4")
	err := errors.New("convert: ffprobe: [mov,mp4,m4a,3gp,3g2,mj2 @ 00000247d74d59c0] moov atom not found\n" + in + ": Invalid data")
	got := CleanError(err, in)
	want := "convert: ffprobe: [mov,mp4,m4a,3gp,3g2,mj2] moov atom not found edge_truncado.mp4: Invalid data"
	if got != want {
		t.Errorf("CleanError:\n got %q\nwant %q", got, want)
	}
}

func TestCheckInput_Vacio(t *testing.T) {
	p := filepath.Join(t.TempDir(), "vacio.mp4")
	if err := os.WriteFile(p, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := CheckInput(p); !errors.Is(err, ErrEmptyInput) {
		t.Errorf("0 bytes: %v", err)
	}
	if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := CheckInput(p); err != nil {
		t.Errorf("1 byte: %v", err)
	}
	if err := CheckInput(p + ".no"); err == nil || !strings.HasPrefix(err.Error(), "entrada:") {
		t.Errorf("inexistente: %v", err)
	}
}
