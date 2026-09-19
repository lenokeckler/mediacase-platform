package cases

import (
	"testing"
	"time"

	"github.com/lenokeckler/mediacase-platform/internal/models"
)

func ts(base time.Time, secs int) *time.Time {
	t := base.Add(time.Duration(secs) * time.Second)
	return &t
}

func TestBuildReport_ResumenYAgrupacion(t *testing.T) {
	base := time.Date(2026, 9, 10, 10, 41, 0, 0, time.UTC)
	c := &models.Case{ID: "C1", Name: "boda", Status: models.CasePartiallyCompleted, TotalJobs: 4,
		CreatedAt: base, StartedAt: ts(base, 2), CompletedAt: ts(base, 40)}
	jobs := []*models.Job{
		{ID: "j1", FilePath: "a.mp4", FileType: models.FileVideo, Operation: models.OpConvert, Status: models.StatusCompleted,
			WorkerID: "node1", StartedAt: ts(base, 2), CompletedAt: ts(base, 37), ResultURL: "http://x/a.mp4"},
		{ID: "j2", FilePath: "b.mp4", FileType: models.FileVideo, Operation: models.OpConvert, Status: models.StatusCompleted,
			WorkerID: "node1", StartedAt: ts(base, 3), CompletedAt: ts(base, 30)},
		{ID: "j3", FilePath: "c.mp3", FileType: models.FileAudio, Operation: models.OpConvertAudio, Status: models.StatusCompleted,
			WorkerID: "node2", StartedAt: ts(base, 2), CompletedAt: ts(base, 10)},
		{ID: "j4", FilePath: "d.jpg", FileType: models.FileImage, Operation: models.OpThumbnail, Status: models.StatusFailed,
			WorkerID: "node3", StartedAt: ts(base, 2), CompletedAt: ts(base, 3), ErrorMsg: "formato no soportado\nffmpeg: ..."},
	}
	r := BuildReport(c, jobs)

	want := "de 4 archivos — 2 videos convertidos, 1 audio convertido, 1 fallido (formato no soportado)"
	if r.Summary != want {
		t.Fatalf("resumen:\n got: %s\nwant: %s", r.Summary, want)
	}
	if r.Totals.Total != 4 || r.Totals.Completed != 3 || r.Totals.Failed != 1 {
		t.Fatalf("totales: %+v", r.Totals)
	}
	if len(r.ByTypeAndOperation) != 3 {
		t.Fatalf("grupos tipo/operación: quería 3, hay %d: %+v", len(r.ByTypeAndOperation), r.ByTypeAndOperation)
	}
	if g := r.ByTypeAndOperation[0]; g.FileType != models.FileVideo || g.Completed != 2 {
		t.Errorf("los grupos deben venir del más numeroso al menos; primero video (2): %+v", g)
	}
	if r.DurationSeconds != 38 {
		t.Errorf("duración del caso: %v, quería 38", r.DurationSeconds)
	}
	if r.SubTasks[0].DurationSeconds != 35 || r.SubTasks[0].WorkerID != "node1" {
		t.Errorf("sub-tarea j1: %+v", r.SubTasks[0])
	}
	if r.SubTasks[3].Error != "formato no soportado\nffmpeg: ..." {
		t.Errorf("el detalle del error debe conservarse completo en la sub-tarea")
	}
}

func TestSummary_Plurales(t *testing.T) {
	c := &models.Case{ID: "C2", Status: models.CaseCompleted, TotalJobs: 1}
	r := BuildReport(c, []*models.Job{
		{FilePath: "a.mp3", FileType: models.FileAudio, Operation: models.OpConvertAudio, Status: models.StatusCompleted},
	})
	if r.Summary != "de 1 archivo — 1 audio convertido" {
		t.Errorf("singular: %q", r.Summary)
	}

	c = &models.Case{ID: "C3", Status: models.CaseFailed, TotalJobs: 2}
	r = BuildReport(c, []*models.Job{
		{FilePath: "a.mp4", FileType: models.FileVideo, Operation: models.OpConvert, Status: models.StatusFailed, ErrorMsg: "x"},
		{FilePath: "b.mp4", FileType: models.FileVideo, Operation: models.OpConvert, Status: models.StatusFailed, ErrorMsg: "x"},
	})
	if r.Summary != "de 2 archivos — 2 fallidos (x)" {
		t.Errorf("todo fallido: %q", r.Summary)
	}
}

