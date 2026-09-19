// cmd/ingest/main.go
// Ingesta del dataset y generación automática de casos (consigna §5) + generador de carga.
//
//	ingest upload --dir dataset/files --manifest dataset/manifest.json      # sube al bucket dataset/
//	ingest cases  --group-by session [--dry-run] [--limit N] [--only homogeneous|heterogeneous]
//	ingest load   --cases 20 --concurrency 5 --group-by session [--wait]    # carga concurrente
//
// Variables: COORDINATOR_URL (http://localhost:8080), MINIO_ENDPOINT/ACCESS_KEY/SECRET_KEY.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/lenokeckler/mediacase-platform/internal/ingest"
	"github.com/lenokeckler/mediacase-platform/internal/storage"
)

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	switch os.Args[1] {
	case "upload":
		cmdUpload(os.Args[2:])
	case "cases":
		cmdCases(os.Args[2:])
	case "load":
		cmdLoad(os.Args[2:])
	default:
		usage()
	}
}

func usage() {
	fmt.Println(`uso:
  ingest upload --dir dataset/files --manifest dataset/manifest.json [--concurrency 4]
  ingest cases  --group-by event|session|batch|user|folder|type|tier [--only homogeneous|heterogeneous]
                [--priority 5] [--limit N] [--dry-run] [--manifest ...] [--coordinator URL]
  ingest load   --cases 20 --concurrency 5 --group-by session [--priority 5] [--wait]`)
	os.Exit(2)
}

// ── upload ───────────────────────────────────────────────────────────────────

func cmdUpload(args []string) {
	fs := flag.NewFlagSet("upload", flag.ExitOnError)
	dir := fs.String("dir", "dataset/files", "carpeta con los archivos")
	manifestPath := fs.String("manifest", "dataset/manifest.json", "manifest del dataset")
	conc := fs.Int("concurrency", 4, "subidas en paralelo")
	fs.Parse(args)

	m, err := ingest.LoadManifest(*manifestPath)
	if err != nil {
		log.Fatal(err)
	}
	// MINIO_ENDPOINT por defecto: localhost (la ingesta corre en node-1)
	if os.Getenv("MINIO_ENDPOINT") == "" {
		os.Setenv("MINIO_ENDPOINT", "localhost:9000")
	}
	mc, err := storage.NewMinIOClient()
	if err != nil {
		log.Fatalf("MinIO: %v", err)
	}
	ctx := context.Background()

	// Lo que ya está (misma clave y mismo tamaño) se salta: la ingesta es reanudable.
	existing := map[string]int64{}
	if objs, err := mc.ListObjects(ctx, storage.DatasetBucket, ""); err == nil {
		for _, o := range objs {
			existing[o.Key] = o.Size
		}
	}

	var done, skipped, failed atomic.Int64
	var bytesUp atomic.Int64
	start := time.Now()
	sem := make(chan struct{}, *conc)
	var wg sync.WaitGroup
	for _, f := range m.Files {
		f := f
		if sz, ok := existing[f.Key]; ok && sz == f.SizeBytes {
			skipped.Add(1)
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			local := filepath.Join(*dir, f.Filename)
			if err := mc.UploadObject(ctx, storage.DatasetBucket, f.Key, local); err != nil {
				log.Printf("  ✗ %s: %v", f.Key, err)
				failed.Add(1)
				return
			}
			done.Add(1)
			bytesUp.Add(f.SizeBytes)
			if n := done.Load(); n%25 == 0 {
				log.Printf("  %d subidos, %.1f GB, %s", n, float64(bytesUp.Load())/1e9, time.Since(start).Round(time.Second))
			}
		}()
	}
	wg.Wait()
	fmt.Printf("upload: %d subidos (%.2f GB), %d ya estaban, %d fallidos, %s\n",
		done.Load(), float64(bytesUp.Load())/1e9, skipped.Load(), failed.Load(), time.Since(start).Round(time.Second))
	if failed.Load() > 0 {
		os.Exit(1)
	}
}

// ── cases ────────────────────────────────────────────────────────────────────

type caseResp struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Status    string `json:"status"`
	TotalJobs int    `json:"total_jobs"`
}

func postCase(coord string, req ingest.CaseRequest) (*caseResp, error) {
	body, _ := json.Marshal(req)
	resp, err := http.Post(coord+"/cases", "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		msg, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(msg)))
	}
	var c caseResp
	return &c, json.NewDecoder(resp.Body).Decode(&c)
}

func selectGroups(manifestPath, criterion, only string, limit int) []ingest.Group {
	m, err := ingest.LoadManifest(manifestPath)
	if err != nil {
		log.Fatal(err)
	}
	groups, err := ingest.GroupBy(m.Files, criterion)
	if err != nil {
		log.Fatal(err)
	}
	groups = ingest.Filter(groups, only)
	if limit > 0 && len(groups) > limit {
		groups = groups[:limit]
	}
	return groups
}

