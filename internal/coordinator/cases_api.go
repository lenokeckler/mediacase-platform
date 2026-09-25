package coordinator

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/lenokeckler/mediacase-platform/internal/cases"
	"github.com/lenokeckler/mediacase-platform/internal/db"
	"github.com/lenokeckler/mediacase-platform/internal/models"
)

type caseFileReq struct {
	Key       string           `json:"key"`
	Operation models.Operation `json:"operation,omitempty"`
	Target    string           `json:"target,omitempty"`
	Width     int              `json:"width,omitempty"`

	Enrichment *models.Enrichment `json:"enrichment,omitempty"`
}

type submitCaseReq struct {
	Name     string        `json:"name"`
	Priority int           `json:"priority"`
	Files    []caseFileReq `json:"files"`
}

const maxFilesPerCase = 2000

func (a *API) submitCase(w http.ResponseWriter, r *http.Request) {
	var req submitCaseReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "body inválido", http.StatusBadRequest)
		return
	}
	if len(req.Files) == 0 {
		http.Error(w, "files[] no puede estar vacío", http.StatusBadRequest)
		return
	}
	if len(req.Files) > maxFilesPerCase {
		http.Error(w, "demasiados archivos en un caso", http.StatusBadRequest)
		return
	}

	if err := http.NewResponseController(w).SetWriteDeadline(time.Now().Add(5 * time.Minute)); err != nil {
		log.Printf("[cases] no se pudo extender el plazo de escritura: %v", err)
	}
	if req.Priority == 0 {
		req.Priority = 5
	}

	decisions := make([]cases.RouteDecision, len(req.Files))
	for i, f := range req.Files {
		if f.Key == "" {
			http.Error(w, "files["+itoa(i)+"]: falta key", http.StatusBadRequest)
			return
		}
		d, err := cases.RouteWith(f.Key, f.Operation, f.Target, f.Width)
		if err != nil {
			http.Error(w, "archivo "+f.Key+": "+err.Error(), http.StatusBadRequest)
			return
		}
		decisions[i] = d
	}

	routingNotes := inspectContent(r.Context(), a.minio, req.Files, decisions)

	c := &models.Case{
		ID: uuid.New().String(), Name: req.Name, Status: models.CaseQueued,
		Priority: req.Priority, TotalJobs: len(req.Files), CreatedAt: time.Now(),
	}
	if err := db.InsertCase(a.db, c); err != nil {
		log.Printf("[cases] insert case: %v", err)
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}

	c.Jobs = make([]*models.Job, 0, len(req.Files))
	for i, f := range req.Files {
		job := &models.Job{
			ID: uuid.New().String(), CaseID: c.ID, FileID: f.Key, FilePath: f.Key,
			FileType: decisions[i].FileType, Operation: decisions[i].Operation, Pool: decisions[i].Pool,
			Target: decisions[i].Target, Width: decisions[i].Width,
			Enrichment:  cases.DefaultEnrichment(decisions[i].Operation, req.Name, f.Key, f.Enrichment),
			RoutingNote: routingNotes[i],
			Priority:    req.Priority, Status: models.StatusPending, MaxRetries: 3, CreatedAt: time.Now(),
		}
		if err := db.InsertJob(a.db, job); err != nil {
			log.Printf("[cases] insert job %s (%s): %v", job.ID, f.Key, err)
			continue
		}
		if err := a.queue.Enqueue(r.Context(), job); err != nil {
			log.Printf("[cases] enqueue job %s (%s): %v", job.ID, f.Key, err)
		}
		c.Jobs = append(c.Jobs, job)
	}

	log.Printf("[cases] caso %s (%q): %d sub-tareas encoladas, prioridad %d", c.ID, c.Name, len(c.Jobs), c.Priority)
	writeJSON(w, http.StatusCreated, c)
}

func (a *API) listCases(w http.ResponseWriter, r *http.Request) {
	list, err := db.ListCases(a.db, r.URL.Query().Get("status"), 500)
	if err != nil {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (a *API) getCase(w http.ResponseWriter, r *http.Request) {
	c, err := db.GetCase(a.db, r.PathValue("id"))
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	c.Jobs, _ = db.ListJobsByCase(a.db, c.ID)
	writeJSON(w, http.StatusOK, c)
}

func (a *API) getCaseReport(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	raw, err := db.GetCaseReport(a.db, id)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if raw == nil {
		c, _ := db.GetCase(a.db, id)
		st := "desconocido"
		if c != nil {
			st = string(c.Status)
		}
		http.Error(w, "el caso aún no ha terminado (estado: "+st+")", http.StatusConflict)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(raw)
}

func (a *API) cancelCase(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	res, err := a.db.Exec(`UPDATE cases SET status='cancelled', completed_at=NOW()
		WHERE id=$1 AND status IN ('queued','processing','retrying')`, id)
	if err != nil {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		http.Error(w, "el caso no existe o ya es terminal", http.StatusConflict)
		return
	}
	a.db.Exec(`UPDATE jobs SET status='cancelled', completed_at=NOW()
		WHERE case_id=$1 AND status IN ('pending','assigned')`, id)
	log.Printf("[cases] caso %s cancelado", id)
	if a.onCaseClosed != nil {
		a.onCaseClosed(id)
	}
	w.WriteHeader(http.StatusOK)
}

func caseOf(database *sql.DB, jobID string) string {
	var caseID sql.NullString
	database.QueryRow(`SELECT case_id FROM jobs WHERE id=$1`, jobID).Scan(&caseID)
	return caseID.String
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func itoa(i int) string { return strconv.Itoa(i) }
