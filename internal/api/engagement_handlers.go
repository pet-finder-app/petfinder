package api

import (
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/pet-finder-app/petfinder-api/internal/auth"
	"github.com/pet-finder-app/petfinder-api/internal/database"
)

type notificationResponse struct {
	ID        string     `json:"id"`
	Kind      string     `json:"kind"`
	SubjectID string     `json:"subject_id"`
	CreatedAt time.Time  `json:"created_at"`
	ReadAt    *time.Time `json:"read_at"`
}
type readMessageRequest struct {
	MessageID string `json:"message_id" format:"uuid"`
}
type conversationReadState struct {
	ConversationID    string  `json:"conversation_id"`
	LastReadMessageID *string `json:"last_read_message_id"`
	UnreadCount       int     `json:"unread_count"`
}
type unreadConversations struct {
	Items       []conversationReadState `json:"items"`
	UnreadCount int                     `json:"unread_count"`
}
type startConversationRequest struct {
	UserID string `json:"user_id" format:"uuid"`
}
type interestedPerson struct {
	UserID string `json:"user_id"`
	Name   string `json:"name"`
}
type interestResponse struct {
	LikeCount  int                `json:"like_count"`
	Applicants int                `json:"applicants"`
	People     []interestedPerson `json:"people"`
	Page       pageResponse       `json:"page"`
}
type metricDay struct {
	Date         string `json:"date"`
	Likes        int    `json:"likes"`
	Applications int    `json:"applications"`
}
type organizationAnalytics struct {
	AnimalStatuses map[string]int `json:"animal_statuses"`
	Likes          int            `json:"likes"`
	Applications   int            `json:"applications"`
	Days           []metricDay    `json:"days"`
}

