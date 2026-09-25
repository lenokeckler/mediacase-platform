package cases

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/lenokeckler/mediacase-platform/internal/models"
)

// Reporte consolidado por caso (consigna §7). Se genera al cerrar el caso e incluye:
// identificador y momentos de creación/cierre, archivos agrupados por tipo y operación,
// resultado individual de cada sub-tarea con detalle del error, tiempos de inicio y fin
// del caso y de cada sub-tarea, worker responsable de cada una, y un resumen agregado.

type SubTaskResult struct {
	JobID           string             `json:"job_id"`
	File            string             `json:"file"`
	FileType        models.FileType    `json:"file_type"`
	Operation       models.Operation   `json:"operation"`
	SourceExt       string             `json:"source_ext,omitempty"` // formato de entrada (mkv, flac…)
	Assignment      string             `json:"assignment,omitempty"` // afinidad | ayuda
	Target          string             `json:"target,omitempty"`     // formato de salida (mp4, mp3, json…)
	Status          models.JobStatus   `json:"status"`
	WorkerID        string             `json:"worker_id,omitempty"`
	StartedAt       *time.Time         `json:"started_at,omitempty"`
	CompletedAt     *time.Time         `json:"completed_at,omitempty"`
	DurationSeconds float64            `json:"duration_seconds"`
	ResultURL       string             `json:"result_url,omitempty"`
	Error           string             `json:"error,omitempty"`
	Enrichment      *models.Enrichment `json:"enrichment,omitempty"` // recursos integrados (solo enrich_*)
	// RoutingNote: la inspección de contenido detectó que la extensión no correspondía al
	// contenido real, o el archivo estaba vacío. "" = extensión OK.
	RoutingNote string `json:"routing_note,omitempty"`
}

type GroupCount struct {
	FileType  models.FileType  `json:"file_type"`
	Operation models.Operation `json:"operation"`
	Target    string           `json:"target,omitempty"`
	Completed int              `json:"completed"`
	Failed    int              `json:"failed"`
	Cancelled int              `json:"cancelled"`
}

type Totals struct {
	Total     int `json:"total"`
	Completed int `json:"completed"`
	Failed    int `json:"failed"`
	Cancelled int `json:"cancelled"`
}

type Report struct {
	CaseID             string            `json:"case_id"`
	Name               string            `json:"name"`
	Status             models.CaseStatus `json:"status"`
	CreatedAt          time.Time         `json:"created_at"`
	StartedAt          *time.Time        `json:"started_at,omitempty"`
	CompletedAt        *time.Time        `json:"completed_at,omitempty"`
	DurationSeconds    float64           `json:"duration_seconds"`
	Totals             Totals            `json:"totals"`
	ByTypeAndOperation []GroupCount      `json:"by_type_and_operation"`
	SubTasks           []SubTaskResult   `json:"sub_tasks"`
	Summary            string            `json:"summary"`
}

// Etiquetas para el resumen: singular y plural por operación; el destino se agrega después
// ("videos convertidos a MP4", "audios extraídos a FLAC").
var opLabels = map[models.Operation][2]string{
	models.OpConvert:      {"video convertido", "videos convertidos"},
	models.OpConvertAudio: {"audio convertido", "audios convertidos"},
	models.OpExtractAudio: {"audio extraído", "audios extraídos"},
	models.OpThumbnail:    {"miniatura generada", "miniaturas generadas"},
	models.OpMetadata:     {"archivo con metadatos extraídos", "archivos con metadatos extraídos"},
	models.OpEnrichAudio:  {"audio enriquecido", "audios enriquecidos"},
	models.OpEnrichVideo:  {"video enriquecido", "videos enriquecidos"},
}

