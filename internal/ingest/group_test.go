package ingest

import (
	"encoding/json"
	"testing"
)

func sample() []ManifestFile {
	return []ManifestFile{
		{Key: "v1.mp4", Type: "video", Event: "boda", Session: "boda-s1", Batch: "lote-1", User: "leno"},
		{Key: "v2.mkv", Type: "video", Event: "boda", Session: "boda-s1", Batch: "lote-2", User: "jennifer"},
		{Key: "a1.mp3", Type: "audio", Event: "boda", Session: "boda-s2", Batch: "lote-1", User: "leno"},
		{Key: "i1.jpg", Type: "image", Event: "clase", Session: "clase-s1", Batch: "lote-1", User: "jonathan"},
		{Key: "a2.wav", Type: "audio", Event: "clase", Session: "clase-s1", Batch: "lote-2", User: "jonathan"},
		{Key: "casos/sesion3/v3.mp4", Type: "video"}, // manifest v1: sin metadatos, solo carpeta
	}
}

func TestGroupBy_Event(t *testing.T) {
	groups, err := GroupBy(sample(), "event")
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 2 || groups[0].Value != "boda" || groups[1].Value != "clase" {
		t.Fatalf("grupos: %+v", groups)
	}
	if len(groups[0].Files) != 3 || groups[0].Homogeneous {
		t.Errorf("boda: 3 archivos y heterogéneo (video+audio): %+v", groups[0])
	}
	if got := groups[0].Types(); len(got) != 2 || got[0] != "audio" || got[1] != "video" {
		t.Errorf("tipos de boda: %v", got)
	}
	if groups[0].Name() != "event=boda" {
		t.Errorf("nombre: %s", groups[0].Name())
	}
}

func TestGroupBy_SessionProduceHomogeneosYHeterogeneos(t *testing.T) {
	groups, _ := GroupBy(sample(), "session")
	hom := Filter(groups, "homogeneous")
	het := Filter(groups, "heterogeneous")
	if len(hom) != 2 { // boda-s1 (2 videos) y boda-s2 (1 audio)
		t.Errorf("homogéneos: %d %+v", len(hom), hom)
	}
	if len(het) != 1 || het[0].Value != "clase-s1" {
		t.Errorf("heterogéneos: %+v", het)
	}
	if len(Filter(groups, "")) != 3 {
		t.Errorf("sin filtro deben ser 3")
	}
}

func TestGroupBy_Folder(t *testing.T) {
	groups, _ := GroupBy(sample(), "folder")
	var names []string
	for _, g := range groups {
		names = append(names, g.Value)
	}
	if len(groups) != 2 || groups[0].Value != "(raíz)" || groups[1].Value != "casos/sesion3" {
		t.Errorf("carpetas: %v", names)
	}
}

func TestGroupBy_OmiteSinValorYRechazaCriterioMalo(t *testing.T) {
	groups, _ := GroupBy(sample(), "user")
	total := 0
	for _, g := range groups {
		total += len(g.Files)
	}
	if total != 5 { // el archivo v1 sin user se omite
		t.Errorf("archivos agrupados por user: %d, quería 5", total)
	}
	if _, err := GroupBy(sample(), "color"); err == nil {
		t.Error("criterio inválido debe fallar")
	}
}

func TestGroupBy_TypeSiempreHomogeneo(t *testing.T) {
	groups, _ := GroupBy(sample(), "type")
	if len(groups) != 3 {
		t.Fatalf("tipos: %+v", groups)
	}
	for _, g := range groups {
		if !g.Homogeneous {
			t.Errorf("%s debería ser homogéneo", g.Name())
		}
	}
}

func TestToRequest(t *testing.T) {
	groups, _ := GroupBy(sample(), "batch")
	r := groups[0].ToRequest(7)
	if r.Name != "batch=lote-1" || r.Priority != 7 || len(r.Files) != 3 || r.Files[0].Key != "a1.mp3" {
		t.Errorf("request: %+v", r)
	}
}

