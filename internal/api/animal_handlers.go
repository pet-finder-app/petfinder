package api

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/pet-finder-app/petfinder-api/internal/auth"
	"github.com/pet-finder-app/petfinder-api/internal/database"
	"github.com/pet-finder-app/petfinder-api/internal/domain"
)

func validateAnimal(request *animalRequest) fieldErrors {
	errs := fieldErrors{}
	request.Name = required(errs, "name", request.Name, 100)
	if request.Species != "dog" && request.Species != "cat" && request.Species != "other" {
		errs.add("species", "must be dog, cat, or other")
	}
	if request.Size != "small" && request.Size != "medium" && request.Size != "large" && request.Size != "extra_large" {
		errs.add("size", "must be small, medium, large, or extra_large")
	}
	if request.Sex != "female" && request.Sex != "male" && request.Sex != "unknown" {
		errs.add("sex", "must be female, male, or unknown")
	}
	request.Description = required(errs, "description", request.Description, 5000)
	if len(request.Description) < 20 {
		errs.add("description", "must contain at least 20 characters")
	}
	request.HealthNotes = optional(errs, "health_notes", request.HealthNotes, 3000)
	request.BehaviorNotes = optional(errs, "behavior_notes", request.BehaviorNotes, 3000)
	request.Location.City = required(errs, "location.city", request.Location.City, 100)
	request.Location.State = required(errs, "location.state", request.Location.State, 100)
	request.Location.CountryCode = strings.ToUpper(strings.TrimSpace(request.Location.CountryCode))
	if len(request.Location.CountryCode) != 2 {
		errs.add("location.country_code", "must be an ISO 3166-1 alpha-2 code")
	}
	if (request.Location.Latitude == nil) != (request.Location.Longitude == nil) {
		errs.add("location", "latitude and longitude must be provided together")
	}
	return errs
}

func birthDate(value *string, errs fieldErrors) pgtype.Date {
	if value == nil || strings.TrimSpace(*value) == "" {
		return pgtype.Date{}
	}
	parsed, err := time.Parse(time.DateOnly, *value)
	if err != nil || parsed.After(time.Now()) {
		errs.add("approximate_birth_date", "must be a valid date not in the future")
		return pgtype.Date{}
	}
	return pgtype.Date{Time: parsed, Valid: true}
}

func animalParams(request animalRequest) database.CreatePrivateAnimalParams {
	return database.CreatePrivateAnimalParams{
		Name: request.Name, Species: request.Species, Breed: request.Breed, Sex: request.Sex, Size: request.Size,
		Description: request.Description, HealthNotes: request.HealthNotes, BehaviorNotes: request.BehaviorNotes,
		City: request.Location.City, StateCode: request.Location.State, CountryCode: request.Location.CountryCode,
		Latitude: numeric(request.Location.Latitude), Longitude: numeric(request.Location.Longitude),
	}
}

func (s *Server) listAnimals(w http.ResponseWriter, r *http.Request) {
	offset, size, page := pagination(r)
	query := r.URL.Query()
	ptr := func(key string) *string {
		if value := strings.TrimSpace(query.Get(key)); value != "" {
			return &value
		}
		return nil
	}
	latValue, _ := strconv.ParseFloat(query.Get("latitude"), 64)
	lngValue, _ := strconv.ParseFloat(query.Get("longitude"), 64)
	var lat, lng *float64
	if query.Get("latitude") != "" && query.Get("longitude") != "" {
		lat, lng = &latValue, &lngValue
	}
	viewer, _ := subjectUUID(r)
	items, err := s.queries.ListAvailableAnimals(r.Context(), database.ListAvailableAnimalsParams{
		ViewerLatitude: numeric(lat), ViewerLongitude: numeric(lng), Species: ptr("species"), Size: ptr("size"),
		StateCode: ptr("state"), City: ptr("city"), ViewerID: viewer, PageOffset: offset, PageSize: size,
	})
	if err != nil {
		handleDBError(w, err)
		return
	}
	page.Total = len(items)
	writeJSON(w, http.StatusOK, collectionResponse[database.ListAvailableAnimalsRow]{Items: items, Page: page})
}

