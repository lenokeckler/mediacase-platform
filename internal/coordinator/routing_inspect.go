// internal/coordinator/routing_inspect.go
// Inspección de contenido de POST /cases (consigna §2: "inspeccionar cada archivo y determinar
// la operación correspondiente"): el routing inicial de submitCase decide por extensión; esta
// corrige el tipo cuando el contenido real dice otra cosa.
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

// sniffHeadBytes es cuánto encabezado se baja de cada archivo para inspeccionarlo: alcanza para
// todas las firmas binarias que reconoce cases.SniffType.
const sniffHeadBytes = 64 << 10

// sniffConcurrency acota cuántos archivos se inspeccionan a la vez: un caso de cientos de
// archivos no debe abrir cientos de lecturas a MinIO en paralelo sin límite.
const sniffConcurrency = 8

// inspectContent revisa el contenido real de cada archivo del caso y corrige decisions[i] cuando
// la extensión no correspondía. Devuelve una nota por archivo ("" = extensión correcta, o no se
// pudo inspeccionar). minio == nil (no disponible) deja todo en el routing por extensión.
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

// inspectOne inspecciona un archivo y, si hace falta, reemplaza *d por el routing correcto.
// Si la operación pedida por el cliente no aplica al tipo real, cae al default de ese tipo en
// vez de rechazar el caso entero (la extensión mentía, no el cliente).
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
		return "" // contenido no reconocible o ambiguo: se conserva el routing por extensión
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
		// La operación/destino que pidió el cliente no aplica al tipo real: se usa el default de
		// ese tipo en vez de rechazar el caso entero (la extensión, no el cliente, estaba mal).
		nd, err = cases.RouteAs(realType, f.Key, "", "", 0)
	}
	if err != nil {
		log.Printf("[cases] %s: no se pudo enrutar por su contenido real (%s): %v", f.Key, realType, err)
		return "" // se queda con el routing por extensión, que probablemente fallará en el worker
	}
	*d = nd
	return cases.MisleadingTypeNote(ext, extType, realType, format)
}
