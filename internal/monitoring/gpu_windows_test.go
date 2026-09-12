//go:build windows

package monitoring

import "testing"

func TestParseEngineInstance(t *testing.T) {
	luid, eng, ok := parseEngineInstance("pid_22124_luid_0x00000000_0x000116f2_phys_0_eng_0_engtype_3d")
	if !ok || luid != "0x116f2" || eng != "3d" {
		t.Errorf("got %q %q %v", luid, eng, ok)
	}
	luid, eng, ok = parseEngineInstance("pid_4_luid_0x00000000_0x00014c24_phys_0_eng_3_engtype_video_decode")
	if !ok || luid != "0x14c24" || eng != "video_decode" {
		t.Errorf("got %q %q %v", luid, eng, ok)
	}
	if _, _, ok := parseEngineInstance("_Total"); ok {
		t.Errorf("_Total no es una instancia de GPU")
	}
}

func TestParseMemoryInstance(t *testing.T) {
	if luid, ok := parseMemoryInstance("luid_0x00000000_0x00014bb0_phys_0"); !ok || luid != "0x14bb0" {
		t.Errorf("got %q %v", luid, ok)
	}
}

func TestVendorOf(t *testing.T) {
	cases := []struct {
		name       string
		vendor     string
		integrated bool
	}{
		{"AMD Radeon 740M Graphics", "amd", true},
		{"AMD Radeon RX 7600", "amd", false},
		{"NVIDIA GeForce RTX 4050 Laptop GPU", "nvidia", false},
		{"Intel(R) Iris(R) Xe Graphics", "intel", true},
	}
	for _, c := range cases {
		v, i := vendorOf(c.name)
		if v != c.vendor || i != c.integrated {
			t.Errorf("%s: got %s/%v", c.name, v, i)
		}
	}
}
