package api

import (
	"encoding/json"
	"time"
)

type postalAddress struct {
	PostalCode   string `json:"postal_code,omitempty" maxLength:"20"`
	Street       string `json:"street,omitempty" maxLength:"200"`
	Number       string `json:"number,omitempty" maxLength:"20"`
	Complement   string `json:"complement,omitempty" maxLength:"100"`
	Neighborhood string `json:"neighborhood,omitempty" maxLength:"100"`
}
type personalProfile struct {
	CPF                      string        `json:"cpf,omitempty" pattern:"^[0-9]{11}$"`
	BirthDate                string        `json:"birth_date,omitempty" format:"date"`
	Occupation               string        `json:"occupation,omitempty" maxLength:"120"`
	Biography                string        `json:"biography,omitempty" maxLength:"3000"`
	SocialLinks              []string      `json:"social_links,omitempty" maxItems:"10" items:""`
	Address                  postalAddress `json:"address,omitempty"`
	Housing                  string        `json:"housing,omitempty" maxLength:"2000"`
	Preferences              []string      `json:"preferences,omitempty" maxItems:"30"`
	Documents                []string      `json:"documents,omitempty" maxItems:"20"`
	AvatarURL                string        `json:"avatar_url,omitempty" maxLength:"2000"`
	AllowOrganizationContact bool          `json:"allow_organization_contact,omitempty"`
}
type organizationPublicProfile struct {
	Type           string   `json:"type,omitempty" enum:"ngo,shelter,rescue_group,independent"`
	LogoURL        string   `json:"logo_url,omitempty" maxLength:"2000"`
	Gallery        []string `json:"gallery,omitempty" maxItems:"30"`
	Capacity       int      `json:"capacity,omitempty" minimum:"0" maximum:"100000"`
	ServiceAreas   []string `json:"service_areas,omitempty" maxItems:"100"`
	AdoptionPolicy string   `json:"adoption_policy,omitempty" maxLength:"10000"`
}
type organizationPrivateProfile struct {
	RepresentativeName    string   `json:"representative_name,omitempty" maxLength:"120"`
	RepresentativeCPF     string   `json:"representative_cpf,omitempty" pattern:"^[0-9]{11}$"`
	RepresentativeEmail   string   `json:"representative_email,omitempty" format:"email"`
	RepresentativePhone   string   `json:"representative_phone,omitempty" maxLength:"30"`
	Documents             []string `json:"documents,omitempty" maxItems:"30"`
	ConsentPublicContact  bool     `json:"consent_public_contact,omitempty"`
	ConsentDocumentReview bool     `json:"consent_document_review,omitempty"`
}
type verificationEvidence struct {
	Documents []string `json:"documents" minItems:"1" maxItems:"20"`
	Notes     string   `json:"notes,omitempty" maxLength:"5000"`
}
type verificationDecision struct {
	Decision string `json:"decision" enum:"under_review,approved,rejected"`
	Reason   string `json:"reason" minLength:"5" maxLength:"2000"`
}
type verificationRecord struct {
	ID          string               `json:"id" format:"uuid"`
	Evidence    verificationEvidence `json:"evidence"`
	Status      string               `json:"status"`
	Reason      *string              `json:"reason"`
	SubmittedBy string               `json:"submitted_by" format:"uuid"`
	ReviewedBy  *string              `json:"reviewed_by"`
	CreatedAt   time.Time            `json:"created_at"`
	ReviewedAt  *time.Time           `json:"reviewed_at"`
}
type verificationHistory struct {
	Items []verificationRecord `json:"items"`
}

type verificationRequirements struct {
	RequiredDocuments []string `json:"required_documents"`
	MaximumDocuments  int      `json:"maximum_documents"`
}

// verificationDocument describes one evidence URL; file fields are null when
// the URL does not point to an uploaded Go media file.
type verificationDocument struct {
	URL         string  `json:"url"`
	Filename    *string `json:"filename"`
	ContentType *string `json:"content_type"`
	ByteSize    *int64  `json:"byte_size"`
}
type verificationReviewItem struct {
	Verification   verificationRecord `json:"verification"`
	UserID         *string            `json:"user_id"`
	OrganizationID *string            `json:"organization_id"`
	// DisplayName is the current name of the person or organization under review.
	DisplayName string                 `json:"display_name"`
	Documents   []verificationDocument `json:"documents" doc:"Evidence documents in submission order, with their file details."`
	Profile     json.RawMessage        `json:"profile"`
}
type verificationStatusQuery struct {
	Status string `json:"status,omitempty" enum:"submitted,under_review,approved,rejected" doc:"Omit to list every submission."`
}