func TestSummary_CanceladasNoCuentanComoFallidas(t *testing.T) {
	c := &models.Case{ID: "C4", Status: models.CaseCancelled, TotalJobs: 2}
	r := BuildReport(c, []*models.Job{
		{FilePath: "a.mp4", FileType: models.FileVideo, Operation: models.OpConvert, Status: models.StatusCompleted},
		{FilePath: "b.mp4", FileType: models.FileVideo, Operation: models.OpConvert, Status: models.StatusCancelled},
	})
	if r.Totals.Failed != 0 || r.Totals.Cancelled != 1 {
		t.Errorf("totales: %+v", r.Totals)
	}
	if r.Summary != "de 2 archivos — 1 video convertido, 1 cancelado" {
		t.Errorf("resumen con cancelada: %q", r.Summary)
	}
}

func TestBuildReport_DestinoEnResumenYGrupos(t *testing.T) {
	base := time.Date(2026, 9, 11, 21, 0, 0, 0, time.UTC)
	c := &models.Case{ID: "C2", Name: "mixto", Status: models.CaseCompleted, TotalJobs: 4, CreatedAt: base}
	jobs := []*models.Job{
		{ID: "j1", FilePath: "a.mkv", FileType: models.FileVideo, Operation: models.OpConvert, Target: "mp4", Status: models.StatusCompleted},
		{ID: "j2", FilePath: "b.mov", FileType: models.FileVideo, Operation: models.OpConvert, Target: "webm", Status: models.StatusCompleted},
		{ID: "j3", FilePath: "c.aiff", FileType: models.FileAudio, Operation: models.OpConvertAudio, Target: "flac", Status: models.StatusCompleted},
		{ID: "j4", FilePath: "d.mp4", FileType: models.FileVideo, Operation: models.OpMetadata, Target: "json", Status: models.StatusCompleted},
	}
	r := BuildReport(c, jobs)
	want := "de 4 archivos — 1 audio convertido a FLAC, 1 video convertido a MP4, 1 video convertido a WEBM, 1 archivo con metadatos extraídos"
	if r.Summary != want {
		t.Fatalf("resumen:\n got: %s\nwant: %s", r.Summary, want)
	}
	if len(r.ByTypeAndOperation) != 4 {
		t.Fatalf("un grupo por tipo/operación/destino: %+v", r.ByTypeAndOperation)
	}
	if r.SubTasks[0].SourceExt != "mkv" || r.SubTasks[0].Target != "mp4" || r.SubTasks[2].SourceExt != "aiff" {
		t.Errorf("cada sub-tarea lleva origen → destino: %+v", r.SubTasks)
	}
}

// Las sub-tareas enriquecidas se cuentan con su etiqueta ("audios enriquecidos"), y el reporte
// conserva qué recursos se integraron (etiquetas, letra) para que el detalle del caso lo muestre.
func TestBuildReport_Enriquecidos(t *testing.T) {
	base := time.Date(2026, 9, 18, 21, 0, 0, 0, time.UTC)
	c := &models.Case{ID: "C3", Name: "concierto", Status: models.CaseCompleted, TotalJobs: 3, CreatedAt: base}
	jobs := []*models.Job{
		{ID: "j1", FilePath: "a.mp3", FileType: models.FileAudio, Operation: models.OpEnrichAudio, Target: "mp3", Status: models.StatusCompleted,
			Enrichment: &models.Enrichment{Title: "Tema 1", Artist: "JLJ", Album: "concierto", Lyrics: "la la la"}},
		{ID: "j2", FilePath: "b.wav", FileType: models.FileAudio, Operation: models.OpEnrichAudio, Target: "mp3", Status: models.StatusCompleted,
			Enrichment: &models.Enrichment{Title: "Tema 2", Album: "concierto"}},
		{ID: "j3", FilePath: "v.mkv", FileType: models.FileVideo, Operation: models.OpEnrichVideo, Target: "mkv", Status: models.StatusCompleted,
			Enrichment: &models.Enrichment{Title: "Apertura", Lyrics: "descripción del video"}},
	}
	r := BuildReport(c, jobs)
	want := "de 3 archivos — 2 audios enriquecidos a MP3, 1 video enriquecido a MKV"
	if r.Summary != want {
		t.Fatalf("resumen:\n got: %s\nwant: %s", r.Summary, want)
	}
	if r.SubTasks[0].Enrichment == nil || r.SubTasks[0].Enrichment.Lyrics != "la la la" || r.SubTasks[1].Enrichment.Artist != "" {
		t.Errorf("cada sub-tarea enriquecida lleva sus recursos: %+v", r.SubTasks)
	}
}
