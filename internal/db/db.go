package db

import (
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/lenokeckler/mediacase-platform/internal/models"
	_ "github.com/lib/pq"
)

func Connect(dsn string) (*sql.DB, error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("ping db: %w", err)
	}
	return db, nil
}

func Migrate(db *sql.DB) error {
	_, err := db.Exec(`
	CREATE TABLE IF NOT EXISTS jobs (
		id            TEXT PRIMARY KEY,
		file_id       TEXT NOT NULL,
		file_path     TEXT NOT NULL,
		operation     TEXT NOT NULL,
		output_path   TEXT,
		status        TEXT NOT NULL DEFAULT 'pending',
		priority      INT  NOT NULL DEFAULT 5,
		worker_id     TEXT,
		progress      INT  NOT NULL DEFAULT 0,
		error_msg     TEXT,
		result_url    TEXT,
		retries       INT  NOT NULL DEFAULT 0,
		max_retries   INT  NOT NULL DEFAULT 3,
		created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		started_at    TIMESTAMPTZ,
		completed_at  TIMESTAMPTZ
	);

	CREATE TABLE IF NOT EXISTS workers (
		id           TEXT PRIMARY KEY,
		hostname     TEXT NOT NULL,
		status       TEXT NOT NULL DEFAULT 'idle',
		active_jobs  INT  NOT NULL DEFAULT 0,
		cpu_percent  FLOAT,
		mem_percent  FLOAT,
		last_seen    TIMESTAMPTZ NOT NULL DEFAULT NOW()
	);

	CREATE TABLE IF NOT EXISTS worker_registry (
		id          TEXT PRIMARY KEY,
		hostname    TEXT NOT NULL,
		status      TEXT NOT NULL DEFAULT 'idle',
		last_seen   TIMESTAMPTZ NOT NULL DEFAULT NOW()
	);

	CREATE INDEX IF NOT EXISTS idx_jobs_status   ON jobs(status);
	CREATE INDEX IF NOT EXISTS idx_jobs_worker   ON jobs(worker_id);
	CREATE INDEX IF NOT EXISTS idx_jobs_priority ON jobs(priority DESC);

	-- Casos (consigna v2.0): la unidad de trabajo. Las sub-tareas cuelgan de jobs.case_id.
	CREATE TABLE IF NOT EXISTS cases (
		id           TEXT PRIMARY KEY,
		name         TEXT NOT NULL DEFAULT '',
		status       TEXT NOT NULL DEFAULT 'queued',
		priority     INT  NOT NULL DEFAULT 5,
		total_jobs   INT  NOT NULL DEFAULT 0,
		report       JSONB,
		created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		started_at   TIMESTAMPTZ,
		completed_at TIMESTAMPTZ
	);
	ALTER TABLE jobs ADD COLUMN IF NOT EXISTS case_id   TEXT REFERENCES cases(id);
	ALTER TABLE jobs ADD COLUMN IF NOT EXISTS file_type TEXT NOT NULL DEFAULT '';
	ALTER TABLE jobs ADD COLUMN IF NOT EXISTS pool      TEXT NOT NULL DEFAULT '';
	ALTER TABLE jobs ADD COLUMN IF NOT EXISTS assigned_at TIMESTAMPTZ;
	CREATE INDEX IF NOT EXISTS idx_jobs_case    ON jobs(case_id);
	CREATE INDEX IF NOT EXISTS idx_cases_status ON cases(status);

	-- Pools especializados: qué atiende cada worker (sobrevive reinicios del coordinador)
	ALTER TABLE worker_registry ADD COLUMN IF NOT EXISTS role         TEXT NOT NULL DEFAULT '';
	ALTER TABLE worker_registry ADD COLUMN IF NOT EXISTS capabilities TEXT NOT NULL DEFAULT '';
	ALTER TABLE jobs ADD COLUMN IF NOT EXISTS target TEXT NOT NULL DEFAULT '';
	ALTER TABLE jobs ADD COLUMN IF NOT EXISTS width  INT  NOT NULL DEFAULT 0;
	ALTER TABLE jobs ADD COLUMN IF NOT EXISTS assignment TEXT NOT NULL DEFAULT '';
	-- Recursos asociados de las sub-tareas enrich_* (título, artista, álbum, letra…)
	ALTER TABLE jobs ADD COLUMN IF NOT EXISTS enrichment JSONB;
	ALTER TABLE worker_registry ADD COLUMN IF NOT EXISTS hardware     JSONB;
	ALTER TABLE worker_registry ADD COLUMN IF NOT EXISTS registered_at TIMESTAMPTZ NOT NULL DEFAULT NOW();
	`)
	return err
}

