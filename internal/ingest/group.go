package ingest

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"sort"
	"strings"
)

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

	Source  string `json:"source,omitempty"`
	Origin  string `json:"origin,omitempty"`
	License string `json:"license,omitempty"`
	Author  string `json:"author,omitempty"`
	URL     string `json:"url,omitempty"`
	Note    string `json:"note,omitempty"`
}

type TestCase struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Kind        string         `json:"kind,omitempty"`
	Files       []TestCaseFile `json:"files"`
}

type TestCaseFile struct {
	Key        string      `json:"key"`
	Operation  string      `json:"operation,omitempty"`
	Target     string      `json:"target,omitempty"`
	Width      int         `json:"width,omitempty"`
	Enrichment *Enrichment `json:"enrichment,omitempty"`
}

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

var Criteria = []string{"event", "session", "batch", "user", "folder", "type", "tier"}

type Group struct {
	Criterion   string
	Value       string
	Files       []ManifestFile
	Homogeneous bool
}

func (g Group) Name() string { return g.Criterion + "=" + g.Value }

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

type Enrichment struct {
	Title   string `json:"title,omitempty"`
	Artist  string `json:"artist,omitempty"`
	Album   string `json:"album,omitempty"`
	Date    string `json:"date,omitempty"`
	Comment string `json:"comment,omitempty"`
	Lyrics  string `json:"lyrics,omitempty"`
}

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

func (g Group) ToRequest(priority int) CaseRequest { return g.ToRequestWith(priority, false) }

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