func cmdCases(args []string) {
	fs := flag.NewFlagSet("cases", flag.ExitOnError)
	manifestPath := fs.String("manifest", "dataset/manifest.json", "manifest del dataset")
	criterion := fs.String("group-by", "session", "criterio: event|session|batch|user|folder|type|tier")
	only := fs.String("only", "", "homogeneous | heterogeneous | (vacío = todos)")
	priority := fs.Int("priority", 5, "prioridad de los casos (1-10)")
	limit := fs.Int("limit", 0, "máximo de casos a crear (0 = todos)")
	dryRun := fs.Bool("dry-run", false, "solo mostrar la agrupación, no crear casos")
	enrich := fs.Bool("enrich", false, "audios y videos como enrich_* con los recursos del manifest (usuario → artista, evento → álbum)")
	coord := fs.String("coordinator", env("COORDINATOR_URL", "http://localhost:8080"), "URL del coordinador")
	fs.Parse(args)

	groups := selectGroups(*manifestPath, *criterion, *only, *limit)
	fmt.Printf("agrupación por %s: %d casos%s\n\n", *criterion, len(groups), map[bool]string{true: " (dry-run)", false: ""}[*dryRun])
	fmt.Printf("%-32s %6s %-22s %-12s %s\n", "CASO", "ARCH.", "TIPOS", "CLASE", "ID")
	hom, het := 0, 0
	for _, g := range groups {
		kind := "heterogéneo"
		if g.Homogeneous {
			kind = "homogéneo"
			hom++
		} else {
			het++
		}
		id := "-"
		if !*dryRun {
			c, err := postCase(*coord, g.ToRequestWith(*priority, *enrich))
			if err != nil {
				id = "ERROR: " + err.Error()
			} else {
				id = c.ID[:8]
			}
		}
		fmt.Printf("%-32s %6d %-22s %-12s %s\n", g.Name(), len(g.Files), strings.Join(g.Types(), "+"), kind, id)
	}
	fmt.Printf("\n%d homogéneos, %d heterogéneos\n", hom, het)
}

// ── load ─────────────────────────────────────────────────────────────────────

func cmdLoad(args []string) {
	fs := flag.NewFlagSet("load", flag.ExitOnError)
	manifestPath := fs.String("manifest", "dataset/manifest.json", "manifest del dataset")
	criterion := fs.String("group-by", "session", "criterio de agrupación")
	nCases := fs.Int("cases", 20, "cuántos casos enviar")
	conc := fs.Int("concurrency", 5, "envíos en paralelo")
	priority := fs.Int("priority", 5, "prioridad")
	wait := fs.Bool("wait", false, "esperar a que todos cierren e imprimir el resumen por estado")
	enrich := fs.Bool("enrich", false, "audios y videos como enrich_* con los recursos del manifest")
	coord := fs.String("coordinator", env("COORDINATOR_URL", "http://localhost:8080"), "URL del coordinador")
	fs.Parse(args)

	groups := selectGroups(*manifestPath, *criterion, "", 0)
	if len(groups) == 0 {
		log.Fatal("no hay grupos")
	}
	// Si piden más casos que grupos, se repiten grupos (casos concurrentes sobre los mismos archivos).
	reqs := make([]ingest.CaseRequest, 0, *nCases)
	for i := 0; i < *nCases; i++ {
		g := groups[i%len(groups)]
		r := g.ToRequestWith(*priority, *enrich)
		r.Name = fmt.Sprintf("carga-%02d-%s", i+1, g.Name())
		reqs = append(reqs, r)
	}

	fmt.Printf("generador de carga: %d casos, %d en paralelo, criterio %s\n", *nCases, *conc, *criterion)
	start := time.Now()
	ids := make([]string, len(reqs))
	subTasks := 0
	var mu sync.Mutex
	sem := make(chan struct{}, *conc)
	var wg sync.WaitGroup
	for i, r := range reqs {
		i, r := i, r
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			t := time.Now()
			c, err := postCase(*coord, r)
			if err != nil {
				log.Printf("  ✗ %s: %v", r.Name, err)
				return
			}
			mu.Lock()
			ids[i] = c.ID
			subTasks += c.TotalJobs
			mu.Unlock()
			fmt.Printf("  %-40s %3d sub-tareas  creado en %s\n", r.Name, c.TotalJobs, time.Since(t).Round(time.Millisecond))
		}()
	}
	wg.Wait()
	fmt.Printf("\n%d casos (%d sub-tareas) enviados en %s\n", len(ids), subTasks, time.Since(start).Round(time.Millisecond))
	if !*wait {
		return
	}

	// Esperar el cierre de todos y resumir.
	fmt.Println("esperando a que cierren…")
	for {
		time.Sleep(3 * time.Second)
		counts := map[string]int{}
		open := 0
		for _, id := range ids {
			if id == "" {
				continue
			}
			r, err := http.Get(*coord + "/cases/" + id)
			if err != nil {
				continue
			}
			var c caseResp
			json.NewDecoder(r.Body).Decode(&c)
			r.Body.Close()
			counts[c.Status]++
			switch c.Status {
			case "completed", "partially_completed", "failed", "cancelled":
			default:
				open++
			}
		}
		keys := make([]string, 0, len(counts))
		for k := range counts {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		parts := make([]string, 0, len(keys))
		for _, k := range keys {
			parts = append(parts, fmt.Sprintf("%s=%d", k, counts[k]))
		}
		fmt.Printf("  [%4.0fs] %s\n", time.Since(start).Seconds(), strings.Join(parts, "  "))
		if open == 0 {
			break
		}
	}
	fmt.Printf("todos cerrados en %s\n", time.Since(start).Round(time.Second))
}
