package domain

import "fmt"

var animalTransitions = map[AnimalStatus]map[AnimalStatus]bool{
	AnimalPending: {
		AnimalAvailable:   true,
		AnimalUnavailable: true,
		AnimalUnderReview: true,
	},
	AnimalAvailable: {
		AnimalInAdoption:  true,
		AnimalUnavailable: true,
		AnimalUnderReview: true,
	},
	AnimalInAdoption: {
		AnimalAvailable:   true,
		AnimalAdopted:     true,
		AnimalUnavailable: true,
		AnimalUnderReview: true,
	},
	AnimalUnderReview: {
		AnimalPending:     true,
		AnimalAvailable:   true,
		AnimalUnavailable: true,
	},
}

func ValidateAnimalTransition(from, to AnimalStatus) error {
	if from == to {
		return nil
	}
	if !animalTransitions[from][to] {
		return fmt.Errorf("animal cannot transition from %q to %q", from, to)
	}
	return nil
}

var applicationTransitions = map[ApplicationStatus]map[ApplicationStatus]bool{
	ApplicationPending: {
		ApplicationScreening: true,
		ApplicationMoreInfo:  true,
		ApplicationAccepted:  true,
		ApplicationRejected:  true,
		ApplicationCanceled:  true,
		ApplicationWithdrawn: true,
	},
	ApplicationScreening: {
		ApplicationMoreInfo:  true,
		ApplicationAccepted:  true,
		ApplicationRejected:  true,
		ApplicationCanceled:  true,
		ApplicationWithdrawn: true,
	},
	ApplicationMoreInfo: {
		ApplicationScreening: true,
		ApplicationAccepted:  true,
		ApplicationRejected:  true,
		ApplicationCanceled:  true,
		ApplicationWithdrawn: true,
	},
	ApplicationAccepted: {
		ApplicationCanceled: true,
		ApplicationComplete: true,
	},
}

func ValidateApplicationTransition(from, to ApplicationStatus) error {
	if from == to {
		return nil
	}
	if !applicationTransitions[from][to] {
		return fmt.Errorf("application cannot transition from %q to %q", from, to)
	}
	return nil
}
