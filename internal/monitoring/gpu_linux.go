//go:build linux

package monitoring

import (
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/lenokeckler/mediacase-platform/internal/models"
)

// GPUs en Linux vía /sys/class/drm/cardN/device: vendor (0x1002 AMD, 0x8086 Intel, 0x10de
// NVIDIA), gpu_busy_percent y mem_info_vram_{used,total} (amdgpu). Para NVIDIA el driver
// propietario no expone esto en sysfs: lo cubre nvidia-smi.

func hideWindow(*exec.Cmd) {}

func openProbes() []gpuProbe {
	p := &sysfsProbe{}
	if len(p.list()) == 0 {
		return nil
	}
	return []gpuProbe{p}
}

type sysfsProbe struct{}

func readTrim(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

func (p *sysfsProbe) cards() []string {
	cards, _ := filepath.Glob("/sys/class/drm/card[0-9]")
	sort.Strings(cards)
	return cards
}

func (p *sysfsProbe) list() []GPUInfo {
	var out []GPUInfo
	for i, c := range p.cards() {
		dev := filepath.Join(c, "device")
		vendorID := readTrim(filepath.Join(dev, "vendor"))
		if vendorID == "" {
			continue
		}
		g := GPUInfo{GPUInfo: models.GPUInfo{Index: i, Source: "sysfs"}, key: filepath.Base(c)}
		switch vendorID {
		case "0x1002":
			g.Vendor, g.Name = "amd", "AMD Radeon"
			if n := readTrim(filepath.Join(dev, "product_name")); n != "" {
				g.Name = "AMD " + n
			}
			g.Integrated = readTrim(filepath.Join(dev, "mem_info_vram_total")) != "" &&
				parseUint(readTrim(filepath.Join(dev, "mem_info_vram_total"))) < 2<<30
		case "0x8086":
			g.Vendor, g.Name, g.Integrated = "intel", "Intel Graphics", true
		case "0x10de":
			g.Vendor, g.Name = "nvidia", "NVIDIA"
		case "0x15ad":
			g.Vendor, g.Name = "virtual", "VMware SVGA (virtual)" // VirtualBox y VMware
		case "0x80ee":
			g.Vendor, g.Name = "virtual", "VirtualBox Graphics (virtual)"
		case "0x1234", "0x1b36":
			g.Vendor, g.Name = "virtual", "QEMU (virtual)"
		default:
			g.Vendor, g.Name = "other", "GPU "+vendorID
		}
		g.VRAMTotalBytes = parseUint(readTrim(filepath.Join(dev, "mem_info_vram_total")))
		out = append(out, g)
	}
	return out
}

func parseUint(s string) uint64 {
	v, _ := strconv.ParseUint(s, 10, 64)
	return v
}

func (p *sysfsProbe) read() []gpuReading {
	var out []gpuReading
	for _, c := range p.cards() {
		dev := filepath.Join(c, "device")
		r := gpuReading{key: filepath.Base(c)}
		if s := readTrim(filepath.Join(dev, "gpu_busy_percent")); s != "" {
			if v, err := strconv.ParseFloat(s, 64); err == nil {
				r.percent = &v
			}
		}
		if s := readTrim(filepath.Join(dev, "mem_info_vram_used")); s != "" {
			v := parseUint(s)
			r.vramUsed = &v
		}
		out = append(out, r)
	}
	return out
}

func (p *sysfsProbe) close() {}
