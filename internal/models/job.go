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
)

type Job struct {
	ID          string     `json:"id"`
	CaseID      string     `json:"case_id,omitempty"` // caso al que pertenece ("" = job suelto)
	FileID      string     `json:"file_id"`
	FilePath    string     `json:"file_path"` // clave del objeto en el bucket de entradas
	FileType    FileType   `json:"file_type"` // decidido por el coordinador (routing por tipo)
	Pool        string     `json:"pool"`      // pool de workers que la ejecuta
	Operation   Operation  `json:"operation"`
	OutputPath  string     `json:"output_path"`
	Status      JobStatus  `json:"status"`
	Priority    int        `json:"priority"`
	WorkerID    string     `json:"worker_id"`
	Progress    int        `json:"progress"`
	ErrorMsg    string     `json:"error_msg,omitempty"`
	ResultURL   string     `json:"result_url,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	StartedAt   *time.Time `json:"started_at,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	Retries     int        `json:"retries"`
	MaxRetries  int        `json:"max_retries"`
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
}

type JobEvent struct {
	JobID    string    `json:"job_id"`
	Status   JobStatus `json:"status"`
	Progress int       `json:"progress"`
	WorkerID string    `json:"worker_id"`
	Message  string    `json:"message,omitempty"`
}
