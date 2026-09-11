package main

import "testing"

func TestRoleCapabilities(t *testing.T) {
	tests := []struct {
		role string
		want []string
	}{
		{"video", []string{"video"}},
		{"audio", []string{"audio"}},
		{"metadata", []string{"metadata"}},
		{"all", []string{"video", "audio", "metadata"}},
		{"", []string{"video", "audio", "metadata"}},
		{"bogus", []string{"video", "audio", "metadata"}}, // rol desconocido → genérico, no ocioso
	}
	for _, tc := range tests {
		got := RoleCapabilities(tc.role)
		if len(got) != len(tc.want) {
			t.Errorf("rol %q: got %v, want %v", tc.role, got, tc.want)
			continue
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("rol %q: got %v, want %v", tc.role, got, tc.want)
			}
		}
	}
}

func TestLoadConfig_Role(t *testing.T) {
	t.Setenv("WORKER_ROLE", "audio")
	if cfg := loadConfig(); cfg.role != "audio" {
		t.Fatalf("role = %q", cfg.role)
	}
	t.Setenv("WORKER_ROLE", "")
	if cfg := loadConfig(); cfg.role != "all" {
		t.Fatalf("role default = %q, quería all", cfg.role)
	}
}
