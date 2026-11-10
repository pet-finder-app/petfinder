package moderation

import "testing"

func TestInspect(t *testing.T) {
	tests := []struct {
		body    string
		flagged bool
	}{
		{"A ONG confirmou a visita amanhã.", false},
		{"Pague a taxa por PIX antes da entrega", true},
		{"Me chama no WhatsApp para combinarmos", true},
		{"Veja http://example.test fora daqui", true},
	}
	for _, tt := range tests {
		t.Run(tt.body, func(t *testing.T) {
			if got := Inspect(tt.body); got.Flagged != tt.flagged {
				t.Fatalf("Inspect(%q).Flagged = %v, reasons=%v", tt.body, got.Flagged, got.Reasons)
			}
		})
	}
}
