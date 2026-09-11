// cmd/client/main.go
// Test client: single job submission and batch/concurrent load testing.
// Usage:
//
//	Single:  ./client -file /path/to/video.mp4 -op convert -priority 5 -watch
//	Batch:   ./client -batch -manifest dataset/manifest.json -concurrency 50 -watch
//	Stats:   ./client -stats
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

// ── Types ─────────────────────────────────────────────────────────────────────

type submitRequest struct {
	FilePath  string `json:"file_path"`
	Operation string `json:"operation"`
	Priority  int    `json:"priority"`
}

type jobResponse struct {
	ID        string `json:"id"`
	FilePath  string `json:"file_path"`
	Operation string `json:"operation"`
	Status    string `json:"status"`
	Priority  int    `json:"priority"`
	Progress  int    `json:"progress"`
	WorkerID  string `json:"worker_id"`
	ErrorMsg  string `json:"error_msg"`
	ResultURL string `json:"result_url"`
	CreatedAt string `json:"created_at"`
}

type manifestFile struct {
	Filename   string   `json:"filename"`
	Path       string   `json:"path"`
	Type       string   `json:"type"`
	Format     string   `json:"format"`
	Operations []string `json:"operations"`
}

type manifest struct {
	Total int            `json:"total"`
	Files []manifestFile `json:"files"`
}

// ── HTTP helpers ──────────────────────────────────────────────────────────────

func submitJob(coordinatorURL string, req submitRequest) (*jobResponse, error) {
	body, _ := json.Marshal(req)
	resp, err := http.Post(coordinatorURL+"/jobs", "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("POST /jobs: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		raw, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("coordinator returned %d: %s", resp.StatusCode, string(raw))
	}
	var job jobResponse
	if err := json.NewDecoder(resp.Body).Decode(&job); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return &job, nil
}

func getJob(coordinatorURL, jobID string) (*jobResponse, error) {
	resp, err := http.Get(fmt.Sprintf("%s/jobs/%s", coordinatorURL, jobID))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var job jobResponse
	json.NewDecoder(resp.Body).Decode(&job)
	return &job, nil
}

func listJobs(coordinatorURL, status string) ([]jobResponse, error) {
	url := coordinatorURL + "/jobs"
	if status != "" {
		url += "?status=" + status
	}
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var jobs []jobResponse
	json.NewDecoder(resp.Body).Decode(&jobs)
	return jobs, nil
}

// ── Single job mode ───────────────────────────────────────────────────────────

func runSingle(coordinatorURL, filePath, operation string, priority int, watch bool) {
	fmt.Printf("Submitting job: op=%s file=%s priority=%d\n", operation, filePath, priority)

	job, err := submitJob(coordinatorURL, submitRequest{
		FilePath:  filePath,
		Operation: operation,
		Priority:  priority,
	})
	if err != nil {
		log.Fatalf("Failed to submit job: %v", err)
	}

	fmt.Printf("✅ Job submitted: %s\n", job.ID)
	fmt.Printf("   Status:    %s\n", job.Status)
	fmt.Printf("   Operation: %s\n", job.Operation)

	if !watch {
		fmt.Println("\nTip: run with -watch to poll until completion.")
		return
	}

	fmt.Printf("\nWatching job %s...\n", job.ID)
	watchSingleJob(coordinatorURL, job.ID)
}

func watchSingleJob(coordinatorURL, jobID string) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	lastStatus := ""
	for range ticker.C {
		job, err := getJob(coordinatorURL, jobID)
		if err != nil {
			fmt.Printf("  poll error: %v\n", err)
			continue
		}
		if job.Status != lastStatus || job.Status == "running" {
			fmt.Printf("  [%s] status=%-10s progress=%3d%%  worker=%s\n",
				time.Now().Format("15:04:05"), job.Status, job.Progress, job.WorkerID)
			lastStatus = job.Status
		}
		if job.Status == "completed" {
			fmt.Printf("\n✅ Completed! Result URL:\n   %s\n", job.ResultURL)
			return
		}
		if job.Status == "failed" {
			fmt.Printf("\n❌ Failed: %s\n", job.ErrorMsg)
			return
		}
	}
}

// ── Batch mode ────────────────────────────────────────────────────────────────

