package ingest

import "testing"

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
