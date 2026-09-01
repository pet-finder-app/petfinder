package internal

import "testing"

func TestAdd(t *testing.T) {
	a, b := 2, 2
	expected := a + b
	got := Add(a, b)

	if got != expected {
		t.Errorf("expected %d, got %d", expected, got)
	}
}
