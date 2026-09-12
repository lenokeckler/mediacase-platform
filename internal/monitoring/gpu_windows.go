//go:build windows

package monitoring

import (
	"fmt"
	"log"
	"os/exec"
	"strings"
	"syscall"
	"unsafe"

	"github.com/lenokeckler/mediacase-platform/internal/models"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// GPUs en Windows, como las ve el Administrador de tareas:
//   - HKLM\SOFTWARE\Microsoft\DirectX\{...}: una clave por adaptador con Description, AdapterLuid
//     y DedicatedVideoMemory. Se descarta el "Microsoft Basic Render Driver" (software).
//   - Contadores PDH "\GPU Engine(*)\Utilization Percentage" (una instancia por proceso × motor,
//     con el LUID en el nombre) y "\GPU Adapter Memory(*)\Dedicated Usage". El % de una GPU es,
//     como en el Administrador de tareas, el motor más ocupado (suma de todos los procesos).

func hideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
}

func openProbes() []gpuProbe {
	p, err := newPDHProbe()
	if err != nil {
		log.Printf("[hardware] contadores de GPU no disponibles: %v", err)
		return nil
	}
	return []gpuProbe{p}
}

// ── registro de DirectX ─────────────────────────────────────────────────────────────────

type dxAdapter struct {
	luid string // "0x116f2", como aparece en las instancias PDH (sin ceros a la izquierda)
	name string
	vram uint64
}