// jobColumns es la lista de columnas que scanJob espera, en ese orden.
const jobColumns = `id, file_path, operation, status, priority,
		worker_id, progress, error_msg, result_url, retries, max_retries,
		created_at, started_at, completed_at, case_id, file_type, pool, target, width, assignment, enrichment`

// InsertJob stores a new job in PostgreSQL.
func InsertJob(db *sql.DB, job *models.Job) error {
	_, err := db.Exec(`
		INSERT INTO jobs (id, file_id, file_path, operation, status, priority, max_retries,
		                  created_at, case_id, file_type, pool, target, width, enrichment)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NULLIF($9, ''), $10, $11, $12, $13, $14)`,
		job.ID, job.FileID, job.FilePath, job.Operation,
		job.Status, job.Priority, job.MaxRetries, job.CreatedAt,
		job.CaseID, job.FileType, job.Pool, job.Target, job.Width, enrichmentJSON(job.Enrichment),
	)
	return err
}

// enrichmentJSON serializa los recursos asociados para la columna JSONB (NULL si no hay).
func enrichmentJSON(e *models.Enrichment) any {
	if e == nil {
		return nil
	}
	b, err := json.Marshal(e)
	if err != nil {
		return nil
	}
	return string(b)
}

// GetJob returns a job by ID.
func GetJob(db *sql.DB, id string) (*models.Job, error) {
	row := db.QueryRow(`SELECT `+jobColumns+` FROM jobs WHERE id=$1`, id)
	return scanJob(row)
}

// ListJobs returns all jobs, optionally filtered by status.
func ListJobs(db *sql.DB, status string) ([]*models.Job, error) {
	query := `SELECT ` + jobColumns + ` FROM jobs`
	args := []any{}
	if status != "" {
		query += " WHERE status=$1"
		args = append(args, status)
	}
	query += " ORDER BY created_at DESC LIMIT 2000"
	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	jobs := make([]*models.Job, 0)
	for rows.Next() {
		j, err := scanJob(rows)
		if err == nil {
			jobs = append(jobs, j)
		}
	}
	return jobs, nil
}

// ListLiveJobs devuelve solo las sub-tareas no terminales (pending, assigned, running): es lo
// que el dashboard dibuja en vivo. Mandar las 2000 más recientes cada segundo por WebSocket
// pesaba ~1 MB por cliente con el dataset real; el historial se consulta aparte (GET /jobs).
func ListLiveJobs(db *sql.DB) ([]*models.Job, error) {
	rows, err := db.Query(`SELECT ` + jobColumns + ` FROM jobs
		WHERE status IN ('pending', 'assigned', 'running') ORDER BY created_at DESC LIMIT 2000`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	jobs := make([]*models.Job, 0)
	for rows.Next() {
		if j, err := scanJob(rows); err == nil {
			jobs = append(jobs, j)
		}
	}
	return jobs, nil
}

// GetStats returns counts by status for the dashboard.
func GetStats(db *sql.DB) (map[string]int, error) {
	rows, err := db.Query(`
		SELECT status, COUNT(*) FROM jobs GROUP BY status`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	stats := map[string]int{}
	for rows.Next() {
		var status string
		var count int
		rows.Scan(&status, &count)
		stats[status] = count
	}
	return stats, nil
}

// scanJob is a helper to avoid repeating Scan.
func scanJob(row interface {
	Scan(...any) error
}) (*models.Job, error) {
	j := &models.Job{}
	var workerID, errorMsg, resultURL, caseID sql.NullString
	var enrichment []byte
	err := row.Scan(
		&j.ID, &j.FilePath, &j.Operation, &j.Status, &j.Priority,
		&workerID, &j.Progress, &errorMsg, &resultURL,
		&j.Retries, &j.MaxRetries, &j.CreatedAt, &j.StartedAt, &j.CompletedAt,
		&caseID, &j.FileType, &j.Pool, &j.Target, &j.Width, &j.Assignment, &enrichment,
	)
	if err != nil {
		return nil, err
	}
	if len(enrichment) > 0 {
		var e models.Enrichment
		if json.Unmarshal(enrichment, &e) == nil {
			j.Enrichment = &e
		}
	}
	j.WorkerID = workerID.String
	j.ErrorMsg = errorMsg.String
	j.ResultURL = resultURL.String
	j.CaseID = caseID.String
	return j, nil
}