func runBatch(coordinatorURL, manifestPath string, concurrency, priorityOverride int, watch bool) {
	raw, err := os.ReadFile(manifestPath)
	if err != nil {
		log.Fatalf("Cannot read manifest %s: %v", manifestPath, err)
	}
	var m manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		log.Fatalf("Cannot parse manifest: %v", err)
	}

	// Expand: one request per (file, operation) pair
	type task struct {
		file manifestFile
		op   string
	}
	var tasks []task
	for _, f := range m.Files {
		for _, op := range f.Operations {
			tasks = append(tasks, task{file: f, op: op})
		}
	}

	total := len(tasks)
	fmt.Printf("Batch mode: %d tasks from %d files | concurrency=%d\n\n",
		total, len(m.Files), concurrency)

	var (
		submitted    int64
		submitFailed int64
		jobIDs       []string
		mu           sync.Mutex
		wg           sync.WaitGroup
		sem          = make(chan struct{}, concurrency)
		start        = time.Now()
	)

	for i, t := range tasks {
		wg.Add(1)
		sem <- struct{}{}
		go func(idx int, tk task) {
			defer wg.Done()
			defer func() { <-sem }()

			priority := priorityOverride
			if priority == 0 {
				priority = 5
			}

			job, err := submitJob(coordinatorURL, submitRequest{
				FilePath:  tk.file.Path,
				Operation: tk.op,
				Priority:  priority,
			})
			if err != nil {
				atomic.AddInt64(&submitFailed, 1)
				fmt.Printf("  [%4d/%d] ❌ SUBMIT FAILED  %s %s: %v\n",
					idx+1, total, tk.op, tk.file.Filename, err)
				return
			}

			mu.Lock()
			jobIDs = append(jobIDs, job.ID)
			mu.Unlock()

			n := atomic.AddInt64(&submitted, 1)
			if n%10 == 0 || int(n) == total {
				fmt.Printf("  [%4d/%d] submitted %d/%d (%.1f jobs/s)\n",
					idx+1, total, n, total,
					float64(n)/time.Since(start).Seconds())
			}
		}(i, t)
	}

	wg.Wait()
	elapsed := time.Since(start)

	fmt.Printf("\n── Submission complete ──────────────────────────\n")
	fmt.Printf("  Submitted:       %d\n", submitted)
	fmt.Printf("  Submit failures: %d\n", submitFailed)
	fmt.Printf("  Time:            %s\n", elapsed.Round(time.Millisecond))
	if submitted > 0 {
		fmt.Printf("  Rate:            %.1f jobs/s\n",
			float64(submitted)/elapsed.Seconds())
	}

	if watch && len(jobIDs) > 0 {
		fmt.Printf("\nWatching %d jobs until all finish...\n\n", len(jobIDs))
		watchAllJobs(coordinatorURL, jobIDs)
	} else if len(jobIDs) > 0 {
		fmt.Println("\nTip: run with -watch to poll until all jobs finish.")
	}
}

// watchAllJobs polls /jobs every 2 seconds and prints a live summary line
// until every submitted job is completed or failed.
func watchAllJobs(coordinatorURL string, jobIDs []string) {
	// Build a lookup set so we only track OUR jobs, not leftover ones in the DB
	idSet := make(map[string]bool, len(jobIDs))
	for _, id := range jobIDs {
		idSet[id] = true
	}

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	start := time.Now()

	// Track per-job processing times for the final report
	type jobMeta struct {
		startedAt   time.Time
		completedAt time.Time
		operation   string
	}
	durations := make(map[string]jobMeta)

	for range ticker.C {
		all, err := listJobs(coordinatorURL, "")
		if err != nil {
			fmt.Printf("  poll error: %v\n", err)
			continue
		}

		counts := map[string]int{
			"pending":   0,
			"assigned":  0,
			"running":   0,
			"completed": 0,
			"failed":    0,
		}

		var failedJobs []jobResponse

		for _, j := range all {
			if !idSet[j.ID] {
				continue
			}
			counts[j.Status]++
			if j.Status == "failed" {
				failedJobs = append(failedJobs, j)
			}
		}

		total := len(jobIDs)
		done := counts["completed"] + counts["failed"]
		elapsed := time.Since(start).Round(time.Second)

		fmt.Printf(
			"  [%6s] pending=%-4d assigned=%-4d running=%-4d completed=%-4d failed=%-4d  (%d/%d done)\n",
			elapsed,
			counts["pending"],
			counts["assigned"],
			counts["running"],
			counts["completed"],
			counts["failed"],
			done, total,
		)

		if done == total {
			// Final report
			fmt.Printf("\n── All %d jobs finished ─────────────────────────\n", total)
			fmt.Printf("  ✅ Completed:   %d\n", counts["completed"])
			fmt.Printf("  ❌ Failed:      %d\n", counts["failed"])
			fmt.Printf("  Total time:    %s\n", elapsed)
			if counts["completed"] > 0 {
				fmt.Printf("  Throughput:    %.2f jobs/s\n",
					float64(counts["completed"])/time.Since(start).Seconds())
			}
			if len(failedJobs) > 0 {
				fmt.Println("\n  Failed job details:")
				for _, j := range failedJobs {
					fmt.Printf("    %s  op=%-15s  error=%s\n",
						j.ID[:8], j.Operation, j.ErrorMsg)
				}
			}
			_ = durations
			return
		}
	}
}

