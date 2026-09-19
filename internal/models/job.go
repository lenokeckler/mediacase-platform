package models

import "time"

type JobStatus string

const (
	StatusPending   JobStatus = "pending"
	StatusAssigned  JobStatus = "assigned"
	StatusRunning   JobStatus = "running"
	StatusCompleted JobStatus = "completed"
	StatusFailed    JobStatus = "failed"
	StatusCancelled JobStatus = "cancelled" // el caso se canceló antes de que empezara
)

type Operation string

const (
	OpConvert      Operation = "convert"
	OpExtractAudio Operation = "extract_audio"
	OpThumbnail    Operation = "thumbnail"
	OpConvertAudio Operation = "convert_audio"
	OpMetadata     Operation = "metadata" // ffprobe → JSON con duración, códecs, resolución, etiquetas
	// "Integración de letras o recursos informativos asociados" (consigna): el mismo archivo con
	// portada, etiquetas y letra/descripción embebidas. Remux liviano, sin recodificar.
	OpEnrichAudio Operation = "enrich_audio"
	OpEnrichVideo Operation = "enrich_video"
)

// Enrichment son los recursos asociados que se integran en una sub-tarea enrich_*. Los llena el
// cliente (formulario o ingest desde el manifest) y el coordinador completa los defaults.
type Enrichment struct {
	Title   string `json:"title,omitempty"`
	Artist  string `json:"artist,omitempty"`  // artista / autor
	Album   string `json:"album,omitempty"`   // álbum / evento
	Date    string `json:"date,omitempty"`    // año o fecha
	Comment string `json:"comment,omitempty"` // sesión, lote, nota
	Lyrics  string `json:"lyrics,omitempty"`  // letra (audio) o descripción (video)
}

// IsEmpty dice si no hay ningún recurso que integrar además de la portada.
func (e *Enrichment) IsEmpty() bool {
	return e == nil || (e.Title == "" && e.Artist == "" && e.Album == "" && e.Date == "" && e.Comment == "" && e.Lyrics == "")
}

type Job struct {
	ID          string      `json:"id"`
	CaseID      string      `json:"case_id,omitempty"` // caso al que pertenece ("" = job suelto)
	FileID      string      `json:"file_id"`
	FilePath    string      `json:"file_path"` // clave del objeto en el bucket de entradas
	FileType    FileType    `json:"file_type"` // decidido por el coordinador (routing por tipo)
	Pool        string      `json:"pool"`      // pool de workers que la ejecuta
	Operation   Operation   `json:"operation"`
	Target      string      `json:"target,omitempty"`     // formato de salida: mp4, mp3, flac, jpg, json…
	Assignment  string      `json:"assignment,omitempty"` // afinidad | ayuda: cómo el planificador eligió el worker
	Width       int         `json:"width,omitempty"`      // ancho de la miniatura (solo thumbnail)
	Enrichment  *Enrichment `json:"enrichment,omitempty"` // recursos asociados (solo enrich_*)
	OutputPath  string      `json:"output_path"`
	Status      JobStatus   `json:"status"`
	Priority    int         `json:"priority"`
	WorkerID    string      `json:"worker_id"`
	Progress    int         `json:"progress"`
	ErrorMsg    string      `json:"error_msg,omitempty"`
	ResultURL   string      `json:"result_url,omitempty"`
	CreatedAt   time.Time   `json:"created_at"`
	StartedAt   *time.Time  `json:"started_at,omitempty"`
	CompletedAt *time.Time  `json:"completed_at,omitempty"`
	Retries     int         `json:"retries"`
	MaxRetries  int         `json:"max_retries"`
}

type WorkerInfo struct {
	ID           string    `json:"id"`
	Instance     string    `json:"instance,omitempty"` // identifica al PROCESO: cambia en cada arranque
	Hostname     string    `json:"hostname"`
	Role         string    `json:"role,omitempty"`         // video | audio | metadata | all
	Capabilities []string  `json:"capabilities,omitempty"` // pools que este worker atiende
	Status       string    `json:"status"`
	ActiveJobs   int       `json:"active_jobs"`
	CPUPercent   float64   `json:"cpu_percent"`
	MemPercent   float64   `json:"mem_percent"`
	LastSeen     time.Time `json:"last_seen"`
	RegisteredAt time.Time `json:"registered_at"` // primera vez que llegó: fija el orden en el dashboard

	// Telemetría de hardware al estilo del Administrador de tareas. Hardware llega al
	// registrarse; Metrics en cada heartbeat. nil = el worker no lo manda o no pudo medirlo.
	Hardware *Hardware    `json:"hardware,omitempty"`
	Metrics  *NodeMetrics `json:"metrics,omitempty"`
}

// Hardware es la parte fija de un nodo: CPU, RAM total y sus GPUs.
type Hardware struct {
	OS            string    `json:"os"`
	Arch          string    `json:"arch"`
	CPUModel      string    `json:"cpu_model"`
	CPUCores      int       `json:"cpu_cores"`
	CPUThreads    int       `json:"cpu_threads"`
	MemTotalBytes uint64    `json:"mem_total_bytes"`
	GPUs          []GPUInfo `json:"gpus"`
}

// GPUInfo describe una GPU; Index sigue el orden del sistema (GPU 0, GPU 1...).
type GPUInfo struct {
	Index          int    `json:"index"`
	Name           string `json:"name"`
	Vendor         string `json:"vendor"` // nvidia | amd | intel | other
	Integrated     bool   `json:"integrated"`
	VRAMTotalBytes uint64 `json:"vram_total_bytes,omitempty"`
	Source         string `json:"source"` // nvidia-smi | directx+pdh | sysfs
}

// NodeMetrics es la parte variable, muestreada cada segundo en el worker.
type NodeMetrics struct {
	SampledAt     time.Time    `json:"sampled_at"`
	CPUPercent    float64      `json:"cpu_percent"`
	MemUsedBytes  uint64       `json:"mem_used_bytes"`
	MemTotalBytes uint64       `json:"mem_total_bytes"`
	MemPercent    float64      `json:"mem_percent"`
	DiskPercent   *float64     `json:"disk_percent,omitempty"`
	GPUs          []GPUMetrics `json:"gpus"`
}

// GPUMetrics son las lecturas de una GPU; nil = no disponible en esa máquina.
type GPUMetrics struct {
	Index         int      `json:"index"`
	Percent       *float64 `json:"percent"`
	VRAMUsedBytes *uint64  `json:"vram_used_bytes,omitempty"`
	TempC         *float64 `json:"temp_c,omitempty"`
}

type JobEvent struct {
	JobID    string    `json:"job_id"`
	Status   JobStatus `json:"status"`
	Progress int       `json:"progress"`
	WorkerID string    `json:"worker_id"`
	Message  string    `json:"message,omitempty"`
}
