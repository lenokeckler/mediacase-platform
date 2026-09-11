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

// Handlers de casos (consigna §2): recibir el caso, inspeccionar cada archivo y decidir
// la operación (routing por tipo), registrarlo, descomponerlo en sub-tareas y encolarlas.

type caseFileReq struct {
	Key       string           `json:"key"`                 // clave del objeto en el bucket de entradas
	Operation models.Operation `json:"operation,omitempty"` // opcional: si falta, decide el coordinador
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
	if req.Priority == 0 {
		req.Priority = 5
	}

	// 1. Routing por tipo, de TODOS los archivos, antes de tocar la base: el caso se acepta
	//    entero o se rechaza entero, y la respuesta dice exactamente qué archivo no sirve.
	decisions := make([]cases.RouteDecision, len(req.Files))
	for i, f := range req.Files {
		if f.Key == "" {
			http.Error(w, "files["+itoa(i)+"]: falta key", http.StatusBadRequest)
			return
		}
		d, err := cases.Route(f.Key, f.Operation)
		if err != nil {
			http.Error(w, "archivo "+f.Key+": "+err.Error(), http.StatusBadRequest)
			return
		}
		decisions[i] = d
	}

	// 2. Registrar el caso.
	c := &models.Case{
		ID: uuid.New().String(), Name: req.Name, Status: models.CaseQueued,
		Priority: req.Priority, TotalJobs: len(req.Files), CreatedAt: time.Now(),
	}
	if err := db.InsertCase(a.db, c); err != nil {
		log.Printf("[cases] insert case: %v", err)
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}

	// 3. Descomponer en sub-tareas y encolar cada una en su pool.
	c.Jobs = make([]*models.Job, 0, len(req.Files))
	for i, f := range req.Files {
		job := &models.Job{
			ID: uuid.New().String(), CaseID: c.ID, FileID: f.Key, FilePath: f.Key,
			FileType: decisions[i].FileType, Operation: decisions[i].Operation, Pool: decisions[i].Pool,
			Priority: req.Priority, Status: models.StatusPending, MaxRetries: 3, CreatedAt: time.Now(),
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

// getCaseReport devuelve el reporte consolidado; 409 si el caso aún no cerró.
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

// cancelCase marca el caso cancelado y sus sub-tareas aún no iniciadas. Las que ya corren
// terminan, pero el barrier ignora casos terminales, así que el caso no vuelve a cambiar.
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
		a.onCaseClosed(id) // el reporte también se genera para un caso cancelado
	}
	w.WriteHeader(http.StatusOK)
}

// caseOf devuelve el case_id de un job ("" si es un job suelto).
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