// ── Stats mode ────────────────────────────────────────────────────────────────

func runStats(coordinatorURL string) {
	resp, err := http.Get(coordinatorURL + "/stats")
	if err != nil {
		log.Fatalf("GET /stats: %v", err)
	}
	defer resp.Body.Close()
	var stats map[string]int
	json.NewDecoder(resp.Body).Decode(&stats)

	fmt.Println("── System Stats ────────────────────────────────")
	for k, v := range stats {
		fmt.Printf("  %-12s %d\n", k, v)
	}

	resp2, err := http.Get(coordinatorURL + "/workers")
	if err != nil {
		return
	}
	defer resp2.Body.Close()
	var workers []map[string]interface{}
	json.NewDecoder(resp2.Body).Decode(&workers)

	fmt.Printf("\n── Workers (%d) ─────────────────────────────────\n", len(workers))
	for _, w := range workers {
		fmt.Printf("  %-12s  status=%-6s  active_jobs=%.0f  cpu=%.1f%%\n",
			w["id"], w["status"], w["active_jobs"], w["cpu_percent"])
	}
}

// ── main ──────────────────────────────────────────────────────────────────────

func main() {
	coordinatorURL := flag.String("coordinator", "",
		"Coordinator URL (default: $COORDINATOR_URL or http://localhost:8080)")

	// Single job flags
	filePath := flag.String("file", "", "Path to the input file")
	operation := flag.String("op", "convert", "Operation: convert | extract_audio | thumbnail")
	priority := flag.Int("priority", 5, "Job priority 1-10 (10=highest)")
	watch := flag.Bool("watch", false,
		"Poll until the single job (or all batch jobs) finish")

	// Batch flags
	batch := flag.Bool("batch", false, "Enable batch mode (reads -manifest)")
	manifestPath := flag.String("manifest", "dataset/manifest.json",
		"Path to dataset manifest.json")
	concurrency := flag.Int("concurrency", 20,
		"Number of concurrent submissions in batch mode")

	// Stats flag
	statsMode := flag.Bool("stats", false,
		"Print system stats and worker list, then exit")

	flag.Parse()

	// Resolve coordinator URL
	url := *coordinatorURL
	if url == "" {
		url = os.Getenv("COORDINATOR_URL")
	}
	if url == "" {
		url = "http://localhost:8080"
	}

	fmt.Printf("[client] coordinator → %s\n\n", url)

	switch {
	case *statsMode:
		runStats(url)

	case *batch:
		runBatch(url, *manifestPath, *concurrency, *priority, *watch)

	default:
		if *filePath == "" {
			fmt.Println("Usage:")
			fmt.Println("  Single job:  client -file <path> -op <operation> [-priority N] [-watch]")
			fmt.Println("  Batch load:  client -batch -manifest dataset/manifest.json [-concurrency 50] [-watch]")
			fmt.Println("  Stats:       client -stats")
			fmt.Println("\nOperations: convert | extract_audio | thumbnail")
			os.Exit(1)
		}
		runSingle(url, *filePath, *operation, *priority, *watch)
	}
}