func registerEngagementRoutes(s *Server, reg *routeRegistry) {
	registerRoute[noBody, collectionResponse[notificationResponse]](reg, "GET", "/v1/notifications", "listNotifications", "List your notifications", "Notifications", 200, s.protected(s.listNotifications), bearerAuth(false), queryParameters[paginationQuery])
	registerRoute[noBody, countResponse](reg, "GET", "/v1/notifications/unread", "notificationUnreadCount", "Count your unread notifications", "Notifications", 200, s.protected(s.notificationUnread), bearerAuth(false))
	registerRoute[noBody, noBody](reg, "PUT", "/v1/notifications/read", "readAllNotifications", "Mark your notifications read", "Notifications", 204, s.protected(s.readNotifications), bearerAuth(false))
	registerRoute[noBody, noBody](reg, "PUT", "/v1/notifications/{notificationId}/read", "readNotification", "Mark a notification read", "Notifications", 204, s.protected(s.readNotifications), bearerAuth(false))
	registerRoute[noBody, conversationReadState](reg, "GET", "/v1/conversations/{conversationId}/read-state", "getConversationReadState", "Read your message cursor and unread count", "Conversations", 200, s.protected(s.conversationReadState), bearerAuth(false))
	registerRoute[readMessageRequest, conversationReadState](reg, "PUT", "/v1/conversations/{conversationId}/read-state", "markConversationRead", "Advance your message read cursor", "Conversations", 200, s.protected(s.markConversationRead), bearerAuth(false))
	registerRoute[noBody, unreadConversations](reg, "GET", "/v1/conversations/unread", "conversationUnreadCounts", "Count unread messages in accessible conversations", "Conversations", 200, s.protected(s.conversationUnread), bearerAuth(false))
	registerRoute[startConversationRequest, database.Conversation](reg, "POST", "/v1/animals/{animalId}/conversations", "startOrganizationConversation", "Contact a consenting interested person through the organization", "Conversations", 201, s.protected(s.startOrganizationConversation), bearerAuth(false))
	registerRoute[noBody, interestResponse](reg, "GET", "/v1/animals/{animalId}/interest", "animalInterest", "Read animal interest counts and consenting people", "Analytics", 200, s.protected(s.animalInterest), bearerAuth(false), queryParameters[paginationQuery])
	registerRoute[noBody, organizationAnalytics](reg, "GET", "/v1/organizations/{organizationId}/analytics", "organizationAnalytics", "Read organization totals and ninety daily aggregates", "Analytics", 200, s.protected(s.organizationAnalytics), bearerAuth(false))
}
func (s *Server) listNotifications(w http.ResponseWriter, r *http.Request) {
	if err := s.materializeDueFollowups(r); err != nil {
		handleDBError(w, err)
		return
	}
	actor, _ := subjectUUID(r)
	offset, size, page := pagination(r)
	rows, err := s.pool.Query(r.Context(), "SELECT id::text,kind,subject_id::text,created_at,read_at FROM notifications WHERE user_id=$1 ORDER BY created_at DESC,id LIMIT $2 OFFSET $3", actor, size, offset)
	if err != nil {
		handleDBError(w, err)
		return
	}
	defer rows.Close()
	items := []notificationResponse{}
	for rows.Next() {
		var v notificationResponse
		if err = rows.Scan(&v.ID, &v.Kind, &v.SubjectID, &v.CreatedAt, &v.ReadAt); err != nil {
			handleDBError(w, err)
			return
		}
		items = append(items, v)
	}
	if err = rows.Err(); err != nil {
		handleDBError(w, err)
		return
	}
	page.Total = len(items)
	writeJSON(w, 200, collectionResponse[notificationResponse]{Items: items, Page: page})
}
func (s *Server) notificationUnread(w http.ResponseWriter, r *http.Request) {
	if err := s.materializeDueFollowups(r); err != nil {
		handleDBError(w, err)
		return
	}
	actor, _ := subjectUUID(r)
	var v countResponse
	if err := s.pool.QueryRow(r.Context(), "SELECT count(*) FROM notifications WHERE user_id=$1 AND read_at IS NULL", actor).Scan(&v.Count); err != nil {
		handleDBError(w, err)
		return
	}
	writeJSON(w, 200, v)
}
func (s *Server) readNotifications(w http.ResponseWriter, r *http.Request) {
	actor, _ := subjectUUID(r)
	var id pgtype.UUID
	if r.PathValue("notificationId") != "" {
		var ok bool
		id, ok = pathUUID(r, "notificationId")
		if !ok {
			invalidPath(w)
			return
		}
	}
	tag, err := s.pool.Exec(r.Context(), "UPDATE notifications SET read_at=COALESCE(read_at,now()) WHERE user_id=$1 AND ($2::uuid IS NULL OR id=$2)", actor, id)
	if err != nil {
		handleDBError(w, err)
		return
	}
	if id.Valid && tag.RowsAffected() == 0 {
		writeServiceError(w, ErrNotFound)
		return
	}
	w.WriteHeader(204)
}
func (s *Server) accessibleConversation(w http.ResponseWriter, r *http.Request) (database.Conversation, bool) {
	id, ok := pathUUID(r, "conversationId")
	if !ok {
		invalidPath(w)
		return database.Conversation{}, false
	}
	v, err := s.queries.GetConversationByID(r.Context(), id)
	if err != nil {
		handleDBError(w, err)
		return v, false
	}
	allowed, _ := s.conversationAllowed(r, v)
	if !allowed {
		writeServiceError(w, ErrForbidden)
		return v, false
	}
	return v, true
}

const readStateSQL = `SELECT c.id::text,cr.message_id::text,(SELECT count(*) FROM messages m WHERE m.conversation_id=c.id AND m.sender_id<>$1 AND m.deleted_at IS NULL AND ($3::boolean OR m.moderation_status IN ('visible','flagged')) AND (cr.message_id IS NULL OR (m.created_at,m.id)>(cr.message_created_at,cr.message_id))) FROM conversations c LEFT JOIN conversation_reads cr ON cr.conversation_id=c.id AND cr.user_id=$1 WHERE c.id=$2`

