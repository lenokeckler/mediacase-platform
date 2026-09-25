package coordinator

import (
	"context"
	"log"
	"path/filepath"
	"strings"
	"sync"

	"github.com/lenokeckler/mediacase-platform/internal/cases"
	"github.com/lenokeckler/mediacase-platform/internal/storage"
)

const sniffHeadBytes = 64 << 10

const sniffConcurrency = 8

func inspectContent(ctx context.Context, minio *storage.MinIOClient, files []caseFileReq, decisions []cases.RouteDecision) []string {
	notes := make([]string, len(files))
	if minio == nil {
		return notes
	}
	sem := make(chan struct{}, sniffConcurrency)
	var wg sync.WaitGroup
	for i := range files {
		i := i
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			notes[i] = inspectOne(ctx, minio, files[i], &decisions[i])
		}()
	}
	wg.Wait()
	return notes
}

func inspectOne(ctx context.Context, minio *storage.MinIOClient, f caseFileReq, d *cases.RouteDecision) string {
	head, err := minio.GetHead(ctx, storage.DatasetBucket, f.Key, sniffHeadBytes)
	if err != nil {
		log.Printf("[cases] no se pudo inspeccionar el contenido de %s: %v", f.Key, err)
		return ""
	}
	if head == nil {
		return cases.EmptyFileNote
	}
	realType, format, ok := cases.SniffType(head)
	if !ok {
		return ""
	}
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(f.Key), "."))
	if realType == d.FileType {
		if cases.SameFormat(ext, format) {
			return ""
		}
		return cases.MisleadingFormatNote(ext, format)
	}

	extType := d.FileType
	nd, err := cases.RouteAs(realType, f.Key, f.Operation, f.Target, f.Width)
	if err != nil {

		nd, err = cases.RouteAs(realType, f.Key, "", "", 0)
	}
	if err != nil {
		log.Printf("[cases] %s: no se pudo enrutar por su contenido real (%s): %v", f.Key, realType, err)
		return ""
	}
	*d = nd
	return cases.MisleadingTypeNote(ext, extType, realType, format)
}
