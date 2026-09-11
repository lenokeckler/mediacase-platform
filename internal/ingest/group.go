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

// ManifestFile es una entrada de dataset/manifest.json (v2). Los campos de agrupación son
// opcionales: un manifest v1 sin metadatos solo puede agruparse por carpeta.
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
}

type Manifest struct {
	Version int            `json:"version"`
	Total   int            `json:"total"`
	Files   []ManifestFile `json:"files"`
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
	Key string `json:"key"`
}

// ToRequest convierte un grupo en la solicitud de caso. La operación no se indica: la decide
// el coordinador por tipo (routing).
func (g Group) ToRequest(priority int) CaseRequest {
	r := CaseRequest{Name: g.Name(), Priority: priority, Files: make([]CaseFile, 0, len(g.Files))}
	for _, f := range g.Files {
		r.Files = append(r.Files, CaseFile{Key: f.Key})
	}
	return r
}
