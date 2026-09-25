package cases

import (
	"context"
	"database/sql"
	"fmt"
	"log"

	"github.com/lenokeckler/mediacase-platform/internal/db"
	"github.com/lenokeckler/mediacase-platform/internal/models"
)

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

type Barrier struct {
	db      *sql.DB
	onClose func(caseID string)
}

func NewBarrier(database *sql.DB, onClose func(caseID string)) *Barrier {
	return &Barrier{db: database, onClose: onClose}
}

func (b *Barrier) SetOnClose(fn func(caseID string)) { b.onClose = fn }

func (b *Barrier) OnJobResolved(ctx context.Context, caseID string) error {
	if caseID == "" {
		return nil
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
		return nil
	}

	counts, err := db.CountJobsByCase(tx, caseID)
	if err != nil {
		return err
	}
	final, closed := ComputeStatus(total, counts.Completed, counts.Failed)
	if !closed {
		return tx.Commit()
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
