package monitoring

import (
	"context"
	"log"
	"os/exec"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/lenokeckler/mediacase-platform/internal/models"
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/host"
	"github.com/shirou/gopsutil/v3/mem"
)

type (
	Hardware   = models.Hardware
	Metrics    = models.NodeMetrics
	GPUMetrics = models.GPUMetrics
)

type GPUInfo struct {
	models.GPUInfo
	key string
}

type gpuReading struct {
	key      string
	percent  *float64
	vramUsed *uint64
	tempC    *float64
}

type gpuProbe interface {
	list() []GPUInfo

	read() []gpuReading
	close()
}

type Collector struct {
	hw     Hardware
	gpus   []GPUInfo
	probes []gpuProbe

	mu   sync.RWMutex
	last Metrics
}

func NewCollector() *Collector {
	c := &Collector{}
	c.hw = detectHardware()
	c.probes = openProbes()
	if nv := newNvidiaProbe(); nv != nil {
		c.probes = append(c.probes, nv)
	}
	c.gpus = mergeGPUs(c.probes)
	c.hw.GPUs = []models.GPUInfo{}
	for _, g := range c.gpus {
		c.hw.GPUs = append(c.hw.GPUs, g.GPUInfo)
		log.Printf("[hardware] GPU %d: %s (%s, %s)", g.Index, g.Name, g.Vendor, g.Source)
	}
	return c
}

func (c *Collector) Hardware() Hardware { return c.hw }

func (c *Collector) Last() Metrics {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.last
}

func (c *Collector) Run(ctx context.Context) {
	t := time.NewTicker(1 * time.Second)
	defer t.Stop()
	defer func() {
		for _, p := range c.probes {
			p.close()
		}
	}()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			m := c.sample()
			c.mu.Lock()
			c.last = m
			c.mu.Unlock()
		}
	}
}

func (c *Collector) sample() Metrics {
	m := Metrics{SampledAt: time.Now()}
	if pcts, err := cpu.Percent(0, false); err == nil && len(pcts) > 0 {
		m.CPUPercent = pcts[0]
	}
	if vm, err := mem.VirtualMemory(); err == nil {
		m.MemUsedBytes, m.MemTotalBytes, m.MemPercent = vm.Used, vm.Total, vm.UsedPercent
	}
	if du, err := disk.Usage("."); err == nil {
		p := du.UsedPercent
		m.DiskPercent = &p
	}
	readings := map[string]gpuReading{}
	for _, p := range c.probes {
		for _, r := range p.read() {
			prev, ok := readings[r.key]
			if !ok {
				readings[r.key] = r
				continue
			}

			if prev.percent == nil {
				prev.percent = r.percent
			}
			if prev.vramUsed == nil {
				prev.vramUsed = r.vramUsed
			}
			if prev.tempC == nil {
				prev.tempC = r.tempC
			}
			readings[r.key] = prev
		}
	}
	m.GPUs = []GPUMetrics{}
	for _, g := range c.gpus {
		gm := GPUMetrics{Index: g.Index}
		if r, ok := readings[g.key]; ok {
			gm.Percent, gm.VRAMUsedBytes, gm.TempC = r.percent, r.vramUsed, r.tempC
		}
		m.GPUs = append(m.GPUs, gm)
	}
	return m
}

func detectHardware() Hardware {
	hw := Hardware{Arch: runtime.GOARCH, OS: runtime.GOOS}
	if hi, err := host.Info(); err == nil {
		hw.OS = prettyOS(hi.Platform, hi.PlatformVersion, hi.PlatformFamily)
	}
	if infos, err := cpu.Info(); err == nil && len(infos) > 0 {
		hw.CPUModel = strings.TrimSpace(infos[0].ModelName)
	}
	hw.CPUCores, _ = cpu.Counts(false)
	hw.CPUThreads, _ = cpu.Counts(true)
	if vm, err := mem.VirtualMemory(); err == nil {
		hw.MemTotalBytes = vm.Total
	}
	return hw
}

func prettyOS(platform, version, family string) string {
	p := strings.ToLower(platform)
	switch {
	case strings.Contains(p, "windows"):
		if strings.Contains(p, "11") || strings.HasPrefix(version, "10.0.2") {
			return "Windows 11"
		}
		return "Windows"
	case p == "arch":
		return "Arch Linux"
	case p == "":
		return family
	default:
		if version != "" {
			return strings.Title(p) + " " + version //nolint:staticcheck
		}
		return strings.Title(p) //nolint:staticcheck
	}
}