func (s *Server) createAnimal(w http.ResponseWriter, r *http.Request) {
	actor, ok := subjectUUID(r)
	if !ok {
		writeProblem(w, http.StatusUnauthorized, "Unauthorized", "invalid identity", nil)
		return
	}
	var request animalRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	errs := validateAnimal(&request)
	date := birthDate(request.ApproximateBirthDate, errs)
	if !errs.empty() {
		writeProblem(w, http.StatusUnprocessableEntity, "Validation failed", "review the invalid fields", errs)
		return
	}
	base := animalParams(request)
	base.SubmittedBy, base.BirthDate = actor, date
	var animal database.Animal
	var err error
	if request.OrganizationID == nil {
		animal, err = s.queries.CreatePrivateAnimal(r.Context(), base)
	} else {
		orgID, valid := pathUUIDValue(*request.OrganizationID)
		if !valid {
			invalidPath(w)
			return
		}
		org, getErr := s.queries.GetOrganizationByID(r.Context(), orgID)
		if getErr != nil {
			handleDBError(w, getErr)
			return
		}
		if org.VerificationStatus != "verified" {
			writeServiceError(w, ErrForbidden)
			return
		}
		member, allowed := s.activeMember(r, orgID, actor)
		if allowed && member.CanPublishAnimals {
			animal, err = s.queries.CreateOrganizationAnimal(r.Context(), database.CreateOrganizationAnimalParams{
				SubmittedBy: actor, ResponsibleOrganizationID: orgID, Name: base.Name, Species: base.Species, Breed: base.Breed,
				Sex: base.Sex, Size: base.Size, BirthDate: base.BirthDate, Description: base.Description,
				HealthNotes: base.HealthNotes, BehaviorNotes: base.BehaviorNotes, City: base.City, StateCode: base.StateCode,
				CountryCode: base.CountryCode, Latitude: base.Latitude, Longitude: base.Longitude,
			})
		} else {
			base.ResponsibleOrganizationID = orgID
			animal, err = s.queries.CreatePrivateAnimal(r.Context(), base)
		}
	}
	if err != nil {
		handleDBError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, animal)
}

func pathUUIDValue(value string) (pgtype.UUID, bool) {
	req, _ := http.NewRequest(http.MethodGet, "/", nil)
	req.SetPathValue("id", value)
	return pathUUID(req, "id")
}

func (s *Server) getAnimal(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(r, "animalId")
	if !ok {
		invalidPath(w)
		return
	}
	animal, err := s.queries.GetAnimalByID(r.Context(), id)
	if err != nil {
		handleDBError(w, err)
		return
	}
	if animal.Status != string(domain.AnimalAvailable) {
		actor, valid := subjectUUID(r)
		allowed := valid && actor == animal.SubmittedBy
		if valid && animal.ResponsibleOrganizationID.Valid {
			_, member := s.activeMember(r, animal.ResponsibleOrganizationID, actor)
			allowed = allowed || member || hasRole(r, auth.RoleAdmin)
		}
		if !allowed {
			writeServiceError(w, ErrNotFound)
			return
		}
	}
	includePending := false
	if actor, valid := subjectUUID(r); valid {
		includePending = actor == animal.SubmittedBy || hasRole(r, auth.RoleAdmin)
		if animal.ResponsibleOrganizationID.Valid {
			_, member := s.activeMember(r, animal.ResponsibleOrganizationID, actor)
			includePending = includePending || member
		}
	}
	images, err := s.queries.ListAnimalImages(r.Context(), database.ListAnimalImagesParams{AnimalID: id, IncludePending: includePending})
	if err != nil {
		handleDBError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, animalDetailResponse{Animal: animal, Images: images})
}

func (s *Server) animalWriteAllowed(r *http.Request, animal database.Animal, publish bool) bool {
	actor, ok := subjectUUID(r)
	if !ok {
		return false
	}
	if hasRole(r, auth.RoleAdmin) {
		return true
	}
	if !publish && actor == animal.SubmittedBy {
		return true
	}
	if animal.ResponsibleOrganizationID.Valid {
		member, active := s.activeMember(r, animal.ResponsibleOrganizationID, actor)
		return active && (!publish || member.CanPublishAnimals)
	}
	return false
}