func (s *Server) conversationReadState(w http.ResponseWriter, r *http.Request) {
	v, ok := s.accessibleConversation(w, r)
	if !ok {
		return
	}
	actor, _ := subjectUUID(r)
	_, moderator := s.conversationAllowed(r, v)
	var state conversationReadState
	if err := s.pool.QueryRow(r.Context(), readStateSQL, actor, v.ID, moderator).Scan(&state.ConversationID, &state.LastReadMessageID, &state.UnreadCount); err != nil {
		handleDBError(w, err)
		return
	}
	writeJSON(w, 200, state)
}
func (s *Server) markConversationRead(w http.ResponseWriter, r *http.Request) {
	v, ok := s.accessibleConversation(w, r)
	if !ok {
		return
	}
	var req readMessageRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	actor, _ := subjectUUID(r)
	id, _ := pathUUIDValue(req.MessageID)
	_, moderator := s.conversationAllowed(r, v)
	valid, err := s.queries.IsVisibleMessageCursor(r.Context(), database.IsVisibleMessageCursorParams{ID: id, ConversationID: v.ID, IncludeModerated: moderator})
	if err != nil {
		handleDBError(w, err)
		return
	}
	if !valid {
		writeServiceError(w, ErrInvalid)
		return
	}
	_, err = s.pool.Exec(r.Context(), `INSERT INTO conversation_reads(conversation_id,user_id,message_id,message_created_at) SELECT $1,$2,id,created_at FROM messages WHERE id=$3 AND conversation_id=$1 ON CONFLICT(conversation_id,user_id) DO UPDATE SET message_id=EXCLUDED.message_id,message_created_at=EXCLUDED.message_created_at,read_at=now() WHERE (EXCLUDED.message_created_at,EXCLUDED.message_id)>(conversation_reads.message_created_at,conversation_reads.message_id)`, v.ID, actor, id)
	if err != nil {
		handleDBError(w, err)
		return
	}
	s.conversationReadState(w, r)
}
func (s *Server) conversationUnread(w http.ResponseWriter, r *http.Request) {
	actor, _ := subjectUUID(r)
	rows, err := s.pool.Query(r.Context(), `SELECT id FROM conversations c WHERE c.counterparty_user_id=$1 OR EXISTS(SELECT 1 FROM organization_members om WHERE om.organization_id=c.organization_id AND om.user_id=$1 AND om.status='active') ORDER BY c.id`, actor)
	if err != nil {
		handleDBError(w, err)
		return
	}
	ids := []pgtype.UUID{}
	for rows.Next() {
		var id pgtype.UUID
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			handleDBError(w, err)
			return
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		handleDBError(w, err)
		return
	}
	result := unreadConversations{Items: []conversationReadState{}}
	for _, id := range ids {
		v, err := s.queries.GetConversationByID(r.Context(), id)
		if err != nil {
			handleDBError(w, err)
			return
		}
		_, moderator := s.conversationAllowed(r, v)
		var state conversationReadState
		if err = s.pool.QueryRow(r.Context(), readStateSQL, actor, id, moderator).Scan(&state.ConversationID, &state.LastReadMessageID, &state.UnreadCount); err != nil {
			handleDBError(w, err)
			return
		}
		result.Items = append(result.Items, state)
		result.UnreadCount += state.UnreadCount
	}
	writeJSON(w, 200, result)
}
func (s *Server) startOrganizationConversation(w http.ResponseWriter, r *http.Request) {
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
	actor, _ := subjectUUID(r)
	member, allowed := s.activeMember(r, animal.ResponsibleOrganizationID, actor)
	if !hasRole(r, auth.RoleAdmin) && (!allowed || !member.CanManageApplications) {
		writeServiceError(w, ErrForbidden)
		return
	}
	if animal.Status != "available" || !s.publicOrganization(r, animal.ResponsibleOrganizationID) {
		writeServiceError(w, ErrConflict)
		return
	}
	var req startConversationRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	user, _ := pathUUIDValue(req.UserID)
	var conversationID pgtype.UUID
	err = s.pool.QueryRow(r.Context(), `INSERT INTO conversations(organization_id,kind,animal_id,counterparty_user_id) SELECT $1,'interested_organization',$2,u.id FROM users u JOIN animal_preferences p ON p.user_id=u.id AND p.animal_id=$2 AND p.preference='liked' WHERE u.id=$3 AND u.deactivated_at IS NULL AND u.deleted_at IS NULL AND u.personal_profile->>'allow_organization_contact'='true' ON CONFLICT(animal_id,counterparty_user_id,organization_id) WHERE kind='interested_organization' DO UPDATE SET status='open',closed_at=NULL,closed_by=NULL RETURNING id`, animal.ResponsibleOrganizationID, id, user).Scan(&conversationID)
	if err != nil {
		writeServiceError(w, ErrConflict)
		return
	}
	conversation, err := s.queries.GetConversationByID(r.Context(), conversationID)
	if err != nil {
		handleDBError(w, err)
		return
	}
	writeJSON(w, 201, conversation)
}
func (s *Server) analyticsAllowed(r *http.Request, org pgtype.UUID) bool {
	actor, _ := subjectUUID(r)
	member, ok := s.activeMember(r, org, actor)
	return hasRole(r, auth.RoleAdmin) || (ok && member.CanManageApplications)
}
func (s *Server) animalInterest(w http.ResponseWriter, r *http.Request) {
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
	if !s.analyticsAllowed(r, animal.ResponsibleOrganizationID) {
		writeServiceError(w, ErrForbidden)
		return
	}
	offset, size, page := pagination(r)
	result := interestResponse{People: []interestedPerson{}, Page: page}
	err = s.pool.QueryRow(r.Context(), "SELECT (SELECT count(*) FROM animal_preferences WHERE animal_id=$1 AND preference='liked'),(SELECT count(*) FROM adoption_applications WHERE animal_id=$1)", id).Scan(&result.LikeCount, &result.Applicants)
	if err != nil {
		handleDBError(w, err)
		return
	}
	rows, err := s.pool.Query(r.Context(), `SELECT u.id::text,u.display_name FROM animal_preferences p JOIN users u ON u.id=p.user_id WHERE p.animal_id=$1 AND p.preference='liked' AND u.deactivated_at IS NULL AND u.deleted_at IS NULL AND u.personal_profile->>'allow_organization_contact'='true' ORDER BY p.created_at DESC,u.id LIMIT $2 OFFSET $3`, id, size, offset)
	if err != nil {
		handleDBError(w, err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var v interestedPerson
		if err = rows.Scan(&v.UserID, &v.Name); err != nil {
			handleDBError(w, err)
			return
		}
		result.People = append(result.People, v)
	}
	if err = rows.Err(); err != nil {
		handleDBError(w, err)
		return
	}
	result.Page.Total = len(result.People)
	writeJSON(w, 200, result)
}
func (s *Server) organizationAnalytics(w http.ResponseWriter, r *http.Request) {
	org, ok := pathUUID(r, "organizationId")
	if !ok {
		invalidPath(w)
		return
	}
	if !s.analyticsAllowed(r, org) {
		writeServiceError(w, ErrForbidden)
		return
	}
	result := organizationAnalytics{AnimalStatuses: map[string]int{}, Days: []metricDay{}}
	rows, err := s.pool.Query(r.Context(), "SELECT status,count(*) FROM animals WHERE responsible_organization_id=$1 AND deleted_at IS NULL GROUP BY status", org)
	if err != nil {
		handleDBError(w, err)
		return
	}
	for rows.Next() {
		var status string
		var count int
		if err = rows.Scan(&status, &count); err != nil {
			rows.Close()
			handleDBError(w, err)
			return
		}
		result.AnimalStatuses[status] = count
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		handleDBError(w, err)
		return
	}
	err = s.pool.QueryRow(r.Context(), "SELECT (SELECT count(*) FROM animal_preferences p JOIN animals a ON a.id=p.animal_id WHERE a.responsible_organization_id=$1 AND p.preference='liked'),(SELECT count(*) FROM adoption_applications WHERE organization_id=$1)", org).Scan(&result.Likes, &result.Applications)
	if err != nil {
		handleDBError(w, err)
		return
	}
	rows, err = s.pool.Query(r.Context(), `SELECT to_char(d,'YYYY-MM-DD'),(SELECT count(*) FROM animal_preference_events p JOIN animals a ON a.id=p.animal_id WHERE a.responsible_organization_id=$1 AND p.preference='liked' AND p.created_at>=d AND p.created_at<d+interval '1 day'),(SELECT count(*) FROM adoption_applications WHERE organization_id=$1 AND created_at>=d AND created_at<d+interval '1 day') FROM generate_series(date_trunc('day',now())-interval '89 days',date_trunc('day',now()),interval '1 day') d ORDER BY d`, org)
	if err != nil {
		handleDBError(w, err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var v metricDay
		if err = rows.Scan(&v.Date, &v.Likes, &v.Applications); err != nil {
			handleDBError(w, err)
			return
		}
		result.Days = append(result.Days, v)
	}
	if err = rows.Err(); err != nil {
		handleDBError(w, err)
		return
	}
	writeJSON(w, 200, result)
}