func mergeGPUs(probes []gpuProbe) []GPUInfo {
	var out []GPUInfo
	var nvidiaOnly []GPUInfo
	for _, p := range probes {
		for _, g := range p.list() {
			if g.Source == "nvidia-smi" {
				nvidiaOnly = append(nvidiaOnly, g)
				continue
			}
			out = append(out, g)
		}
	}
	for _, nv := range nvidiaOnly {
		matched := false
		for i := range out {
			if out[i].Vendor == "nvidia" && sameGPUName(out[i].Name, nv.Name) {
				matched = true
				if out[i].VRAMTotalBytes == 0 {
					out[i].VRAMTotalBytes = nv.VRAMTotalBytes
				}

				nvidiaKeyAlias[nv.key] = out[i].key
				break
			}
		}
		if !matched {
			out = append(out, nv)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Index < out[j].Index })
	for i := range out {
		out[i].Index = i
	}
	return out
}

var nvidiaKeyAlias = map[string]string{}

func sameGPUName(a, b string) bool {
	norm := func(s string) string {
		s = strings.ToLower(s)
		for _, w := range []string{"nvidia", "geforce", "laptop", "gpu", " "} {
			s = strings.ReplaceAll(s, w, "")
		}
		return s
	}
	return norm(a) == norm(b)
}

type nvidiaProbe struct{ bin string }

func newNvidiaProbe() gpuProbe {
	bin, err := exec.LookPath("nvidia-smi")
	if err != nil {
		if runtime.GOOS == "windows" {
			bin = `C:\Windows\System32\nvidia-smi.exe`
			if _, err := exec.LookPath(bin); err != nil {
				return nil
			}
		} else {
			return nil
		}
	}
	p := &nvidiaProbe{bin: bin}
	if len(p.list()) == 0 {
		return nil
	}
	return p
}

func (p *nvidiaProbe) query(fields string) [][]string {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, p.bin, "--query-gpu="+fields, "--format=csv,noheader,nounits")
	hideWindow(cmd)
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	return parseNvidiaCSV(string(out))
}

func parseNvidiaCSV(out string) [][]string {
	var rows [][]string
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var row []string
		for _, f := range strings.Split(line, ",") {
			row = append(row, strings.TrimSpace(f))
		}
		rows = append(rows, row)
	}
	return rows
}

func (p *nvidiaProbe) list() []GPUInfo {
	var out []GPUInfo
	for _, row := range p.query("index,name,memory.total") {
		g, ok := nvidiaInfoFromRow(row)
		if ok {
			out = append(out, g)
		}
	}
	return out
}

func nvidiaInfoFromRow(row []string) (GPUInfo, bool) {
	if len(row) < 3 {
		return GPUInfo{}, false
	}
	idx, err := strconv.Atoi(row[0])
	if err != nil {
		return GPUInfo{}, false
	}
	mib, _ := strconv.ParseFloat(row[2], 64)
	return GPUInfo{GPUInfo: models.GPUInfo{
		Index: 100 + idx, Name: row[1], Vendor: "nvidia", Source: "nvidia-smi",
		VRAMTotalBytes: uint64(mib * 1024 * 1024),
	}, key: "nv:" + row[0]}, true
}

func (p *nvidiaProbe) read() []gpuReading {
	var out []gpuReading
	for _, row := range p.query("index,utilization.gpu,memory.used,temperature.gpu") {
		r, ok := nvidiaReadingFromRow(row)
		if !ok {
			continue
		}
		if alias, ok := nvidiaKeyAlias[r.key]; ok {
			r.key = alias
		}
		out = append(out, r)
	}
	return out
}

func nvidiaReadingFromRow(row []string) (gpuReading, bool) {
	if len(row) < 4 {
		return gpuReading{}, false
	}
	r := gpuReading{key: "nv:" + row[0]}
	if v, err := strconv.ParseFloat(row[1], 64); err == nil {
		r.percent = &v
	}
	if v, err := strconv.ParseFloat(row[2], 64); err == nil {
		b := uint64(v * 1024 * 1024)
		r.vramUsed = &b
	}
	if v, err := strconv.ParseFloat(row[3], 64); err == nil {
		r.tempC = &v
	}
	return r, true
}

func (p *nvidiaProbe) close() {}