// BuildReport arma el reporte a partir del caso y sus sub-tareas. Es una función pura.
func BuildReport(c *models.Case, jobs []*models.Job) *Report {
	r := &Report{
		CaseID: c.ID, Name: c.Name, Status: c.Status,
		CreatedAt: c.CreatedAt, StartedAt: c.StartedAt, CompletedAt: c.CompletedAt,
		SubTasks: make([]SubTaskResult, 0, len(jobs)),
	}
	if c.StartedAt != nil && c.CompletedAt != nil {
		r.DurationSeconds = c.CompletedAt.Sub(*c.StartedAt).Seconds()
	}

	groups := map[string]*GroupCount{}
	for _, j := range jobs {
		st := SubTaskResult{
			JobID: j.ID, File: j.FilePath, FileType: j.FileType, Operation: j.Operation,
			SourceExt: strings.TrimPrefix(strings.ToLower(filepath.Ext(j.FilePath)), "."), Target: j.Target, Assignment: j.Assignment,
			Status: j.Status, WorkerID: j.WorkerID, StartedAt: j.StartedAt, CompletedAt: j.CompletedAt,
			ResultURL: j.ResultURL, Error: j.ErrorMsg, Enrichment: j.Enrichment, RoutingNote: j.RoutingNote,
		}
		if j.StartedAt != nil && j.CompletedAt != nil {
			st.DurationSeconds = j.CompletedAt.Sub(*j.StartedAt).Seconds()
		}
		r.SubTasks = append(r.SubTasks, st)
		r.Totals.Total++

		key := string(j.FileType) + "/" + string(j.Operation) + "/" + j.Target
		g, ok := groups[key]
		if !ok {
			g = &GroupCount{FileType: j.FileType, Operation: j.Operation, Target: j.Target}
			groups[key] = g
		}
		switch j.Status {
		case models.StatusCompleted:
			r.Totals.Completed++
			g.Completed++
		case models.StatusFailed:
			r.Totals.Failed++
			g.Failed++
		case models.StatusCancelled:
			r.Totals.Cancelled++
			g.Cancelled++
		}
	}

	r.ByTypeAndOperation = make([]GroupCount, 0, len(groups))
	for _, g := range groups {
		r.ByTypeAndOperation = append(r.ByTypeAndOperation, *g)
	}
	// Los grupos más numerosos primero (lee natural en el resumen); empate → por tipo y operación.
	sort.Slice(r.ByTypeAndOperation, func(i, k int) bool {
		a, b := r.ByTypeAndOperation[i], r.ByTypeAndOperation[k]
		if a.Completed != b.Completed {
			return a.Completed > b.Completed
		}
		if a.FileType != b.FileType {
			return a.FileType < b.FileType
		}
		if a.Operation != b.Operation {
			return a.Operation < b.Operation
		}
		return a.Target < b.Target
	})
	r.Summary = Summary(r)
	return r
}

// Summary produce la línea agregada, p. ej.
// "de 45 archivos — 30 videos convertidos, 10 audios extraídos, 4 miniaturas generadas, 1 fallido (formato no soportado)".
func Summary(r *Report) string {
	parts := make([]string, 0, len(r.ByTypeAndOperation)+3)
	for _, g := range r.ByTypeAndOperation {
		if g.Completed == 0 {
			continue
		}
		lbl, ok := opLabels[g.Operation]
		if !ok {
			lbl = [2]string{string(g.Operation), string(g.Operation)}
		}
		part := plural(g.Completed, lbl[0], lbl[1])
		if g.Target != "" && g.Operation != models.OpMetadata {
			part += " a " + strings.ToUpper(g.Target)
		}
		parts = append(parts, part)
	}
	if n := misleadingExtCount(r.SubTasks); n > 0 {
		parts = append(parts, plural(n, "archivo con extensión engañosa, enrutado por su contenido real",
			"archivos con extensión engañosa, enrutados por su contenido real"))
	}
	if r.Totals.Failed > 0 {
		reason := ""
		if reasons := failureReasons(r.SubTasks); len(reasons) > 0 {
			reason = " (" + strings.Join(reasons, "; ") + ")"
		}
		parts = append(parts, plural(r.Totals.Failed, "fallido", "fallidos")+reason)
	}
	if r.Totals.Cancelled > 0 {
		parts = append(parts, plural(r.Totals.Cancelled, "cancelado", "cancelados"))
	}
	return fmt.Sprintf("de %s — %s", plural(r.Totals.Total, "archivo", "archivos"), strings.Join(parts, ", "))
}

// misleadingExtCount cuenta cuántas sub-tareas se enrutaron por su contenido real porque la
// extensión mentía (ver sniff.go); el resumen agregado los reporta aparte.
func misleadingExtCount(subs []SubTaskResult) int {
	n := 0
	for _, s := range subs {
		if IsMisleadingExtNote(s.RoutingNote) {
			n++
		}
	}
	return n
}

func plural(n int, singular, pluralForm string) string {
	if n == 1 {
		return "1 " + singular
	}
	return fmt.Sprintf("%d %s", n, pluralForm)
}

// maxFailureReasons y maxReasonLen acotan el resumen: los detalles completos están en cada sub-tarea.
const (
	maxFailureReasons = 3
	maxReasonLen      = 90
)

// failureReasons junta los motivos distintos de las sub-tareas fallidas, en orden de aparición,
// con "N ×" cuando se repiten; si hay más de maxFailureReasons, el resto va como "…".
func failureReasons(subs []SubTaskResult) []string {
	var order []string
	count := map[string]int{}
	for _, s := range subs {
		if s.Status != models.StatusFailed || s.Error == "" {
			continue
		}
		r := firstLine(s.Error)
		if len([]rune(r)) > maxReasonLen {
			r = string([]rune(r)[:maxReasonLen-1]) + "…"
		}
		if count[r] == 0 {
			order = append(order, r)
		}
		count[r]++
	}
	out := make([]string, 0, maxFailureReasons+1)
	for i, r := range order {
		if i == maxFailureReasons {
			out = append(out, "…")
			break
		}
		if count[r] > 1 {
			r = fmt.Sprintf("%d × %s", count[r], r)
		}
		out = append(out, r)
	}
	return out
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return strings.TrimSpace(s)
}
