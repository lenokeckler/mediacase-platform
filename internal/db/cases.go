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
