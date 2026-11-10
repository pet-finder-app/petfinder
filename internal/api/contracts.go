package api

import (
	"time"

	"github.com/pet-finder-app/petfinder-api/internal/database"
	"github.com/pet-finder-app/petfinder-api/internal/domain"
)

// Request types are used by both handlers and the OpenAPI schema generator.
// Their validation tags are also enforced by decodeJSON.
type registerRequest struct {
	Name                  string `json:"name" minLength:"2" maxLength:"120"`
	Email                 string `json:"email" format:"email" maxLength:"254"`
	Password              string `json:"password" minLength:"12" maxLength:"72" writeOnly:"true" doc:"Must contain a letter and a number; maximum 72 bytes."`
	AcceptedTerms         bool   `json:"accepted_terms" enum:"true"`
	AcceptedPrivacyPolicy bool   `json:"accepted_privacy_policy" enum:"true"`
}

type loginRequest struct {
	Email    string `json:"email" format:"email" maxLength:"254"`
	Password string `json:"password" minLength:"1" maxLength:"72" writeOnly:"true"`
}

type refreshRequest struct {
	RefreshToken string `json:"refresh_token" minLength:"32" writeOnly:"true"`
}

type updateProfileRequest struct {
	Name     *string        `json:"name,omitempty" minLength:"2" maxLength:"120"`
	Phone    *string        `json:"phone,omitempty" maxLength:"30"`
	Location *locationInput `json:"location,omitempty"`
}

type locationInput struct {
	City        string   `json:"city" minLength:"1" maxLength:"100"`
	State       string   `json:"state" minLength:"1" maxLength:"8"`
	CountryCode string   `json:"country_code" pattern:"^[A-Z]{2}$"`
	Latitude    *float64 `json:"latitude,omitempty" minimum:"-90" maximum:"90"`
	Longitude   *float64 `json:"longitude,omitempty" minimum:"-180" maximum:"180"`
}

type officialChannels struct {
	Website *string `json:"website,omitempty" format:"uri"`
	Email   string  `json:"email" format:"email" maxLength:"254"`
	Phone   *string `json:"phone,omitempty" maxLength:"30"`
}

type createOrganizationRequest struct {
	Name               string           `json:"name" minLength:"2" maxLength:"120"`
	LegalName          string           `json:"legal_name" minLength:"2" maxLength:"180"`
	RegistrationNumber string           `json:"registration_number" minLength:"8" maxLength:"32"`
	Description        string           `json:"description" minLength:"20" maxLength:"2000"`
	OfficialChannels   officialChannels `json:"official_channels"`
	Location           locationInput    `json:"location"`
}

type updateOfficialChannels struct {
	Website *string `json:"website,omitempty" format:"uri"`
	Email   string  `json:"email,omitempty" format:"email" maxLength:"254"`
	Phone   *string `json:"phone,omitempty" maxLength:"30"`
}

type updateOrganizationRequest struct {
	Name             *string                 `json:"name,omitempty" minLength:"2" maxLength:"120"`
	Description      *string                 `json:"description,omitempty" minLength:"20" maxLength:"2000"`
	OfficialChannels *updateOfficialChannels `json:"official_channels,omitempty"`
	Location         *locationInput          `json:"location,omitempty"`
}

type organizationVerificationRequest struct {
	Decision string `json:"decision" enum:"verified,rejected,suspended"`
	Reason   string `json:"reason" minLength:"10" maxLength:"1000"`
}

type addOrganizationMemberRequest struct {
	Email string `json:"email" format:"email" maxLength:"254"`
	Role  string `json:"role" enum:"coordinator,representative,moderator"`
}

type animalRequest struct {
	OrganizationID       *string       `json:"organization_id,omitempty" format:"uuid"`
	Name                 string        `json:"name" minLength:"1" maxLength:"100"`
	Species              string        `json:"species" enum:"dog,cat,other"`
	Breed                *string       `json:"breed,omitempty" maxLength:"100"`
	Size                 string        `json:"size" enum:"small,medium,large,extra_large"`
	Sex                  string        `json:"sex" enum:"female,male,unknown"`
	ApproximateBirthDate *string       `json:"approximate_birth_date,omitempty" format:"date"`
	Description          string        `json:"description" minLength:"20" maxLength:"5000"`
	HealthNotes          string        `json:"health_notes,omitempty" maxLength:"3000"`
	BehaviorNotes        string        `json:"behavior_notes,omitempty" maxLength:"3000"`
	Location             locationInput `json:"location"`
}

