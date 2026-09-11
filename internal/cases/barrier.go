package cases

import (
	"context"
	"database/sql"
	"fmt"
	"log"

	"github.com/lenokeckler/mediacase-platform/internal/db"
	"github.com/lenokeckler/mediacase-platform/internal/models"
)

// Barrier/join (consigna §6): el coordinador determina el estado agregado de un caso
// ÚNICAMENTE cuando dispone del resultado de todas sus sub-tareas.
//
//   completed            ⇔ todas las sub-tareas finalizaron con éxito
//   partially_completed  ⇔ finalizó con al menos una fallida
//   failed               ⇔ todas fallaron
//
// Mientras falte alguna, el caso sigue abierto aunque las demás ya estén listas.

// ComputeStatus es la regla pura del barrier. closed=false mientras falte alguna sub-tarea.
func ComputeStatus(total, completed, failed int) (status models.CaseStatus, closed bool) {
	if completed+failed < total {
		return "", false
	}
	switch {
	case failed == 0:
		return models.CaseCompleted, true
	case completed == 0:
		return models.CaseFailed, true
	default:
		return models.CasePartiallyCompleted, true
	}
}

// Barrier aplica la regla sobre la base de datos y cierra cada caso exactamente una vez.
type Barrier struct {
	db      *sql.DB
	onClose func(caseID string) // p. ej. generar el reporte consolidado
}

func NewBarrier(database *sql.DB, onClose func(caseID string)) *Barrier {
	return &Barrier{db: database, onClose: onClose}
}

// SetOnClose registra qué hacer cuando un caso cierra (se llama fuera de la transacción).
func (b *Barrier) SetOnClose(fn func(caseID string)) { b.onClose = fn }

// OnJobResolved se llama cada vez que una sub-tarea llega a completed o failed.
// Bloquea la fila del caso (SELECT ... FOR UPDATE) para que dos sub-tareas que terminan
// al mismo tiempo se serialicen y el caso se cierre una sola vez.
func (b *Barrier) OnJobResolved(ctx context.Context, caseID string) error {
	if caseID == "" {
		return nil // job suelto (POST /jobs): no pertenece a ningún caso
	}
	tx, err := b.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var status models.CaseStatus
	var total int
	if err := tx.QueryRow(`SELECT status, total_jobs FROM cases WHERE id=$1 FOR UPDATE`, caseID).
		Scan(&status, &total); err != nil {
		return fmt.Errorf("lock case %s: %w", caseID, err)
	}
	if status.IsTerminal() {
		return nil // ya cerrado o cancelado: nada que hacer
	}

	counts, err := db.CountJobsByCase(tx, caseID)
	if err != nil {
		return err
	}
	final, closed := ComputeStatus(total, counts.Completed, counts.Failed)
	if !closed {
		return tx.Commit() // el barrier sigue esperando a las demás
	}

	if _, err := tx.Exec(`UPDATE cases SET status=$1, completed_at=NOW() WHERE id=$2`, final, caseID); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	log.Printf("[barrier] caso %s cerrado: %s (%d ok, %d fallidas de %d)",
		caseID, final, counts.Completed, counts.Failed, total)
	if b.onClose != nil {
		b.onClose(caseID)
	}
	return nil
}
