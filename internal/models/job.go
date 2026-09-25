package models

import "time"

type JobStatus string

const (
	StatusPending   JobStatus = "pending"
	StatusAssigned  JobStatus = "assigned"
	StatusRunning   JobStatus = "running"
	StatusCompleted JobStatus = "completed"
	StatusFailed    JobStatus = "failed"
	StatusCancelled JobStatus = "cancelled"
)

type Operation string

const (
	OpConvert      Operation = "convert"
	OpExtractAudio Operation = "extract_audio"
	OpThumbnail    Operation = "thumbnail"
	OpConvertAudio Operation = "convert_audio"
	OpMetadata     Operation = "metadata"

	OpEnrichAudio Operation = "enrich_audio"
	OpEnrichVideo Operation = "enrich_video"
)

type Enrichment struct {
	Title   string `json:"title,omitempty"`
	Artist  string `json:"artist,omitempty"`
	Album   string `json:"album,omitempty"`
	Date    string `json:"date,omitempty"`
	Comment string `json:"comment,omitempty"`
	Lyrics  string `json:"lyrics,omitempty"`
}

func (e *Enrichment) IsEmpty() bool {
	return e == nil || (e.Title == "" && e.Artist == "" && e.Album == "" && e.Date == "" && e.Comment == "" && e.Lyrics == "")
}

type Job struct {
	ID         string      `json:"id"`
	CaseID     string      `json:"case_id,omitempty"`
	FileID     string      `json:"file_id"`
	FilePath   string      `json:"file_path"`
	FileType   FileType    `json:"file_type"`
	Pool       string      `json:"pool"`
	Operation  Operation   `json:"operation"`
	Target     string      `json:"target,omitempty"`
	Assignment string      `json:"assignment,omitempty"`
	Width      int         `json:"width,omitempty"`
	Enrichment *Enrichment `json:"enrichment,omitempty"`

	RoutingNote string     `json:"routing_note,omitempty"`
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
	Instance     string    `json:"instance,omitempty"`
	Hostname     string    `json:"hostname"`
	Role         string    `json:"role,omitempty"`
	Capabilities []string  `json:"capabilities,omitempty"`
	Capacity     int       `json:"capacity,omitempty"`
	Status       string    `json:"status"`
	ActiveJobs   int       `json:"active_jobs"`
	CPUPercent   float64   `json:"cpu_percent"`
	MemPercent   float64   `json:"mem_percent"`
	LastSeen     time.Time `json:"last_seen"`
	RegisteredAt time.Time `json:"registered_at"`

	Hardware *Hardware    `json:"hardware,omitempty"`
	Metrics  *NodeMetrics `json:"metrics,omitempty"`
}

type Hardware struct {
	OS            string    `json:"os"`
	Arch          string    `json:"arch"`
	CPUModel      string    `json:"cpu_model"`
	CPUCores      int       `json:"cpu_cores"`
	CPUThreads    int       `json:"cpu_threads"`
	MemTotalBytes uint64    `json:"mem_total_bytes"`
	GPUs          []GPUInfo `json:"gpus"`
}

type GPUInfo struct {
	Index          int    `json:"index"`
	Name           string `json:"name"`
	Vendor         string `json:"vendor"`
	Integrated     bool   `json:"integrated"`
	VRAMTotalBytes uint64 `json:"vram_total_bytes,omitempty"`
	Source         string `json:"source"`
}

type NodeMetrics struct {
	SampledAt     time.Time    `json:"sampled_at"`
	CPUPercent    float64      `json:"cpu_percent"`
	MemUsedBytes  uint64       `json:"mem_used_bytes"`
	MemTotalBytes uint64       `json:"mem_total_bytes"`
	MemPercent    float64      `json:"mem_percent"`
	DiskPercent   *float64     `json:"disk_percent,omitempty"`
	GPUs          []GPUMetrics `json:"gpus"`
}

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