// PATCH keeps the same fields as animalRequest but permits omitted fields.
type updateAnimalRequest struct {
	Name                 string        `json:"name,omitempty" minLength:"1" maxLength:"100"`
	Species              string        `json:"species,omitempty" enum:"dog,cat,other"`
	Breed                *string       `json:"breed,omitempty" maxLength:"100"`
	Size                 string        `json:"size,omitempty" enum:"small,medium,large,extra_large"`
	Sex                  string        `json:"sex,omitempty" enum:"female,male,unknown"`
	ApproximateBirthDate *string       `json:"approximate_birth_date,omitempty" format:"date"`
	Description          string        `json:"description,omitempty" minLength:"20" maxLength:"5000"`
	HealthNotes          string        `json:"health_notes,omitempty" maxLength:"3000"`
	BehaviorNotes        string        `json:"behavior_notes,omitempty" maxLength:"3000"`
	Location             locationInput `json:"location,omitempty"`
}

type animalStatusRequest struct {
	Status domain.AnimalStatus `json:"status" enum:"pending_organization,available,adoption_in_progress,unavailable,under_review" doc:"Adopted is set only after both delivery confirmations."`
	Reason string              `json:"reason" minLength:"5" maxLength:"1000"`
}

type animalImageRequest struct {
	URL      string `json:"url" format:"uri" pattern:"^https?://"`
	AltText  string `json:"alt_text,omitempty" maxLength:"300"`
	Position int16  `json:"position,omitempty" minimum:"0"`
}

type preferenceRequest struct {
	Value string `json:"value" enum:"liked,not_interested"`
}

type applicationRequest struct {
	AnimalID     string `json:"animal_id" format:"uuid"`
	Introduction string `json:"introduction" minLength:"20" maxLength:"3000"`
}

type screeningRequest struct {
	Housing         string `json:"housing" minLength:"1" maxLength:"1000"`
	Household       string `json:"household" minLength:"1" maxLength:"1000"`
	Experience      string `json:"experience" minLength:"1" maxLength:"2000"`
	CarePlan        string `json:"care_plan" minLength:"20" maxLength:"3000"`
	HasOtherAnimals bool   `json:"has_other_animals,omitempty"`
	AcceptsFollowUp bool   `json:"accepts_follow_up,omitempty"`
}

type applicationDecisionRequest struct {
	Decision string `json:"decision" enum:"approved,rejected,needs_information"`
	Reason   string `json:"reason" minLength:"5" maxLength:"2000"`
}

type messageRequest struct {
	Body string `json:"body" minLength:"1" maxLength:"5000"`
}

type appointmentRequest struct {
	Purpose      string    `json:"purpose" enum:"interview,home_visit,meet_and_greet,delivery,follow_up"`
	StartsAt     time.Time `json:"starts_at" doc:"Must be in the future."`
	EndsAt       time.Time `json:"ends_at" doc:"Must be after starts_at."`
	Timezone     string    `json:"timezone" doc:"IANA timezone, e.g. America/Recife."`
	LocationName string    `json:"location_name" minLength:"1" maxLength:"200"`
	Address      string    `json:"address" minLength:"1" maxLength:"500"`
	Instructions string    `json:"instructions,omitempty" maxLength:"2000"`
}

type updateAppointmentRequest struct {
	StartsAt     *time.Time `json:"starts_at,omitempty"`
	EndsAt       *time.Time `json:"ends_at,omitempty"`
	Timezone     *string    `json:"timezone,omitempty"`
	LocationName *string    `json:"location_name,omitempty" minLength:"1" maxLength:"200"`
	Address      *string    `json:"address,omitempty" minLength:"1" maxLength:"500"`
	Instructions *string    `json:"instructions,omitempty" maxLength:"2000"`
	Status       *string    `json:"status,omitempty" enum:"scheduled,cancelled"`
	Reason       *string    `json:"reason,omitempty" minLength:"5" maxLength:"1000"`
}

type appointmentConfirmationRequest struct {
	Type    string `json:"type" enum:"delivery,receipt"`
	OTPCode string `json:"otp_code,omitempty" pattern:"^[0-9]{6}$" writeOnly:"true" doc:"Required for receipt confirmation."`
}

