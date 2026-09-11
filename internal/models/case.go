package models

import "time"

// Un caso es la unidad de trabajo del sistema (consigna v2.0): un conjunto de archivos
// relacionados que entra como UNA solicitud, se descompone en sub-tareas (Job) repartidas
// entre los workers, y se cierra con un barrier cuando todas resolvieron.

type CaseStatus string

const (
	CaseQueued             CaseStatus = "queued"              // registrado, sub-tareas encoladas
	CaseProcessing         CaseStatus = "processing"          // al menos una sub-tarea corriendo
	CaseCompleted          CaseStatus = "completed"           // todas las sub-tareas OK
	CasePartiallyCompleted CaseStatus = "partially_completed" // terminó con ≥1 sub-tarea fallida
	CaseFailed             CaseStatus = "failed"              // todas las sub-tareas fallaron
	CaseRetrying           CaseStatus = "retrying"            // sub-tareas re-encoladas tras caída de un worker
	CaseCancelled          CaseStatus = "cancelled"           // abortado por el cliente
)

// IsTerminal indica si el caso ya no cambia de estado.
func (s CaseStatus) IsTerminal() bool {
	switch s {
	case CaseCompleted, CasePartiallyCompleted, CaseFailed, CaseCancelled:
		return true
	}
	return false
}

// FileType es el tipo de contenido que el coordinador detecta al inspeccionar cada archivo.
type FileType string

const (
	FileVideo FileType = "video"
	FileAudio FileType = "audio"
	FileImage FileType = "image"
)

type Case struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Status      CaseStatus `json:"status"`
	Priority    int        `json:"priority"`
	TotalJobs   int        `json:"total_jobs"`
	CreatedAt   time.Time  `json:"created_at"`
	StartedAt   *time.Time `json:"started_at,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	Jobs        []*Job     `json:"jobs,omitempty"` // sub-tareas, cuando se piden con detalle
}
