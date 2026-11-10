package domain

import "time"

type Role string

const (
	RoleAdopter    Role = "adopter"
	RoleAdvertiser Role = "advertiser"
	RoleNGOMember  Role = "organization_member"
	RoleAdmin      Role = "platform_admin"
)

type AnimalStatus string

const (
	AnimalPending     AnimalStatus = "pending_organization"
	AnimalAvailable   AnimalStatus = "available"
	AnimalInAdoption  AnimalStatus = "adoption_in_progress"
	AnimalAdopted     AnimalStatus = "adopted"
	AnimalUnavailable AnimalStatus = "unavailable"
	AnimalUnderReview AnimalStatus = "under_review"
)

type PreferenceKind string

const (
	PreferenceLiked     PreferenceKind = "liked"
	PreferenceDismissed PreferenceKind = "dismissed"
)

type ApplicationStatus string

const (
	ApplicationPending   ApplicationStatus = "submitted"
	ApplicationScreening ApplicationStatus = "screening"
	ApplicationMoreInfo  ApplicationStatus = "more_information_requested"
	ApplicationAccepted  ApplicationStatus = "approved"
	ApplicationRejected  ApplicationStatus = "rejected"
	ApplicationWithdrawn ApplicationStatus = "withdrawn"
	ApplicationCanceled  ApplicationStatus = "cancelled"
	ApplicationComplete  ApplicationStatus = "completed"
)

type ConversationKind string

const (
	ConversationAdopter    ConversationKind = "adopter_organization"
	ConversationAdvertiser ConversationKind = "owner_organization"
)

type User struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	Roles     []Role    `json:"roles"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Organization struct {
	ID                 string     `json:"id"`
	Name               string     `json:"name"`
	RegistrationNumber string     `json:"registration_number,omitempty"`
	Description        string     `json:"description,omitempty"`
	City               string     `json:"city"`
	State              string     `json:"state"`
	OfficialContact    string     `json:"official_contact,omitempty"`
	Verified           bool       `json:"verified"`
	VerifiedAt         *time.Time `json:"verified_at,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

type Animal struct {
	ID             string        `json:"id"`
	OwnerID        string        `json:"owner_id"`
	OrganizationID *string       `json:"organization_id,omitempty"`
	Name           string        `json:"name"`
	Species        string        `json:"species"`
	Breed          string        `json:"breed,omitempty"`
	Size           string        `json:"size"`
	City           string        `json:"city"`
	State          string        `json:"state"`
	Description    string        `json:"description"`
	HealthNotes    string        `json:"health_notes,omitempty"`
	BehaviorNotes  string        `json:"behavior_notes,omitempty"`
	Status         AnimalStatus  `json:"status"`
	Images         []AnimalImage `json:"images"`
	CreatedAt      time.Time     `json:"created_at"`
	UpdatedAt      time.Time     `json:"updated_at"`
}

type AnimalImage struct {
	ID        string    `json:"id"`
	URL       string    `json:"url"`
	Position  int       `json:"position"`
	CreatedAt time.Time `json:"created_at"`
}

type Application struct {
	ID               string            `json:"id"`
	AnimalID         string            `json:"animal_id"`
	AdopterID        string            `json:"adopter_id"`
	OrganizationID   string            `json:"organization_id"`
	Status           ApplicationStatus `json:"status"`
	ScreeningAnswers map[string]string `json:"screening_answers,omitempty"`
	DecisionReason   string            `json:"decision_reason,omitempty"`
	DecidedAt        *time.Time        `json:"decided_at,omitempty"`
	CreatedAt        time.Time         `json:"created_at"`
	UpdatedAt        time.Time         `json:"updated_at"`
}

type Message struct {
	ID             string    `json:"id"`
	ConversationID string    `json:"conversation_id"`
	SenderID       string    `json:"sender_id"`
	Body           string    `json:"body"`
	Flagged        bool      `json:"flagged"`
	CreatedAt      time.Time `json:"created_at"`
}

type Appointment struct {
	ID            string     `json:"id"`
	ApplicationID string     `json:"application_id"`
	ScheduledAt   time.Time  `json:"scheduled_at"`
	Location      string     `json:"location"`
	OTPExpiresAt  time.Time  `json:"otp_expires_at"`
	DeliveredAt   *time.Time `json:"delivered_at,omitempty"`
	ReceivedAt    *time.Time `json:"received_at,omitempty"`
	OTPVerifiedAt *time.Time `json:"otp_verified_at,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
}

type Page[T any] struct {
	Items    []T `json:"items"`
	Page     int `json:"page"`
	PageSize int `json:"page_size"`
	Total    int `json:"total"`
}
