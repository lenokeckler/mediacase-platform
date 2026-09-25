// Package ingest implementa la generación automática de casos (consigna §5): a partir de la
// lista de archivos y sus metadatos, los agrupa en casos según un criterio (evento, sesión,
// lote, usuario o carpeta) y produce las solicitudes POST /cases correspondientes.
package ingest

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"sort"
	"strings"
)

// ManifestFile es una entrada de dataset/manifest.json (v2, con campos v3 opcionales). Los
// campos de agrupación son opcionales: un manifest v1 sin metadatos solo puede agruparse por
// carpeta.
type ManifestFile struct {
	Filename  string `json:"filename"`
	Key       string `json:"key"`
	Type      string `json:"type"`
	Format    string `json:"format"`
	SizeBytes int64  `json:"size_bytes"`
	Duration  int    `json:"duration_s"`
	Tier      string `json:"tier"`
	Event     string `json:"event"`
	Session   string `json:"session"`
	Batch     string `json:"batch"`
	User      string `json:"user"`
	// v3 (opcionales): procedencia del archivo, para el dataset con casos reales/de borde
	// mezclados con los sintéticos (GET /dataset los expone tal cual al dashboard).
	Source  string `json:"source,omitempty"` // synthetic | real | edge
	Origin  string `json:"origin,omitempty"`
	License string `json:"license,omitempty"`
	Author  string `json:"author,omitempty"`
	URL     string `json:"url,omitempty"`
	Note    string `json:"note,omitempty"`
}

// TestCase es un caso de prueba ya armado en el manifest (v3, campo top-level "test_cases"):
// se envía tal cual con `ingest cases --test-cases`, sin pasar por GroupBy.
type TestCase struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Kind        string         `json:"kind,omitempty"` // homogeneous | heterogeneous
	Files       []TestCaseFile `json:"files"`
}

// TestCaseFile es un archivo de un TestCase, con su operación y destino ya decididos por quien
// armó el manifest (a diferencia de Group.ToRequest, que deja la operación al coordinador).
type TestCaseFile struct {
	Key        string      `json:"key"`
	Operation  string      `json:"operation,omitempty"`
	Target     string      `json:"target,omitempty"`
	Width      int         `json:"width,omitempty"`
	Enrichment *Enrichment `json:"enrichment,omitempty"`
}

// ToRequest convierte el caso de prueba en la solicitud de POST /cases, tal cual está definido
// en el manifest (operación, destino, ancho y recursos asociados por archivo).
func (tc TestCase) ToRequest(priority int) CaseRequest {
	r := CaseRequest{Name: tc.Name, Priority: priority, Files: make([]CaseFile, 0, len(tc.Files))}
	for _, f := range tc.Files {
		r.Files = append(r.Files, CaseFile{
			Key: f.Key, Operation: f.Operation, Target: f.Target, Width: f.Width, Enrichment: f.Enrichment,
		})
	}
	return r
}

type Manifest struct {
	Version   int            `json:"version"`
	Total     int            `json:"total"`
	Files     []ManifestFile `json:"files"`
	TestCases []TestCase     `json:"test_cases,omitempty"`
}

func LoadManifest(p string) (*Manifest, error) {
	raw, err := os.ReadFile(p)
	if err != nil {
		return nil, err
	}
	var m Manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("manifest inválido: %w", err)
	}
	for i := range m.Files {
		if m.Files[i].Key == "" {
			m.Files[i].Key = m.Files[i].Filename
		}
	}
	return &m, nil
}

// Criterios de agrupación admitidos. Los cuatro primeros son metadatos (consigna §5:
// evento, sesión, usuario, lote de ingesta); folder es "misma carpeta local" (manifest v1);
// type y tier producen siempre casos homogéneos (mismo tipo / mismo tamaño).
var Criteria = []string{"event", "session", "batch", "user", "folder", "type", "tier"}

// Group es un caso candidato: los archivos que comparten el valor del criterio.
type Group struct {
	Criterion   string
	Value       string
	Files       []ManifestFile
	Homogeneous bool // todos del mismo tipo de contenido (misma operación por defecto)
}

// Name es el nombre con el que se registra el caso.
func (g Group) Name() string { return g.Criterion + "=" + g.Value }

// Types devuelve los tipos de contenido presentes, ordenados.
func (g Group) Types() []string {
	seen := map[string]bool{}
	for _, f := range g.Files {
		seen[f.Type] = true
	}
	out := make([]string, 0, len(seen))
	for t := range seen {
		out = append(out, t)
	}
	sort.Strings(out)
	return out
}

func keyOf(f ManifestFile, criterion string) string {
	switch criterion {
	case "event":
		return f.Event
	case "session":
		return f.Session
	case "batch":
		return f.Batch
	case "user":
		return f.User
	case "type":
		return f.Type
	case "tier":
		return f.Tier
	case "folder":
		d := path.Dir(strings.ReplaceAll(f.Key, "\\", "/"))
		if d == "." {
			return "(raíz)"
		}
		return d
	}
	return ""
}

