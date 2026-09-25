package monitoring

import (
	"testing"

	"github.com/lenokeckler/mediacase-platform/internal/models"
)

func TestParseNvidiaCSV(t *testing.T) {
	out := "0, NVIDIA GeForce RTX 4050 Laptop GPU, 6141\r\n1, NVIDIA T400, 2048\n"
	rows := parseNvidiaCSV(out)
	if len(rows) != 2 || rows[0][1] != "NVIDIA GeForce RTX 4050 Laptop GPU" || rows[1][2] != "2048" {
		t.Fatalf("rows: %v", rows)
	}
	g, ok := nvidiaInfoFromRow(rows[0])
	if !ok || g.Vendor != "nvidia" || g.VRAMTotalBytes != 6141*1024*1024 || g.key != "nv:0" {
		t.Errorf("info: %+v", g)
	}
}

func TestNvidiaReadingFromRow(t *testing.T) {
	r, ok := nvidiaReadingFromRow([]string{"0", "37", "1234", "61"})
	if !ok || r.percent == nil || *r.percent != 37 || r.vramUsed == nil || *r.vramUsed != 1234*1024*1024 || r.tempC == nil || *r.tempC != 61 {
		t.Errorf("reading: %+v", r)
	}

	r, _ = nvidiaReadingFromRow([]string{"0", "[N/A]", "[N/A]", "[N/A]"})
	if r.percent != nil || r.vramUsed != nil || r.tempC != nil {
		t.Errorf("N/A debe ser nil: %+v", r)
	}
}

func TestMergeGPUs_NvidiaCompletaAlSistema(t *testing.T) {
	sys := fakeProbe{gpus: []GPUInfo{
		{GPUInfo: models.GPUInfo{Index: 0, Name: "AMD Radeon 740M Graphics", Vendor: "amd", Integrated: true, Source: "directx+pdh", VRAMTotalBytes: 441008128}, key: "0x116f2"},
		{GPUInfo: models.GPUInfo{Index: 1, Name: "NVIDIA GeForce RTX 4050 Laptop GPU", Vendor: "nvidia", Source: "directx+pdh"}, key: "0x14c24"},
	}}
	nv := fakeProbe{gpus: []GPUInfo{
		{GPUInfo: models.GPUInfo{Index: 100, Name: "NVIDIA GeForce RTX 4050 Laptop GPU", Vendor: "nvidia", Source: "nvidia-smi", VRAMTotalBytes: 6141 << 20}, key: "nv:0"},
	}}
	got := mergeGPUs([]gpuProbe{sys, nv})
	if len(got) != 2 {
		t.Fatalf("la misma tarjeta vista por dos fuentes debe ser una sola: %+v", got)
	}
	if got[0].Index != 0 || !got[0].Integrated || got[1].Index != 1 {
		t.Errorf("orden/índices: %+v", got)
	}
	if got[1].VRAMTotalBytes != 6141<<20 {
		t.Errorf("nvidia-smi debía completar la VRAM total: %d", got[1].VRAMTotalBytes)
	}
	if nvidiaKeyAlias["nv:0"] != "0x14c24" {
		t.Errorf("la lectura de nvidia-smi debe casarse con el LUID: %v", nvidiaKeyAlias)
	}
}

func TestMergeGPUs_SoloNvidia(t *testing.T) {
	nvidiaKeyAlias = map[string]string{}
	nv := fakeProbe{gpus: []GPUInfo{{GPUInfo: models.GPUInfo{Index: 100, Name: "NVIDIA T400", Vendor: "nvidia", Source: "nvidia-smi"}, key: "nv:0"}}}
	got := mergeGPUs([]gpuProbe{nv})
	if len(got) != 1 || got[0].Index != 0 {
		t.Errorf("sin fuente del sistema, nvidia-smi manda y el índice empieza en 0: %+v", got)
	}
}

func TestPrettyOS(t *testing.T) {
	cases := []struct{ platform, version, family, want string }{
		{"Microsoft Windows 11 Home", "10.0.26200", "Standalone Workstation", "Windows 11"},
		{"arch", "", "arch", "Arch Linux"},
		{"ubuntu", "24.04", "debian", "Ubuntu 24.04"},
	}
	for _, c := range cases {
		if got := prettyOS(c.platform, c.version, c.family); got != c.want {
			t.Errorf("%q: got %q want %q", c.platform, got, c.want)
		}
	}
}

type fakeProbe struct{ gpus []GPUInfo }

func (f fakeProbe) list() []GPUInfo    { return f.gpus }
func (f fakeProbe) read() []gpuReading { return nil }
func (f fakeProbe) close()             {}
