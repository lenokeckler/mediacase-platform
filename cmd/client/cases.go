package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

type caseFile struct {
	Key       string `json:"key"`
	Operation string `json:"operation,omitempty"`
}

type caseRequest struct {
	Name     string     `json:"name"`
	Priority int        `json:"priority"`
	Files    []caseFile `json:"files"`
}

type caseJob struct {
	FilePath  string `json:"file_path"`
	FileType  string `json:"file_type"`
	Operation string `json:"operation"`
	Pool      string `json:"pool"`
	Status    string `json:"status"`
	Progress  int    `json:"progress"`
	WorkerID  string `json:"worker_id"`
}

type caseResponse struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Status    string    `json:"status"`
	TotalJobs int       `json:"total_jobs"`
	Jobs      []caseJob `json:"jobs"`
}

func parseFiles(spec string) []caseFile {
	var out []caseFile
	for _, item := range strings.Split(spec, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		f := caseFile{Key: item}
		if i := strings.LastIndex(item, ":"); i > 0 {
			f.Key, f.Operation = item[:i], item[i+1:]
		}
		out = append(out, f)
	}
	return out
}

func isCaseTerminal(st string) bool {
	switch st {
	case "completed", "partially_completed", "failed", "cancelled":
		return true
	}
	return false
}

func runCase(coordinatorURL, name, files string, priority int, watch bool) {
	req := caseRequest{Name: name, Priority: priority, Files: parseFiles(files)}
	if len(req.Files) == 0 {
		log.Fatal("-files vacío: indicar claves separadas por coma, p. ej. -files a.mp4,b.mp3")
	}
	body, _ := json.Marshal(req)
	resp, err := http.Post(coordinatorURL+"/cases", "application/json", bytes.NewReader(body))
	if err != nil {
		log.Fatalf("POST /cases: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		msg, _ := io.ReadAll(resp.Body)
		log.Fatalf("POST /cases → %d: %s", resp.StatusCode, strings.TrimSpace(string(msg)))
	}
	var c caseResponse
	json.NewDecoder(resp.Body).Decode(&c)

	fmt.Printf("caso %s (%q): %d sub-tareas, estado %s\n", c.ID, c.Name, c.TotalJobs, c.Status)
	for _, j := range c.Jobs {
		fmt.Printf("   %-24s %-6s → %-14s pool=%s\n", j.FilePath, j.FileType, j.Operation, j.Pool)
	}
	if !watch {
		fmt.Printf("\nseguir con:  client -case-status %s\n", c.ID)
		return
	}
	watchCase(coordinatorURL, c.ID)
}

func watchCase(coordinatorURL, id string) {
	start := time.Now()
	last := ""
	for {
		r, err := http.Get(coordinatorURL + "/cases/" + id)
		if err != nil {
			log.Printf("GET /cases/%s: %v", id, err)
			time.Sleep(2 * time.Second)
			continue
		}
		var c caseResponse
		json.NewDecoder(r.Body).Decode(&c)
		r.Body.Close()

		done, running := 0, 0
		for _, j := range c.Jobs {
			switch j.Status {
			case "completed", "failed", "cancelled":
				done++
			case "running":
				running++
			}
		}
		line := fmt.Sprintf("  [%3.0fs] %-20s %d/%d resueltas, %d corriendo", time.Since(start).Seconds(), c.Status, done, len(c.Jobs), running)
		if line != last {
			fmt.Println(line)
			last = line
		}
		if isCaseTerminal(c.Status) {
			break
		}
		time.Sleep(2 * time.Second)
	}

	fmt.Println()
	r, err := http.Get(coordinatorURL + "/cases/" + id + "/report")
	if err != nil {
		log.Fatalf("GET report: %v", err)
	}
	defer r.Body.Close()
	var rep struct {
		Status   string  `json:"status"`
		Summary  string  `json:"summary"`
		Duration float64 `json:"duration_seconds"`
		SubTasks []struct {
			File     string  `json:"file"`
			Op       string  `json:"operation"`
			Status   string  `json:"status"`
			Worker   string  `json:"worker_id"`
			Duration float64 `json:"duration_seconds"`
			Error    string  `json:"error"`
		} `json:"sub_tasks"`
	}
	json.NewDecoder(r.Body).Decode(&rep)
	fmt.Printf("REPORTE — %s en %.1fs\n%s\n", rep.Status, rep.Duration, rep.Summary)
	for _, s := range rep.SubTasks {
		errTxt := ""
		if s.Error != "" {
			errTxt = "  ✗ " + firstLine(s.Error)
		}
		fmt.Printf("   %-24s %-14s %-10s %-10s %5.1fs%s\n", s.File, s.Op, s.Status, s.Worker, s.Duration, errTxt)
	}
	if !isCaseTerminal(rep.Status) || rep.Status == "failed" {
		os.Exit(1)
	}
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
