package coordinator

import (
	"bytes"
	"io"
	"mime"
	"net/http"
	"strings"
	"unicode/utf8"
)

const maxJSONBody = 16 << 20

func requireUTF8JSON(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Body == nil || r.Method == http.MethodGet || !isJSONRequest(r) {
			next.ServeHTTP(w, r)
			return
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, maxJSONBody+1))
		r.Body.Close()
		if err != nil {
			http.Error(w, "no se pudo leer el cuerpo: "+err.Error(), http.StatusBadRequest)
			return
		}
		if len(body) > maxJSONBody {
			http.Error(w, "cuerpo demasiado grande", http.StatusRequestEntityTooLarge)
			return
		}
		if !utf8.Valid(body) {
			http.Error(w, "el cuerpo no está en UTF-8 (tildes o ñ con otra codificación, p. ej. "+
				"Windows-1252); envíelo como UTF-8: en PowerShell use -ContentType "+
				"'application/json; charset=utf-8' con el texto convertido a bytes UTF-8, o guarde "+
				"el JSON en un archivo UTF-8 y mándelo con curl --data-binary @archivo.json",
				http.StatusBadRequest)
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(body))
		next.ServeHTTP(w, r)
	})
}

func isJSONRequest(r *http.Request) bool {
	mt, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	return err != nil || !strings.HasPrefix(mt, "multipart/")
}