// Con -enrich, los audios y videos del grupo van como enrich_* con los recursos del manifest
// (usuario → artista, evento → álbum, sesión y lote → comentario); las imágenes siguen con la
// operación que decide el coordinador.
func TestToRequestEnriched(t *testing.T) {
	groups, _ := GroupBy(sample(), "event")
	var boda Group
	for _, g := range groups {
		if g.Name() == "event=boda" {
			boda = g
		}
	}
	r := boda.ToRequestWith(5, true)
	if len(r.Files) != 3 {
		t.Fatalf("archivos: %+v", r.Files)
	}
	byKey := map[string]CaseFile{}
	for _, f := range r.Files {
		byKey[f.Key] = f
	}
	if v := byKey["v1.mp4"]; v.Operation != "enrich_video" || v.Enrichment == nil || v.Enrichment.Artist != "leno" || v.Enrichment.Album != "boda" || v.Enrichment.Comment != "boda-s1 · lote-1" {
		t.Errorf("video: %+v %+v", v, v.Enrichment)
	}
	if a := byKey["a1.mp3"]; a.Operation != "enrich_audio" || a.Enrichment == nil || a.Enrichment.Artist != "leno" {
		t.Errorf("audio: %+v %+v", a, a.Enrichment)
	}
	// Sin -enrich no cambia nada.
	plain := boda.ToRequestWith(5, false)
	if plain.Files[0].Operation != "" || plain.Files[0].Enrichment != nil {
		t.Errorf("sin enrich: %+v", plain.Files[0])
	}
	// Un archivo sin metadatos (manifest v1) igual va a enriquecer, solo sin artista ni álbum.
	groups, _ = GroupBy(sample(), "folder")
	for _, g := range groups {
		for _, f := range g.ToRequestWith(5, true).Files {
			if f.Key == "casos/sesion3/v3.mp4" && (f.Operation != "enrich_video" || f.Enrichment != nil) {
				t.Errorf("sin metadatos: %+v", f)
			}
		}
	}
}

// Un caso de prueba del manifest (v3, "test_cases") se envía tal cual: operación, destino, ancho
// y recursos asociados por archivo, sin pasar por GroupBy.
func TestTestCase_ToRequest(t *testing.T) {
	tc := TestCase{
		ID: "heterogeneo-1", Name: "caso de prueba heterogéneo", Kind: "heterogeneous",
		Files: []TestCaseFile{
			{Key: "v1.mp4", Operation: "convert", Target: "mkv"},
			{Key: "i1.jpg", Operation: "thumbnail", Target: "webp", Width: 640},
			{Key: "a1.mp3", Operation: "enrich_audio", Enrichment: &Enrichment{Title: "Tema", Artist: "JLJ"}},
		},
	}
	r := tc.ToRequest(8)
	if r.Name != "caso de prueba heterogéneo" || r.Priority != 8 || len(r.Files) != 3 {
		t.Fatalf("request: %+v", r)
	}
	if r.Files[0].Target != "mkv" || r.Files[1].Width != 640 || r.Files[2].Enrichment.Artist != "JLJ" {
		t.Errorf("no conservó operación/destino/ancho/recursos tal cual: %+v", r.Files)
	}
}

// El manifest describe la "descripción" de un video enriquecido con la clave "description"; el
// coordinador solo entiende "lyrics" (letra o descripción según el tipo), así que se alía a ese
// campo al deserializar.
func TestEnrichment_DescriptionEsAliasDeLyrics(t *testing.T) {
	var e Enrichment
	if err := json.Unmarshal([]byte(`{"title":"Apertura","description":"discurso de bienvenida"}`), &e); err != nil {
		t.Fatal(err)
	}
	if e.Title != "Apertura" || e.Lyrics != "discurso de bienvenida" {
		t.Errorf("alias description → lyrics: %+v", e)
	}
	// Si vienen los dos, gana "lyrics".
	var e2 Enrichment
	json.Unmarshal([]byte(`{"lyrics":"letra real","description":"no debería ganar"}`), &e2)
	if e2.Lyrics != "letra real" {
		t.Errorf("lyrics debía ganar sobre description: %+v", e2)
	}
}
