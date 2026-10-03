package domain

import "testing"

func TestAnimalTransitions(t *testing.T) {
	tests := []struct {
		from, to AnimalStatus
		valid    bool
	}{
		{AnimalPending, AnimalAvailable, true},
		{AnimalAvailable, AnimalAdopted, false},
		{AnimalInAdoption, AnimalAdopted, true},
		{AnimalAdopted, AnimalAvailable, false},
		{AnimalUnavailable, AnimalPending, false},
	}
	for _, tt := range tests {
		if got := ValidateAnimalTransition(tt.from, tt.to); (got == nil) != tt.valid {
			t.Errorf("%s -> %s: valid=%v, err=%v", tt.from, tt.to, tt.valid, got)
		}
	}
}

func TestApplicationTransitions(t *testing.T) {
	if err := ValidateApplicationTransition(ApplicationPending, ApplicationAccepted); err != nil {
		t.Fatal(err)
	}
	if err := ValidateApplicationTransition(ApplicationRejected, ApplicationAccepted); err == nil {
		t.Fatal("expected terminal rejected application to remain closed")
	}
}