func directXAdapters() []dxAdapter {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\DirectX`, registry.READ)
	if err != nil {
		return nil
	}
	defer k.Close()
	names, err := k.ReadSubKeyNames(-1)
	if err != nil {
		return nil
	}
	var out []dxAdapter
	for _, n := range names {
		sk, err := registry.OpenKey(k, n, registry.READ)
		if err != nil {
			continue
		}
		desc, _, _ := sk.GetStringValue("Description")
		luid, _, err := sk.GetIntegerValue("AdapterLuid")
		vram, _, _ := sk.GetIntegerValue("DedicatedVideoMemory")
		sk.Close()
		if err != nil || desc == "" || strings.Contains(desc, "Basic Render") {
			continue
		}
		out = append(out, dxAdapter{luid: fmt.Sprintf("0x%x", luid), name: desc, vram: vram})
	}
	return out
}

func vendorOf(name string) (vendor string, integrated bool) {
	n := strings.ToLower(name)
	switch {
	case strings.Contains(n, "nvidia"):
		return "nvidia", false
	case strings.Contains(n, "amd") || strings.Contains(n, "radeon"):
		// Las Radeon integradas de los Ryzen se llaman "AMD Radeon Graphics" / "Radeon 740M/760M/780M".
		return "amd", !strings.Contains(n, "rx ")
	case strings.Contains(n, "intel"):
		return "intel", !strings.Contains(n, "arc")
	}
	return "other", false
}

// ── PDH ─────────────────────────────────────────────────────────────────────────────────

var (
	pdh                          = windows.NewLazySystemDLL("pdh.dll")
	pdhOpenQuery                 = pdh.NewProc("PdhOpenQueryW")
	pdhAddEnglishCounter         = pdh.NewProc("PdhAddEnglishCounterW")
	pdhCollectQueryData          = pdh.NewProc("PdhCollectQueryData")
	pdhGetFormattedCounterArrayW = pdh.NewProc("PdhGetFormattedCounterArrayW")
	pdhCloseQuery                = pdh.NewProc("PdhCloseQuery")
)

const (
	pdhFmtDouble     = 0x00000200
	pdhMoreData      = 0x800007D2
	pdhNoData        = 0x800007D5
	pdhInvalidData   = 0xC0000BC6
	pdhCStatusValid  = 0
	pdhCStatusNewDat = 1
)

// PDH_FMT_COUNTERVALUE_ITEM_W en amd64: puntero al nombre + {CStatus uint32, pad, double}.
type pdhFmtCounterValueItem struct {
	szName  *uint16
	cStatus uint32
	_       uint32
	double  float64
}

type pdhProbe struct {
	query    uintptr
	engine   uintptr
	memory   uintptr
	adapters []dxAdapter
	primed   bool
}

func newPDHProbe() (*pdhProbe, error) {
	adapters := directXAdapters()
	if len(adapters) == 0 {
		return nil, fmt.Errorf("sin adaptadores en el registro de DirectX")
	}
	p := &pdhProbe{adapters: adapters}
	if r, _, _ := pdhOpenQuery.Call(0, 0, uintptr(unsafe.Pointer(&p.query))); r != 0 {
		return nil, fmt.Errorf("PdhOpenQuery: 0x%x", r)
	}
	add := func(path string, h *uintptr) error {
		pp, _ := windows.UTF16PtrFromString(path)
		if r, _, _ := pdhAddEnglishCounter.Call(p.query, uintptr(unsafe.Pointer(pp)), 0, uintptr(unsafe.Pointer(h))); r != 0 {
			return fmt.Errorf("PdhAddEnglishCounter(%s): 0x%x", path, r)
		}
		return nil
	}
	if err := add(`\GPU Engine(*)\Utilization Percentage`, &p.engine); err != nil {
		p.close()
		return nil, err
	}
	if err := add(`\GPU Adapter Memory(*)\Dedicated Usage`, &p.memory); err != nil {
		p.close()
		return nil, err
	}
	// Los porcentajes necesitan dos muestras; la primera solo ceba el contador.
	pdhCollectQueryData.Call(p.query)
	return p, nil
}

func (p *pdhProbe) list() []GPUInfo {
	var out []GPUInfo
	for i, a := range p.adapters {
		vendor, integrated := vendorOf(a.name)
		out = append(out, GPUInfo{GPUInfo: models.GPUInfo{
			Index: i, Name: a.name, Vendor: vendor, Integrated: integrated,
			VRAMTotalBytes: a.vram, Source: "directx+pdh",
		}, key: a.luid})
	}
	return out
}

func (p *pdhProbe) read() []gpuReading {
	if r, _, _ := pdhCollectQueryData.Call(p.query); r != 0 {
		return nil
	}
	if !p.primed {
		p.primed = true
	}
	engine := p.counterArray(p.engine)
	memory := p.counterArray(p.memory)

	// % por (LUID, tipo de motor) sumando procesos; el % de la GPU es el motor más alto.
	byLuidEngine := map[string]map[string]float64{}
	for name, v := range engine {
		luid, eng, ok := parseEngineInstance(name)
		if !ok {
			continue
		}
		if byLuidEngine[luid] == nil {
			byLuidEngine[luid] = map[string]float64{}
		}
		byLuidEngine[luid][eng] += v
	}
	vramByLuid := map[string]uint64{}
	for name, v := range memory {
		if luid, ok := parseMemoryInstance(name); ok {
			vramByLuid[luid] += uint64(v)
		}
	}
	var out []gpuReading
	for _, a := range p.adapters {
		r := gpuReading{key: a.luid}
		if engines, ok := byLuidEngine[a.luid]; ok {
			best := 0.0
			for _, v := range engines {
				if v > best {
					best = v
				}
			}
			if best > 100 {
				best = 100
			}
			r.percent = &best
		} else if p.primed {
			zero := 0.0
			r.percent = &zero // sin procesos usando la GPU = 0 %, no "no disponible"
		}
		if v, ok := vramByLuid[a.luid]; ok {
			r.vramUsed = &v
		}
		out = append(out, r)
	}
	return out
}

// counterArray devuelve {nombre de instancia: valor} de un contador con comodín.
func (p *pdhProbe) counterArray(h uintptr) map[string]float64 {
	var size, count uint32
	r, _, _ := pdhGetFormattedCounterArrayW.Call(h, pdhFmtDouble, uintptr(unsafe.Pointer(&size)), uintptr(unsafe.Pointer(&count)), 0)
	if r != pdhMoreData || size == 0 {
		return nil
	}
	buf := make([]byte, size)
	r, _, _ = pdhGetFormattedCounterArrayW.Call(h, pdhFmtDouble, uintptr(unsafe.Pointer(&size)), uintptr(unsafe.Pointer(&count)), uintptr(unsafe.Pointer(&buf[0])))
	if r != 0 {
		return nil
	}
	items := unsafe.Slice((*pdhFmtCounterValueItem)(unsafe.Pointer(&buf[0])), int(count))
	out := make(map[string]float64, count)
	for _, it := range items {
		if it.cStatus != pdhCStatusValid && it.cStatus != pdhCStatusNewDat {
			continue
		}
		out[windows.UTF16PtrToString(it.szName)] = it.double
	}
	return out
}

// parseEngineInstance: "pid_22124_luid_0x00000000_0x000116f2_phys_0_eng_0_engtype_3d" →
// luid "0x116f2", motor "3d".
func parseEngineInstance(name string) (luid, engine string, ok bool) {
	parts := strings.Split(name, "_")
	for i, p := range parts {
		switch {
		case p == "luid" && i+2 < len(parts):
			luid = normalizeLuid(parts[i+2])
		case p == "engtype" && i+1 < len(parts):
			engine = strings.Join(parts[i+1:], "_")
		}
	}
	return luid, engine, luid != "" && engine != ""
}

// parseMemoryInstance: "luid_0x00000000_0x000116f2_phys_0" → "0x116f2".
func parseMemoryInstance(name string) (string, bool) {
	parts := strings.Split(name, "_")
	for i := 0; i+2 < len(parts); i++ {
		if parts[i] == "luid" {
			return normalizeLuid(parts[i+2]), true
		}
	}
	return "", false
}

// normalizeLuid: "0x000116f2" → "0x116f2", para que coincida con fmt.Sprintf("0x%x") del registro.
func normalizeLuid(s string) string {
	s = strings.ToLower(strings.TrimPrefix(s, "0x"))
	s = strings.TrimLeft(s, "0")
	if s == "" {
		s = "0"
	}
	return "0x" + s
}

func (p *pdhProbe) close() {
	if p.query != 0 {
		pdhCloseQuery.Call(p.query)
		p.query = 0
	}
}
