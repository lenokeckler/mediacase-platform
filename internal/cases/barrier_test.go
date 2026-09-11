package cases

import (
	"testing"

	"github.com/lenokeckler/mediacase-platform/internal/models"
)

func TestComputeStatus(t *testing.T) {
	tests := []struct {
		name                string
		total, done, failed int
		want                models.CaseStatus
		closed              bool
	}{
		{"faltan dos", 3, 1, 0, "", false},
		{"falta una: NO cierra aunque dos ya estén listas", 3, 2, 0, "", false},
		{"una corriendo y una fallida: sigue abierto", 3, 1, 1, "", false},
		{"todas OK", 3, 3, 0, models.CaseCompleted, true},
		{"una falló", 3, 2, 1, models.CasePartiallyCompleted, true},
		{"todas fallaron", 3, 0, 3, models.CaseFailed, true},
		{"única y falló", 1, 0, 1, models.CaseFailed, true},
		{"única y OK", 1, 1, 0, models.CaseCompleted, true},
		{"caso vacío se cierra completo", 0, 0, 0, models.CaseCompleted, true},
	}
	for _, tc := range tests {
		got, closed := ComputeStatus(tc.total, tc.done, tc.failed)
		if closed != tc.closed || (closed && got != tc.want) {
			t.Errorf("%s (%d,%d,%d): got %q/%v, want %q/%v",
				tc.name, tc.total, tc.done, tc.failed, got, closed, tc.want, tc.closed)
		}
	}
}
