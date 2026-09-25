package coordinator

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRequireUTF8JSON(t *testing.T) {
	var got []byte
	h := requireUTF8JSON(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, _ = io.ReadAll(r.Body)
	}))
	send := func(body []byte, ct string) int {
		req := httptest.NewRequest(http.MethodPost, "/cases", bytes.NewReader(body))
		if ct != "" {
			req.Header.Set("Content-Type", ct)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}

	utf8Body := []byte(`{"name":"prueba subida engañosa"}`)
	if code := send(utf8Body, "application/json"); code != http.StatusOK || !bytes.Equal(got, utf8Body) {
		t.Fatalf("UTF-8 válido: code=%d body=%q; debía pasar intacto", code, got)
	}

	cp1252 := []byte("{\"name\":\"prueba subida enga\xf1osa\"}")
	for _, ct := range []string{"application/json", "application/x-www-form-urlencoded", ""} {
		if code := send(cp1252, ct); code != http.StatusBadRequest {
			t.Errorf("Windows-1252 con Content-Type %q: code=%d; debía ser 400", ct, code)
		}
	}

	if code := send([]byte{0xff, 0xfe, 0xf1}, "multipart/form-data; boundary=x"); code != http.StatusOK {
		t.Errorf("multipart: code=%d; no debía revisarse", code)
	}
}