func (s *Server) updateAnimal(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(r, "animalId")
	if !ok {
		invalidPath(w)
		return
	}
	current, err := s.queries.GetAnimalByID(r.Context(), id)
	if err != nil {
		handleDBError(w, err)
		return
	}
	if !s.animalWriteAllowed(r, current, false) {
		writeServiceError(w, ErrForbidden)
		return
	}
	var patch updateAnimalRequest
	if !decodeJSON(w, r, &patch) {
		return
	}
	request := animalRequest{
		Name: patch.Name, Species: patch.Species, Breed: patch.Breed, Size: patch.Size, Sex: patch.Sex,
		ApproximateBirthDate: patch.ApproximateBirthDate, Description: patch.Description,
		HealthNotes: patch.HealthNotes, BehaviorNotes: patch.BehaviorNotes, Location: patch.Location,
	}
	if request.Name == "" {
		request.Name = current.Name
	}
	if request.Species == "" {
		request.Species = current.Species
	}
	if request.Size == "" {
		request.Size = current.Size
	}
	if request.Sex == "" {
		request.Sex = current.Sex
	}
	if request.Description == "" {
		request.Description = current.Description
	}
	if request.HealthNotes == "" {
		request.HealthNotes = current.HealthNotes
	}
	if request.BehaviorNotes == "" {
		request.BehaviorNotes = current.BehaviorNotes
	}
	if request.Location.City == "" {
		request.Location = locationInput{City: current.City, State: current.StateCode, CountryCode: current.CountryCode}
	}
	if request.Breed == nil {
		request.Breed = current.Breed
	}
	errs := validateAnimal(&request)
	date := current.BirthDate
	if request.ApproximateBirthDate != nil {
		date = birthDate(request.ApproximateBirthDate, errs)
	}
	if !errs.empty() {
		writeProblem(w, http.StatusUnprocessableEntity, "Validation failed", "review the invalid fields", errs)
		return
	}
	updated, err := s.queries.UpdateAnimalDetails(r.Context(), database.UpdateAnimalDetailsParams{
		Name: request.Name, Species: request.Species, Breed: request.Breed, Sex: request.Sex, Size: request.Size, BirthDate: date,
		Description: request.Description, HealthNotes: request.HealthNotes, BehaviorNotes: request.BehaviorNotes,
		Vaccinated: current.Vaccinated, Neutered: current.Neutered, SpecialNeeds: current.SpecialNeeds,
		City: request.Location.City, StateCode: request.Location.State, CountryCode: request.Location.CountryCode,
		Latitude: numeric(request.Location.Latitude), Longitude: numeric(request.Location.Longitude), ID: id,
	})
	if err != nil {
		handleDBError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (s *Server) updateAnimalStatus(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(r, "animalId")
	if !ok {
		invalidPath(w)
		return
	}
	actor, _ := subjectUUID(r)
	current, err := s.queries.GetAnimalByID(r.Context(), id)
	if err != nil {
		handleDBError(w, err)
		return
	}
	if !s.animalWriteAllowed(r, current, true) {
		writeServiceError(w, ErrForbidden)
		return
	}
	var request animalStatusRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	if request.Status == domain.AnimalAdopted {
		writeProblem(w, http.StatusUnprocessableEntity, "Invalid operation", "adoption is completed through delivery and receipt confirmations", nil)
		return
	}
	if err := domain.ValidateAnimalTransition(domain.AnimalStatus(current.Status), request.Status); err != nil {
		writeProblem(w, http.StatusConflict, "Invalid state transition", err.Error(), nil)
		return
	}
	reason := strings.TrimSpace(request.Reason)
	if len(reason) < 5 {
		writeProblem(w, http.StatusUnprocessableEntity, "Validation failed", "reason must contain at least 5 characters", nil)
		return
	}
	var updated database.Animal
	if current.Status == string(domain.AnimalPending) && request.Status == domain.AnimalAvailable && current.Source == "private_submission" {
		updated, err = s.queries.ReviewAnimalSubmission(r.Context(), database.ReviewAnimalSubmissionParams{ReviewStatus: "accepted", ReviewNotes: &reason, ReviewedBy: actor, ID: id, OrganizationID: current.ResponsibleOrganizationID})
	} else {
		updated, err = s.queries.SetAnimalStatus(r.Context(), database.SetAnimalStatusParams{Status: string(request.Status), Reason: &reason, ID: id, OrganizationID: current.ResponsibleOrganizationID})
	}
	if err != nil {
		handleDBError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (s *Server) addAnimalImage(w http.ResponseWriter, r *http.Request) {
	animalID, ok := pathUUID(r, "animalId")
	if !ok {
		invalidPath(w)
		return
	}
	animal, err := s.queries.GetAnimalByID(r.Context(), animalID)
	if err != nil {
		handleDBError(w, err)
		return
	}
	if !s.animalWriteAllowed(r, animal, false) {
		writeServiceError(w, ErrForbidden)
		return
	}
	var request animalImageRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	errs := fieldErrors{}
	request.URL = validImageURL(errs, "url", request.URL)
	request.AltText = optional(errs, "alt_text", request.AltText, 300)
	if request.Position < 0 {
		errs.add("position", "must not be negative")
	}
	if !errs.empty() {
		writeProblem(w, http.StatusUnprocessableEntity, "Validation failed", "review the invalid fields", errs)
		return
	}
	image, err := s.queries.AddAnimalImage(r.Context(), database.AddAnimalImageParams{AnimalID: animalID, StorageKey: request.URL, AltText: request.AltText, Position: request.Position})
	if err != nil {
		handleDBError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, animalImageResponse{ID: uuidString(image.ID), URL: image.StorageKey, AltText: image.AltText, Position: image.Position, ModerationStatus: image.ModerationStatus, CreatedAt: timestamp(image.CreatedAt)})
}

func (s *Server) removeAnimalImage(w http.ResponseWriter, r *http.Request) {
	animalID, ok := pathUUID(r, "animalId")
	if !ok {
		invalidPath(w)
		return
	}
	imageID, ok := pathUUID(r, "imageId")
	if !ok {
		invalidPath(w)
		return
	}
	animal, err := s.queries.GetAnimalByID(r.Context(), animalID)
	if err != nil {
		handleDBError(w, err)
		return
	}
	if !s.animalWriteAllowed(r, animal, false) {
		writeServiceError(w, ErrForbidden)
		return
	}
	affected, err := s.queries.RemoveAnimalImage(r.Context(), database.RemoveAnimalImageParams{ID: imageID, AnimalID: animalID})
	if err != nil {
		handleDBError(w, err)
		return
	}
	if affected == 0 {
		writeServiceError(w, ErrNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) listPreferences(w http.ResponseWriter, r *http.Request) {
	actor, _ := subjectUUID(r)
	offset, size, page := pagination(r)
	preference := r.URL.Query().Get("value")
	if preference == "" {
		preference = "liked"
	}
	if preference != "liked" && preference != "not_interested" {
		writeProblem(w, http.StatusUnprocessableEntity, "Validation failed", "value must be liked or not_interested", nil)
		return
	}
	items, err := s.queries.ListPreferredAnimals(r.Context(), database.ListPreferredAnimalsParams{UserID: actor, Preference: preference, PageOffset: offset, PageSize: size})
	if err != nil {
		handleDBError(w, err)
		return
	}
	page.Total = len(items)
	writeJSON(w, http.StatusOK, collectionResponse[database.ListPreferredAnimalsRow]{Items: items, Page: page})
}

func (s *Server) setPreference(w http.ResponseWriter, r *http.Request) {
	actor, _ := subjectUUID(r)
	animalID, ok := pathUUID(r, "animalId")
	if !ok {
		invalidPath(w)
		return
	}
	var request preferenceRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	if request.Value != "liked" && request.Value != "not_interested" {
		writeProblem(w, http.StatusUnprocessableEntity, "Validation failed", "value must be liked or not_interested", nil)
		return
	}
	animal, err := s.queries.GetAnimalByID(r.Context(), animalID)
	if err != nil {
		handleDBError(w, err)
		return
	}
	if animal.Status != string(domain.AnimalAvailable) {
		writeServiceError(w, ErrConflict)
		return
	}
	preference, err := s.queries.UpsertAnimalPreference(r.Context(), database.UpsertAnimalPreferenceParams{UserID: actor, AnimalID: animalID, Preference: request.Value})
	if err != nil {
		handleDBError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, preference)
}
