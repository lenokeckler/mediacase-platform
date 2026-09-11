package db

import (
	"database/sql"
	"strconv"

	"github.com/lenokeckler/mediacase-platform/internal/models"
)

const caseColumns = `id, name, status, priority, total_jobs, created_at, started_at, completed_at`

func InsertCase(db *sql.DB, c *models.Case) error {
	_, err := db.Exec(`INSERT INTO cases (id, name, status, priority, total_jobs, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)`, c.ID, c.Name, c.Status, c.Priority, c.TotalJobs, c.CreatedAt)
	return err
}

func scanCase(row interface{ Scan(...any) error }) (*models.Case, error) {
	c := &models.Case{}
	if err := row.Scan(&c.ID, &c.Name, &c.Status, &c.Priority, &c.TotalJobs,
		&c.CreatedAt, &c.StartedAt, &c.CompletedAt); err != nil {
		return nil, err
	}
	return c, nil
}

// GetCase devuelve el caso sin sus sub-tareas (ver ListJobsByCase).
func GetCase(db *sql.DB, id string) (*models.Case, error) {
	return scanCase(db.QueryRow(`SELECT `+caseColumns+` FROM cases WHERE id=$1`, id))
}

// ListCases devuelve los casos más recientes, opcionalmente filtrados por estado.
func ListCases(db *sql.DB, status string, limit int) ([]*models.Case, error) {
	q := `SELECT ` + caseColumns + ` FROM cases`
	args := []any{}
	if status != "" {
		q += ` WHERE status=$1`
		args = append(args, status)
	}
	q += ` ORDER BY created_at DESC LIMIT ` + strconv.Itoa(limit)
	rows, err := db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]*models.Case, 0)
	for rows.Next() {
		if c, err := scanCase(rows); err == nil {
			out = append(out, c)
		}
	}
	return out, nil
}

// ListJobsByCase devuelve las sub-tareas de un caso en orden de creación.
func ListJobsByCase(db *sql.DB, caseID string) ([]*models.Job, error) {
	rows, err := db.Query(`SELECT `+jobColumns+` FROM jobs WHERE case_id=$1 ORDER BY created_at, id`, caseID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]*models.Job, 0)
	for rows.Next() {
		if j, err := scanJob(rows); err == nil {
			out = append(out, j)
		}
	}
	return out, nil
}

func SetCaseStatus(db *sql.DB, id string, st models.CaseStatus) error {
	_, err := db.Exec(`UPDATE cases SET status=$1 WHERE id=$2`, st, id)
	return err
}

// SaveCaseReport guarda el reporte consolidado (JSON) del caso.
func SaveCaseReport(db *sql.DB, id string, report []byte) error {
	_, err := db.Exec(`UPDATE cases SET report=$1 WHERE id=$2`, report, id)
	return err
}

// GetCaseReport devuelve el reporte guardado; nil (sin error) si el caso aún no cerró.
func GetCaseReport(db *sql.DB, id string) ([]byte, error) {
	var raw []byte
	err := db.QueryRow(`SELECT report FROM cases WHERE id=$1`, id).Scan(&raw)
	return raw, err
}

// CaseCounts resume las sub-tareas de un caso por estado.
type CaseCounts struct{ Total, Completed, Failed, Running, Pending, Cancelled int }

// Resolved es cuántas sub-tareas ya no van a cambiar (completadas o fallidas).
func (c CaseCounts) Resolved() int { return c.Completed + c.Failed }

// CountJobsByCase cuenta dentro de una transacción: lo usa el barrier con la fila del caso bloqueada.
func CountJobsByCase(tx *sql.Tx, caseID string) (CaseCounts, error) {
	var c CaseCounts
	err := tx.QueryRow(`
		SELECT COUNT(*),
		       COUNT(*) FILTER (WHERE status = 'completed'),
		       COUNT(*) FILTER (WHERE status = 'failed'),
		       COUNT(*) FILTER (WHERE status = 'running'),
		       COUNT(*) FILTER (WHERE status IN ('pending', 'assigned')),
		       COUNT(*) FILTER (WHERE status = 'cancelled')
		FROM jobs WHERE case_id = $1`, caseID).
		Scan(&c.Total, &c.Completed, &c.Failed, &c.Running, &c.Pending, &c.Cancelled)
	return c, err
}

// ── Agregados para el monitoreo (Fase 5) ────────────────────────────────────

// CountCasesByStatus devuelve cuántos casos hay en cada estado.
func CountCasesByStatus(db *sql.DB) (map[string]int, error) {
	rows, err := db.Query(`SELECT status, COUNT(*) FROM cases GROUP BY status`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var st string
		var n int
		if rows.Scan(&st, &n) == nil {
			out[st] = n
		}
	}
	return out, rows.Err()
}

// JobsByStatusPool es una celda de la matriz estado × pool.
type JobsByStatusPool struct {
	Status string
	Pool   string
	Count  int
}

// CountJobsByStatusPool devuelve cuántas sub-tareas hay por estado y pool.
func CountJobsByStatusPool(db *sql.DB) ([]JobsByStatusPool, error) {
	rows, err := db.Query(`SELECT status, COALESCE(pool, ''), COUNT(*) FROM jobs GROUP BY 1, 2`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []JobsByStatusPool
	for rows.Next() {
		var c JobsByStatusPool
		if rows.Scan(&c.Status, &c.Pool, &c.Count) == nil {
			out = append(out, c)
		}
	}
	return out, rows.Err()
}

// ActiveCaseSummary es un caso abierto con sus sub-tareas agrupadas por estado: lo que la
// consigna pide ver en el monitoreo ("sub-tareas activas o en espera, agrupadas por caso").
type ActiveCaseSummary struct {
	CaseID    string `json:"case_id"`
	Name      string `json:"name"`
	Status    string `json:"status"`
	Priority  int    `json:"priority"`
	Total     int    `json:"total"`
	Running   int    `json:"running"`
	Pending   int    `json:"pending"`
	Completed int    `json:"completed"`
	Failed    int    `json:"failed"`
}

// ListActiveCases devuelve los casos no terminales, del más antiguo al más nuevo, con el
// conteo de sub-tareas por estado.
func ListActiveCases(db *sql.DB) ([]ActiveCaseSummary, error) {
	rows, err := db.Query(`
		SELECT c.id, c.name, c.status, c.priority, c.total_jobs,
		       COUNT(j.id) FILTER (WHERE j.status = 'running'),
		       COUNT(j.id) FILTER (WHERE j.status IN ('pending', 'assigned')),
		       COUNT(j.id) FILTER (WHERE j.status = 'completed'),
		       COUNT(j.id) FILTER (WHERE j.status = 'failed')
		FROM cases c LEFT JOIN jobs j ON j.case_id = c.id
		WHERE c.status IN ('queued', 'processing', 'retrying')
		GROUP BY c.id ORDER BY c.created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ActiveCaseSummary{}
	for rows.Next() {
		var s ActiveCaseSummary
		if rows.Scan(&s.CaseID, &s.Name, &s.Status, &s.Priority, &s.Total,
			&s.Running, &s.Pending, &s.Completed, &s.Failed) == nil {
			out = append(out, s)
		}
	}
	return out, rows.Err()
}