type reportRequest struct {
	Category    string `json:"category" enum:"payment_request,fraud,harassment,inappropriate_content,off_platform_contact,animal_welfare,other"`
	Description string `json:"description" minLength:"10" maxLength:"5000"`
	SubjectType string `json:"subject_type" enum:"animal,organization,application,conversation,message"`
	SubjectID   string `json:"subject_id" format:"uuid"`
}

type updateReportRequest struct {
	Status     string `json:"status" enum:"investigating,resolved,dismissed"`
	Resolution string `json:"resolution,omitempty" minLength:"5" maxLength:"3000" doc:"Required when resolving or dismissing a report."`
}

// Response structs describe and serialize the actual wire format. Database
// structs already provide typed response bodies for resources returned directly.
type noBody struct{}

type healthResponse struct {
	Status string `json:"status" enum:"ok"`
}

type authErrorResponse struct {
	Error string `json:"error"`
}

type locationResponse struct {
	City        *string `json:"city"`
	State       *string `json:"state"`
	CountryCode string  `json:"country_code"`
}

type userResponse struct {
	ID        string            `json:"id" format:"uuid"`
	Name      string            `json:"name"`
	Email     string            `json:"email" format:"email"`
	Phone     *string           `json:"phone"`
	Roles     []string          `json:"roles"`
	Location  *locationResponse `json:"location"`
	CreatedAt *time.Time        `json:"created_at"`
	UpdatedAt *time.Time        `json:"updated_at"`
}

type authSessionResponse struct {
	AccessToken  string       `json:"access_token"`
	RefreshToken string       `json:"refresh_token"`
	TokenType    string       `json:"token_type" enum:"Bearer"`
	ExpiresIn    int          `json:"expires_in"`
	User         userResponse `json:"user"`
}

type organizationResponse struct {
	ID                 string                   `json:"id" format:"uuid"`
	Name               string                   `json:"name"`
	Description        string                   `json:"description"`
	VerificationStatus string                   `json:"verification_status" enum:"pending,verified,rejected,suspended"`
	VerifiedAt         *time.Time               `json:"verified_at"`
	OfficialChannels   officialChannelsResponse `json:"official_channels"`
	Location           locationResponse         `json:"location"`
	CreatedAt          *time.Time               `json:"created_at"`
	UpdatedAt          *time.Time               `json:"updated_at"`
	LegalName          *string                  `json:"legal_name,omitempty" doc:"Visible to authorized representatives and administrators."`
	RegistrationNumber **string                 `json:"registration_number,omitempty"`
	VerificationNotes  **string                 `json:"verification_notes,omitempty"`
}

type officialChannelsResponse struct {
	Website *string `json:"website"`
	Email   string  `json:"email" format:"email"`
	Phone   *string `json:"phone"`
}

type pageResponse struct {
	Page     int `json:"page" minimum:"1"`
	PageSize int `json:"page_size" minimum:"1" maximum:"100"`
	Total    int `json:"total" minimum:"0" doc:"Number of items in this page."`
}

type collectionResponse[T any] struct {
	Items []T          `json:"items"`
	Page  pageResponse `json:"page"`
}

type memberListResponse struct {
	Items []database.ListOrganizationMembersRow `json:"items"`
}

type animalDetailResponse struct {
	Animal database.Animal        `json:"animal"`
	Images []database.AnimalImage `json:"images"`
}

type animalImageResponse struct {
	ID               string     `json:"id" format:"uuid"`
	URL              string     `json:"url" format:"uri"`
	AltText          string     `json:"alt_text"`
	Position         int16      `json:"position"`
	ModerationStatus string     `json:"moderation_status" enum:"pending,approved,rejected"`
	CreatedAt        *time.Time `json:"created_at"`
}

type applicationCreatedResponse struct {
	Application    database.AdoptionApplication `json:"application"`
	ConversationID string                       `json:"conversation_id" format:"uuid"`
}

type applicationDetailResponse struct {
	Application database.GetAdoptionApplicationByIDRow `json:"application"`
	History     []database.ApplicationStatusHistory    `json:"history"`
	Screening   *database.ApplicationScreening         `json:"screening,omitempty"`
}

type messageResponse struct {
	Message database.Message `json:"message"`
	Warning string           `json:"warning,omitempty"`
}

type issuedOTPResponse struct {
	Code      string    `json:"code" pattern:"^[0-9]{6}$"`
	ExpiresAt time.Time `json:"expires_at"`
}
