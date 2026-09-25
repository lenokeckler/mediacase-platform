package coordinator

import "testing"

func TestSafeKey(t *testing.T) {
	cases := map[string]string{
		"grabación subida.mp4":               "grabación subida.mp4",
		"Grieg - La mañana (Peer Gynt).flac": "Grieg - La mañana (Peer Gynt).flac",
		`C:\Users\x\Desktop\foto#1?.jpg`:     "foto_1_.jpg",
		"../../etc/passwd":                   "passwd",
		" .oculto.png":                       "oculto.png",
	}
	for in, want := range cases {
		if got := safeKey(in); got != want {
			t.Errorf("safeKey(%q) = %q, quería %q", in, got, want)
		}
	}
}
