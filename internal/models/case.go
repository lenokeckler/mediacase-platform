package models

import "time"

type CaseStatus string

const (
	CaseQueued             CaseStatus = "queued"
	CaseProcessing         CaseStatus = "processing"
	CaseCompleted          CaseStatus = "completed"
	CasePartiallyCompleted CaseStatus = "partially_completed"
	CaseFailed             CaseStatus = "failed"
	CaseRetrying           CaseStatus = "retrying"
	CaseCancelled          CaseStatus = "cancelled"
)

func (s CaseStatus) IsTerminal() bool {
	switch s {
	case CaseCompleted, CasePartiallyCompleted, CaseFailed, CaseCancelled:
		return true
	}
	return false
}

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
	Jobs        []*Job     `json:"jobs,omitempty"`
}