// GroupBy agrupa los archivos por el criterio. Los archivos sin valor para ese criterio se
// omiten. Devuelve los grupos ordenados por nombre para que el resultado sea reproducible.
func GroupBy(files []ManifestFile, criterion string) ([]Group, error) {
	valid := false
	for _, c := range Criteria {
		if c == criterion {
			valid = true
		}
	}
	if !valid {
		return nil, fmt.Errorf("criterio %q no soportado; usar uno de %v", criterion, Criteria)
	}
	byKey := map[string][]ManifestFile{}
	for _, f := range files {
		k := keyOf(f, criterion)
		if k == "" {
			continue
		}
		byKey[k] = append(byKey[k], f)
	}
	groups := make([]Group, 0, len(byKey))
	for k, fs := range byKey {
		sort.Slice(fs, func(i, j int) bool { return fs[i].Key < fs[j].Key })
		g := Group{Criterion: criterion, Value: k, Files: fs}
		g.Homogeneous = len(g.Types()) == 1
		groups = append(groups, g)
	}
	sort.Slice(groups, func(i, j int) bool { return groups[i].Value < groups[j].Value })
	return groups, nil
}

// Filter deja solo grupos homogéneos, solo heterogéneos, o todos (mode = "homogeneous" |
// "heterogeneous" | "").
func Filter(groups []Group, mode string) []Group {
	if mode == "" {
		return groups
	}
	out := groups[:0:0]
	for _, g := range groups {
		if (mode == "homogeneous" && g.Homogeneous) || (mode == "heterogeneous" && !g.Homogeneous) {
			out = append(out, g)
		}
	}
	return out
}

// CaseRequest es el body de POST /cases.
type CaseRequest struct {
	Name     string     `json:"name"`
	Priority int        `json:"priority"`
	Files    []CaseFile `json:"files"`
}

type CaseFile struct {
	Key        string      `json:"key"`
	Operation  string      `json:"operation,omitempty"`
	Target     string      `json:"target,omitempty"`
	Width      int         `json:"width,omitempty"`
	Enrichment *Enrichment `json:"enrichment,omitempty"`
}

// Enrichment son los recursos asociados que el coordinador integra en enrich_* (misma forma que
// models.Enrichment; se repite aquí para que el cliente no dependa del coordinador).
type Enrichment struct {
	Title   string `json:"title,omitempty"`
	Artist  string `json:"artist,omitempty"`
	Album   string `json:"album,omitempty"`
	Date    string `json:"date,omitempty"`
	Comment string `json:"comment,omitempty"`
	Lyrics  string `json:"lyrics,omitempty"`
}

// UnmarshalJSON acepta "description" como alias de "lyrics": el manifest usa ese nombre para los
// test_cases de video, pero el coordinador solo conoce "lyrics" (letra o descripción según el
// tipo; ver models.Enrichment). Si vienen los dos, gana "lyrics".
func (e *Enrichment) UnmarshalJSON(data []byte) error {
	type alias Enrichment
	aux := struct {
		*alias
		Description string `json:"description,omitempty"`
	}{alias: (*alias)(e)}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	if e.Lyrics == "" {
		e.Lyrics = aux.Description
	}
	return nil
}

// ToRequest convierte un grupo en la solicitud de caso. La operación no se indica: la decide
// el coordinador por tipo (routing).
func (g Group) ToRequest(priority int) CaseRequest { return g.ToRequestWith(priority, false) }

// ToRequestWith es ToRequest y, con enrich, pide enrich_* para audios y videos con los recursos
// del manifest: usuario → artista, evento → álbum, sesión y lote → comentario. El título y el
// álbum que falten los completa el coordinador.
func (g Group) ToRequestWith(priority int, enrich bool) CaseRequest {
	r := CaseRequest{Name: g.Name(), Priority: priority, Files: make([]CaseFile, 0, len(g.Files))}
	for _, f := range g.Files {
		cf := CaseFile{Key: f.Key}
		if enrich {
			switch f.Type {
			case "audio":
				cf.Operation = "enrich_audio"
			case "video":
				cf.Operation = "enrich_video"
			}
			if cf.Operation != "" {
				cf.Enrichment = enrichmentFromManifest(f)
			}
		}
		r.Files = append(r.Files, cf)
	}
	return r
}

func enrichmentFromManifest(f ManifestFile) *Enrichment {
	e := &Enrichment{Artist: f.User, Album: f.Event}
	switch {
	case f.Session != "" && f.Batch != "":
		e.Comment = f.Session + " · " + f.Batch
	case f.Session != "":
		e.Comment = f.Session
	case f.Batch != "":
		e.Comment = f.Batch
	}
	if *e == (Enrichment{}) {
		return nil
	}
	return e
}
